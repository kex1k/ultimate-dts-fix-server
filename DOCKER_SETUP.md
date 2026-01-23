# Настройка Docker для DTS to FLAC Converter

## Обзор

Документ описывает настройку Docker окружения для сервиса конвертации DTS-HD MA 5.1 в FLAC 7.1.

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
      # Поддержка нескольких директорий с видеофайлами
      - /path/to/media/library1:/media/library1
      - /path/to/media/library2:/media/library2
      - /path/to/media/library3:/media/library3
      # Можно добавить любое количество директорий
    environment:
      - GIN_MODE=release
      - DATABASE_PATH=/app/data/tasks.json
      - LOG_LEVEL=info
      - PORT=3001
    ports:
      - "3001:3001"
    restart: unless-stopped
    user: "1000:1000"  # Замените на ваши UID:GID

  frontend:
    image: nginx:alpine
    container_name: dts-converter-frontend
    volumes:
      - ./frontend:/usr/share/nginx/html
      - ./frontend/nginx.conf:/etc/nginx/nginx.conf
    ports:
      - "6969:80"
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

## Логика работы с файлами

### Сохранение конвертированных файлов

**Важно:** Система спроектирована так, что конвертированные файлы сохраняются в ту же директорию, где находится исходный файл. Это обеспечивает:

1. **Сохранение структуры** - файлы остаются в своих директориях
2. **Удобство навигации** - не нужно искать конвертированные файлы в отдельной папке
3. **Простоту организации** - каждая медиатека остается самодостаточной
4. **Совместимость** - сохранение расширения .mkv обеспечивает совместимость с медиаплеерами

**Пример:**
- Исходный файл: `/media/library1/movies/Action/movie.DTS-HD.5.1.mkv`
- Конвертированный файл: `/media/library1/movies/Action/movie.FLAC.7.1.mkv`
- Бэкап исходного: `/media/library1/movies/Action/movie.DTS-HD.5.1.mkv.bak`

### Правила именования

- **Точная замена**: Только строка `"DTS-HD.5.1"` заменяется на `"FLAC.7.1"`
- **Расширение сохраняется**: `.mkv` остается без изменений
- **Бэкап создается**: Исходный файл переименовывается с добавлением `.bak`

### Добавление файлов

Файлы добавляются **вручную** через веб-интерфейс:
1. Введите полный путь к файлу (например: `/media/library1/movie.mkv`)
2. Нажмите "Добавить" для добавления в список
3. Нажмите "Добавить все в очередь" для начала конвертации

**Примечание:** Автоматическое сканирование директорий удалено для безопасности.

## Установка и запуск

### 1. Подготовка директорий
```bash
# Создаем необходимые директории для сервиса
mkdir -p data logs

# Создаем директории для медиафайлов (может быть любое количество)
mkdir -p /path/to/media/library1
mkdir -p /path/to/media/library2
mkdir -p /path/to/media/library3
```

### 2. Настройка прав доступа
```bash
# Даем права на запись в директории данных
chmod 755 data logs

# Даем права на запись в медиатеки (для сохранения конвертированных файлов)
chmod 755 /path/to/media/library1
chmod 755 /path/to/media/library2
chmod 755 /path/to/media/library3

# Проверяем права доступа к медиафайлам
ls -la /path/to/media/library1
ls -la /path/to/media/library2
ls -la /path/to/media/library3
```

### 3. Настройка docker-compose.yml
Перед запуском убедитесь, что в `docker-compose.yml` правильно указаны пути к вашим директориям:

```yaml
volumes:
  # Замените /path/to/media/library1 на ваши реальные пути
  - /your/real/path/library1:/media/library1
  - /your/real/path/library2:/media/library2
  - /your/real/path/library3:/media/library3
```

**Важно:** Переменная окружения `MEDIA_DIRS` больше не используется. Файлы добавляются вручную через веб-интерфейс.

### 4. Настройка пользователя
Измените `user` в docker-compose.yml на ваши UID:GID:

```bash
# Узнайте ваши UID и GID
id -u  # UID
id -g  # GID

# Обновите в docker-compose.yml
user: "1000:1000"  # Замените на ваши значения
```

### 5. Запуск сервиса
```bash
# Запускаем все сервисы
docker-compose up -d

# Проверяем статус
docker-compose ps

# Просмотр логов
docker-compose logs -f backend
```

### 6. Проверка работоспособности
```bash
# Проверяем доступность API
curl http://localhost:3001/api/status

# Открываем веб-интерфейс
# http://localhost:6969
```

## Использование веб-интерфейса

### Добавление файлов в очередь
1. Откройте http://localhost:6969
2. Введите полный путь к файлу в поле ввода
3. Нажмите "Добавить" для добавления в список
4. Повторите для всех нужных файлов
5. Нажмите "Добавить все в очередь"

### Мониторинг конвертации
- **Текущая конвертация**: Показывает активную задачу с выводом FFmpeg
- **Очередь конвертации**: Список ожидающих задач
- **История конвертаций**: Завершенные задачи с временем и статусом
- **Логи**: Real-time логи системы

### Отмена конвертации
Нажмите кнопку "Отменить" в секции "Текущая конвертация" для остановки процесса.

## Оптимизация ресурсов

### Ограничение ресурсов:
```yaml
# В docker-compose.yml для backend
backend:
  deploy:
    resources:
      limits:
        memory: 50M
        cpus: '0.5'
      reservations:
        memory: 20M
        cpus: '0.25'
```

### Переменные окружения:
```bash
# Backend environment
GIN_MODE=release                # Продакшен режим
LOG_LEVEL=warn                  # Минимальное логирование
PORT=3001                       # Порт API
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

# Логи только backend
docker-compose logs -f backend

# Логи только frontend
docker-compose logs -f frontend
```

