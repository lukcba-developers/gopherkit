// Package monitoring - Prometheus Integration System
// Migrated from ClubPulse to gopherkit with significant improvements
package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusIntegration gestiona la integración completa con Prometheus mejorada
type PrometheusIntegration struct {
	client       api.Client
	queryAPI     v1.API
	registry     *prometheus.Registry
	metrics      *GopherKitMetrics
	config       *PrometheusConfig
	httpServer   *http.Server
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	alertManager *AlertManager
	isRunning    bool
	mu           sync.RWMutex
}

// PrometheusConfig configuración mejorada de la integración con Prometheus
type PrometheusConfig struct {
	PrometheusURL     string        `json:"prometheus_url"`
	MetricsPort       string        `json:"metrics_port"`
	ScrapeInterval    time.Duration `json:"scrape_interval"`
	QueryTimeout      time.Duration `json:"query_timeout"`
	EnablePushGateway bool          `json:"enable_push_gateway"`
	PushGatewayURL    string        `json:"push_gateway_url"`
	JobName           string        `json:"job_name"`
	Instance          string        `json:"instance"`
	ServiceName       string        `json:"service_name"`
	Environment       string        `json:"environment"`
}

// DefaultPrometheusConfig retorna configuración por defecto mejorada
func DefaultPrometheusConfig() *PrometheusConfig {
	return &PrometheusConfig{
		PrometheusURL:     "http://localhost:9090",
		MetricsPort:       "9091",
		ScrapeInterval:    time.Second * 15,
		QueryTimeout:      time.Second * 30,
		EnablePushGateway: false,
		JobName:           "gopherkit-monitoring",
		Instance:          "localhost:9091",
		ServiceName:       "gopherkit",
		Environment:       "development",
	}
}

// GopherKitMetrics métricas específicas de GopherKit con mejoras
type GopherKitMetrics struct {
	// Métricas de Sistema
	HTTPRequests        *prometheus.CounterVec
	HTTPDuration        *prometheus.HistogramVec
	ActiveConnections   *prometheus.GaugeVec
	ServiceHealth       *prometheus.GaugeVec
	DatabaseConnections *prometheus.GaugeVec
	CacheHitRate        *prometheus.GaugeVec

	// Métricas de Negocio Genéricas (aplicables a cualquier dominio)
	BusinessEvents      *prometheus.CounterVec
	TransactionEvents   *prometheus.CounterVec
	AuthenticationEvents *prometheus.CounterVec
	ProcessingEvents    *prometheus.CounterVec
	NotificationEvents  *prometheus.CounterVec

	// Métricas de Performance
	ResponseTimeP95 *prometheus.GaugeVec
	ErrorRate       *prometheus.GaugeVec
	ThroughputRPS   *prometheus.GaugeVec
	QueueLength     *prometheus.GaugeVec

	// Métricas de Infraestructura
	CPUUsage    *prometheus.GaugeVec
	MemoryUsage *prometheus.GaugeVec
	DiskUsage   *prometheus.GaugeVec
	NetworkIO   *prometheus.CounterVec

	// Métricas de Calidad
	CodeCoverage     *prometheus.GaugeVec
	TestResults      *prometheus.CounterVec
	BuildDuration    *prometheus.HistogramVec
	DeploymentEvents *prometheus.CounterVec
}

