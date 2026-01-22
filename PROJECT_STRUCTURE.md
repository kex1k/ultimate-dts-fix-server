# Структура проекта DTS to FLAC Converter

## Обзор структуры

```
dts-converter/
├── README.md                    # Основная документация
├── ARCHITECTURE.md              # Архитектура системы
├── FINAL_RECOMMENDATION.md      # Финальная рекомендация
├── PROJECT_STRUCTURE.md         # Этот файл
├── DOCKER_SETUP.md              # Настройка Docker
├── dts-2-flac.txt              # Рабочая FFmpeg команда
├── docker-compose.yml           # Docker конфигурация
├── .env.example                # Пример переменных окружения
├── .gitignore                  # Git ignore файл
│
├── backend/                    # Backend сервис на Go
│   ├── Dockerfile              # Docker образ для backend
│   ├── go.mod                  # Зависимости Go
│   ├── go.sum                  # Lock файл зависимостей
│   ├── main.go                 # Главный файл приложения
│   ├── models/                 # Модели данных
│   │   └── task.go             # Модель задачи
│   ├── services/               # Сервисы
│   │   ├── converter.go        # Сервис конвертации
│   │   └── queue.go            # Сервис очереди
│   ├── handlers/               # Обработчики HTTP
│   │   ├── api.go              # API обработчики
│   │   └── websocket.go        # WebSocket обработчики
│   └── database/               # Работа с базой данных
│       └── db.go               # Подключение к SQLite
│
└── frontend/                   # Frontend приложение
    ├── index.html              # Главная страница
    ├── app.js                  # JavaScript приложение
    ├── styles.css              # Стили
    └── nginx.conf              # Конфигурация Nginx
```

## Описание компонентов

### Backend (Go)

#### Основные файлы:
- **main.go** - точка входа приложения
- **go.mod/go.sum** - управление зависимостями

#### Модели:
- **task.go** - структура задачи конвертации

#### Сервисы:
- **converter.go** - управление FFmpeg конвертацией
- **queue.go** - управление очередью задач

#### Обработчики:
- **api.go** - REST API эндпоинты
- **websocket.go** - WebSocket соединения

#### База данных:
- **db.go** - работа с SQLite

### Frontend (Vanilla JavaScript)

#### Основные файлы:
- **index.html** - HTML структура
- **app.js** - JavaScript логика
- **styles.css** - стили интерфейса
- **nginx.conf** - конфигурация веб-сервера

## Потоки данных

### 1. Поток конвертации:
```
Frontend → API → QueueService → ConverterService → FFmpeg → WebSocket → Frontend
```

### 2. Поток сканирования:
```
Frontend → API → ScannerService → FFprobe → QueueService → WebSocket → Frontend
```

### 3. Поток конфигурации:
```
Frontend → API → ConfigService → SQLite → WebSocket → Frontend
```

## Зависимости

### Backend зависимости:
- **github.com/gin-gonic/gin** - HTTP фреймворк
- **github.com/gorilla/websocket** - WebSocket поддержка
- **github.com/mattn/go-sqlite3** - драйвер SQLite

### Frontend зависимости:
- **Чистый JavaScript** - без внешних библиотек
- **WebSocket API** - встроенный в браузеры

## Конфигурация Docker

### Сервисы:
- **backend** - Go приложение на порту 3001
- **frontend** - Nginx на порту 3000

### Volumes:
- **./data** - база данных и конфигурация
- **/path/to/videos** - исходные видео файлы (read-only)
- **/path/to/output** - конвертированные файлы

## Переменные окружения

### Backend:
```bash
GIN_MODE=release
DATABASE_PATH=/app/data/database.sqlite
LOG_LEVEL=info
MAX_CONCURRENT_TASKS=1
```

### Frontend:
```bash
REACT_APP_API_URL=http://localhost:3001
REACT_APP_WS_URL=ws://localhost:3001
```

Эта структура обеспечивает четкое разделение ответственности и масштабируемость приложения с оптимизированным потреблением ресурсов.