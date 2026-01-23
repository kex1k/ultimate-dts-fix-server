// frontend/app.js
class DTSConverterApp {
    constructor() {
        this.ws = null;
        this.isConnected = false;
        this.queue = [];
        this.history = [];
        this.activeTask = null;
        this.searchResults = [];
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
        const searchFilesBtn = document.getElementById('search-files-btn');
        const closeSearchBtn = document.getElementById('close-search-btn');

        searchFilesBtn.addEventListener('click', () => {
            this.searchFiles();
        });

        closeSearchBtn.addEventListener('click', () => {
            this.closeSearchResults();
        });

        filePathInput.addEventListener('keypress', (event) => {
            if (event.key === 'Enter') {
                this.searchFiles();
            }
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
                this.updateConversionProgress(data.data);
                break;
            case 'log':
                this.addLog(data.data.message, data.data.level);
                break;
            default:
                console.log('Неизвестный тип сообщения:', data);
        }
    }

    async searchFiles() {
        const filePathInput = document.getElementById('file-path-input');
        const pattern = filePathInput.value.trim();
        
        // Пустой паттерн разрешен - будет использован дефолтный DTS.*5\.1
        const searchPattern = pattern || 'DTS.*5\\.1';
        this.addLog(`Поиск файлов: ${searchPattern}`, 'info');
        
        try {
            const response = await fetch('/api/search-files', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ pattern: pattern })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Ошибка поиска файлов');
            }

            const data = await response.json();
            this.searchResults = data.files || [];
            this.renderSearchResults();
            this.addLog(`Найдено файлов: ${data.count}`, 'info');
        } catch (error) {
            this.addLog(`Ошибка поиска: ${error.message}`, 'error');
        }
    }

    renderSearchResults() {
        const container = document.getElementById('search-results-container');
        const tbody = document.getElementById('search-results-body');
        const countSpan = document.getElementById('search-count');
        
        if (this.searchResults.length === 0) {
            container.style.display = 'none';
            this.addLog('Файлы не найдены', 'warning');
            return;
        }

        container.style.display = 'block';
        countSpan.textContent = this.searchResults.length;
        
        tbody.innerHTML = this.searchResults.map((file, index) => {
            const size = this.formatFileSize(file.size);
            const date = new Date(file.modified * 1000).toLocaleString();
            
            return `
                <tr onclick="app.addFileFromSearch(${index})">
                    <td class="file-name-cell">${file.name}</td>
                    <td class="file-path-cell" title="${file.path}">${file.path}</td>
                    <td class="file-size-cell">${size}</td>
                    <td class="file-date-cell">${date}</td>
                </tr>
            `;
        }).join('');
    }

    async addFileFromSearch(index) {
        const file = this.searchResults[index];
        
        if (!file) {
            return;
        }

        this.addLog(`Добавление в очередь: ${file.name}`, 'info');
        
        try {
            const response = await fetch('/api/tasks', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ filePath: file.path })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Ошибка добавления файла');
            }

            this.addLog(`Файл добавлен в очередь: ${file.name}`, 'info');
        } catch (error) {
            this.addLog(`Ошибка: ${error.message}`, 'error');
        }
    }

    closeSearchResults() {
        const container = document.getElementById('search-results-container');
        container.style.display = 'none';
        this.searchResults = [];
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    }

    formatBitrate(bitrate) {
        const rate = parseInt(bitrate);
        if (isNaN(rate)) return bitrate;
        
        if (rate >= 1000000) {
            return (rate / 1000000).toFixed(1) + ' Mbps';
        } else if (rate >= 1000) {
            return (rate / 1000).toFixed(0) + ' kbps';
        }
        return rate + ' bps';
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
        
        // Формируем информацию об аудио
        let audioInfoHtml = '';
        if (this.activeTask.audioInfo) {
            const audio = this.activeTask.audioInfo;
            audioInfoHtml = `
                <div class="audio-info-section">
                    <h4>Исходное аудио:</h4>
                    <div class="audio-info-grid">
                        <div class="audio-info-item">
                            <span class="audio-label">Кодек:</span>
                            <span class="audio-value">${audio.codecName}</span>
                        </div>
                        <div class="audio-info-item">
                            <span class="audio-label">Каналы:</span>
                            <span class="audio-value">${audio.channelLayout} (${audio.channels})</span>
                        </div>
                        <div class="audio-info-item">
                            <span class="audio-label">Sample Rate:</span>
                            <span class="audio-value">${audio.sampleRate} Hz</span>
                        </div>
                        ${audio.bitRate ? `
                        <div class="audio-info-item">
                            <span class="audio-label">Bitrate:</span>
                            <span class="audio-value">${this.formatBitrate(audio.bitRate)}</span>
                        </div>
                        ` : ''}
                    </div>
                </div>
            `;
        }
        
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
                ${audioInfoHtml}
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

        queueList.innerHTML = this.queue.map(item => {
            let audioInfoHtml = '';
            if (item.audioInfo) {
                const audio = item.audioInfo;
                audioInfoHtml = `
                    <div class="queue-audio-info">
                        <span class="audio-badge">${audio.codecName}</span>
                        <span class="audio-badge">${audio.channelLayout} (${audio.channels}ch)</span>
                        <span class="audio-badge">${audio.sampleRate} Hz</span>
                    </div>
                `;
            }
            
            // Кнопка удаления доступна всегда
            const deleteButton = `<button class="btn btn-danger btn-small" onclick="app.deleteTask('${item.id}', ${item.status === 'processing'})">Удалить</button>`;
            
            return `
                <div class="queue-item">
                    <div class="queue-item-info">
                        <div class="queue-item-name">${this.getFileName(item.filePath)}</div>
                        <div class="queue-item-path">${item.filePath}</div>
                        ${audioInfoHtml}
                    </div>
                    <div class="queue-item-actions">
                        <div class="queue-item-status status-${item.status}">
                            ${this.getStatusText(item.status)}
                        </div>
                        ${deleteButton}
                    </div>
                </div>
            `;
        }).join('');
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
            
            let audioInfoHtml = '';
            if (item.audioInfo) {
                const audio = item.audioInfo;
                audioInfoHtml = `
                    <div class="history-audio-info">
                        <span class="audio-badge-small">${audio.codecName}</span>
                        <span class="audio-badge-small">${audio.channelLayout}</span>
                    </div>
                `;
            }
            
            return `
                <div class="history-item ${hasError ? 'history-item-error' : ''}">
                    <div class="history-item-info">
                        <div class="history-item-name">${this.getFileName(item.filePath)}</div>
                        <div class="history-item-path">${item.filePath}</div>
                        ${audioInfoHtml}
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
        // Обновляем FFmpeg output если есть сообщение
        if (data && data.message && data.message.trim()) {
            const ffmpegOutput = document.getElementById('ffmpeg-output');
            if (ffmpegOutput) {
                ffmpegOutput.textContent = data.message;
                ffmpegOutput.scrollTop = ffmpegOutput.scrollHeight;
            }
        }
        
        // Обновляем статус задачи только если изменился
        if (data && data.status && this.activeTask && this.activeTask.status !== data.status) {
            this.loadActiveTask();
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

    async deleteTask(taskId, isProcessing = false) {
        let confirmMessage = 'Вы уверены, что хотите удалить задачу из очереди?';
        
        if (isProcessing) {
            confirmMessage = 'Задача в процессе конвертации. Вы уверены, что хотите принудительно удалить её? Это может привести к зависанию процесса ffmpeg.';
        }
        
        if (!confirm(confirmMessage)) {
            return;
        }

        try {
            const response = await fetch('/api/tasks/delete', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    taskId,
                    force: isProcessing
                })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Ошибка удаления задачи');
            }

            this.addLog('Задача удалена из очереди', 'info');
        } catch (error) {
            this.addLog(`Ошибка удаления задачи: ${error.message}`, 'error');
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
        // Игнорируем пустые или undefined сообщения
        if (!message || message === 'undefined') {
            return;
        }
        
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