// NewPrometheusIntegration crea una nueva instancia mejorada de integración con Prometheus
func NewPrometheusIntegration(config *PrometheusConfig, alertManager *AlertManager) (*PrometheusIntegration, error) {
	if config == nil {
		config = DefaultPrometheusConfig()
	}

	// Validar configuración
	if err := validatePrometheusConfig(config); err != nil {
		return nil, errors.Wrap(err, "invalid Prometheus configuration")
	}

	// Crear cliente de Prometheus
	client, err := api.NewClient(api.Config{
		Address: config.PrometheusURL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Prometheus client")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pi := &PrometheusIntegration{
		client:       client,
		queryAPI:     v1.NewAPI(client),
		registry:     prometheus.NewRegistry(),
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		alertManager: alertManager,
		isRunning:    false,
	}

	// Inicializar métricas
	if err := pi.initializeMetrics(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to initialize metrics")
	}

	return pi, nil
}

// validatePrometheusConfig valida la configuración de Prometheus
func validatePrometheusConfig(config *PrometheusConfig) error {
	if config.PrometheusURL == "" {
		return errors.New("prometheus_url is required")
	}
	if config.MetricsPort == "" {
		return errors.New("metrics_port is required")
	}
	if config.ScrapeInterval <= 0 {
		return errors.New("scrape_interval must be positive")
	}
	if config.QueryTimeout <= 0 {
		return errors.New("query_timeout must be positive")
	}
	return nil
}

// Start inicia la integración con Prometheus
func (pi *PrometheusIntegration) Start() error {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if pi.isRunning {
		return errors.New("Prometheus integration already running")
	}

	// Iniciar servidor de métricas
	if err := pi.startMetricsServer(); err != nil {
		return errors.Wrap(err, "failed to start metrics server")
	}

	// Iniciar recolector de métricas
	pi.startMetricsCollector()

	pi.isRunning = true
	log.Printf("Prometheus integration started successfully on port %s", pi.config.MetricsPort)
	return nil
}

// initializeMetrics inicializa todas las métricas de GopherKit con mejoras
func (pi *PrometheusIntegration) initializeMetrics() error {
	pi.metrics = &GopherKitMetrics{
		// Métricas de Sistema
		HTTPRequests: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_http_requests_total",
				Help: "Total number of HTTP requests processed by GopherKit services",
				ConstLabels: prometheus.Labels{
					"service":     pi.config.ServiceName,
					"environment": pi.config.Environment,
				},
			},
			[]string{"service", "method", "endpoint", "status"},
		),

		HTTPDuration: promauto.With(pi.registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gopherkit_http_request_duration_seconds",
				Help:    "HTTP request latency in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
				ConstLabels: prometheus.Labels{
					"service":     pi.config.ServiceName,
					"environment": pi.config.Environment,
				},
			},
			[]string{"service", "method", "endpoint"},
		),

		ActiveConnections: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_active_connections",
				Help: "Number of active connections per service",
			},
			[]string{"service", "type"},
		),

		ServiceHealth: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_service_health",
				Help: "Health status of GopherKit services (1=healthy, 0=unhealthy)",
			},
			[]string{"service", "endpoint"},
		),

		DatabaseConnections: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_database_connections",
				Help: "Number of active database connections",
			},
			[]string{"service", "database", "status"},
		),

		CacheHitRate: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_cache_hit_rate",
				Help: "Cache hit rate percentage",
			},
			[]string{"service", "cache_type"},
		),

		// Métricas de Negocio Genéricas
		BusinessEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_business_events_total",
				Help: "Total number of business events processed",
			},
			[]string{"event_type", "status", "domain"},
		),

		TransactionEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_transaction_events_total",
				Help: "Total number of transaction events",
			},
			[]string{"transaction_type", "status", "method"},
		),

		AuthenticationEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_authentication_events_total",
				Help: "Total number of authentication events",
			},
			[]string{"event_type", "status", "provider"},
		),

		ProcessingEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_processing_events_total",
				Help: "Total number of processing events",
			},
			[]string{"process_type", "status", "stage"},
		),

		NotificationEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_notification_events_total",
				Help: "Total number of notification events",
			},
			[]string{"channel", "type", "status"},
		),

		// Métricas de Performance
		ResponseTimeP95: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_response_time_p95_seconds",
				Help: "95th percentile response time in seconds",
			},
			[]string{"service", "endpoint"},
		),

		ErrorRate: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_error_rate",
				Help: "Error rate percentage",
			},
			[]string{"service", "error_type"},
		),

		ThroughputRPS: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_throughput_rps",
				Help: "Requests per second throughput",
			},
			[]string{"service", "endpoint"},
		),

		QueueLength: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_queue_length",
				Help: "Length of processing queues",
			},
			[]string{"service", "queue_type"},
		),

		// Métricas de Infraestructura
		CPUUsage: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_cpu_usage_percent",
				Help: "CPU usage percentage per service",
			},
			[]string{"service", "container"},
		),

		MemoryUsage: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_memory_usage_bytes",
				Help: "Memory usage in bytes per service",
			},
			[]string{"service", "container", "type"},
		),

		DiskUsage: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_disk_usage_bytes",
				Help: "Disk usage in bytes per service",
			},
			[]string{"service", "container", "mount_point"},
		),

		NetworkIO: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_network_io_bytes_total",
				Help: "Total network I/O in bytes",
			},
			[]string{"service", "container", "direction"},
		),

		// Métricas de Calidad
		CodeCoverage: promauto.With(pi.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gopherkit_code_coverage_percent",
				Help: "Code coverage percentage",
			},
			[]string{"service", "package"},
		),

		TestResults: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_test_results_total",
				Help: "Total number of test results",
			},
			[]string{"service", "test_type", "result"},
		),

		BuildDuration: promauto.With(pi.registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gopherkit_build_duration_seconds",
				Help:    "Build duration in seconds",
				Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
			},
			[]string{"service", "stage"},
		),

		DeploymentEvents: promauto.With(pi.registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "gopherkit_deployment_events_total",
				Help: "Total number of deployment events",
			},
			[]string{"service", "environment", "status"},
		),
	}

	return nil
}

