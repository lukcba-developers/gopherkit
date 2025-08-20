// Package monitoring - Alert Management System
// Migrated from ClubPulse to gopherkit with improvements
package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// AlertSeverity representa el nivel de severidad de una alerta
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityFatal    AlertSeverity = "fatal"
)

// AlertChannel define los tipos de canales de alerta disponibles
type AlertChannel string

const (
	AlertChannelWebhook AlertChannel = "webhook"
	AlertChannelSlack   AlertChannel = "slack"
	AlertChannelEmail   AlertChannel = "email"
	AlertChannelLog     AlertChannel = "log"
)

// Alert representa una alerta del sistema
type Alert struct {
	ID          string                 `json:"id"`
	Service     string                 `json:"service"`
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	Severity    AlertSeverity          `json:"severity"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	RetryCount  int                    `json:"retry_count"`
	LastAttempt time.Time              `json:"last_attempt"`
}

// AlertRule define reglas para activación automática de alertas
type AlertRule struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Condition string                 `json:"condition"`
	Threshold float64                `json:"threshold"`
	Duration  time.Duration          `json:"duration"`
	Severity  AlertSeverity          `json:"severity"`
	Enabled   bool                   `json:"enabled"`
	Channels  []AlertChannel         `json:"channels"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	LastFired *time.Time             `json:"last_fired,omitempty"`
	FireCount int                    `json:"fire_count"`
}

// AlertConfig configuración del sistema de alertas
type AlertConfig struct {
	WebhookURL        string         `json:"webhook_url"`
	SlackToken        string         `json:"slack_token"`
	SlackChannel      string         `json:"slack_channel"`
	EmailSMTPServer   string         `json:"email_smtp_server"`
	EmailFrom         string         `json:"email_from"`
	EmailPassword     string         `json:"email_password"`
	MaxRetries        int            `json:"max_retries"`
	RetryInterval     time.Duration  `json:"retry_interval"`
	AlertCooldown     time.Duration  `json:"alert_cooldown"`
	EnabledChannels   []AlertChannel `json:"enabled_channels"`
	DefaultSeverity   AlertSeverity  `json:"default_severity"`
	AlertHistoryLimit int            `json:"alert_history_limit"`
}

// DefaultAlertConfig retorna configuración por defecto
func DefaultAlertConfig() *AlertConfig {
	return &AlertConfig{
		MaxRetries:        3,
		RetryInterval:     time.Minute * 5,
		AlertCooldown:     time.Minute * 10,
		EnabledChannels:   []AlertChannel{AlertChannelLog},
		DefaultSeverity:   AlertSeverityWarning,
		AlertHistoryLimit: 1000,
	}
}

