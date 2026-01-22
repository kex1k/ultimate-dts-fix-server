package database

import (
	"ultimate-dts-fix-server/backend/models"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JSONStore - простое хранилище на основе JSON файла
type JSONStore struct {
	tasks    map[string]*models.Task
	mu       sync.RWMutex
	filePath string
}

// NewJSONStore создает новое хранилище
func NewJSONStore(filePath string) (*JSONStore, error) {
	store := &JSONStore{
		tasks:    make(map[string]*models.Task),
		filePath: filePath,
	}

	// Создаем директорию если не существует
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Загружаем существующие данные
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Запускаем автосохранение каждые 30 секунд
	go store.autoSave()

	return store, nil
}

// CreateTask создает новую задачу
func (s *JSONStore) CreateTask(task *models.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks[task.ID] = task
	return s.save()
}

// UpdateTask обновляет задачу
func (s *JSONStore) UpdateTask(task *models.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks[task.ID] = task
	return s.save()
}

// GetPendingTasks возвращает задачи в статусе pending или processing
func (s *JSONStore) GetPendingTasks() ([]*models.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*models.Task
	for _, task := range s.tasks {
		if task.Status == models.StatusPending || task.Status == models.StatusProcessing {
			tasks = append(tasks, task)
		}
	}

	// Сортируем по времени создания
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].CreatedAt.After(tasks[j].CreatedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}

	return tasks, nil
}

// GetAllTasks возвращает все задачи
func (s *JSONStore) GetAllTasks() ([]*models.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*models.Task
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}

	// Сортируем по времени создания (новые первыми)
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].CreatedAt.Before(tasks[j].CreatedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}

	// Ограничиваем до 100 задач
	if len(tasks) > 100 {
		tasks = tasks[:100]
	}

	return tasks, nil
}

// save сохраняет данные в файл
func (s *JSONStore) save() error {
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// load загружает данные из файла
func (s *JSONStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.tasks)
}

// autoSave периодически сохраняет данные
func (s *JSONStore) autoSave() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		if len(s.tasks) > 0 {
			s.mu.RUnlock()
			s.mu.Lock()
			_ = s.save()
			s.mu.Unlock()
		} else {
			s.mu.RUnlock()
		}
	}
}

// Close закрывает хранилище и сохраняет данные
func (s *JSONStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}