// startMetricsServer inicia el servidor HTTP mejorado para exponer métricas
func (pi *PrometheusIntegration) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(pi.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Endpoint de salud específico para el servidor de métricas
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now(),
			"service":   pi.config.ServiceName,
		})
	})

	// Endpoint de estadísticas de métricas mejorado
	mux.HandleFunc("/metrics/stats", pi.handleMetricsStats)

	// Endpoint de configuración
	mux.HandleFunc("/config", pi.handleConfigInfo)

	pi.httpServer = &http.Server{
		Addr:         ":" + pi.config.MetricsPort,
		Handler:      mux,
		ReadTimeout:  time.Second * 15,
		WriteTimeout: time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	pi.wg.Add(1)
	go func() {
		defer pi.wg.Done()
		log.Printf("Starting GopherKit Prometheus metrics server on port %s", pi.config.MetricsPort)
		if err := pi.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	return nil
}

// startMetricsCollector inicia el recolector de métricas periódico mejorado
func (pi *PrometheusIntegration) startMetricsCollector() {
	pi.wg.Add(1)
	go func() {
		defer pi.wg.Done()

		ticker := time.NewTicker(pi.config.ScrapeInterval)
		defer ticker.Stop()

		// Ejecutar primera recolección inmediatamente
		pi.collectAndUpdateMetrics()

		for {
			select {
			case <-pi.ctx.Done():
				return
			case <-ticker.C:
				pi.collectAndUpdateMetrics()
			}
		}
	}()
}

// collectAndUpdateMetrics recolecta y actualiza todas las métricas con manejo de errores mejorado
func (pi *PrometheusIntegration) collectAndUpdateMetrics() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in metrics collection: %v", r)
		}
	}()

	// Obtener métricas del sistema
	systemMetrics, err := pi.querySystemMetrics()
	if err != nil {
		log.Printf("Error querying system metrics: %v", err)
		return
	}

	// Actualizar métricas de Prometheus
	pi.updatePrometheusMetrics(systemMetrics)

	// Evaluar reglas de alerta si hay AlertManager
	if pi.alertManager != nil {
		pi.alertManager.EvaluateRules(systemMetrics)
	}
}

// querySystemMetrics consulta métricas del sistema desde Prometheus con queries mejoradas
func (pi *PrometheusIntegration) querySystemMetrics() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pi.config.QueryTimeout)
	defer cancel()

	metrics := make(map[string]interface{})

	// Queries mejoradas para servicios genéricos
	queries := map[string]string{
		// Queries para health checks genéricos
		"service_health": fmt.Sprintf(`up{job="%s"}`, pi.config.JobName),
		"auth_health":    `up{job="auth-api"}`,
		"api_health":     `up{job=~".*-api"}`,

		// Queries para performance
		"response_time_p95": fmt.Sprintf(`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{job="%s"}[5m]))`, pi.config.JobName),
		"error_rate":        fmt.Sprintf(`rate(http_requests_total{job="%s",status=~"5.."}[5m]) / rate(http_requests_total{job="%s"}[5m])`, pi.config.JobName, pi.config.JobName),
		"throughput":        fmt.Sprintf(`rate(http_requests_total{job="%s"}[1m])`, pi.config.JobName),

		// Queries para database genéricos
		"db_connections_failed": fmt.Sprintf(`increase(database_connection_errors_total{job="%s"}[5m])`, pi.config.JobName),
		"db_connections_active": fmt.Sprintf(`database_connections_active{job="%s"}`, pi.config.JobName),

		// Queries para cache
		"cache_hit_rate": fmt.Sprintf(`rate(cache_hits_total{job="%s"}[5m]) / rate(cache_requests_total{job="%s"}[5m])`, pi.config.JobName, pi.config.JobName),

		// Queries para métricas de negocio genéricas
		"business_event_rate":     fmt.Sprintf(`rate(gopherkit_business_events_total{job="%s"}[5m])`, pi.config.JobName),
		"transaction_success_rate": fmt.Sprintf(`rate(gopherkit_transaction_events_total{job="%s",status="success"}[5m]) / rate(gopherkit_transaction_events_total{job="%s"}[5m])`, pi.config.JobName, pi.config.JobName),
		"auth_failures":           fmt.Sprintf(`increase(gopherkit_authentication_events_total{job="%s",status="failed"}[5m])`, pi.config.JobName),

		// Queries para infraestructura
		"cpu_usage":    `avg(container_cpu_usage_seconds_total{container_label_com_docker_compose_service=~".*"})`,
		"memory_usage": `avg(container_memory_usage_bytes{container_label_com_docker_compose_service=~".*"})`,
	}

	for key, query := range queries {
		result, warnings, err := pi.queryAPI.Query(ctx, query, time.Now())
		if err != nil {
			log.Printf("Error querying %s: %v", key, err)
			continue
		}

		if len(warnings) > 0 {
			log.Printf("Warnings for query %s: %v", key, warnings)
		}

		// Procesar resultado y extraer valor numérico
		if value := pi.extractNumericValue(result); value != nil {
			metrics[key] = *value
		}
	}

	return metrics, nil
}

