# backend/Dockerfile
FROM golang:1.21-alpine AS builder

# Устанавливаем только FFmpeg
RUN apk add --no-cache ffmpeg

WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./

# Скачиваем зависимости
RUN go mod download

# Копируем весь исходный код включая статические файлы
COPY . .

# Собираем приложение (без CGO - чистый Go!)
RUN CGO_ENABLED=0 go build -o main .

# Финальный образ
FROM alpine:latest

# Устанавливаем runtime зависимости
RUN apk --no-cache add ca-certificates ffmpeg

# Создаем директории для данных
RUN mkdir -p /app/data /app/logs

WORKDIR /app

# Копируем собранное приложение
COPY --from=builder /app/main .

EXPOSE 3001

CMD ["./main"]