// frontend/app.js
class DTSConverterApp {
    constructor() {
        this.ws = null;
        this.isConnected = false;
        this.queue = [];
        this.history = [];
        this.activeTask = null;
        this.filePaths = [];
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.connectWebSocket();
        this.checkServerStatus();
        this.loadQueue();
        this.loadHistory();
        this.loadActiveTask();
    }

    setupEventListeners() {
        const filePathInput = document.getElementById('file-path-input');
        const addFileBtn = document.getElementById('add-file-btn');

        addFileBtn.addEventListener('click', () => {
            this.addFilePath();
        });

        filePathInput.addEventListener('keypress', (event) => {
            if (event.key === 'Enter') {
                this.addFilePath();
            }
        });

        document.getElementById('add-to-queue-btn').addEventListener('click', () => {
            this.addFilesToQueue();
        });
    }

    async checkServerStatus() {
        try {
            const response = await fetch('/api/status');
            if (response.ok) {
                this.updateServerStatus('online');
            } else {
                this.updateServerStatus('offline');
            }
        } catch (error) {
            this.updateServerStatus('offline');
            this.addLog('Ошибка подключения к серверу', 'error');
        }
    }

    async loadQueue() {
        try {
            const response = await fetch('/api/queue');
            if (response.ok) {
                const data = await response.json();
                this.updateQueue(data.queue || []);
            }
        } catch (error) {
            console.error('Ошибка загрузки очереди:', error);
        }
    }

    async loadHistory() {
        try {
            const response = await fetch('/api/history');
            if (response.ok) {
                const data = await response.json();
                this.updateHistory(data.history || []);
            }
        } catch (error) {
            console.error('Ошибка загрузки истории:', error);
        }
    }