// AlertManager gestiona el sistema completo de alertas
type AlertManager struct {
	config       *AlertConfig
	alerts       map[string]*Alert
	rules        map[string]*AlertRule
	alertHistory []*Alert
	mutex        sync.RWMutex
	httpClient   *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// AlertStats estadísticas del sistema de alertas
type AlertStats struct {
	TotalAlerts      int                   `json:"total_alerts"`
	ActiveAlerts     int                   `json:"active_alerts"`
	ResolvedAlerts   int                   `json:"resolved_alerts"`
	AlertsBySeverity map[AlertSeverity]int `json:"alerts_by_severity"`
	AlertsByService  map[string]int        `json:"alerts_by_service"`
	AlertsByChannel  map[AlertChannel]int  `json:"alerts_by_channel"`
	FailedAlerts     int                   `json:"failed_alerts"`
	LastAlert        *time.Time            `json:"last_alert,omitempty"`
}

// NewAlertManager crea una nueva instancia del AlertManager mejorado
func NewAlertManager(config *AlertConfig) *AlertManager {
	ctx, cancel := context.WithCancel(context.Background())

	if config == nil {
		config = DefaultAlertConfig()
	}

	am := &AlertManager{
		config:       config,
		alerts:       make(map[string]*Alert),
		rules:        make(map[string]*AlertRule),
		alertHistory: make([]*Alert, 0),
		httpClient: &http.Client{
			Timeout: time.Second * 30,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Cargar reglas de alerta predeterminadas de ClubPulse
	am.loadDefaultRules()

	// Iniciar procesador de alertas en background
	am.startAlertProcessor()

	return am
}

// loadDefaultRules carga las reglas de alerta específicas para ClubPulse/gopherkit
func (am *AlertManager) loadDefaultRules() {
	defaultRules := []*AlertRule{
		{
			ID:        "service_down",
			Name:      "Service Down",
			Condition: "service_health == 0",
			Threshold: 0,
			Duration:  time.Minute * 2,
			Severity:  AlertSeverityCritical,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelLog},
		},
		{
			ID:        "high_error_rate",
			Name:      "High Error Rate",
			Condition: "error_rate > threshold",
			Threshold: 0.05, // 5%
			Duration:  time.Minute * 5,
			Severity:  AlertSeverityWarning,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelLog},
		},
		{
			ID:        "high_latency",
			Name:      "High Latency",
			Condition: "response_time_p95 > threshold",
			Threshold: 2000, // 2 seconds
			Duration:  time.Minute * 5,
			Severity:  AlertSeverityWarning,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelLog},
		},
		{
			ID:        "database_connection_failure",
			Name:      "Database Connection Failure",
			Condition: "db_connections_failed > threshold",
			Threshold: 3,
			Duration:  time.Minute * 1,
			Severity:  AlertSeverityCritical,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelSlack, AlertChannelLog},
		},
		{
			ID:        "high_cpu_usage",
			Name:      "High CPU Usage",
			Condition: "cpu_usage > threshold",
			Threshold: 80.0, // 80%
			Duration:  time.Minute * 10,
			Severity:  AlertSeverityWarning,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelLog},
		},
		{
			ID:        "high_memory_usage",
			Name:      "High Memory Usage",
			Condition: "memory_usage > threshold",
			Threshold: 85.0, // 85%
			Duration:  time.Minute * 10,
			Severity:  AlertSeverityWarning,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelLog},
		},
		{
			ID:        "cache_hit_rate_low",
			Name:      "Cache Hit Rate Too Low",
			Condition: "cache_hit_rate < threshold",
			Threshold: 0.7, // 70%
			Duration:  time.Minute * 15,
			Severity:  AlertSeverityWarning,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelLog},
		},
		{
			ID:        "disk_usage_high",
			Name:      "Disk Usage High",
			Condition: "disk_usage > threshold",
			Threshold: 90.0, // 90%
			Duration:  time.Minute * 5,
			Severity:  AlertSeverityCritical,
			Enabled:   true,
			Channels:  []AlertChannel{AlertChannelWebhook, AlertChannelSlack, AlertChannelLog},
		},
	}

	for _, rule := range defaultRules {
		am.rules[rule.ID] = rule
	}
}

// FireAlert dispara una nueva alerta
func (am *AlertManager) FireAlert(service, title, message string, severity AlertSeverity, metadata map[string]interface{}) string {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	alertID := fmt.Sprintf("%s-%d", service, time.Now().UnixNano())

	alert := &Alert{
		ID:        alertID,
		Service:   service,
		Title:     title,
		Message:   message,
		Severity:  severity,
		Timestamp: time.Now(),
		Resolved:  false,
		Metadata:  metadata,
	}

	am.alerts[alertID] = alert
	am.addToHistory(alert)

	// Enviar alerta a canales configurados
	go am.sendAlert(alert)

	log.Printf("[ALERT] %s - %s: %s", severity, service, message)
	return alertID
}

// ResolveAlert marca una alerta como resuelta
func (am *AlertManager) ResolveAlert(alertID string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if alert, exists := am.alerts[alertID]; exists {
		now := time.Now()
		alert.Resolved = true
		alert.ResolvedAt = &now

		// Enviar notificación de resolución
		go am.sendAlertResolution(alert)

		log.Printf("[ALERT RESOLVED] %s - %s resolved", alert.Service, alert.Title)
		return true
	}

	return false
}

// EvaluateRules evalúa todas las reglas de alerta contra métricas actuales
func (am *AlertManager) EvaluateRules(metrics map[string]interface{}) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	for _, rule := range am.rules {
		if !rule.Enabled {
			continue
		}

		// Evaluar condición de la regla
		if am.evaluateCondition(rule, metrics) {
			// Verificar cooldown
			if rule.LastFired != nil && time.Since(*rule.LastFired) < am.config.AlertCooldown {
				continue
			}

			// Disparar alerta
			alertMetadata := map[string]interface{}{
				"rule_id":   rule.ID,
				"rule_name": rule.Name,
				"threshold": rule.Threshold,
				"metrics":   metrics,
			}

			am.FireAlert(
				"alert-rule",
				rule.Name,
				fmt.Sprintf("Rule '%s' triggered: %s", rule.Name, rule.Condition),
				rule.Severity,
				alertMetadata,
			)

			now := time.Now()
			rule.LastFired = &now
			rule.FireCount++
		}
	}
}

