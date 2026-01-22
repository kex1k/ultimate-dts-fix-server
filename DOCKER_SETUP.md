# Настройка Docker для DTS to FLAC Converter

## Обзор

Документ описывает настройку Docker окружения для сервиса конвертации DTS в FLAC.

## Файлы конфигурации

### docker-compose.yml
```yaml
version: '3.8'

services:
  backend:
    build: ./backend
    container_name: dts-converter-backend
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
      - /path/to/your/videos:/videos:ro
      - /path/to/output:/output
    environment:
      - GIN_MODE=release
      - DATABASE_PATH=/app/data/database.sqlite
      - LOG_LEVEL=info
      - MAX_CONCURRENT_TASKS=1
    ports:
      - "3001:3001"
    restart: unless-stopped

  frontend:
    image: nginx:alpine
    container_name: dts-converter-frontend
    volumes:
      - ./frontend:/usr/share/nginx/html
      - ./frontend/nginx.conf:/etc/nginx/nginx.conf
    ports:
      - "3000:80"
    depends_on:
      - backend
    restart: unless-stopped

volumes:
  data:
    driver: local
  logs:
    driver: local
```

### Backend Dockerfile
```dockerfile
# backend/Dockerfile
FROM golang:1.21-alpine AS builder

# Устанавливаем FFmpeg
RUN apk add --no-cache ffmpeg

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates ffmpeg
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/data ./data

EXPOSE 3001
CMD ["./main"]
```

### Frontend nginx.conf
```nginx
# frontend/nginx.conf
events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    server {
        listen 80;
        server_name localhost;

        root /usr/share/nginx/html;
        index index.html;

        # Статические файлы
        location / {
            try_files $uri $uri/ /index.html;
        }

        # Прокси для API
        location /api {
            proxy_pass http://backend:3001;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        # Прокси для WebSocket
        location /ws {
            proxy_pass http://backend:3001;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
```

## Установка и запуск

### 1. Подготовка директорий
```bash
# Создаем необходимые директории
mkdir -p data logs
mkdir -p /path/to/videos  # Директория с исходными файлами
mkdir -p /path/to/output  # Директория для результатов
```

### 2. Настройка прав доступа
```bash
# Даем права на запись в директории данных
chmod 755 data logs
chmod 755 /path/to/output

# Проверяем права доступа к видео файлам
ls -la /path/to/videos
```

### 3. Запуск сервиса
```bash
# Запускаем все сервисы
docker-compose up -d

# Проверяем статус
docker-compose ps

# Просмотр логов
docker-compose logs -f backend
```

### 4. Проверка работоспособности
```bash
# Проверяем доступность API
curl http://localhost:3001/api/status

# Открываем веб-интерфейс
# http://localhost:3000
```

## Оптимизация ресурсов

### Ограничение ресурсов:
```yaml
# В docker-compose.yml для backend
backend:
  # Ограничиваем ресурсы
  deploy:
    resources:
      limits:
        memory: 50M
        cpus: '0.5'
      reservations:
        memory: 20M
        cpus: '0.25'
```

### Переменные окружения для оптимизации:
```bash
# Backend environment
MAX_CONCURRENT_TASKS=1          # Одна задача за раз
GIN_MODE=release                # Продакшен режим
LOG_LEVEL=warn                  # Минимальное логирование
```

## Мониторинг и управление

### Просмотр состояния:
```bash
# Статус контейнеров
docker-compose ps

# Использование ресурсов
docker stats

# Логи в реальном времени
docker-compose logs -f
```

### Остановка и перезапуск:
```bash
# Остановка сервиса
docker-compose down

# Перезапуск с обновлениями
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Бэкап данных:
```bash
# Бэкап базы данных
docker-compose exec backend cp /app/data/database.sqlite ./backup/

# Бэкап логов
tar -czf logs-backup-$(date +%Y%m%d).tar.gz ./logs
```

## Решение проблем

### Частые проблемы:

**Недостаточно прав доступа:**
```bash
# Проверяем права на mounted volumes
ls -la /path/to/videos
ls -la /path/to/output

# Исправляем права
chmod 755 /path/to/videos
chmod 777 /path/to/output
```

**FFmpeg не найден:**
```bash
# Проверяем установку FFmpeg в контейнере
docker-compose exec backend which ffmpeg
```

**Проблемы с WebSocket:**
```bash
# Проверяем подключение WebSocket
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" http://localhost:3001/ws
```

Эта конфигурация обеспечивает стабильную работу сервиса с минимальным потреблением ресурсов.