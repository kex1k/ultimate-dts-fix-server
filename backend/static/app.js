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

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.isConnected = true;
            this.updateWebSocketStatus('online');
            this.updateServerStatus('online');
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
            this.updateServerStatus('offline');
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

    sendCommand(type, data = {}) {
        if (!this.isConnected) {
            this.addLog('WebSocket не подключен', 'error');
            return;
        }

        const message = {
            type: type,
            data: data
        };

        this.ws.send(JSON.stringify(message));
    }

    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'initial_state':
                this.handleInitialState(data.data);
                break;
            case 'queue_update':
                this.loadState();
                break;
            case 'conversion_progress':
                this.updateConversionProgress(data.data);
                break;
            case 'log':
                this.addLog(data.data.message, data.data.level);
                break;
            case 'search_files_response':
                this.handleSearchResponse(data);
                break;
            case 'add_task_response':
                this.handleAddTaskResponse(data);
                break;
            case 'cancel_task_response':
                this.handleCancelTaskResponse(data);
                break;
            case 'delete_task_response':
                this.handleDeleteTaskResponse(data);
                break;
            default:
                console.log('Неизвестный тип сообщения:', data);
        }
    }

    handleInitialState(data) {
        this.updateQueue(data.queue || []);
        this.updateHistory(data.history || []);
        this.updateActiveTask(data.activeTask);
    }

    loadState() {
        this.sendCommand('get_state');
    }

    searchFiles() {
        const filePathInput = document.getElementById('file-path-input');
        const pattern = filePathInput.value.trim();
        
        this.addLog(`Поиск файлов: ${pattern || 'DTS.*5\\.1'}`, 'info');
        this.sendCommand('search_files', { pattern: pattern });
    }

    handleSearchResponse(response) {
        if (response.error) {
            this.addLog(`Ошибка поиска: ${response.error}`, 'error');
            return;
        }

        this.searchResults = response.data.files || [];
        this.renderSearchResults();
        this.addLog(`Найдено файлов: ${response.data.count}`, 'info');
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

    addFileFromSearch(index) {
        const file = this.searchResults[index];
        
        if (!file) {
            return;
        }

        this.addLog(`Добавление в очередь: ${file.name}`, 'info');
        this.sendCommand('add_task', { filePath: file.path });
    }

    handleAddTaskResponse(response) {
        if (response.error) {
            this.addLog(`Ошибка: ${response.error}`, 'error');
        } else {
            this.addLog(`Файл добавлен в очередь`, 'info');
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
        const progress = this.activeTask.progress || 0;
        const duration = this.activeTask.duration || 0;
        const currentTime = this.activeTask.currentTime || 0;
        
        let audioInfoHtml = '';
        if (this.activeTask.audioInfo) {
            const audio = this.activeTask.audioInfo;
            audioInfoHtml = `
                <div class="current-audio-info">
                    <span class="audio-badge-small">${audio.codecName}</span>
                    <span class="audio-badge-small">${audio.channelLayout} (${audio.channels}ch)</span>
                    <span class="audio-badge-small">${audio.sampleRate} Hz</span>
                </div>
            `;
        }

        let progressHtml = '';
        if (duration > 0) {
            progressHtml = `
                <div class="progress-section">
                    <div class="progress-info">
                        <span class="progress-percentage">${progress}%</span>
                        <span class="progress-time">${this.formatTime(currentTime)} / ${this.formatTime(duration)}</span>
                    </div>
                    <div class="progress-bar-container">
                        <div class="progress-bar" style="width: ${progress}%"></div>
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
                ${progressHtml}
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
                        <span class="audio-badge-small">${audio.codecName}</span>
                        <span class="audio-badge-small">${audio.channelLayout} (${audio.channels}ch)</span>
                        <span class="audio-badge-small">${audio.sampleRate} Hz</span>
                    </div>
                `;
            }
            
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
        if (!this.activeTask) {
            return;
        }

        // Обновляем прогресс
        if (data.progress !== undefined) {
            this.activeTask.progress = data.progress;
        }

        // Перерисовываем только если изменился статус или есть значимое изменение прогресса
        if (data.status && this.activeTask.status !== data.status) {
            this.loadState();
        } else if (data.progress !== undefined) {
            // Обновляем только прогресс-бар без полной перерисовки
            const progressBar = document.querySelector('.progress-bar');
            const progressPercentage = document.querySelector('.progress-percentage');
            const progressTime = document.querySelector('.progress-time');
            
            if (progressBar) {
                progressBar.style.width = data.progress + '%';
            }
            if (progressPercentage) {
                progressPercentage.textContent = data.progress + '%';
            }
            if (progressTime && this.activeTask.currentTime && this.activeTask.duration) {
                progressTime.textContent = `${this.formatTime(this.activeTask.currentTime)} / ${this.formatTime(this.activeTask.duration)}`;
            }
        }
    }

    formatTime(seconds) {
        if (!seconds || seconds <= 0) return '00:00:00';
        
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = Math.floor(seconds % 60);
        
        return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
    }

    cancelTask(taskId) {
        if (!confirm('Вы уверены, что хотите отменить конвертацию?')) {
            return;
        }

        this.sendCommand('cancel_task', { taskId: taskId });
    }

    handleCancelTaskResponse(response) {
        if (response.error) {
            this.addLog(`Ошибка отмены: ${response.error}`, 'error');
        } else {
            this.addLog('Задача отменена', 'warning');
        }
    }

    deleteTask(taskId, isProcessing = false) {
        let confirmMessage = 'Вы уверены, что хотите удалить задачу из очереди?';
        
        if (isProcessing) {
            confirmMessage = 'Задача в процессе конвертации. Вы уверены, что хотите принудительно удалить её?';
        }
        
        if (!confirm(confirmMessage)) {
            return;
        }

        this.sendCommand('delete_task', {
            taskId: taskId,
            force: isProcessing
        });
    }

    handleDeleteTaskResponse(response) {
        if (response.error) {
            this.addLog(`Ошибка удаления: ${response.error}`, 'error');
        } else {
            this.addLog('Задача удалена', 'info');
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