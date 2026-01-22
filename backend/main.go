package main

import (
	"ultimate-dts-fix-server/backend/database"
	"ultimate-dts-fix-server/backend/handlers"
	"ultimate-dts-fix-server/backend/services"
	"log"
	"os"
)

func main() {
	// Инициализация хранилища данных
	db, err := database.InitDB()
	if err != nil {
		log.Fatal("Ошибка инициализации хранилища данных:", err)
	}
	defer db.Close()

	// Инициализация сервисов
	queueService := services.NewQueueService(db)
	converterService := services.NewConverterService(queueService)
	wsService := services.NewWebSocketService()

	// Установка связей между сервисами
	queueService.SetWebSocketService(wsService)
	converterService.SetWebSocketService(wsService)

	// Запуск сервисов
	go queueService.Start()
	go converterService.Start()

	// Инициализация обработчиков HTTP
	handler := handlers.NewHandler(queueService, converterService, wsService)

	// Запуск HTTP сервера
	port := getPort()
	log.Printf("Сервер запущен на порту %s", port)
	
	if err := handler.Start(":" + port); err != nil {
		log.Fatal("Ошибка запуска сервера:", err)
	}
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
	return port
}