    async loadActiveTask() {
        try {
            const response = await fetch('/api/active-task');
            if (response.ok) {
                const data = await response.json();
                this.updateActiveTask(data.activeTask);
            }
        } catch (error) {
            console.error('Ошибка загрузки активной задачи:', error);
        }
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.isConnected = true;
            this.updateWebSocketStatus('online');
            this.addLog('WebSocket подключен', 'info');
        };

        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.handleWebSocketMessage(data);
            } catch (error) {
                console.error('Ошибка парсинга WebSocket сообщения:', error);
            }
        };

        this.ws.onclose = () => {
            this.isConnected = false;
            this.updateWebSocketStatus('offline');
            this.addLog('WebSocket отключен', 'warning');
            
            // Попытка переподключения через 5 секунд
            setTimeout(() => {
                this.connectWebSocket();
            }, 5000);
        };

        this.ws.onerror = (error) => {
            this.addLog('Ошибка WebSocket', 'error');
            console.error('WebSocket error:', error);
        };
    }

    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'queue_update':
                this.loadQueue();
                this.loadHistory();
                this.loadActiveTask();
                break;
            case 'conversion_progress':
                this.updateConversionProgress(data);
                break;
            case 'log':
                this.addLog(data.message, data.level);
                break;
            default:
                console.log('Неизвестный тип сообщения:', data);
        }
    }

    addFilePath() {
        const filePathInput = document.getElementById('file-path-input');
        const filePath = filePathInput.value.trim();
        
        if (!filePath) {
            this.addLog('Введите путь к файлу', 'warning');
            return;
        }

        // Проверяем, что путь начинается с /media/
        if (!filePath.startsWith('/media/')) {
            this.addLog('Путь должен начинаться с /media/', 'warning');
            return;
        }

        // Проверяем, что файл не добавлен уже
        if (this.filePaths.includes(filePath)) {
            this.addLog('Файл уже добавлен в список', 'warning');
            return;
        }

        this.filePaths.push(filePath);
        this.renderFileList();
        filePathInput.value = '';
        this.addLog(`Добавлен файл: ${filePath}`, 'info');
    }

    removeFilePath(index) {
        const removedPath = this.filePaths[index];
        this.filePaths.splice(index, 1);
        this.renderFileList();
        this.addLog(`Удален файл: ${removedPath}`, 'info');
    }

    renderFileList() {
        const fileList = document.getElementById('file-list');
        
        if (this.filePaths.length === 0) {
            fileList.innerHTML = '<div class="empty-queue">Файлы не добавлены</div>';
            return;
        }

        fileList.innerHTML = this.filePaths.map((filePath, index) => `
            <div class="file-item">
                <div class="file-item-name">${filePath}</div>
                <button class="file-item-remove" onclick="app.removeFilePath(${index})">×</button>
            </div>
        `).join('');
    }

    async addFilesToQueue() {
        if (this.filePaths.length === 0) {
            this.addLog('Нет файлов для добавления в очередь', 'warning');
            return;
        }

        try {
            for (const filePath of this.filePaths) {
                const response = await fetch('/api/tasks', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ filePath })
                });

                if (!response.ok) {
                    throw new Error(`Ошибка добавления файла: ${filePath}`);
                }

                const result = await response.json();
                this.addLog(`Файл добавлен в очередь: ${filePath}`, 'info');
            }

            this.filePaths = [];
            this.renderFileList();
            this.addLog('Все файлы добавлены в очередь', 'info');
        } catch (error) {
            this.addLog(`Ошибка добавления файлов: ${error.message}`, 'error');
        }
    }

    updateActiveTask(task) {
        this.activeTask = task;
        this.renderActiveTask();
    }

    renderActiveTask() {
        const currentFile = document.getElementById('current-file');
        
        if (!this.activeTask) {
            currentFile.innerHTML = '<div class="empty-current">Нет активной конвертации</div>';
            return;
        }

        const startTime = this.activeTask.startedAt ? new Date(this.activeTask.startedAt).toLocaleString() : 'N/A';
        
        currentFile.innerHTML = `
            <div class="current-file-card">
                <div class="current-file-header">
                    <div class="current-file-name">${this.getFileName(this.activeTask.filePath)}</div>
                    <button class="btn btn-danger btn-small" onclick="app.cancelTask('${this.activeTask.id}')">Отменить</button>
                </div>
                <div class="current-file-path">${this.activeTask.filePath}</div>
                <div class="current-file-info">
                    <div class="info-item">
                        <span class="info-label">Статус:</span>
                        <span class="status-badge status-${this.activeTask.status}">${this.getStatusText(this.activeTask.status)}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Начало:</span>
                        <span>${startTime}</span>
                    </div>
                </div>
                <div class="current-file-progress">
                    <pre id="ffmpeg-output" class="ffmpeg-output"></pre>
                </div>
            </div>
        `;
    }

    updateQueue(queue) {
        this.queue = queue || [];
        this.renderQueue();
    }

    renderQueue() {
        const queueList = document.getElementById('queue-list');
        
        if (this.queue.length === 0) {
            queueList.innerHTML = '<div class="empty-queue">Очередь пуста</div>';
            return;
        }

        queueList.innerHTML = this.queue.map(item => `
            <div class="queue-item">
                <div class="queue-item-info">
                    <div class="queue-item-name">${this.getFileName(item.filePath)}</div>
                    <div class="queue-item-path">${item.filePath}</div>
                </div>
                <div class="queue-item-status status-${item.status}">
                    ${this.getStatusText(item.status)}
                </div>
            </div>
        `).join('');
    }

    updateHistory(history) {
        this.history = history || [];
        this.renderHistory();
    }

    renderHistory() {
        const historyList = document.getElementById('history-list');
        
        if (this.history.length === 0) {
            historyList.innerHTML = '<div class="empty-history">История пуста</div>';
            return;
        }

        historyList.innerHTML = this.history.map(item => {
            const completedTime = item.completedAt ? new Date(item.completedAt).toLocaleString() : 'N/A';
            const hasError = item.status === 'error';
            
            return `
                <div class="history-item ${hasError ? 'history-item-error' : ''}">
                    <div class="history-item-info">
                        <div class="history-item-name">${this.getFileName(item.filePath)}</div>
                        <div class="history-item-path">${item.filePath}</div>
                        ${hasError ? `<div class="history-item-error-msg">${item.error || 'Неизвестная ошибка'}</div>` : ''}
                    </div>
                    <div class="history-item-meta">
                        <div class="history-item-time">${completedTime}</div>
                        <div class="history-item-status status-${item.status}">
                            ${this.getStatusText(item.status)}
                        </div>
                    </div>
                </div>
            `;
        }).join('');
    }

    updateConversionProgress(data) {
        // Обновляем активную задачу
        this.loadActiveTask();
        
        // Обновляем FFmpeg output если есть
        if (data.message) {
            const ffmpegOutput = document.getElementById('ffmpeg-output');
            if (ffmpegOutput) {
                ffmpegOutput.textContent = data.message;
                ffmpegOutput.scrollTop = ffmpegOutput.scrollHeight;
            }
        }
    }

    async cancelTask(taskId) {
        if (!confirm('Вы уверены, что хотите отменить конвертацию?')) {
            return;
        }

        try {
            const response = await fetch('/api/tasks/cancel', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ taskId })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Ошибка отмены задачи');
            }

            this.addLog('Задача отменена', 'warning');
        } catch (error) {
            this.addLog(`Ошибка отмены задачи: ${error.message}`, 'error');
        }
    }

    getFileName(filePath) {
        return filePath.split('/').pop() || filePath;
    }

    getStatusText(status) {
        const statusMap = {
            'pending': 'Ожидание',
            'processing': 'В процессе',
            'completed': 'Завершено',
            'error': 'Ошибка'
        };
        return statusMap[status] || status;
    }

    updateServerStatus(status) {
        const element = document.getElementById('server-status');
        element.textContent = status === 'online' ? 'Онлайн' : 'Отключен';
        element.className = status === 'online' ? 'status-online' : 'status-offline';
    }

    updateWebSocketStatus(status) {
        const element = document.getElementById('ws-status');
        element.textContent = status === 'online' ? 'Подключен' : 'Отключен';
        element.className = status === 'online' ? 'status-online' : 'status-offline';
    }

    addLog(message, level = 'info') {
        const logsContainer = document.getElementById('logs');
        const timestamp = new Date().toLocaleTimeString();
        const logEntry = document.createElement('div');
        logEntry.className = `log-entry log-${level}`;
        logEntry.innerHTML = `<span class="log-time">[${timestamp}]</span> ${message}`;
        
        logsContainer.appendChild(logEntry);
        logsContainer.scrollTop = logsContainer.scrollHeight;
    }
}

// Глобальная переменная для доступа к приложению
let app;

// Инициализация приложения при загрузке страницы
document.addEventListener('DOMContentLoaded', () => {
    app = new DTSConverterApp();
});