// evaluateCondition evalúa si una condición de regla se cumple (mejorado)
func (am *AlertManager) evaluateCondition(rule *AlertRule, metrics map[string]interface{}) bool {
	switch rule.Condition {
	case "service_health == 0":
		if health, ok := metrics["service_health"].(float64); ok {
			return health == 0
		}
	case "error_rate > threshold":
		if errorRate, ok := metrics["error_rate"].(float64); ok {
			return errorRate > rule.Threshold
		}
	case "response_time_p95 > threshold":
		if p95, ok := metrics["response_time_p95"].(float64); ok {
			return p95 > rule.Threshold
		}
	case "db_connections_failed > threshold":
		if failures, ok := metrics["db_connections_failed"].(float64); ok {
			return failures > rule.Threshold
		}
	case "cpu_usage > threshold":
		if cpuUsage, ok := metrics["cpu_usage"].(float64); ok {
			return cpuUsage > rule.Threshold
		}
	case "memory_usage > threshold":
		if memUsage, ok := metrics["memory_usage"].(float64); ok {
			return memUsage > rule.Threshold
		}
	case "cache_hit_rate < threshold":
		if hitRate, ok := metrics["cache_hit_rate"].(float64); ok {
			return hitRate < rule.Threshold
		}
	case "disk_usage > threshold":
		if diskUsage, ok := metrics["disk_usage"].(float64); ok {
			return diskUsage > rule.Threshold
		}
	}

	return false
}

// sendAlert envía una alerta a todos los canales configurados
func (am *AlertManager) sendAlert(alert *Alert) {
	for _, channel := range am.config.EnabledChannels {
		switch channel {
		case AlertChannelWebhook:
			am.sendWebhookAlert(alert)
		case AlertChannelSlack:
			am.sendSlackAlert(alert)
		case AlertChannelEmail:
			am.sendEmailAlert(alert)
		case AlertChannelLog:
			am.sendLogAlert(alert)
		}
	}
}

// sendWebhookAlert envía alerta via webhook
func (am *AlertManager) sendWebhookAlert(alert *Alert) {
	if am.config.WebhookURL == "" {
		return
	}

	payload := map[string]interface{}{
		"text":      fmt.Sprintf("🚨 ClubPulse Alert: %s - %s", alert.Title, alert.Message),
		"service":   alert.Service,
		"severity":  alert.Severity,
		"timestamp": alert.Timestamp,
		"alert_id":  alert.ID,
		"metadata":  alert.Metadata,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling webhook payload: %v", err)
		return
	}

	am.sendWithRetry(func() error {
		resp, err := am.httpClient.Post(am.config.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}

		return nil
	}, alert)
}

// sendSlackAlert envía alerta a Slack (implementación mejorada)
func (am *AlertManager) sendSlackAlert(alert *Alert) {
	if am.config.SlackToken == "" || am.config.SlackChannel == "" {
		return
	}

	// Implementación simplificada - en producción usarías la API de Slack
	log.Printf("[SLACK ALERT] %s: %s - %s", alert.Severity, alert.Title, alert.Message)
}

// sendEmailAlert envía alerta por email (implementación mejorada)
func (am *AlertManager) sendEmailAlert(alert *Alert) {
	if am.config.EmailSMTPServer == "" || am.config.EmailFrom == "" {
		return
	}

	// Implementación simplificada - en producción usarías SMTP
	log.Printf("[EMAIL ALERT] %s: %s - %s", alert.Severity, alert.Title, alert.Message)
}

// sendLogAlert envía alerta a logs
func (am *AlertManager) sendLogAlert(alert *Alert) {
	severityEmoji := map[AlertSeverity]string{
		AlertSeverityInfo:     "ℹ️",
		AlertSeverityWarning:  "⚠️",
		AlertSeverityCritical: "🚨",
		AlertSeverityFatal:    "💀",
	}

	emoji := severityEmoji[alert.Severity]
	log.Printf("[%s %s] %s - %s: %s", emoji, alert.Severity, alert.Service, alert.Title, alert.Message)
}

