package handlers

import (
	"ultimate-dts-fix-server/backend/models"
	"ultimate-dts-fix-server/backend/services"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	
	// Проверяем, что файл не видео
	if !isVideoFile(request.FilePath) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Файл не является видеофайлом: " + request.FilePath,
		})
		return
	}
	
	task := &models.Task{
		ID:        generateTaskID(),
		FilePath:  request.FilePath,
		Status:    models.StatusPending,
		Progress:  0,
		CreatedAt: time.Now(),
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