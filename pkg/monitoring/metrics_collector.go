// Package monitoring - Metrics Collection System
// Migrated from ClubPulse to gopherkit with improvements
package monitoring

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// MetricsCollector recolector de métricas del sistema mejorado
type MetricsCollector struct {
	client        *http.Client
	dockerClient  *DockerStatsCollector
	services      []string
	lastCollected time.Time
	config        *MetricsCollectorConfig
}

// MetricsCollectorConfig configuración del recolector de métricas
type MetricsCollectorConfig struct {
	ServicePorts     map[string]int    `json:"service_ports"`
	HealthEndpoint   string            `json:"health_endpoint"`
	CollectInterval  time.Duration     `json:"collect_interval"`
	RequestTimeout   time.Duration     `json:"request_timeout"`
	EnableDocker     bool              `json:"enable_docker"`
	EnableSystemInfo bool              `json:"enable_system_info"`
	TenantID         string            `json:"tenant_id"`
}

// DefaultMetricsCollectorConfig retorna configuración por defecto
func DefaultMetricsCollectorConfig() *MetricsCollectorConfig {
	return &MetricsCollectorConfig{
		ServicePorts: map[string]int{
			"auth-api":         8083,
			"user-api":         8081,
			"calendar-api":     8087,
			"championship-api": 8084,
			"membership-api":   8088,
			"facilities-api":   8089,
			"notification-api": 8090,
			"booking-api":      8086,
			"payments-api":     8091,
			"super-admin-api":  8092,
			"bff-api":          8085,
		},
		HealthEndpoint:   "/health",
		CollectInterval:  time.Second * 30,
		RequestTimeout:   time.Second * 10,
		EnableDocker:     true,
		EnableSystemInfo: true,
		TenantID:         "550e8400-e29b-41d4-a716-446655440000",
	}
}

// DockerStatsCollector recolector de estadísticas de Docker mejorado
type DockerStatsCollector struct {
	containerStats map[string]DockerStats
	lastUpdate     time.Time
	enabled        bool
}

// NewMetricsCollector crea un nuevo recolector de métricas mejorado
func NewMetricsCollector() (*MetricsCollector, error) {
	config := DefaultMetricsCollectorConfig()
	
	return &MetricsCollector{
		client: &http.Client{
			Timeout: config.RequestTimeout,
		},
		dockerClient: &DockerStatsCollector{
			containerStats: make(map[string]DockerStats),
			enabled:        config.EnableDocker,
		},
		services: extractServiceNames(config.ServicePorts),
		config:   config,
	}, nil
}

// NewMetricsCollectorWithConfig crea un recolector con configuración personalizada
func NewMetricsCollectorWithConfig(config *MetricsCollectorConfig) (*MetricsCollector, error) {
	if config == nil {
		config = DefaultMetricsCollectorConfig()
	}
	
	return &MetricsCollector{
		client: &http.Client{
			Timeout: config.RequestTimeout,
		},
		dockerClient: &DockerStatsCollector{
			containerStats: make(map[string]DockerStats),
			enabled:        config.EnableDocker,
		},
		services: extractServiceNames(config.ServicePorts),
		config:   config,
	}, nil
}

// extractServiceNames extrae nombres de servicios del mapa de puertos
func extractServiceNames(servicePorts map[string]int) []string {
	services := make([]string, 0, len(servicePorts))
	for service := range servicePorts {
		services = append(services, service)
	}
	return services
}

// CollectServiceHealth recolecta el estado de salud de servicios
func (mc *MetricsCollector) CollectServiceHealth() (map[string]ServiceMetrics, error) {
	serviceMetrics := make(map[string]ServiceMetrics)

	for serviceName, port := range mc.config.ServicePorts {
		metrics, err := mc.collectSingleServiceHealth(serviceName, port)
		if err != nil {
			log.Printf("Error collecting health for %s: %v", serviceName, err)
			// Crear métricas de error en lugar de fallar completamente
			metrics = ServiceMetrics{
				Name:            serviceName,
				Status:          "error",
				ResponseTime:    0,
				RequestsPerSec:  0,
				ErrorRate:       1.0,
				CPUUsage:        0,
				MemoryUsage:     0,
				ActiveUsers:     0,
				LastHealthCheck: time.Now(),
			}
		}
		serviceMetrics[serviceName] = metrics
	}

	mc.lastCollected = time.Now()
	return serviceMetrics, nil
}