// extractNumericValue extrae un valor numérico de un resultado de Prometheus con mejor manejo
func (pi *PrometheusIntegration) extractNumericValue(result interface{}) *float64 {
	// Implementación mejorada para diferentes tipos de resultados
	str := fmt.Sprintf("%v", result)
	if value, err := strconv.ParseFloat(str, 64); err == nil {
		return &value
	}
	return nil
}

// updatePrometheusMetrics actualiza las métricas de Prometheus con los valores recolectados mejorado
func (pi *PrometheusIntegration) updatePrometheusMetrics(systemMetrics map[string]interface{}) {
	// Actualizar métricas de salud de servicios
	if health, ok := systemMetrics["service_health"].(float64); ok {
		pi.metrics.ServiceHealth.WithLabelValues("all", "health").Set(health)
	}

	if authHealth, ok := systemMetrics["auth_health"].(float64); ok {
		pi.metrics.ServiceHealth.WithLabelValues("auth-api", "health").Set(authHealth)
	}

	// Actualizar métricas de performance
	if p95, ok := systemMetrics["response_time_p95"].(float64); ok {
		pi.metrics.ResponseTimeP95.WithLabelValues("all", "average").Set(p95)
	}

	if errorRate, ok := systemMetrics["error_rate"].(float64); ok {
		pi.metrics.ErrorRate.WithLabelValues("all", "http").Set(errorRate)
	}

	if throughput, ok := systemMetrics["throughput"].(float64); ok {
		pi.metrics.ThroughputRPS.WithLabelValues("all", "average").Set(throughput)
	}

	// Actualizar métricas de database
	if dbConnFailed, ok := systemMetrics["db_connections_failed"].(float64); ok {
		pi.metrics.DatabaseConnections.WithLabelValues("all", "all", "failed").Set(dbConnFailed)
	}

	if dbConnActive, ok := systemMetrics["db_connections_active"].(float64); ok {
		pi.metrics.DatabaseConnections.WithLabelValues("all", "all", "active").Set(dbConnActive)
	}

	// Actualizar métricas de cache
	if cacheHitRate, ok := systemMetrics["cache_hit_rate"].(float64); ok {
		pi.metrics.CacheHitRate.WithLabelValues("all", "redis").Set(cacheHitRate)
	}

	// Actualizar métricas de infraestructura
	if cpuUsage, ok := systemMetrics["cpu_usage"].(float64); ok {
		pi.metrics.CPUUsage.WithLabelValues("all", "all").Set(cpuUsage * 100) // Convertir a porcentaje
	}

	if memUsage, ok := systemMetrics["memory_usage"].(float64); ok {
		pi.metrics.MemoryUsage.WithLabelValues("all", "all", "used").Set(memUsage)
	}
}

// RecordHTTPRequest registra una petición HTTP con validación mejorada
func (pi *PrometheusIntegration) RecordHTTPRequest(service, method, endpoint, status string, duration time.Duration) {
	if pi.metrics == nil {
		return
	}
	pi.metrics.HTTPRequests.WithLabelValues(service, method, endpoint, status).Inc()
	pi.metrics.HTTPDuration.WithLabelValues(service, method, endpoint).Observe(duration.Seconds())
}

