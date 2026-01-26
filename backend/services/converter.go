package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"ultimate-dts-fix-server/backend/models"
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

	// Получаем длительность видео
	duration, durationErr := s.getVideoDuration(task.FilePath)
	if durationErr != nil {
		log.Printf("Предупреждение: не удалось получить длительность видео: %v", durationErr)
		duration = 0
	} else {
		task.Duration = duration
		log.Printf("Длительность видео: %.2f секунд (%.2f минут)", duration, duration/60)
	}

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

	// Логируем информацию об аудио (уже получена при добавлении в очередь)
	if task.AudioInfo != nil {
		log.Printf("Аудио формат: %s, каналы: %s (%d), sample rate: %s",
			task.AudioInfo.CodecName, task.AudioInfo.ChannelLayout,
			task.AudioInfo.Channels, task.AudioInfo.SampleRate)
	}

	// Обновляем задачу в базе через QueueService
	if err := s.queueService.UpdateTask(task); err != nil {
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
		log.Printf("FFmpeg завершил работу успешно для: %s", task.FilePath)

		task.Status = models.StatusCompleted
		now = time.Now()
		task.CompletedAt = &now
		task.Progress = 100
		log.Printf("Конвертация завершена: %s -> %s", task.FilePath, outputPath)

		// Проверяем существование выходного файла
		if _, err := os.Stat(outputPath); err != nil {
			log.Printf("ОШИБКА: Выходной файл не найден: %s, ошибка: %v", outputPath, err)
		} else {
			log.Printf("Выходной файл создан успешно: %s", outputPath)
		}

		// Переименовываем исходный файл в .bak
		log.Printf("Попытка переименовать исходный файл: %s", task.FilePath)
		if err := s.renameInputToBak(task.FilePath); err != nil {
			log.Printf("ОШИБКА: не удалось переименовать исходный файл в .bak: %v", err)
			log.Printf("Путь к файлу: %s", task.FilePath)
			// Проверяем существование исходного файла
			if _, statErr := os.Stat(task.FilePath); statErr != nil {
				log.Printf("ОШИБКА: Исходный файл не найден: %v", statErr)
			} else {
				log.Printf("Исходный файл существует, но не удалось переименовать")
			}
		} else {
			log.Printf("Исходный файл успешно переименован в .bak: %s", task.FilePath)
		}

		if s.wsService != nil {
			s.wsService.BroadcastConversionProgress(task.ID, 100, models.StatusCompleted,
				"Конвертация завершена")
			log.Printf("Отправлено WebSocket уведомление о завершении")
		}
	}

	// Обновляем задачу в базе через QueueService (это также отправит WebSocket уведомление)
	log.Printf("Обновление задачи в базе: ID=%s, Status=%s", task.ID, task.Status)
	if err := s.queueService.UpdateTask(task); err != nil {
		log.Printf("ОШИБКА обновления задачи после конвертации: %v", err)
	} else {
		log.Printf("Задача успешно обновлена в базе: %s, статус: %s", task.ID, task.Status)
	}

	log.Printf("Завершение обработки задачи: %s", task.ID)
}

func (s *ConverterService) generateOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	filename := filepath.Base(inputPath)

	// Используем регулярное выражение для замены DTS.*5.1 на FLAC.7.1
	// Паттерн: начинается с DTS, между ними могут быть точки, буквы и дефисы, заканчивается на 5.1
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
	re := regexp.MustCompile(`DTS[.\-A-Za-z]*5\.1`)
	baseName = re.ReplaceAllString(baseName, "FLAC.7.1")

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

// FFProbeFormat структура для получения длительности
type FFProbeFormat struct {
	Duration string `json:"duration"`
}

// FFProbeFormatOutput структура для парсинга вывода ffprobe с форматом
type FFProbeFormatOutput struct {
	Format FFProbeFormat `json:"format"`
}

// getAudioInfo получает информацию об аудио потоке через ffprobe
func (s *ConverterService) getAudioInfo(filePath string) (*AudioStreamInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "a:0",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения ffprobe: %v", err)
	}

	var probeOutput FFProbeOutput
	if err := json.Unmarshal(output, &probeOutput); err != nil {
		return nil, fmt.Errorf("ошибка парсинга вывода ffprobe: %v", err)
	}

	if len(probeOutput.Streams) == 0 {
		return nil, fmt.Errorf("аудио поток не найден")
	}

	return &probeOutput.Streams[0], nil
}

