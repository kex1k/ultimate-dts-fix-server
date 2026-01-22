package services

import (
	"ultimate-dts-fix-server/backend/database"
	"ultimate-dts-fix-server/backend/models"
	"log"
	"time"
)

type QueueService struct {
	db        *database.TaskRepository
	taskChan  chan *models.Task
	stopChan  chan bool
	wsService *WebSocketService
}

func NewQueueService(db *database.TaskRepository) *QueueService {
	return &QueueService{
		db:       db,
		taskChan: make(chan *models.Task, 100),
		stopChan: make(chan bool),
	}
}

func (s *QueueService) SetWebSocketService(wsService *WebSocketService) {
	s.wsService = wsService
}

func (s *QueueService) Start() {
	log.Println("Сервис очереди запущен")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.broadcastQueueUpdate()
		case task := <-s.taskChan:
			s.addTask(task)
		case <-s.stopChan:
			log.Println("Сервис очереди остановлен")
			return
		}
	}
}

func (s *QueueService) Stop() {
	s.stopChan <- true
}

func (s *QueueService) AddTask(task *models.Task) {
	s.taskChan <- task
}

func (s *QueueService) addTask(task *models.Task) {
	if err := s.db.CreateTask(task); err != nil {
		log.Printf("Ошибка добавления задачи в базу: %v", err)
		return
	}
	
	log.Printf("Задача добавлена в очередь: %s", task.FilePath)
	s.broadcastQueueUpdate()
}

func (s *QueueService) broadcastQueueUpdate() {
	if s.wsService != nil {
		tasks, err := s.db.GetAllTasks()
		if err != nil {
			log.Printf("Ошибка получения задач для broadcast: %v", err)
			return
		}
		s.wsService.BroadcastQueueUpdate(tasks)
	}
}

func (s *QueueService) GetQueue() ([]*models.Task, error) {
	return s.db.GetAllTasks()
}