// RecordBusinessEvent registra un evento de negocio genérico
func (pi *PrometheusIntegration) RecordBusinessEvent(eventType, domain, status string, labels map[string]string) {
	if pi.metrics == nil {
		return
	}
	pi.metrics.BusinessEvents.WithLabelValues(eventType, status, domain).Inc()
}

// RecordTransactionEvent registra un evento de transacción
func (pi *PrometheusIntegration) RecordTransactionEvent(transactionType, method, status string) {
	if pi.metrics == nil {
		return
	}
	pi.metrics.TransactionEvents.WithLabelValues(transactionType, status, method).Inc()
}

// UpdateInfrastructureMetrics actualiza métricas de infraestructura con validación
func (pi *PrometheusIntegration) UpdateInfrastructureMetrics(service, container string, metrics map[string]float64) {
	if pi.metrics == nil {
		return
	}

	if cpuUsage, ok := metrics["cpu_usage"]; ok {
		pi.metrics.CPUUsage.WithLabelValues(service, container).Set(cpuUsage)
	}

	if memUsage, ok := metrics["memory_usage"]; ok {
		pi.metrics.MemoryUsage.WithLabelValues(service, container, "used").Set(memUsage)
	}

	if memLimit, ok := metrics["memory_limit"]; ok {
		pi.metrics.MemoryUsage.WithLabelValues(service, container, "limit").Set(memLimit)
	}

	if diskUsage, ok := metrics["disk_usage"]; ok {
		pi.metrics.DiskUsage.WithLabelValues(service, container, "/").Set(diskUsage)
	}

	if netRx, ok := metrics["network_rx"]; ok {
		pi.metrics.NetworkIO.WithLabelValues(service, container, "rx").Add(netRx)
	}

	if netTx, ok := metrics["network_tx"]; ok {
		pi.metrics.NetworkIO.WithLabelValues(service, container, "tx").Add(netTx)
	}
}

// RecordTestResults registra resultados de pruebas
func (pi *PrometheusIntegration) RecordTestResults(service, testType, result string) {
	if pi.metrics == nil {
		return
	}
	pi.metrics.TestResults.WithLabelValues(service, testType, result).Inc()
}

// RecordBuildDuration registra duración de build
func (pi *PrometheusIntegration) RecordBuildDuration(service, stage string, duration time.Duration) {
	if pi.metrics == nil {
		return
	}
	pi.metrics.BuildDuration.WithLabelValues(service, stage).Observe(duration.Seconds())
}

// handleMetricsStats maneja el endpoint de estadísticas de métricas mejorado
func (pi *PrometheusIntegration) handleMetricsStats(w http.ResponseWriter, r *http.Request) {
	gathererMetrics, err := pi.registry.Gather()
	if err != nil {
		http.Error(w, "Error gathering metrics", http.StatusInternalServerError)
		return
	}

	stats := map[string]interface{}{
		"prometheus_url":    pi.config.PrometheusURL,
		"metrics_port":      pi.config.MetricsPort,
		"scrape_interval":   pi.config.ScrapeInterval.String(),
		"job_name":          pi.config.JobName,
		"instance":          pi.config.Instance,
		"service_name":      pi.config.ServiceName,
		"environment":       pi.config.Environment,
		"registry_metrics":  len(gathererMetrics),
		"server_status":     "running",
		"last_collection":   time.Now(),
		"version":           "1.0.0",
		"runtime":           time.Since(time.Now()).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleConfigInfo maneja el endpoint de información de configuración
func (pi *PrometheusIntegration) handleConfigInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pi.config)
}

// GetGopherKitMetrics retorna las métricas para uso externo
func (pi *PrometheusIntegration) GetGopherKitMetrics() *GopherKitMetrics {
	return pi.metrics
}

// GetConfig retorna la configuración actual
func (pi *PrometheusIntegration) GetConfig() *PrometheusConfig {
	return pi.config
}

// IsRunning verifica si la integración está ejecutándose
func (pi *PrometheusIntegration) IsRunning() bool {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.isRunning
}

// Stop detiene la integración con Prometheus con mejor limpieza
func (pi *PrometheusIntegration) Stop() error {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if !pi.isRunning {
		return errors.New("Prometheus integration not running")
	}

	pi.cancel()

	// Detener servidor HTTP
	if pi.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		if err := pi.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down metrics server: %v", err)
		}
	}

	pi.wg.Wait()
	pi.isRunning = false
	log.Println("GopherKit Prometheus integration stopped successfully")
	return nil
}