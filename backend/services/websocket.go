package services

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"ultimate-dts-fix-server/backend/models"

	"github.com/gorilla/websocket"
)

// WSMessage структура входящего сообщения
type WSMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// WSResponse структура ответа
type WSResponse struct {
	Type  string      `json:"type"`
	Data  interface{} `json:"data"`
	Error string      `json:"error,omitempty"`
}

type WebSocketService struct {
	clients          map[*websocket.Conn]bool
	clientsMux       sync.RWMutex
	upgrader         websocket.Upgrader
	queueService     *QueueService
	converterService *ConverterService
}

func NewWebSocketService() *WebSocketService {
	return &WebSocketService{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// SetServices устанавливает зависимости
func (s *WebSocketService) SetServices(queueService *QueueService, converterService *ConverterService) {
	s.queueService = queueService
	s.converterService = converterService
}

func (s *WebSocketService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Ошибка обновления WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Добавляем клиента
	s.clientsMux.Lock()
	s.clients[conn] = true
	s.clientsMux.Unlock()

	log.Printf("WebSocket клиент подключен. Всего клиентов: %d", len(s.clients))

	// Отправляем начальное состояние
	s.sendInitialState(conn)

	// Обработка сообщений от клиента
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.removeClient(conn)
			break
		}

		// Обработка входящих команд
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Ошибка парсинга WebSocket сообщения: %v", err)
			continue
		}

		s.handleMessage(conn, &msg)
	}
}

// sendInitialState отправляет начальное состояние при подключении
func (s *WebSocketService) sendInitialState(conn *websocket.Conn) {
	if s.queueService == nil {
		return
	}

	tasks, _ := s.queueService.GetQueue()

	var queueTasks, historyTasks []*models.Task
	for _, task := range tasks {
		if task.Status == models.StatusPending || task.Status == models.StatusProcessing {
			queueTasks = append(queueTasks, task)
		} else if task.Status == models.StatusCompleted || task.Status == models.StatusError {
			historyTasks = append(historyTasks, task)
		}
	}

	var activeTask *models.Task
	if s.converterService != nil {
		activeTask = s.converterService.GetActiveTask()
	}

	response := WSResponse{
		Type: "initial_state",
		Data: map[string]interface{}{
			"queue":      queueTasks,
			"history":    historyTasks,
			"activeTask": activeTask,
			"status":     "online",
			"timestamp":  time.Now().Unix(),
		},
	}

	jsonData, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, jsonData)
}

// handleMessage обрабатывает входящие команды
func (s *WebSocketService) handleMessage(conn *websocket.Conn, msg *WSMessage) {
	var response WSResponse
	response.Type = msg.Type + "_response"

	switch msg.Type {
	case "get_state":
		s.handleGetState(conn, msg, &response)
	case "search_files":
		s.handleSearchFiles(conn, msg, &response)
	case "add_task":
		s.handleAddTask(conn, msg, &response)
	case "cancel_task":
		s.handleCancelTask(conn, msg, &response)
	case "delete_task":
		s.handleDeleteTask(conn, msg, &response)
	default:
		response.Error = "Unknown command: " + msg.Type
	}

	jsonData, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, jsonData)
}

// handleGetState возвращает текущее состояние
func (s *WebSocketService) handleGetState(conn *websocket.Conn, msg *WSMessage, response *WSResponse) {
	s.sendInitialState(conn)
}

// handleSearchFiles ищет файлы
func (s *WebSocketService) handleSearchFiles(conn *websocket.Conn, msg *WSMessage, response *WSResponse) {
	pattern, _ := msg.Data["pattern"].(string)
	if pattern == "" {
		pattern = "DTS.*5\\.1"
	}

	files, err := searchVideoFiles("/media", pattern)
	if err != nil {
		response.Error = "Ошибка поиска: " + err.Error()
		return
	}

	response.Data = map[string]interface{}{
		"files": files,
		"count": len(files),
	}
}

// handleAddTask добавляет задачу
func (s *WebSocketService) handleAddTask(conn *websocket.Conn, msg *WSMessage, response *WSResponse) {
	filePath, ok := msg.Data["filePath"].(string)
	if !ok || filePath == "" {
		response.Error = "filePath required"
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		response.Error = "Файл не существует"
		return
	}

	if !isVideoFile(filePath) {
		response.Error = "Файл не является видеофайлом"
		return
	}

	audioInfo, err := getAudioInfo(filePath)
	if err != nil {
		response.Error = "Ошибка получения аудио информации"
		return
	}

	task := &models.Task{
		ID:        time.Now().Format("20060102150405"),
		FilePath:  filePath,
		Status:    models.StatusPending,
		Progress:  0,
		CreatedAt: time.Now(),
		AudioInfo: &models.AudioInfo{
			CodecName:     audioInfo.CodecName,
			ChannelLayout: audioInfo.ChannelLayout,
			Channels:      audioInfo.Channels,
			SampleRate:    audioInfo.SampleRate,
			BitRate:       audioInfo.BitRate,
		},
	}

	s.queueService.AddTask(task)
	s.BroadcastLog("Задача добавлена: "+filePath, "info")

	response.Data = map[string]interface{}{
		"taskId":  task.ID,
		"message": "Задача добавлена",
	}
}

