package handlers

import (
	"embed"
	"io/fs"
	"net/http"
	"ultimate-dts-fix-server/backend/services"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	wsService   *services.WebSocketService
	staticFiles embed.FS
}

func NewHandler(queueService *services.QueueService, converterService *services.ConverterService, wsService *services.WebSocketService, staticFiles embed.FS) *Handler {
	// Устанавливаем связи между сервисами
	queueService.SetWebSocketService(wsService)
	converterService.SetWebSocketService(wsService)

	return &Handler{
		wsService:   wsService,
		staticFiles: staticFiles,
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

	// WebSocket endpoint - единственный API endpoint
	router.GET("/ws", func(c *gin.Context) {
		h.wsService.HandleWebSocket(c.Writer, c.Request)
	})

	// Встроенные статические файлы
	staticFS, err := fs.Sub(h.staticFiles, "static")
	if err != nil {
		panic(err)
	}
	router.StaticFS("/static", http.FS(staticFS))

	// Корневой маршрут - раздаем index.html
	router.GET("/", func(c *gin.Context) {
		data, err := h.staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load index.html"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	return router
}