// collectSingleServiceHealth recolecta métricas de un servicio individual
func (mc *MetricsCollector) collectSingleServiceHealth(serviceName string, port int) (ServiceMetrics, error) {
	start := time.Now()
	
	// Construir URL de health check
	url := fmt.Sprintf("http://localhost:%d%s", port, mc.config.HealthEndpoint)
	
	// Crear request con headers necesarios
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ServiceMetrics{}, errors.Wrap(err, "failed to create request")
	}
	
	// Agregar headers multi-tenant si es necesario
	if mc.config.TenantID != "" {
		req.Header.Set("X-Tenant-ID", mc.config.TenantID)
	}
	
	// Realizar request
	resp, err := mc.client.Do(req)
	if err != nil {
		return ServiceMetrics{}, errors.Wrap(err, "failed to make health check request")
	}
	defer resp.Body.Close()
	
	responseTime := time.Since(start)
	
	// Determinar estado basado en código de respuesta
	status := "unhealthy"
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status = "healthy"
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		status = "degraded"
	}
	
	// Recolectar métricas de Docker si está habilitado
	var cpuUsage, memoryUsage float64
	if mc.dockerClient.enabled {
		if dockerStats, exists := mc.dockerClient.containerStats[serviceName]; exists {
			cpuUsage = dockerStats.CPUPercent
			memoryUsage = float64(dockerStats.MemoryUsage) / (1024 * 1024) // Convert to MB
		}
	}
	
	return ServiceMetrics{
		Name:            serviceName,
		Status:          status,
		ResponseTime:    responseTime,
		RequestsPerSec:  0, // Implementar si hay métricas disponibles
		ErrorRate:       0, // Implementar si hay métricas disponibles
		CPUUsage:        cpuUsage,
		MemoryUsage:     memoryUsage,
		ActiveUsers:     0, // Implementar si hay métricas disponibles
		LastHealthCheck: time.Now(),
	}, nil
}

// CollectDockerStats recolecta estadísticas de contenedores Docker
func (mc *MetricsCollector) CollectDockerStats() (map[string]DockerStats, error) {
	if !mc.dockerClient.enabled {
		return mc.dockerClient.containerStats, nil
	}

	// Ejecutar comando docker stats
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", 
		"table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}")
	
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute docker stats command")
	}
	
	// Parsear output
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return mc.dockerClient.containerStats, nil
	}
	
	// Procesar cada línea (omitir header)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		
		stats, err := mc.parseDockerStatsLine(line)
		if err != nil {
			log.Printf("Error parsing docker stats line: %s, error: %v", line, err)
			continue
		}
		
		// Buscar el nombre del servicio basado en el nombre del contenedor
		serviceName := mc.extractServiceNameFromContainer(stats.ContainerName)
		if serviceName != "" {
			mc.dockerClient.containerStats[serviceName] = stats
		}
	}
	
	mc.dockerClient.lastUpdate = time.Now()
	return mc.dockerClient.containerStats, nil
}

// parseDockerStatsLine parsea una línea de docker stats
func (mc *MetricsCollector) parseDockerStatsLine(line string) (DockerStats, error) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return DockerStats{}, errors.New("insufficient fields in docker stats line")
	}
	
	stats := DockerStats{
		ContainerName: fields[0],
	}
	
	// Parsear CPU percentage
	cpuStr := strings.TrimSuffix(fields[1], "%")
	if cpu, err := strconv.ParseFloat(cpuStr, 64); err == nil {
		stats.CPUPercent = cpu
	}
	
	// Parsear Memory usage (formato: "used / limit")
	memParts := strings.Split(fields[2], " / ")
	if len(memParts) >= 1 {
		if mem, err := mc.parseMemoryValue(memParts[0]); err == nil {
			stats.MemoryUsage = mem
		}
		if len(memParts) >= 2 {
			if limit, err := mc.parseMemoryValue(memParts[1]); err == nil {
				stats.MemoryLimit = limit
			}
		}
	}
	
	// Parsear Network I/O
	netParts := strings.Split(fields[4], " / ")
	if len(netParts) >= 2 {
		if rx, err := mc.parseMemoryValue(netParts[0]); err == nil {
			stats.NetworkRX = rx
		}
		if tx, err := mc.parseMemoryValue(netParts[1]); err == nil {
			stats.NetworkTX = tx
		}
	}
	
	// Parsear Block I/O
	blockParts := strings.Split(fields[5], " / ")
	if len(blockParts) >= 2 {
		if read, err := mc.parseMemoryValue(blockParts[0]); err == nil {
			stats.BlockRead = read
		}
		if write, err := mc.parseMemoryValue(blockParts[1]); err == nil {
			stats.BlockWrite = write
		}
	}
	
	return stats, nil
}

// parseMemoryValue parsea valores de memoria con unidades (MB, GB, etc.)
func (mc *MetricsCollector) parseMemoryValue(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	
	// Remover caracteres no numéricos excepto punto decimal
	var numStr string
	var unit string
	
	for i, char := range value {
		if char >= '0' && char <= '9' || char == '.' {
			numStr += string(char)
		} else {
			unit = value[i:]
			break
		}
	}
	
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	
	// Convertir según la unidad
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "B":
		return int64(num), nil
	case "KB", "KIB":
		return int64(num * 1024), nil
	case "MB", "MIB":
		return int64(num * 1024 * 1024), nil
	case "GB", "GIB":
		return int64(num * 1024 * 1024 * 1024), nil
	case "TB", "TIB":
		return int64(num * 1024 * 1024 * 1024 * 1024), nil
	default:
		return int64(num), nil
	}
}