### Остановка и перезапуск:
```bash
# Остановка сервиса
docker-compose down

# Перезапуск с обновлениями
docker-compose down
docker-compose build --no-cache
docker-compose up -d

# Перезапуск без пересборки
docker-compose restart
```

### Бэкап данных:
```bash
# Бэкап JSON хранилища
cp ./data/tasks.json ./backup/tasks-$(date +%Y%m%d).json

# Бэкап логов
tar -czf logs-backup-$(date +%Y%m%d).tar.gz ./logs

# Полный бэкап
tar -czf full-backup-$(date +%Y%m%d).tar.gz ./data ./logs
```

## Примеры конфигурации

### Пример 1: Домашняя медиатека
```yaml
version: '3.8'

services:
  backend:
    build: ./backend
    container_name: dts-converter-backend
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
      # Домашняя медиатека
      - /home/user/Movies:/media/movies
      - /home/user/TVShows:/media/tvshows
      - /home/user/Documentaries:/media/docs
    environment:
      - GIN_MODE=release
      - DATABASE_PATH=/app/data/tasks.json
      - LOG_LEVEL=info
      - PORT=3001
    ports:
      - "3001:3001"
    restart: unless-stopped
    user: "1000:1000"

  frontend:
    image: nginx:alpine
    container_name: dts-converter-frontend
    volumes:
      - ./frontend:/usr/share/nginx/html
      - ./frontend/nginx.conf:/etc/nginx/nginx.conf
    ports:
      - "6969:80"
    depends_on:
      - backend
    restart: unless-stopped
```

### Пример 2: Сервер с несколькими дисками
```yaml
version: '3.8'

services:
  backend:
    build: ./backend
    container_name: dts-converter-backend
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
      # Множественные диски
      - /mnt/disk1/media:/media/disk1
      - /mnt/disk2/media:/media/disk2
      - /mnt/disk3/media:/media/disk3
      - /mnt/storage/collection:/media/collection
    environment:
      - GIN_MODE=release
      - DATABASE_PATH=/app/data/tasks.json
      - LOG_LEVEL=info
      - PORT=3001
    ports:
      - "3001:3001"
    restart: unless-stopped
    user: "1000:1000"

  frontend:
    image: nginx:alpine
    container_name: dts-converter-frontend
    volumes:
      - ./frontend:/usr/share/nginx/html
      - ./frontend/nginx.conf:/etc/nginx/nginx.conf
    ports:
      - "6969:80"
    depends_on:
      - backend
    restart: unless-stopped
```

## Решение проблем

### Недостаточно прав доступа
```bash
# Проверяем права на все медиатеки
ls -la /media/movies
ls -la /media/tvshows

# Исправляем права
chmod 755 /media/movies
chmod 755 /media/tvshows

# Проверяем владельца
chown -R 1000:1000 /media/movies
```

### Проблемы с монтированием томов
```bash
# Проверяем монтирование томов в контейнере
docker-compose exec backend ls -la /media/

# Проверяем доступность каждой директории
docker-compose exec backend ls -la /media/movies
docker-compose exec backend ls -la /media/tvshows
```

### FFmpeg не найден
```bash
# Проверяем установку FFmpeg в контейнере
docker-compose exec backend which ffmpeg

# Проверяем версию
docker-compose exec backend ffmpeg -version

# Пересобираем образ
docker-compose build --no-cache backend
```

### Проблемы с WebSocket
```bash
# Проверяем подключение WebSocket
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" http://localhost:3001/ws

# Проверяем nginx конфигурацию
docker-compose exec frontend cat /etc/nginx/nginx.conf

# Проверяем логи nginx
docker-compose logs frontend
```

### Проблемы с портом 6969
```bash
# Проверяем, не занят ли порт
sudo netstat -tlnp | grep 6969

# Или используйте lsof
sudo lsof -i :6969

# Если порт занят, измените в docker-compose.yml
# ports:
#   - "6970:80"  # Используйте другой порт
```

### Файлы не конвертируются
```bash
# Проверяем логи для определения ошибки
docker-compose logs -f backend | grep -i error

# Проверяем права на запись
docker-compose exec backend touch /media/movies/test.txt
docker-compose exec backend rm /media/movies/test.txt

# Проверяем доступность FFmpeg
docker-compose exec backend ffmpeg -version
```

### JSON хранилище повреждено
```bash
# Остановите сервис
docker-compose down

# Удалите JSON файл
rm ./data/tasks.json

# Запустите снова (файл создастся автоматически)
docker-compose up -d
```

### Контейнер не запускается
```bash
# Проверяем логи
docker-compose logs backend

# Проверяем образ
docker images | grep dts-converter

# Пересобираем с нуля
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

## Обновление системы

### Обновление кода
```bash
# Получить последние изменения
git pull

# Пересобрать и перезапустить
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Обновление зависимостей
```bash
# Backend (Go modules)
cd backend
go get -u ./...
go mod tidy

# Пересобрать образ
cd ..
docker-compose build --no-cache backend
docker-compose up -d
```

## Производительность

### Типичное использование ресурсов
- **Backend**: 5-15 MB RAM (Go процесс)
- **Frontend**: 2-5 MB RAM (Nginx)
- **FFmpeg**: 100-500 MB RAM (во время конвертации)
- **CPU**: 1-2 ядра (зависит от FFmpeg)
- **Диск**: ~2x размера исходного файла (временно)

### Рекомендации
- Используйте SSD для лучшей производительности
- Обеспечьте достаточно свободного места (минимум 2x размера файла)
- Для больших файлов рассмотрите увеличение лимитов памяти

Эта конфигурация обеспечивает стабильную работу сервиса с поддержкой нескольких медиатек и минимальным потреблением ресурсов.