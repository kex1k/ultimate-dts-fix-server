package services

import (
	"ultimate-dts-fix-server/backend/models"
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ConverterService struct {
	queueService   *QueueService
	stopChan       chan bool
	wsService      *WebSocketService
	activeTask     *models.Task
	activeCmd      *exec.Cmd
	activeCtx      context.Context
	activeCancelFn context.CancelFunc
	mu             sync.RWMutex
}

func NewConverterService(queueService *QueueService) *ConverterService {
	return &ConverterService{
		queueService: queueService,
		stopChan:     make(chan bool),
	}
}

func (s *ConverterService) SetWebSocketService(wsService *WebSocketService) {
	s.wsService = wsService
}

func (s *ConverterService) Start() {
	log.Println("Сервис конвертации запущен")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkForConversion()
		case <-s.stopChan:
			log.Println("Сервис конвертации остановлен")
			return
		}
	}
}

func (s *ConverterService) Stop() {
	s.stopChan <- true
}

func (s *ConverterService) checkForConversion() {
	// Получаем задачи для конвертации
	tasks, err := s.queueService.db.GetPendingTasks()
	if err != nil {
		log.Printf("Ошибка получения задач для конвертации: %v", err)
		return
	}

	// Обрабатываем только первую задачу (одна задача за раз)
	if len(tasks) > 0 {
		task := tasks[0]
		if task.Status == models.StatusPending {
			go s.convertTask(task)
		}
	}
}

func (s *ConverterService) CancelConversion(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeTask == nil || s.activeTask.ID != taskID {
		return fmt.Errorf("задача не активна")
	}

	if s.activeCancelFn != nil {
		s.activeCancelFn()
		log.Printf("Отмена конвертации задачи: %s", taskID)
		return nil
	}

	return fmt.Errorf("невозможно отменить задачу")
}

func (s *ConverterService) GetActiveTask() *models.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTask
}

func (s *ConverterService) convertTask(task *models.Task) {
	log.Printf("Начало конвертации: %s", task.FilePath)
	
	// Устанавливаем активную задачу
	s.mu.Lock()
	s.activeTask = task
	ctx, cancel := context.WithCancel(context.Background())
	s.activeCtx = ctx
	s.activeCancelFn = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.activeTask = nil
		s.activeCmd = nil
		s.activeCtx = nil
		s.activeCancelFn = nil
		s.mu.Unlock()
	}()

	// Обновляем статус задачи
	task.Status = models.StatusProcessing
	now := time.Now()
	task.StartedAt = &now
	
	// Обновляем задачу в базе
	if err := s.queueService.db.UpdateTask(task); err != nil {
		log.Printf("Ошибка обновления статуса задачи: %v", err)
		return
	}
	
	if s.wsService != nil {
		s.wsService.BroadcastConversionProgress(task.ID, 0, models.StatusProcessing, "Начало конвертации")
	}
	
	// Генерируем путь для выходного файла
	outputPath := s.generateOutputPath(task.FilePath)
	task.OutputPath = outputPath
	
	// Выполняем конвертацию
	err := s.executeFFmpegConversion(ctx, task)
	
	if err != nil {
		if ctx.Err() == context.Canceled {
			task.Status = models.StatusError
			task.Error = "Конвертация отменена пользователем"
			log.Printf("Конвертация отменена: %s", task.FilePath)
			
			if s.wsService != nil {
				s.wsService.BroadcastConversionProgress(task.ID, 0, models.StatusError,
					"Конвертация отменена")
			}
		} else {
			task.Status = models.StatusError
			task.Error = err.Error()
			log.Printf("Ошибка конвертации: %v", err)
			
			if s.wsService != nil {
				s.wsService.BroadcastConversionProgress(task.ID, 0, models.StatusError,
					"Ошибка конвертации: "+err.Error())
			}
		}
	} else {
		task.Status = models.StatusCompleted
		now = time.Now()
		task.CompletedAt = &now
		task.Progress = 100
		log.Printf("Конвертация завершена: %s -> %s", task.FilePath, outputPath)
		
		// Переименовываем исходный файл в .bak
		if err := s.renameInputToBak(task.FilePath); err != nil {
			log.Printf("Предупреждение: не удалось переименовать исходный файл в .bak: %v", err)
		}
		
		if s.wsService != nil {
			s.wsService.BroadcastConversionProgress(task.ID, 100, models.StatusCompleted,
				"Конвертация завершена")
		}
	}
	
	// Обновляем задачу в базе
	if err := s.queueService.db.UpdateTask(task); err != nil {
		log.Printf("Ошибка обновления задачи после конвертации: %v", err)
	}
}

