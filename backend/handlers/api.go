package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"ultimate-dts-fix-server/backend/models"
	"ultimate-dts-fix-server/backend/services"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	queueService     *services.QueueService
	converterService *services.ConverterService
	wsService        *services.WebSocketService
}

func NewAPIHandler(queueService *services.QueueService, converterService *services.ConverterService, wsService *services.WebSocketService) *APIHandler {
	return &APIHandler{
		queueService:     queueService,
		converterService: converterService,
		wsService:        wsService,
	}
}

// Status возвращает статус сервера
func (h *APIHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "online",
		"timestamp": time.Now().Unix(),
		"clients":   h.wsService.GetClientCount(),
	})
}

// GetQueue возвращает текущую очередь задач (pending и processing)
func (h *APIHandler) GetQueue(c *gin.Context) {
	tasks, err := h.queueService.GetQueue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ошибка получения очереди",
		})
		return
	}

	// Фильтруем только pending и processing задачи
	var queueTasks []*models.Task
	for _, task := range tasks {
		if task.Status == models.StatusPending || task.Status == models.StatusProcessing {
			queueTasks = append(queueTasks, task)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"queue": queueTasks,
	})
}

// GetActiveTask возвращает текущую обрабатываемую задачу
func (h *APIHandler) GetActiveTask(c *gin.Context) {
	activeTask := h.converterService.GetActiveTask()

	if activeTask == nil {
		c.JSON(http.StatusOK, gin.H{
			"activeTask": nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"activeTask": activeTask,
	})
}

// GetHistory возвращает историю завершенных задач
func (h *APIHandler) GetHistory(c *gin.Context) {
	tasks, err := h.queueService.GetQueue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ошибка получения истории",
		})
		return
	}

	// Фильтруем только completed и error задачи
	var historyTasks []*models.Task
	for _, task := range tasks {
		if task.Status == models.StatusCompleted || task.Status == models.StatusError {
			historyTasks = append(historyTasks, task)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"history": historyTasks,
	})
}

// AddTask добавляет задачу в очередь
func (h *APIHandler) AddTask(c *gin.Context) {
	var request struct {
		FilePath string `json:"filePath" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверный формат запроса",
		})
		return
	}

	// Проверяем существование файла
	if _, err := os.Stat(request.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Файл не существует: " + request.FilePath,
		})
		return
	}

	// Проверяем, что файл видео
	if !isVideoFile(request.FilePath) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Файл не является видеофайлом: " + request.FilePath,
		})
		return
	}

	// Получаем информацию об аудио
	audioInfo, err := getAudioInfo(request.FilePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Ошибка получения информации об аудио: " + err.Error(),
		})
		return
	}

	task := &models.Task{
		ID:        generateTaskID(),
		FilePath:  request.FilePath,
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

	h.queueService.AddTask(task)

	if h.wsService != nil {
		h.wsService.BroadcastLog("Задача добавлена в очередь: "+request.FilePath, "info")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Задача добавлена в очередь",
		"taskId":  task.ID,
	})
}

// CancelTask отменяет текущую задачу конвертации
func (h *APIHandler) CancelTask(c *gin.Context) {
	var request struct {
		TaskID string `json:"taskId" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверный формат запроса",
		})
		return
	}

	err := h.converterService.CancelConversion(request.TaskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if h.wsService != nil {
		h.wsService.BroadcastLog("Задача отменена: "+request.TaskID, "warning")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Задача отменена",
	})
}

// DeleteTask удаляет задачу из очереди
func (h *APIHandler) DeleteTask(c *gin.Context) {
	var request struct {
		TaskID string `json:"taskId" binding:"required"`
		Force  bool   `json:"force"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверный формат запроса",
		})
		return
	}

	// Получаем задачу
	task, err := h.queueService.GetTask(request.TaskID)
	if err != nil || task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Задача не найдена",
		})
		return
	}

	// Если задача в процессе конвертации, проверяем активна ли она реально
	if task.Status == models.StatusProcessing {
		activeTask := h.converterService.GetActiveTask()

		// Если задача не активна в конвертере, это зависшая задача - можно удалить
		if activeTask == nil || activeTask.ID != task.ID {
			log.Printf("Обнаружена зависшая задача в статусе processing: %s", task.ID)
		} else if !request.Force {
			// Задача действительно активна и force не установлен
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Задача в процессе конвертации. Сначала отмените её или используйте force=true для принудительного удаления.",
			})
			return
		} else {
			// Force удаление - отменяем конвертацию
			log.Printf("Принудительное удаление активной задачи: %s", task.ID)
			_ = h.converterService.CancelConversion(task.ID)
		}
	}

	// Удаляем задачу
	if err := h.queueService.DeleteTask(request.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ошибка удаления задачи",
		})
		return
	}

	if h.wsService != nil {
		h.wsService.BroadcastLog("Задача удалена: "+request.TaskID, "info")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Задача удалена",
	})
}

// SearchFiles ищет видеофайлы в медиа папках
func (h *APIHandler) SearchFiles(c *gin.Context) {
	var request struct {
		Pattern string `json:"pattern"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверный формат запроса",
		})
		return
	}

	// Если паттерн пустой, используем дефолтный: DTS.*5.1
	pattern := request.Pattern
	if pattern == "" {
		pattern = "DTS.*5\\.1"
	}

	// Поиск файлов в /media/
	files, err := searchVideoFiles("/media", pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ошибка поиска файлов: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"files":          files,
		"count":          len(files),
		"defaultPattern": "DTS.*5\\.1",
	})
}

// searchVideoFiles рекурсивно ищет видеофайлы по паттерну (поддерживает regex)
func searchVideoFiles(rootPath, pattern string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	// Компилируем регулярное выражение (регистронезависимое)
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		// Если паттерн не валидный regex, используем простой поиск подстроки
		re = nil
	}

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Игнорируем ошибки доступа к файлам
			return nil
		}

		// Пропускаем директории
		if info.IsDir() {
			return nil
		}

		// Проверяем, что это видеофайл
		if !isVideoFile(path) {
			return nil
		}

		// Проверяем соответствие паттерну
		fileName := filepath.Base(path)
		matched := false

		if re != nil {
			// Используем regex
			matched = re.MatchString(fileName)
		} else {
			// Используем простой поиск подстроки (регистронезависимый)
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

// AudioStreamInfo содержит информацию об аудио потоке
type AudioStreamInfo struct {
	CodecName     string `json:"codec_name"`
	ChannelLayout string `json:"channel_layout"`
	Channels      int    `json:"channels"`
	SampleRate    string `json:"sample_rate"`
	BitRate       string `json:"bit_rate"`
}

// FFProbeOutput структура для парсинга вывода ffprobe
type FFProbeOutput struct {
	Streams []AudioStreamInfo `json:"streams"`
}

// getAudioInfo получает информацию об аудио потоке через ffprobe
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

// Вспомогательная функция для проверки видеофайла
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

// Вспомогательная функция для генерации ID задачи
func generateTaskID() string {
	return time.Now().Format("20060102150405")
}
