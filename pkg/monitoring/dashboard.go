// Package monitoring - Dashboard Generation System
// Migrated from ClubPulse to gopherkit with improvements
package monitoring

import (
	"fmt"
)

// generateDashboardHTML genera el HTML completo del dashboard de monitoreo
func (rtm *RealTimeMonitor) generateDashboardHTML() string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GopherKit - Monitor en Tiempo Real</title>
    <style>
        %s
    </style>
</head>
<body>
    <div class="container">
        <!-- Header -->
        <header class="header">
            <div class="header-content">
                <h1>🐹 GopherKit - Monitor en Tiempo Real</h1>
                <div class="status-indicators">
                    <div class="status-item">
                        <span class="status-dot status-online" id="connection-status"></span>
                        <span>WebSocket</span>
                    </div>
                    <div class="status-item">
                        <span class="status-dot status-online" id="system-status"></span>
                        <span>Sistema</span>
                    </div>
                    <div class="timestamp" id="last-update">
                        Última actualización: --:--:--
                    </div>
                </div>
            </div>
        </header>

        <!-- Alertas -->
        <div class="alerts-container" id="alerts-container">
            <!-- Alertas dinámicas se insertan aquí -->
        </div>

        <!-- Grid Principal -->
        <div class="dashboard-grid">
            <!-- Servicios Overview -->
            <div class="card services-overview">
                <h2>🔧 Estado de Servicios</h2>
                <div class="services-grid" id="services-grid">
                    <!-- Servicios dinámicos se insertan aquí -->
                </div>
            </div>

            <!-- Métricas del Sistema -->
            <div class="card system-metrics">
                <h2>📊 Métricas del Sistema</h2>
                <div class="metrics-grid">
                    <div class="metric-item">
                        <div class="metric-label">CPU Promedio</div>
                        <div class="metric-value" id="cpu-usage">--%%</div>
                        <div class="metric-chart" id="cpu-chart"></div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Memoria</div>
                        <div class="metric-value" id="memory-usage">-- MB</div>
                        <div class="metric-chart" id="memory-chart"></div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Conexiones DB</div>
                        <div class="metric-value" id="db-connections">--</div>
                        <div class="metric-chart" id="db-chart"></div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Cache Hit Rate</div>
                        <div class="metric-value" id="cache-hit-rate">--%%</div>
                        <div class="metric-chart" id="cache-chart"></div>
                    </div>
                </div>
            </div>

            <!-- Performance -->
            <div class="card performance-metrics">
                <h2>⚡ Performance</h2>
                <div class="performance-grid">
                    <div class="perf-item">
                        <span class="perf-label">Latencia P95</span>
                        <span class="perf-value" id="latency-p95">-- ms</span>
                    </div>
                    <div class="perf-item">
                        <span class="perf-label">Throughput</span>
                        <span class="perf-value" id="throughput">-- req/s</span>
                    </div>
                    <div class="perf-item">
                        <span class="perf-label">Error Rate</span>
                        <span class="perf-value" id="error-rate">--%%</span>
                    </div>
                    <div class="perf-item">
                        <span class="perf-label">Uptime</span>
                        <span class="perf-value" id="uptime">--</span>
                    </div>
                </div>
            </div>

            <!-- Métricas de Negocio -->
            <div class="card business-metrics">
                <h2>💼 Métricas de Negocio</h2>
                <div class="business-grid">
                    <div class="business-item">
                        <div class="business-icon">🎾</div>
                        <div class="business-content">
                            <div class="business-label">Reservas Hoy</div>
                            <div class="business-value" id="bookings-today">--</div>
                        </div>
                    </div>
                    <div class="business-item">
                        <div class="business-icon">💳</div>
                        <div class="business-content">
                            <div class="business-label">Pagos Procesados</div>
                            <div class="business-value" id="payments-today">--</div>
                        </div>
                    </div>
                    <div class="business-item">
                        <div class="business-icon">👥</div>
                        <div class="business-content">
                            <div class="business-label">Usuarios Activos</div>
                            <div class="business-value" id="active-users">--</div>
                        </div>
                    </div>
                    <div class="business-item">
                        <div class="business-icon">🏆</div>
                        <div class="business-content">
                            <div class="business-label">Torneos Activos</div>
                            <div class="business-value" id="active-tournaments">--</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Logs en Tiempo Real -->
            <div class="card logs-container">
                <h2>📋 Logs en Tiempo Real</h2>
                <div class="logs-header">
                    <select id="log-level-filter">
                        <option value="all">Todos los niveles</option>
                        <option value="error">Errores</option>
                        <option value="warning">Advertencias</option>
                        <option value="info">Información</option>
                    </select>
                    <button id="clear-logs">Limpiar</button>
                </div>
                <div class="logs-content" id="logs-content">
                    <!-- Logs dinámicos se insertan aquí -->
                </div>
            </div>

            <!-- Database Status -->
            <div class="card database-status">
                <h2>🗃️ Estado de Base de Datos</h2>
                <div class="db-mode-indicator">
                    <span class="db-mode-label">Modo Actual:</span>
                    <span class="db-mode-value" id="db-mode">--</span>
                    <span class="fallback-status" id="fallback-status">--</span>
                </div>
                <div class="database-grid" id="database-grid">
                    <!-- Databases dinámicas se insertan aquí -->
                </div>
            </div>
        </div>

        <!-- Footer -->
        <footer class="footer">
            <div class="footer-content">
                <span>GopherKit Monitoring Dashboard v1.0</span>
                <span>Actualización automática cada 5 segundos</span>
                <span id="stats-summary">-- servicios monitoreados</span>
            </div>
        </footer>
    </div>

    <script>
        %s
    </script>