// sendAlertResolution envía notificación de resolución
func (am *AlertManager) sendAlertResolution(alert *Alert) {
	payload := map[string]interface{}{
		"text":        fmt.Sprintf("✅ ClubPulse Alert Resolved: %s - %s", alert.Title, alert.Message),
		"service":     alert.Service,
		"alert_id":    alert.ID,
		"resolved_at": alert.ResolvedAt,
	}

	if am.config.WebhookURL != "" {
		jsonData, _ := json.Marshal(payload)
		am.httpClient.Post(am.config.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	}

	log.Printf("[ALERT RESOLVED] ✅ %s - %s resolved", alert.Service, alert.Title)
}

// sendWithRetry envía una alerta con reintentos
func (am *AlertManager) sendWithRetry(sendFunc func() error, alert *Alert) {
	maxRetries := am.config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := sendFunc(); err == nil {
			return // Éxito
		} else {
			log.Printf("Alert send attempt %d/%d failed: %v", attempt+1, maxRetries, err)
			alert.RetryCount++
			alert.LastAttempt = time.Now()

			if attempt < maxRetries-1 {
				time.Sleep(am.config.RetryInterval)
			}
		}
	}

	log.Printf("Failed to send alert %s after %d attempts", alert.ID, maxRetries)
}

// startAlertProcessor inicia el procesador de alertas en background
func (am *AlertManager) startAlertProcessor() {
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-am.ctx.Done():
				return
			case <-ticker.C:
				am.cleanupOldAlerts()
			}
		}
	}()
}

// cleanupOldAlerts limpia alertas antiguas del historial
func (am *AlertManager) cleanupOldAlerts() {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	// Limpiar alertas resueltas antiguas
	cutoff := time.Now().Add(-24 * time.Hour)
	for id, alert := range am.alerts {
		if alert.Resolved && alert.ResolvedAt != nil && alert.ResolvedAt.Before(cutoff) {
			delete(am.alerts, id)
		}
	}

	// Mantener límite del historial
	if len(am.alertHistory) > am.config.AlertHistoryLimit {
		am.alertHistory = am.alertHistory[len(am.alertHistory)-am.config.AlertHistoryLimit:]
	}
}

// addToHistory agrega una alerta al historial
func (am *AlertManager) addToHistory(alert *Alert) {
	am.alertHistory = append(am.alertHistory, alert)
}

// GetActiveAlerts retorna todas las alertas activas
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	var activeAlerts []*Alert
	for _, alert := range am.alerts {
		if !alert.Resolved {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	return activeAlerts
}

// GetAlertHistory retorna el historial de alertas
func (am *AlertManager) GetAlertHistory() []*Alert {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	// Retornar copia para evitar modificaciones externas
	history := make([]*Alert, len(am.alertHistory))
	copy(history, am.alertHistory)
	return history
}

// GetAlertStats retorna estadísticas del sistema de alertas
func (am *AlertManager) GetAlertStats() *AlertStats {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	stats := &AlertStats{
		AlertsBySeverity: make(map[AlertSeverity]int),
		AlertsByService:  make(map[string]int),
		AlertsByChannel:  make(map[AlertChannel]int),
	}

	for _, alert := range am.alerts {
		stats.TotalAlerts++

		if alert.Resolved {
			stats.ResolvedAlerts++
		} else {
			stats.ActiveAlerts++
		}

		stats.AlertsBySeverity[alert.Severity]++
		stats.AlertsByService[alert.Service]++

		if stats.LastAlert == nil || alert.Timestamp.After(*stats.LastAlert) {
			stats.LastAlert = &alert.Timestamp
		}
	}

	return stats
}

// AddRule agrega una nueva regla de alerta
func (am *AlertManager) AddRule(rule *AlertRule) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	am.rules[rule.ID] = rule
	log.Printf("Alert rule added: %s - %s", rule.ID, rule.Name)
}

// RemoveRule elimina una regla de alerta
func (am *AlertManager) RemoveRule(ruleID string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if _, exists := am.rules[ruleID]; exists {
		delete(am.rules, ruleID)
		log.Printf("Alert rule removed: %s", ruleID)
		return true
	}

	return false
}

// GetRules retorna todas las reglas de alerta
func (am *AlertManager) GetRules() []*AlertRule {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	var rules []*AlertRule
	for _, rule := range am.rules {
		rules = append(rules, rule)
	}

	return rules
}

// Stop detiene el AlertManager
func (am *AlertManager) Stop() {
	am.cancel()
	am.wg.Wait()
	log.Println("AlertManager stopped")
}