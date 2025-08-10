package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// MetricsConfig holds configuration for metrics
type MetricsConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Enabled        bool
}

// MetricsCollector provides OpenTelemetry metrics collection
type MetricsCollector struct {
	config   MetricsConfig
	logger   logger.Logger
	provider *sdkmetric.MeterProvider
	meter    metric.Meter

	// Common metrics
	httpRequestsTotal     metric.Int64Counter
	httpRequestDuration   metric.Float64Histogram
	httpRequestsInFlight  metric.Int64UpDownCounter
	databaseConnections   metric.Int64UpDownCounter
	cacheHits             metric.Int64Counter
	cacheMisses           metric.Int64Counter
	businessEvents        metric.Int64Counter
	errorRate             metric.Int64Counter
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config MetricsConfig, logger logger.Logger) (*MetricsCollector, error) {
	if !config.Enabled {
		return &MetricsCollector{config: config, logger: logger}, nil
	}

	collector := &MetricsCollector{
		config: config,
		logger: logger,
	}

	if err := collector.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics collector: %w", err)
	}

	return collector, nil
}

// initialize sets up the OpenTelemetry metrics provider
func (mc *MetricsCollector) initialize() error {
	// Create resource
	res, err := mc.createResource()
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create Prometheus exporter
	prometheusExporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Create meter provider
	mc.provider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(prometheusExporter),
	)

	// Set global meter provider
	otel.SetMeterProvider(mc.provider)

	// Get meter for this service
	mc.meter = mc.provider.Meter(mc.config.ServiceName)

	// Create metrics
	if err := mc.createMetrics(); err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	mc.logger.WithFields(map[string]interface{}{
		"service":     mc.config.ServiceName,
		"version":     mc.config.ServiceVersion,
		"environment": mc.config.Environment,
	}).LogBusinessEvent(context.Background(), "metrics_initialized", nil)

	return nil
}

// createResource creates an OpenTelemetry resource
func (mc *MetricsCollector) createResource() (*resource.Resource, error) {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(mc.config.ServiceName),
		semconv.ServiceVersion(mc.config.ServiceVersion),
		semconv.DeploymentEnvironment(mc.config.Environment),
	)
}

// createMetrics creates all common metrics
func (mc *MetricsCollector) createMetrics() error {
	var err error

	// HTTP metrics
	mc.httpRequestsTotal, err = mc.meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	mc.httpRequestDuration, err = mc.meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	mc.httpRequestsInFlight, err = mc.meter.Int64UpDownCounter(
		"http_requests_in_flight",
		metric.WithDescription("Number of HTTP requests currently being processed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	// Database metrics
	mc.databaseConnections, err = mc.meter.Int64UpDownCounter(
		"database_connections",
		metric.WithDescription("Number of active database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	// Cache metrics
	mc.cacheHits, err = mc.meter.Int64Counter(
		"cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	mc.cacheMisses, err = mc.meter.Int64Counter(
		"cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	// Business metrics
	mc.businessEvents, err = mc.meter.Int64Counter(
		"business_events_total",
		metric.WithDescription("Total number of business events"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	// Error metrics
	mc.errorRate, err = mc.meter.Int64Counter(
		"errors_total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	return nil
}

// HTTP Metrics
func (mc *MetricsCollector) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	if !mc.config.Enabled || mc.httpRequestsTotal == nil {
		return
	}

	attributes := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status_code", statusCode),
	}

	mc.httpRequestsTotal.Add(ctx, 1, metric.WithAttributes(attributes...))
	mc.httpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attributes...))
}

func (mc *MetricsCollector) IncHTTPRequestsInFlight(ctx context.Context) {
	if !mc.config.Enabled || mc.httpRequestsInFlight == nil {
		return
	}
	mc.httpRequestsInFlight.Add(ctx, 1)
}

func (mc *MetricsCollector) DecHTTPRequestsInFlight(ctx context.Context) {
	if !mc.config.Enabled || mc.httpRequestsInFlight == nil {
		return
	}
	mc.httpRequestsInFlight.Add(ctx, -1)
}

// Database Metrics
func (mc *MetricsCollector) RecordDatabaseConnection(ctx context.Context, delta int64) {
	if !mc.config.Enabled || mc.databaseConnections == nil {
		return
	}
	mc.databaseConnections.Add(ctx, delta)
}

// Cache Metrics
func (mc *MetricsCollector) RecordCacheHit(ctx context.Context, cacheType string) {
	if !mc.config.Enabled || mc.cacheHits == nil {
		return
	}
	mc.cacheHits.Add(ctx, 1, metric.WithAttributes(attribute.String("cache_type", cacheType)))
}

func (mc *MetricsCollector) RecordCacheMiss(ctx context.Context, cacheType string) {
	if !mc.config.Enabled || mc.cacheMisses == nil {
		return
	}
	mc.cacheMisses.Add(ctx, 1, metric.WithAttributes(attribute.String("cache_type", cacheType)))
}

// Business Event Metrics
func (mc *MetricsCollector) RecordBusinessEvent(ctx context.Context, eventType string, labels map[string]string) {
	if !mc.config.Enabled || mc.businessEvents == nil {
		return
	}

	attributes := []attribute.KeyValue{attribute.String("event_type", eventType)}
	for k, v := range labels {
		attributes = append(attributes, attribute.String(k, v))
	}

	mc.businessEvents.Add(ctx, 1, metric.WithAttributes(attributes...))
}

// Error Metrics
func (mc *MetricsCollector) RecordError(ctx context.Context, errorType string, component string) {
	if !mc.config.Enabled || mc.errorRate == nil {
		return
	}

	mc.errorRate.Add(ctx, 1, metric.WithAttributes(
		attribute.String("error_type", errorType),
		attribute.String("component", component),
	))
}

// GetMeter returns the OpenTelemetry meter for custom metrics
func (mc *MetricsCollector) GetMeter() metric.Meter {
	return mc.meter
}

// Shutdown gracefully shuts down the metrics provider
func (mc *MetricsCollector) Shutdown(ctx context.Context) error {
	if !mc.config.Enabled || mc.provider == nil {
		return nil
	}

	if err := mc.provider.Shutdown(ctx); err != nil {
		mc.logger.LogError(ctx, err, "failed to shutdown metrics provider", nil)
		return err
	}

	mc.logger.LogBusinessEvent(ctx, "metrics_shutdown", nil)
	return nil
}