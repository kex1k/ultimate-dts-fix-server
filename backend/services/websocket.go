package services

import (
	"ultimate-dts-fix-server/backend/models"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocketService struct {
	clients    map[*websocket.Conn]bool
	clientsMux sync.RWMutex
	upgrader   websocket.Upgrader
}

func NewWebSocketService() *WebSocketService {
	return &WebSocketService{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Разрешаем все запросы в development
				// В production настройте проверку origin
				return true
			},
		},
	}
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

	// Обработка сообщений от клиента
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.removeClient(conn)
			break
		}

		// Обработка входящих сообщений
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Ошибка парсинга WebSocket сообщения: %v", err)
			continue
		}

		log.Printf("Получено WebSocket сообщение: %v", msg)
	}
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

	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()

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