// getVideoDuration получает длительность видео через ffprobe
func (s *ConverterService) getVideoDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ошибка выполнения ffprobe: %v", err)
	}

	var probeOutput FFProbeFormatOutput
	if err := json.Unmarshal(output, &probeOutput); err != nil {
		return 0, fmt.Errorf("ошибка парсинга вывода ffprobe: %v", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(probeOutput.Format.Duration, "%f", &duration); err != nil {
		return 0, fmt.Errorf("ошибка парсинга длительности: %v", err)
	}

	return duration, nil
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
	const updateInterval = 2 * time.Second

	progressMap := make(map[string]string)

	for scanner.Scan() {
		line := scanner.Text()

		// Парсим ключ=значение из FFmpeg progress
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				progressMap[key] = value
			}
		}

		// Отправляем обновление только раз в 2 секунды
		if time.Since(lastUpdate) >= updateInterval && len(progressMap) > 0 {
			if s.wsService != nil {
				// Вычисляем прогресс
				progress := s.calculateProgress(progressMap, task)
				currentTime := s.parseCurrentTime(progressMap)

				// Обновляем текущее время в задаче
				task.CurrentTime = currentTime

				log.Printf("[FFmpeg Progress] Task %s: %.1f%% (%.1f/%.1f sec)",
					task.ID, progress, currentTime, task.Duration)

				s.wsService.BroadcastConversionProgress(
					task.ID,
					int(progress),
					models.StatusProcessing,
					"",
				)
			}
			lastUpdate = time.Now()
		}
	}

	// Отправляем последнее обновление если есть данные
	if len(progressMap) > 0 && s.wsService != nil {
		progress := s.calculateProgress(progressMap, task)
		currentTime := s.parseCurrentTime(progressMap)
		task.CurrentTime = currentTime

		log.Printf("[FFmpeg Progress Final] Task %s: %.1f%% (%.1f/%.1f sec)",
			task.ID, progress, currentTime, task.Duration)

		s.wsService.BroadcastConversionProgress(
			task.ID,
			int(progress),
			models.StatusProcessing,
			"",
		)
	}
}

// parseCurrentTime извлекает текущее время из прогресса FFmpeg
func (s *ConverterService) parseCurrentTime(progressMap map[string]string) float64 {
	outTime, ok := progressMap["out_time_ms"]
	if !ok || outTime == "" {
		// Пробуем out_time в формате HH:MM:SS.MS
		outTime, ok = progressMap["out_time"]
		if !ok || outTime == "" {
			return 0
		}
		// Парсим формат HH:MM:SS.MS
		var hours, minutes int
		var seconds float64
		if _, err := fmt.Sscanf(outTime, "%d:%d:%f", &hours, &minutes, &seconds); err == nil {
			return float64(hours*3600+minutes*60) + seconds
		}
		return 0
	}

	// out_time_ms в микросекундах
	var timeMs int64
	if _, err := fmt.Sscanf(outTime, "%d", &timeMs); err != nil {
		return 0
	}

	return float64(timeMs) / 1000000.0
}

// calculateProgress вычисляет процент прогресса
func (s *ConverterService) calculateProgress(progressMap map[string]string, task *models.Task) float64 {
	if task.Duration <= 0 {
		return 0
	}

	currentTime := s.parseCurrentTime(progressMap)
	if currentTime <= 0 {
		return 0
	}

	progress := (currentTime / task.Duration) * 100.0
	if progress > 100 {
		progress = 100
	}

	return progress
}

func (s *ConverterService) formatFFmpegProgress(progressMap map[string]string) string {
	var parts []string

	if frame, ok := progressMap["frame"]; ok && frame != "" {
		parts = append(parts, fmt.Sprintf("Frame: %s", frame))
	}

	if fps, ok := progressMap["fps"]; ok && fps != "" {
		parts = append(parts, fmt.Sprintf("FPS: %s", fps))
	}

	if speed, ok := progressMap["speed"]; ok && speed != "" {
		parts = append(parts, fmt.Sprintf("Speed: %s", speed))
	}

	if outTime, ok := progressMap["out_time"]; ok && outTime != "" {
		parts = append(parts, fmt.Sprintf("Time: %s", outTime))
	}

	if len(parts) == 0 {
		return "Processing..."
	}

	return strings.Join(parts, " | ")
}

func (s *ConverterService) readFFmpegStderr(stderr io.ReadCloser, task *models.Task) {
	scanner := bufio.NewScanner(stderr)
	lastUpdate := time.Now()
	const updateInterval = 2 * time.Second

	var stderrData strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		stderrData.WriteString(line)
		stderrData.WriteString("\n")

		// Отправляем обновление только раз в 2 секунды
		if time.Since(lastUpdate) >= updateInterval {
			if s.wsService != nil {
				stderrMsg := stderrData.String()
				log.Printf("[FFmpeg Stderr] Task %s: %s", task.ID, stderrMsg)
				s.wsService.BroadcastLog(stderrMsg, "info")
			}
			stderrData.Reset()
			lastUpdate = time.Now()
		}
	}

	// Отправляем последнее обновление если есть данные
	if stderrData.Len() > 0 && s.wsService != nil {
		stderrMsg := stderrData.String()
		log.Printf("[FFmpeg Stderr Final] Task %s: %s", task.ID, stderrMsg)
		s.wsService.BroadcastLog(stderrMsg, "info")
	}
}