func (s *ConverterService) generateOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	filename := filepath.Base(inputPath)
	
	// Заменяем ТОЛЬКО "DTS-HD.5.1" на "FLAC.7.1" в имени файла
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
	baseName = strings.ReplaceAll(baseName, "DTS-HD.5.1", "FLAC.7.1")
	
	// Добавляем суффикс если файл уже существует
	outputPath := filepath.Join(dir, baseName+filepath.Ext(filename))
	counter := 1
	
	for {
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			break
		}
		outputPath = filepath.Join(dir, fmt.Sprintf("%s_%d%s", baseName, counter, filepath.Ext(filename)))
		counter++
	}
	
	return outputPath
}

func (s *ConverterService) renameInputToBak(inputPath string) error {
	bakPath := inputPath + ".bak"
	
	// Проверяем, не существует ли уже .bak файл
	if _, err := os.Stat(bakPath); err == nil {
		// Если существует, добавляем счетчик
		counter := 1
		for {
			bakPath = fmt.Sprintf("%s.bak.%d", inputPath, counter)
			if _, err := os.Stat(bakPath); os.IsNotExist(err) {
				break
			}
			counter++
		}
	}
	
	return os.Rename(inputPath, bakPath)
}

func (s *ConverterService) executeFFmpegConversion(ctx context.Context, task *models.Task) error {
	// Команда FFmpeg на основе dts-2-flac.txt
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", task.FilePath,
		"-c:v", "copy",
		"-c:s", "copy",
		"-c:a:0", "flac",
		"-b:a", "384k",
		"-map", "0",
		"-compression_level", "8",
		"-channel_layout", "7.1",
		"-ac", "8",
		"-af", "pan=7.1|FL=FL|FR=FR|FC=FC|LFE=LFE|BL=SL|BR=SR|SL=SL|SR=SR",
		"-progress", "pipe:1",
		"-nostats",
		"-loglevel", "info",
		task.OutputPath,
	)
	
	// Сохраняем команду для возможной отмены
	s.mu.Lock()
	s.activeCmd = cmd
	s.mu.Unlock()
	
	// Создаем pipe для чтения прогресса
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ошибка создания pipe: %v", err)
	}
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ошибка создания stderr pipe: %v", err)
	}
	
	// Запускаем команду
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ошибка запуска FFmpeg: %v", err)
	}
	
	// Читаем прогресс в реальном времени с throttling
	go s.readFFmpegProgress(stdout, task)
	
	// Читаем stderr для логирования
	go s.readFFmpegStderr(stderr, task)
	
	// Ждем завершения команды
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		return fmt.Errorf("ошибка конвертации: %v", err)
	}
	
	return nil
}

func (s *ConverterService) readFFmpegProgress(stdout io.ReadCloser, task *models.Task) {
	scanner := bufio.NewScanner(stdout)
	lastUpdate := time.Now()
	const updateInterval = 10 * time.Second
	
	var progressData strings.Builder
	
	for scanner.Scan() {
		line := scanner.Text()
		progressData.WriteString(line)
		progressData.WriteString("\n")
		
		// Отправляем обновление только раз в 10 секунд
		if time.Since(lastUpdate) >= updateInterval {
			if s.wsService != nil {
				s.wsService.BroadcastConversionProgress(
					task.ID,
					0,
					models.StatusProcessing,
					progressData.String(),
				)
			}
			progressData.Reset()
			lastUpdate = time.Now()
		}
	}
	
	// Отправляем последнее обновление если есть данные
	if progressData.Len() > 0 && s.wsService != nil {
		s.wsService.BroadcastConversionProgress(
			task.ID,
			0,
			models.StatusProcessing,
			progressData.String(),
		)
	}
}

func (s *ConverterService) readFFmpegStderr(stderr io.ReadCloser, task *models.Task) {
	scanner := bufio.NewScanner(stderr)
	lastUpdate := time.Now()
	const updateInterval = 10 * time.Second
	
	var stderrData strings.Builder
	
	for scanner.Scan() {
		line := scanner.Text()
		stderrData.WriteString(line)
		stderrData.WriteString("\n")
		
		// Отправляем обновление только раз в 10 секунд
		if time.Since(lastUpdate) >= updateInterval {
			if s.wsService != nil {
				s.wsService.BroadcastLog(stderrData.String(), "info")
			}
			stderrData.Reset()
			lastUpdate = time.Now()
		}
	}
	
	// Отправляем последнее обновление если есть данные
	if stderrData.Len() > 0 && s.wsService != nil {
		s.wsService.BroadcastLog(stderrData.String(), "info")
	}
}