// handleCancelTask отменяет задачу
func (s *WebSocketService) handleCancelTask(conn *websocket.Conn, msg *WSMessage, response *WSResponse) {
	taskID, ok := msg.Data["taskId"].(string)
	if !ok || taskID == "" {
		response.Error = "taskId required"
		return
	}

	err := s.converterService.CancelConversion(taskID)
	if err != nil {
		response.Error = err.Error()
		return
	}

	s.BroadcastLog("Задача отменена: "+taskID, "warning")
	response.Data = map[string]interface{}{
		"message": "Задача отменена",
	}
}

// handleDeleteTask удаляет задачу
func (s *WebSocketService) handleDeleteTask(conn *websocket.Conn, msg *WSMessage, response *WSResponse) {
	taskID, ok := msg.Data["taskId"].(string)
	if !ok || taskID == "" {
		response.Error = "taskId required"
		return
	}

	force, _ := msg.Data["force"].(bool)

	task, err := s.queueService.GetTask(taskID)
	if err != nil || task == nil {
		response.Error = "Задача не найдена"
		return
	}

	if task.Status == models.StatusProcessing {
		activeTask := s.converterService.GetActiveTask()
		if activeTask != nil && activeTask.ID == task.ID && !force {
			response.Error = "Задача в процессе. Используйте force=true"
			return
		}
		if force {
			_ = s.converterService.CancelConversion(taskID)
		}
	}

	if err := s.queueService.DeleteTask(taskID); err != nil {
		response.Error = "Ошибка удаления"
		return
	}

	s.BroadcastLog("Задача удалена: "+taskID, "info")
	response.Data = map[string]interface{}{
		"message": "Задача удалена",
	}
}

// Вспомогательные функции

func searchVideoFiles(rootPath, pattern string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		re = nil
	}

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !isVideoFile(path) {
			return nil
		}

		fileName := filepath.Base(path)
		matched := false

		if re != nil {
			matched = re.MatchString(fileName)
		} else {
			matched = strings.Contains(strings.ToLower(fileName), strings.ToLower(pattern))
		}

		if matched {
			results = append(results, map[string]interface{}{
				"path":     path,
				"name":     fileName,
				"size":     info.Size(),
				"modified": info.ModTime().Unix(),
			})
		}

		return nil
	})

	return results, err
}

func isVideoFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	videoExts := []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v"}
	for _, videoExt := range videoExts {
		if ext == videoExt {
			return true
		}
	}
	return false
}

func getAudioInfo(filePath string) (*AudioStreamInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "a:0",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var probeOutput FFProbeOutput
	if err := json.Unmarshal(output, &probeOutput); err != nil {
		return nil, err
	}

	if len(probeOutput.Streams) == 0 {
		return nil, nil
	}

	return &probeOutput.Streams[0], nil
}

func (s *WebSocketService) removeClient(conn *websocket.Conn) {
	s.clientsMux.Lock()
	delete(s.clients, conn)
	s.clientsMux.Unlock()
	log.Printf("WebSocket клиент отключен. Осталось клиентов: %d", len(s.clients))
}

func (s *WebSocketService) BroadcastMessage(messageType string, data interface{}) {
	message := map[string]interface{}{
		"type": messageType,
		"data": data,
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		log.Printf("Ошибка маршалинга WebSocket сообщения: %v", err)
		return
	}

	s.clientsMux.Lock()
	defer s.clientsMux.Unlock()

	for client := range s.clients {
		err := client.WriteMessage(websocket.TextMessage, jsonMessage)
		if err != nil {
			log.Printf("Ошибка отправки WebSocket сообщения: %v", err)
			client.Close()
			delete(s.clients, client)
		}
	}
}

func (s *WebSocketService) BroadcastQueueUpdate(tasks []*models.Task) {
	s.BroadcastMessage("queue_update", map[string]interface{}{
		"queue": tasks,
	})
}

func (s *WebSocketService) BroadcastConversionProgress(taskID string, progress int, status models.TaskStatus, message string) {
	s.BroadcastMessage("conversion_progress", map[string]interface{}{
		"taskId":   taskID,
		"progress": progress,
		"status":   status,
		"message":  message,
	})
}

func (s *WebSocketService) BroadcastScanProgress(progress int, message string, completed bool) {
	s.BroadcastMessage("scan_progress", map[string]interface{}{
		"progress":  progress,
		"message":   message,
		"completed": completed,
	})
}

func (s *WebSocketService) BroadcastLog(message string, level string) {
	s.BroadcastMessage("log", map[string]interface{}{
		"message": message,
		"level":   level,
	})
}

func (s *WebSocketService) GetClientCount() int {
	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()
	return len(s.clients)
}