</body>
</html>`, generateDashboardCSS(), generateDashboardJS())
}

// generateDashboardCSS genera los estilos CSS del dashboard
func generateDashboardCSS() string {
	return `
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            line-height: 1.6;
            min-height: 100vh;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 20px;
        }

        /* Header */
        .header {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
        }

        .header-content {
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
        }

        .header h1 {
            color: #2c3e50;
            margin: 0;
            font-size: 2rem;
        }

        .status-indicators {
            display: flex;
            align-items: center;
            gap: 20px;
            flex-wrap: wrap;
        }

        .status-item {
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .status-dot {
            width: 12px;
            height: 12px;
            border-radius: 50%;
            animation: pulse 2s infinite;
        }

        .status-online {
            background: #27ae60;
        }

        .status-warning {
            background: #f39c12;
        }

        .status-error {
            background: #e74c3c;
        }

        .timestamp {
            color: #7f8c8d;
            font-size: 0.9rem;
        }

        /* Alertas */
        .alerts-container {
            margin-bottom: 20px;
        }

        .alert {
            background: #fff;
            border-left: 4px solid #e74c3c;
            border-radius: 8px;
            padding: 15px;
            margin-bottom: 10px;
            box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
            animation: slideIn 0.3s ease-out;
        }

        .alert.warning {
            border-left-color: #f39c12;
        }

        .alert.info {
            border-left-color: #3498db;
        }

        .alert.success {
            border-left-color: #27ae60;
        }

        /* Grid Principal */
        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
        }

        .card {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
            transition: transform 0.3s ease;
        }

        .card:hover {
            transform: translateY(-5px);
        }

        .card h2 {
            color: #2c3e50;
            margin-bottom: 15px;
            font-size: 1.3rem;
            border-bottom: 2px solid #ecf0f1;
            padding-bottom: 10px;
        }

        /* Servicios Grid */
        .services-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
            gap: 15px;
        }

        .service-item {
            text-align: center;
            padding: 15px;
            border-radius: 10px;
            transition: all 0.3s ease;
            cursor: pointer;
        }

        .service-healthy {
            background: linear-gradient(135deg, #a8e6cf, #88d8a3);
        }

        .service-warning {
            background: linear-gradient(135deg, #ffd93d, #f39c12);
        }

        .service-error {
            background: linear-gradient(135deg, #ff6b6b, #e74c3c);
        }

        .service-name {
            font-weight: bold;
            margin-bottom: 5px;
        }

        .service-status {
            font-size: 0.9rem;
            opacity: 0.8;
        }

        /* Métricas Grid */
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
        }

        .metric-item {
            text-align: center;
            padding: 15px;
            border-radius: 10px;
            background: linear-gradient(135deg, #f8f9fa, #e9ecef);
        }

        .metric-label {
            font-size: 0.9rem;
            color: #6c757d;
            margin-bottom: 5px;
        }

        .metric-value {
            font-size: 1.8rem;
            font-weight: bold;
            color: #2c3e50;
            margin-bottom: 10px;
        }

        .metric-chart {
            height: 40px;
            background: #ecf0f1;
            border-radius: 5px;
            position: relative;
            overflow: hidden;
        }

        /* Performance Grid */
        .performance-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
        }

        .perf-item {
            display: flex;
            flex-direction: column;
            text-align: center;
            padding: 15px;
            border-radius: 10px;
            background: linear-gradient(135deg, #e3f2fd, #bbdefb);
        }

        .perf-label {
            font-size: 0.9rem;
            color: #1976d2;
            margin-bottom: 5px;
        }

        .perf-value {
            font-size: 1.5rem;
            font-weight: bold;
            color: #0d47a1;
        }

        /* Business Metrics */
        .business-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
        }

        .business-item {
            display: flex;
            align-items: center;
            padding: 15px;
            border-radius: 10px;
            background: linear-gradient(135deg, #fff3e0, #ffe0b2);
            transition: transform 0.3s ease;
        }

        .business-item:hover {
            transform: scale(1.05);
        }

        .business-icon {
            font-size: 2rem;
            margin-right: 15px;
        }

        .business-label {
            font-size: 0.9rem;
            color: #e65100;
            margin-bottom: 5px;
        }

        .business-value {
            font-size: 1.5rem;
            font-weight: bold;
            color: #bf360c;
        }

        /* Logs */
        .logs-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
            gap: 10px;
        }

        .logs-header select, .logs-header button {
            padding: 8px 12px;
            border: 1px solid #ddd;
            border-radius: 5px;
            background: white;
            cursor: pointer;
        }

        .logs-content {
            max-height: 300px;
            overflow-y: auto;
            background: #2c3e50;
            color: #ecf0f1;
            padding: 15px;
            border-radius: 10px;
            font-family: 'Courier New', monospace;
            font-size: 0.85rem;
            line-height: 1.4;
        }

        .log-entry {
            margin-bottom: 5px;
            padding: 3px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .log-timestamp {
            color: #95a5a6;
        }

        .log-level {
            font-weight: bold;
            margin: 0 10px;
        }

        .log-error { color: #e74c3c; }
        .log-warning { color: #f39c12; }
        .log-info { color: #3498db; }
        .log-success { color: #27ae60; }

        /* Database Status */
        .db-mode-indicator {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 15px;
            padding: 10px;
            background: linear-gradient(135deg, #e8f5e8, #c8e6c9);
            border-radius: 8px;
        }

        .db-mode-label {
            font-weight: bold;
            color: #2e7d32;
        }

        .db-mode-value {
            background: #4caf50;
            color: white;
            padding: 4px 8px;
            border-radius: 4px;
            font-weight: bold;
        }

        .fallback-status {
            background: #ff9800;
            color: white;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.8rem;
        }

        .database-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
            gap: 10px;
        }

        .database-item {
            text-align: center;
            padding: 10px;
            border-radius: 8px;
            transition: all 0.3s ease;
        }

        .database-healthy {
            background: linear-gradient(135deg, #c8e6c9, #a5d6a7);
        }

        .database-error {
            background: linear-gradient(135deg, #ffcdd2, #ef5350);
        }

        /* Footer */
        .footer {
            margin-top: 30px;
            text-align: center;
            color: rgba(255, 255, 255, 0.8);
        }

        .footer-content {
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 10px;
        }

        /* Animaciones */
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.5; }
            100% { opacity: 1; }
        }

        @keyframes slideIn {
            from {
                transform: translateY(-20px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }

        /* Responsive */
        @media (max-width: 768px) {
            .container {
                padding: 10px;
            }

            .header-content {
                flex-direction: column;
                gap: 15px;
            }

            .dashboard-grid {
                grid-template-columns: 1fr;
            }

            .footer-content {
                flex-direction: column;
            }
        }
    `
}

// generateDashboardJS genera el JavaScript del dashboard
func generateDashboardJS() string {
	return `
        class GopherKitMonitor {
            constructor() {
                this.websocket = null;
                this.reconnectAttempts = 0;
                this.maxReconnectAttempts = 5;
                this.reconnectDelay = 5000;
                this.isConnected = false;
                this.logs = [];
                this.maxLogs = 100;
                
                this.init();
            }

            init() {
                this.connectWebSocket();
                this.setupEventListeners();
                this.startUpdateTimer();
                
                // Cargar datos iniciales
                this.loadInitialData();
            }

            connectWebSocket() {
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                const wsUrl = protocol + '//' + window.location.host + '/ws';
                
                try {
                    this.websocket = new WebSocket(wsUrl);
                    
                    this.websocket.onopen = () => {
                        console.log('✅ WebSocket conectado');
                        this.isConnected = true;
                        this.reconnectAttempts = 0;
                        this.updateConnectionStatus(true);
                        this.addLog('info', 'WebSocket conectado exitosamente');
                    };

                    this.websocket.onmessage = (event) => {
                        try {
                            const data = JSON.parse(event.data);
                            this.handleWebSocketMessage(data);
                        } catch (error) {
                            console.error('Error parsing WebSocket message:', error);
                        }
                    };

                    this.websocket.onclose = () => {
                        console.log('❌ WebSocket desconectado');
                        this.isConnected = false;
                        this.updateConnectionStatus(false);
                        this.addLog('warning', 'WebSocket desconectado');
                        this.scheduleReconnect();
                    };

                    this.websocket.onerror = (error) => {
                        console.error('Error en WebSocket:', error);
                        this.addLog('error', 'Error en conexión WebSocket');
                    };

                } catch (error) {
                    console.error('Error creando WebSocket:', error);
                    this.scheduleReconnect();
                }
            }

            scheduleReconnect() {
                if (this.reconnectAttempts < this.maxReconnectAttempts) {
                    this.reconnectAttempts++;
                    setTimeout(() => {
                        console.log('🔄 Intentando reconectar WebSocket (intento ' + this.reconnectAttempts + ')');
                        this.connectWebSocket();
                    }, this.reconnectDelay);
                } else {
                    this.addLog('error', 'Máximo número de reintentos de reconexión alcanzado');
                }
            }

            handleWebSocketMessage(data) {
                // Manejar datos del sistema de monitoreo en tiempo real
                if (data.timestamp) {
                    this.updateMetrics(data);
                } else {
                    // Manejar otros tipos de mensajes
                    switch (data.type) {
                        case 'metrics_update':
                            this.updateMetrics(data.payload);
                            break;
                        case 'service_status':
                            this.updateServiceStatus(data.payload);
                            break;
                        case 'alert':
                            this.showAlert(data.payload);
                            break;
                        case 'log':
                            this.addLogFromServer(data.payload);
                            break;
                        case 'database_status':
                            this.updateDatabaseStatus(data.payload);
                            break;
                        default:
                            console.log('Mensaje WebSocket desconocido:', data);
                    }
                }
            }

            async loadInitialData() {
                try {
                    // Cargar métricas iniciales
                    const metricsResponse = await fetch('/api/metrics');
                    if (metricsResponse.ok) {
                        const metrics = await metricsResponse.json();
                        this.updateMetrics(metrics);
                    }

                    // Cargar estado de servicios
                    const servicesResponse = await fetch('/api/services');
                    if (servicesResponse.ok) {
                        const services = await servicesResponse.json();
                        this.updateServiceStatus(services);
                    }

                    // Cargar estado de base de datos
                    const dbResponse = await fetch('/api/database');
                    if (dbResponse.ok) {
                        const dbStatus = await dbResponse.json();
                        this.updateDatabaseStatus(dbStatus);
                    }

                } catch (error) {
                    console.error('Error cargando datos iniciales:', error);
                    this.addLog('error', 'Error cargando datos iniciales');
                }
            }

            updateMetrics(metrics) {
                // Actualizar métricas del sistema basándose en la estructura de SystemMetrics
                if (metrics.infrastructure_metrics) {
                    const infraMetrics = metrics.infrastructure_metrics;
                    this.updateElement('cpu-usage', '-- %'); // Placeholder
                    this.updateElement('memory-usage', '-- MB'); // Placeholder
                }
                
                if (metrics.database_metrics) {
                    const dbMetrics = metrics.database_metrics;
                    this.updateElement('db-connections', dbMetrics.active_connections || 0);
                    this.updateElement('cache-hit-rate', (dbMetrics.cache_hit_rate || 0).toFixed(1) + '%');
                }

                // Actualizar métricas de performance
                this.updateElement('latency-p95', '-- ms'); // Placeholder
                this.updateElement('throughput', '-- req/s'); // Placeholder
                this.updateElement('error-rate', '--%'); // Placeholder
                this.updateElement('uptime', '--'); // Placeholder

                // Actualizar métricas de negocio
                if (metrics.business_metrics) {
                    const businessMetrics = metrics.business_metrics;
                    this.updateElement('bookings-today', businessMetrics.reservations_today || 0);
                    this.updateElement('payments-today', businessMetrics.payments_today || 0);
                    this.updateElement('active-users', businessMetrics.active_users || 0);
                    this.updateElement('active-tournaments', businessMetrics.championships_active || 0);
                }

                // Actualizar servicios
                if (metrics.service_metrics) {
                    this.updateServiceStatus(metrics.service_metrics);
                }

                // Actualizar base de datos
                if (metrics.database_metrics) {
                    this.updateDatabaseStatus(metrics.database_metrics);
                }

                // Actualizar timestamp
                this.updateElement('last-update', 'Última actualización: ' + new Date().toLocaleTimeString());
            }

            updateServiceStatus(services) {
                const servicesGrid = document.getElementById('services-grid');
                if (!servicesGrid) return;

                servicesGrid.innerHTML = '';

                const serviceList = Object.keys(services);
                let healthyCount = 0;

                serviceList.forEach(serviceName => {
                    const service = services[serviceName];
                    const serviceElement = this.createServiceElement(serviceName, service);
                    servicesGrid.appendChild(serviceElement);
                    
                    if (service.status === 'healthy') {
                        healthyCount++;
                    }
                });

                // Actualizar estado general del sistema
                const systemHealth = healthyCount === serviceList.length;
                this.updateSystemStatus(systemHealth);
                this.updateElement('stats-summary', healthyCount + '/' + serviceList.length + ' servicios saludables');
            }

            createServiceElement(name, service) {
                const div = document.createElement('div');
                div.className = 'service-item service-' + service.status;
                
                const displayName = name.replace('-api', '').replace(/\b\w/g, l => l.toUpperCase());
                
                div.innerHTML = 
                    '<div class="service-name">' + displayName + '</div>' +
                    '<div class="service-status">' + service.status + '</div>' +
                    '<div class="service-response-time">' + (service.response_time || 0) + 'ms</div>';
                
                return div;
            }

            updateDatabaseStatus(dbStatus) {
                this.updateElement('db-mode', dbStatus.mode || 'unknown');
                this.updateElement('fallback-status', dbStatus.fallback_status === 'ready' ? 'Listo' : 'No disponible');

                // En un entorno real, esto mostraría múltiples databases
                const databaseGrid = document.getElementById('database-grid');
                if (!databaseGrid) return;

                databaseGrid.innerHTML = '';
                
                // Simular estado de base de datos principal
                const dbElement = this.createDatabaseElement('Principal', { healthy: true });
                databaseGrid.appendChild(dbElement);
            }

            createDatabaseElement(name, dbInfo) {
                const div = document.createElement('div');
                div.className = 'database-item database-' + (dbInfo.healthy ? 'healthy' : 'error');
                
                div.innerHTML = 
                    '<div class="database-name">' + name + '</div>' +
                    '<div class="database-status">' + (dbInfo.healthy ? 'OK' : 'Error') + '</div>';
                
                return div;
            }

            showAlert(alert) {
                const alertsContainer = document.getElementById('alerts-container');
                if (!alertsContainer) return;

                const alertDiv = document.createElement('div');
                alertDiv.className = 'alert ' + alert.severity;
                alertDiv.innerHTML = 
                    '<strong>' + alert.title + '</strong><br>' +
                    alert.message +
                    '<div style="margin-top: 10px; font-size: 0.9em; color: #666;">' +
                    'Servicio: ' + alert.service + ' | ' + new Date(alert.timestamp).toLocaleString() +
                    '</div>';

                alertsContainer.insertBefore(alertDiv, alertsContainer.firstChild);

                // Auto-remover después de 30 segundos
                setTimeout(() => {
                    if (alertDiv.parentNode) {
                        alertDiv.parentNode.removeChild(alertDiv);
                    }
                }, 30000);

                this.addLog(alert.severity, alert.title + ': ' + alert.message);
            }

            addLog(level, message) {
                const timestamp = new Date().toLocaleTimeString();
                const logEntry = {
                    timestamp: timestamp,
                    level: level,
                    message: message
                };

                this.logs.unshift(logEntry);
                if (this.logs.length > this.maxLogs) {
                    this.logs = this.logs.slice(0, this.maxLogs);
                }

                this.updateLogsDisplay();
            }

            addLogFromServer(logData) {
                this.addLog(logData.level, logData.message);
            }

            updateLogsDisplay() {
                const logsContent = document.getElementById('logs-content');
                if (!logsContent) return;

                const filter = document.getElementById('log-level-filter').value;
                const filteredLogs = filter === 'all' ? this.logs : this.logs.filter(log => log.level === filter);

                logsContent.innerHTML = filteredLogs.map(log => 
                    '<div class="log-entry">' +
                    '<span class="log-timestamp">[' + log.timestamp + ']</span>' +
                    '<span class="log-level log-' + log.level + '">' + log.level.toUpperCase() + '</span>' +
                    '<span class="log-message">' + log.message + '</span>' +
                    '</div>'
                ).join('');

                // Auto-scroll al final
                logsContent.scrollTop = logsContent.scrollHeight;
            }

            updateConnectionStatus(connected) {
                const connectionStatus = document.getElementById('connection-status');
                if (connectionStatus) {
                    connectionStatus.className = 'status-dot ' + (connected ? 'status-online' : 'status-error');
                }
            }

            updateSystemStatus(healthy) {
                const systemStatus = document.getElementById('system-status');
                if (systemStatus) {
                    systemStatus.className = 'status-dot ' + (healthy ? 'status-online' : 'status-warning');
                }
            }

            updateElement(id, value) {
                const element = document.getElementById(id);
                if (element) {
                    element.textContent = value;
                }
            }

            setupEventListeners() {
                // Filtro de logs
                const logFilter = document.getElementById('log-level-filter');
                if (logFilter) {
                    logFilter.addEventListener('change', () => this.updateLogsDisplay());
                }

                // Limpiar logs
                const clearLogsBtn = document.getElementById('clear-logs');
                if (clearLogsBtn) {
                    clearLogsBtn.addEventListener('click', () => {
                        this.logs = [];
                        this.updateLogsDisplay();
                    });
                }

                // Reconectar en caso de error de red
                window.addEventListener('online', () => {
                    if (!this.isConnected) {
                        this.addLog('info', 'Conexión de red restaurada, reconectando...');
                        this.connectWebSocket();
                    }
                });

                window.addEventListener('offline', () => {
                    this.addLog('warning', 'Conexión de red perdida');
                });
            }

            startUpdateTimer() {
                // Actualizar cada 5 segundos si no hay WebSocket
                setInterval(async () => {
                    if (!this.isConnected) {
                        await this.loadInitialData();
                    }
                }, 5000);
            }
        }

        // Inicializar cuando el DOM esté listo
        document.addEventListener('DOMContentLoaded', () => {
            window.gopherKitMonitor = new GopherKitMonitor();
        });
    `
}