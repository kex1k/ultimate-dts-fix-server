# DTS to FLAC Converter

Конвертер аудиодорожек DTS-HD MA 5.1 в FLAC 7.1 с веб-интерфейсом, написанный на Go.

## Особенности

- **Точная конвертация**: Преобразование DTS-HD MA 5.1 в FLAC 7.1 с сохранением качества
- **Высокая производительность**: Нативный Go код с низким потреблением памяти (5-15MB)
- **Real-time мониторинг**: WebSocket для отслеживания прогресса FFmpeg в реальном времени
- **Управление очередью**: Отдельные секции для текущей конвертации, очереди и истории
- **Отмена конвертации**: Возможность отменить текущую задачу
- **Автоматический бэкап**: Исходные файлы переименовываются в .bak после успешной конвертации
- **Docker контейнеризация**: Полная изоляция и простота развертывания
- **Сохранение структуры**: Конвертированные файлы сохраняются в те же директории

## Быстрый старт

### 1. Клонирование и настройка

```bash
git clone https://github.com/kex1k/ultimate-dts-fix-server.git
cd ultimate-dts-fix-server
```

### 2. Подготовка директорий

```bash
# Создаем директории для сервиса
mkdir -p data logs

# Создаем директории для медиафайлов (пример)
mkdir -p /path/to/media/library1
mkdir -p /path/to/media/library2
mkdir -p /path/to/media/library3
```

### 3. Настройка docker-compose.yml

Отредактируйте файл `docker-compose.yml`, заменив пути к вашим медиатекам:

```yaml
volumes:
  # Замените на ваши реальные пути
  - /your/real/path/library1:/media/library1
  - /your/real/path/library2:/media/library2
  - /your/real/path/library3:/media/library3
```

### 4. Запуск сервиса

```bash
# Запуск всех сервисов
docker-compose up -d

# Проверка статуса
docker-compose ps

# Просмотр логов
docker-compose logs -f backend
```

### 5. Проверка работоспособности

```bash
# Проверка API
curl http://localhost:3001/api/status

# Открытие веб-интерфейса
# http://localhost:6969
```

## Конфигурация

### Переменные окружения

Создайте файл `.env` на основе `.env.example`:

```bash
GIN_MODE=release
DATABASE_PATH=/app/data/database.sqlite
LOG_LEVEL=info
PORT=3001
```

### Настройка медиатек

Система поддерживает любое количество медиатек. Добавьте их в `docker-compose.yml`:

```yaml
volumes:
  - /path/to/your/media1:/media/media1
  - /path/to/your/media2:/media/media2
  - /path/to/your/media3:/media/media3
```

## Использование

### Веб-интерфейс

1. **Откройте веб-интерфейс**: http://localhost:6969

2. **Добавление файлов**:
   - Введите полный путь к файлу (например: `/media/library1/movie.mkv`)
   - Нажмите "Добавить" для добавления в список
   - Нажмите "Добавить все в очередь" для начала конвертации

3. **Мониторинг**:
   - **Текущая конвертация**: Показывает активную задачу с выводом FFmpeg
   - **Очередь конвертации**: Список ожидающих задач
   - **История конвертаций**: Завершенные задачи с временем и статусом

4. **Управление**:
   - Кнопка "Отменить" для остановки текущей конвертации
   - Логи в реальном времени внизу страницы

### Конвертация файлов

Система автоматически:
- Конвертирует DTS-HD MA 5.1 в FLAC 7.1
- Переименовывает файл: `movie.DTS-HD.5.1.mkv` → `movie.FLAC.7.1.mkv`
- Переименовывает исходный файл в `.bak` после успешной конвертации
- Сохраняет видео и субтитры без изменений

### API Endpoints

```bash
# Статус сервера
GET /api/status

# Получить очередь (pending/processing)
GET /api/queue

# Получить активную задачу
GET /api/active-task

# Получить историю (completed/error)
GET /api/history

# Добавить задачу
POST /api/tasks
Body: { "filePath": "/media/library1/movie.mkv" }

# Отменить задачу
POST /api/tasks/cancel
Body: { "taskId": "20240122123456" }

# WebSocket для real-time обновлений
WS /ws
```

## Технические детали

### Команда FFmpeg

```bash
ffmpeg -i input.mkv \
  -c:v copy \
  -c:s copy \
  -c:a:0 flac \
  -b:a 384k \
  -map 0 \
  -compression_level 8 \
  -channel_layout 7.1 \
  -ac 8 \
  -af "pan=7.1|FL=FL|FR=FR|FC=FC|LFE=LFE|BL=SL|BR=SR|SL=SL|SR=SR" \
  output.mkv
```

### Особенности реализации

- **Throttling**: WebSocket обновления отправляются раз в 10 секунд
- **Cancellation**: Использует Go context для корректной отмены FFmpeg процессов
- **Thread Safety**: Mutex защита для активной задачи
- **Security**: WebSocket origin validation для предотвращения CSRF
- **Database**: SQLite для хранения истории задач

## Решение проблем

### Порт 6969 занят

Измените порт в `docker-compose.yml`:

```yaml
ports:
  - "6970:80"  # Используйте другой порт
```

### Проблемы с правами доступа

```bash
# Даем права на запись медиатекам
chmod 755 /path/to/media/library1

# Или измените user в docker-compose.yml
user: "1000:1000"  # Замените на ваши UID:GID
```

### Проверка монтирования томов

```bash
# Проверяем внутри контейнера
docker-compose exec backend ls -la /media/
```

### FFmpeg не найден

```bash
# Пересоберите образ
docker-compose build --no-cache backend
```

### WebSocket не подключается

Проверьте nginx конфигурацию и убедитесь, что:
- Backend доступен на порту 3001
- Nginx правильно проксирует WebSocket соединения

## Мониторинг

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

## Остановка сервиса

```bash
# Остановка
docker-compose down

# Остановка с удалением томов
docker-compose down -v

# Остановка и удаление образов
docker-compose down --rmi all
```

## Производительность

- **Память**: 5-15 MB (Go процесс)
- **CPU**: Зависит от FFmpeg (обычно 1-2 ядра)
- **Диск**: Требуется свободное место ~2x размера исходного файла
- **Скорость**: Зависит от размера файла и CPU (обычно 0.5-2x realtime)

## Безопасность

- WebSocket origin validation
- Нет автоматического сканирования файловой системы
- Ручное добавление файлов через веб-интерфейс
- Изолированные Docker контейнеры
- Минимальные права доступа

## Обновление

```bash
# Получить последние изменения
git pull

# Пересобрать и перезапустить
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

## Известные ограничения

- Обрабатывается только одна задача одновременно
- Поддерживаются только видеофайлы (.mkv, .mp4, .avi, .mov, .wmv, .flv, .webm, .m4v)
- Требуется FFmpeg с поддержкой FLAC
- Замена только точного совпадения "DTS-HD.5.1" → "FLAC.7.1"

## Поддержка

Для вопросов и проблем создавайте issue в репозитории.

## Лицензия

MIT License