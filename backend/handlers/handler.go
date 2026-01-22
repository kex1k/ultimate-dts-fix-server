package handlers

import (
	"ultimate-dts-fix-server/backend/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	apiHandler *APIHandler
	wsService  *services.WebSocketService
}

func NewHandler(queueService *services.QueueService, converterService *services.ConverterService, wsService *services.WebSocketService) *Handler {
	// Устанавливаем связи между сервисами
	queueService.SetWebSocketService(wsService)
	converterService.SetWebSocketService(wsService)
	
	apiHandler := NewAPIHandler(queueService, converterService, wsService)
	
	return &Handler{
		apiHandler: apiHandler,
		wsService:  wsService,
	}
}

func (h *Handler) Start(addr string) error {
	router := h.setupRouter()
	return router.Run(addr)
}

func (h *Handler) setupRouter() *gin.Engine {
	if gin.Mode() == gin.ReleaseMode {
		gin.SetMode(gin.ReleaseMode)
	}
	
	router := gin.Default()
	
	// API маршруты
	api := router.Group("/api")
	{
		api.GET("/status", h.apiHandler.Status)
		api.GET("/queue", h.apiHandler.GetQueue)
		api.GET("/active-task", h.apiHandler.GetActiveTask)
		api.GET("/history", h.apiHandler.GetHistory)
		api.POST("/tasks", h.apiHandler.AddTask)
		api.POST("/tasks/cancel", h.apiHandler.CancelTask)
	}
	
	// WebSocket маршрут
	router.GET("/ws", func(c *gin.Context) {
		h.wsService.HandleWebSocket(c.Writer, c.Request)
	})
	
	// Статические файлы для разработки
	router.StaticFS("/static", http.Dir("./static"))
	
	// Корневой маршрут для проверки
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "DTS to FLAC Converter API",
			"version": "1.0.0",
		})
	})
	
	return router
}