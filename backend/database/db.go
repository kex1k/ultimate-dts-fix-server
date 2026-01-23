package database

import (
	"log"
	"os"
	"path/filepath"
	"ultimate-dts-fix-server/backend/models"
)

// TaskRepository предоставляет методы для работы с задачами
type TaskRepository struct {
	store *JSONStore
}

// InitDB инициализирует хранилище данных
func InitDB() (*TaskRepository, error) {
	// Создаем директорию data если её нет
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	// Путь к JSON файлу
	dbPath := filepath.Join(dataDir, "tasks.json")
	if envPath := os.Getenv("DATABASE_PATH"); envPath != "" {
		dbPath = envPath
	}

	// Создаем JSON store
	store, err := NewJSONStore(dbPath)
	if err != nil {
		return nil, err
	}

	log.Printf("JSON хранилище инициализировано: %s", dbPath)

	return &TaskRepository{store: store}, nil
}

// CreateTask создает новую задачу
func (r *TaskRepository) CreateTask(task *models.Task) error {
	return r.store.CreateTask(task)
}

// UpdateTask обновляет задачу
func (r *TaskRepository) UpdateTask(task *models.Task) error {
	return r.store.UpdateTask(task)
}

// GetPendingTasks возвращает задачи в статусе pending или processing
func (r *TaskRepository) GetPendingTasks() ([]*models.Task, error) {
	return r.store.GetPendingTasks()
}

// GetAllTasks возвращает все задачи
func (r *TaskRepository) GetAllTasks() ([]*models.Task, error) {
	return r.store.GetAllTasks()
}

// GetTask возвращает задачу по ID
func (r *TaskRepository) GetTask(taskID string) (*models.Task, error) {
	return r.store.GetTask(taskID)
}

// DeleteTask удаляет задачу по ID
func (r *TaskRepository) DeleteTask(taskID string) error {
	return r.store.DeleteTask(taskID)
}

// Close закрывает хранилище
func (r *TaskRepository) Close() error {
	return r.store.Close()
}