// extractServiceNameFromContainer extrae el nombre del servicio del nombre del contenedor
func (mc *MetricsCollector) extractServiceNameFromContainer(containerName string) string {
	// Lógica para mapear nombres de contenedores a nombres de servicios
	// Asumiendo formato: "project_service_1" o "service-api"
	
	for serviceName := range mc.config.ServicePorts {
		if strings.Contains(containerName, serviceName) {
			return serviceName
		}
		
		// También verificar sin el sufijo "-api"
		baseServiceName := strings.TrimSuffix(serviceName, "-api")
		if strings.Contains(containerName, baseServiceName) {
			return serviceName
		}
	}
	
	return ""
}

// CollectSystemMetrics recolecta métricas generales del sistema
func (mc *MetricsCollector) CollectSystemMetrics() (map[string]interface{}, error) {
	if !mc.config.EnableSystemInfo {
		return make(map[string]interface{}), nil
	}

	metrics := make(map[string]interface{})
	
	// Métricas de Go runtime
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	metrics["go_goroutines"] = runtime.NumGoroutine()
	metrics["go_memory_alloc"] = memStats.Alloc
	metrics["go_memory_total_alloc"] = memStats.TotalAlloc
	metrics["go_memory_sys"] = memStats.Sys
	metrics["go_gc_runs"] = memStats.NumGC
	
	// Información del sistema
	metrics["timestamp"] = time.Now()
	metrics["last_collected"] = mc.lastCollected
	
	return metrics, nil
}

// CollectDatabaseMetrics recolecta métricas de base de datos (mock por ahora)
func (mc *MetricsCollector) CollectDatabaseMetrics() (DatabaseMetrics, error) {
	// Esta implementación sería más compleja en un entorno real
	// Requeriría conexiones a las bases de datos y queries específicas
	
	return DatabaseMetrics{
		Mode:                "single", // Obtener del sistema actual
		ActiveConnections:   15,
		MaxConnections:      25,
		ConnectionPoolUsage: 60.0,
		QueryLatency:        45 * time.Millisecond,
		QueriesPerSecond:    250.5,
		CacheHitRate:        85.2,
		SchemaHealth: map[string]string{
			"auth_schema":         "healthy",
			"user_schema":         "healthy",
			"calendar_schema":     "healthy",
			"championship_schema": "healthy",
		},
		FallbackStatus: "ready",
		RLSStatus:      "active",
	}, nil
}

// CollectBusinessMetrics recolecta métricas de negocio (mock por ahora)
func (mc *MetricsCollector) CollectBusinessMetrics() (BusinessMetrics, error) {
	// Esta implementación requeriría consultas a bases de datos específicas
	// para obtener datos reales de negocio
	
	return BusinessMetrics{
		TotalUsers:          1250,
		ActiveUsers:         89,
		ReservationsToday:   45,
		PaymentsToday:       2350.75,
		ChampionshipsActive: 8,
		MembershipsActive:   1180,
		AvgSessionDuration:  25 * time.Minute,
		ConversionRate:      3.2,
	}, nil
}

// StartPeriodicCollection inicia la recolección periódica de métricas
func (mc *MetricsCollector) StartPeriodicCollection(ctx context.Context) {
	ticker := time.NewTicker(mc.config.CollectInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Recolectar métricas en background
			go func() {
				if _, err := mc.CollectServiceHealth(); err != nil {
					log.Printf("Error collecting service health: %v", err)
				}
				
				if _, err := mc.CollectDockerStats(); err != nil {
					log.Printf("Error collecting docker stats: %v", err)
				}
				
				if _, err := mc.CollectSystemMetrics(); err != nil {
					log.Printf("Error collecting system metrics: %v", err)
				}
			}()
		}
	}
}

// GetLastCollectionTime retorna el tiempo de la última recolección
func (mc *MetricsCollector) GetLastCollectionTime() time.Time {
	return mc.lastCollected
}

// GetServiceList retorna la lista de servicios monitoreados
func (mc *MetricsCollector) GetServiceList() []string {
	return mc.services
}

// UpdateConfig actualiza la configuración del recolector
func (mc *MetricsCollector) UpdateConfig(config *MetricsCollectorConfig) {
	if config != nil {
		mc.config = config
		mc.services = extractServiceNames(config.ServicePorts)
		mc.client.Timeout = config.RequestTimeout
		mc.dockerClient.enabled = config.EnableDocker
	}
}

// GetConfig retorna la configuración actual
func (mc *MetricsCollector) GetConfig() *MetricsCollectorConfig {
	return mc.config
}