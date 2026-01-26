# Архитектура приложения

## Обзор

DTS to FLAC Converter - это компактное веб-приложение, упакованное в один Docker контейнер.

## Компоненты

### 1. Backend (Go)
- **Фреймворк**: Gin
- **Язык**: Go 1.21+
- **Функции**:
  - REST API для управления задачами
  - WebSocket для real-time обновлений
  - Очередь конвертации
  - Управление FFmpeg процессами
  - Хранение данных (JSON)

### 2. Frontend (Встроенный)
- **Технологии**: Vanilla JS, HTML5, CSS3
- **Встраивание**: Go embed directive
- **Функции**:
  - Поиск файлов по regex
  - Управление очередью
  - Real-time мониторинг
  - История конвертаций

### 3. FFmpeg
- **Версия**: Latest (Alpine)
- **Использование**: Конвертация аудио DTS → FLAC

## Схема работы

```
┌─────────────────────────────────────────┐
│         Docker Container                │
│                                         │
│  ┌───────────────────────────────────┐ │
│  │     Go Application (Port 3001)    │ │
│  │                                   │ │
│  │  ┌─────────────┐  ┌────────────┐ │ │
│  │  │   Embedded  │  │    API     │ │ │
│  │  │   Frontend  │  │  Handlers  │ │ │
│  │  │  (Static)   │  │            │ │ │
│  │  └─────────────┘  └────────────┘ │ │
│  │                                   │ │
│  │  ┌─────────────┐  ┌────────────┐ │ │
│  │  │  WebSocket  │  │   Queue    │ │ │
│  │  │   Service   │  │  Service   │ │ │
│  │  └─────────────┘  └────────────┘ │ │
│  │                                   │ │
│  │  ┌─────────────┐  ┌────────────┐ │ │
│  │  │  Converter  │  │   FFmpeg   │ │ │
│  │  │   Service   │  │  Wrapper   │ │ │
│  │  └─────────────┘  └────────────┘ │ │
│  └───────────────────────────────────┘ │
│                                         │
│  ┌───────────────────────────────────┐ │
│  │      Data Storage (JSON)          │ │
│  │  /app/data/tasks.json             │ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
         │
         │ Port 6969 (Host)
         ↓
    [Browser Client]
```

## Поток данных

### 1. Поиск файлов
```
Browser → GET /api/search-files
       → Backend сканирует /media/*
       → Возвращает список файлов
       → Browser отображает результаты
```

### 2. Добавление в очередь
```
Browser → POST /api/tasks {filePath}
       → Backend добавляет в очередь
       → WebSocket уведомляет клиентов
       → Browser обновляет UI
```

### 3. Конвертация
```
Queue Service → Берет задачу из очереди
             → Converter Service запускает FFmpeg
             → FFmpeg конвертирует файл
             → WebSocket отправляет прогресс
             → Browser показывает real-time вывод
             → Задача перемещается в историю
```

## Технические решения

### Встраивание статики
```go
//go:embed static/*
var staticFiles embed.FS
```

Преимущества:
- Один бинарник содержит всё
- Нет внешних зависимостей
- Быстрая раздача статики
- Упрощенное развертывание

### WebSocket для real-time обновлений
```javascript
ws = new WebSocket('ws://localhost:6969/ws')
```

Типы сообщений:
- `queue_update` - изменения в очереди
- `conversion_progress` - прогресс FFmpeg
- `log` - системные логи

### Управление FFmpeg процессами
```go
ctx, cancel := context.WithCancel(context.Background())
cmd := exec.CommandContext(ctx, "ffmpeg", args...)
```

Возможности:
- Graceful cancellation через context
- Захват stdout/stderr
- Throttling обновлений (10 сек)

### Хранение данных
```json
{
  "tasks": [
    {
      "id": "20240122123456",
      "filePath": "/media/movie.mkv",
      "status": "completed",
      "createdAt": "2024-01-22T12:34:56Z"
    }
  ]
}
```

Простой JSON файл вместо SQLite:
- Легкий бэкап
- Читаемый формат
- Нет CGO зависимостей
- Достаточно для задачи

## Безопасность

### WebSocket Origin Validation
```go
if origin != expectedOrigin {
    return errors.New("invalid origin")
}
```

### Изоляция файловой системы
- Доступ только к примонтированным томам
- Нет автоматического сканирования
- Ручное добавление файлов

### Минимальные права
```yaml
user: "1000:1000"
```

## Производительность

### Память
- Go процесс: 5-15 MB
- Embedded статика: ~500 KB
- FFmpeg: зависит от файла

### CPU
- Go процесс: минимальное использование
- FFmpeg: 1-2 ядра при конвертации

### Диск
- Образ: ~50-60 MB
- Временные файлы: 2x размер исходного файла

## Масштабирование

Текущая версия:
- ✅ Один файл за раз
- ✅ Очередь задач
- ✅ Один контейнер

Возможные улучшения:
- Параллельная конвертация
- Распределенная очередь
- Кластер контейнеров
- Shared storage

## Зависимости

### Runtime
- Alpine Linux
- FFmpeg
- ca-certificates

### Build
- Go 1.21+
- Go modules

### Frontend
- Нет внешних зависимостей
- Vanilla JavaScript
- Pure CSS

## Развертывание

### Docker
```bash
docker-compose up -d
```

### Без Docker
```bash
cd backend
go build -o dts-converter .
./dts-converter
```

Требования:
- FFmpeg в PATH
- Права на чтение/запись медиатек