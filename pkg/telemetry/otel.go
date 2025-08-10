package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// OpenTelemetryConfig configuración para OpenTelemetry
type OpenTelemetryConfig struct {
	ServiceName        string
	ServiceVersion     string
	Environment        string
	
	// Tracing configuration
	TracingEnabled     bool
	TracingEndpoint    string
	TracingSampleRatio float64
	
	// Metrics configuration
	MetricsEnabled     bool
	MetricsEndpoint    string
	MetricsInterval    time.Duration
	
	// Logging configuration
	LogsEnabled        bool
	LogsEndpoint       string
	
	// Resource attributes
	DeploymentEnvironment string
	ServiceNamespace      string
	ServiceInstanceID     string
	
	// Propagation
	Propagators           []string
}

// DefaultOpenTelemetryConfig retorna configuración por defecto
func DefaultOpenTelemetryConfig() *OpenTelemetryConfig {
	return &OpenTelemetryConfig{
		ServiceName:           "gopherkit-service",
		ServiceVersion:        "1.0.0",
		Environment:           "development",
		TracingEnabled:        true,
		TracingEndpoint:       "http://localhost:4317",
		TracingSampleRatio:    1.0,
		MetricsEnabled:        true,
		MetricsEndpoint:       "http://localhost:4317", 
		MetricsInterval:       30 * time.Second,
		LogsEnabled:           true,
		LogsEndpoint:          "http://localhost:4317",
		DeploymentEnvironment: "development",
		ServiceNamespace:      "gopherkit",
		ServiceInstanceID:     "instance-1",
		Propagators:           []string{"tracecontext", "baggage"},
	}
}

// OpenTelemetryProvider gestiona la configuración de OpenTelemetry
type OpenTelemetryProvider struct {
	config           *OpenTelemetryConfig
	logger           logger.Logger
	
	// Providers
	tracerProvider   *sdktrace.TracerProvider
	meterProvider    *sdkmetric.MeterProvider
	
	// Instrumentos de métricas
	tracer           trace.Tracer
	meter            metric.Meter
	
	// Counters
	httpRequestsCounter    metric.Int64Counter
	businessEventsCounter  metric.Int64Counter
	errorsCounter          metric.Int64Counter
	
	// Histograms
	httpDurationHistogram  metric.Float64Histogram
	dbDurationHistogram    metric.Float64Histogram
	cacheDurationHistogram metric.Float64Histogram
	
	// Gauges
	activeConnectionsGauge metric.Int64UpDownCounter
	goroutinesGauge        metric.Int64UpDownCounter
	memoryGauge            metric.Int64UpDownCounter
}

// NewOpenTelemetryProvider crea un nuevo proveedor de OpenTelemetry
func NewOpenTelemetryProvider(config *OpenTelemetryConfig, logger logger.Logger) (*OpenTelemetryProvider, error) {
	if config == nil {
		config = DefaultOpenTelemetryConfig()
	}
	
	provider := &OpenTelemetryProvider{
		config: config,
		logger: logger,
	}
	
	// Inicializar resource
	resource, err := provider.initResource()
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	
	// Inicializar tracing
	if config.TracingEnabled {
		if err := provider.initTracing(resource); err != nil {
			return nil, fmt.Errorf("failed to initialize tracing: %w", err)
		}
	}
	
	// Inicializar metrics
	if config.MetricsEnabled {
		if err := provider.initMetrics(resource); err != nil {
			return nil, fmt.Errorf("failed to initialize metrics: %w", err)
		}
	}
	
	// Configurar propagators
	provider.setupPropagation()
	
	// Inicializar instrumentos de métricas
	if err := provider.initInstruments(); err != nil {
		return nil, fmt.Errorf("failed to initialize instruments: %w", err)
	}
	
	logger.LogBusinessEvent(context.Background(), "otel_provider_initialized", map[string]interface{}{
		"service":         config.ServiceName,
		"version":         config.ServiceVersion,
		"tracing_enabled": config.TracingEnabled,
		"metrics_enabled": config.MetricsEnabled,
		"logs_enabled":    config.LogsEnabled,
	})
	
	return provider, nil
}

// initResource inicializa el resource para OpenTelemetry
func (p *OpenTelemetryProvider) initResource() (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(p.config.ServiceName),
			semconv.ServiceVersion(p.config.ServiceVersion),
			semconv.ServiceInstanceID(p.config.ServiceInstanceID),
			semconv.ServiceNamespace(p.config.ServiceNamespace),
			semconv.DeploymentEnvironment(p.config.DeploymentEnvironment),
			attribute.String("gopherkit.version", "1.0.0"),
		),
	)
}

// initTracing inicializa el tracing
func (p *OpenTelemetryProvider) initTracing(res *resource.Resource) error {
	// En un entorno real, aquí se configuraría el exportador (Jaeger, OTLP, etc.)
	// Para este ejemplo, usamos un exportador simplificado
	
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(p.config.TracingSampleRatio)),
		// sdktrace.WithBatcher(otlptracegrpc.New(...)), // En producción
	)
	
	p.tracerProvider = tracerProvider
	p.tracer = tracerProvider.Tracer(
		"github.com/lukcba-developers/gopherkit",
		trace.WithInstrumentationVersion("1.0.0"),
	)
	
	// Configurar globalmente
	otel.SetTracerProvider(tracerProvider)
	
	return nil
}

// initMetrics inicializa las métricas
func (p *OpenTelemetryProvider) initMetrics(res *resource.Resource) error {
	// Configurar exportador Prometheus para métricas
	prometheusExporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}
	
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(prometheusExporter),
		// sdkmetric.WithReader(otlpmetricgrpc.New(...)), // Para OTLP en producción
	)
	
	p.meterProvider = meterProvider
	p.meter = meterProvider.Meter(
		"github.com/lukcba-developers/gopherkit",
		metric.WithInstrumentationVersion("1.0.0"),
	)
	
	// Configurar globalmente
	otel.SetMeterProvider(meterProvider)
	
	return nil
}

// setupPropagation configura la propagación de contexto
func (p *OpenTelemetryProvider) setupPropagation() {
	propagators := []propagation.TextMapPropagator{}
	
	for _, prop := range p.config.Propagators {
		switch prop {
		case "tracecontext":
			propagators = append(propagators, propagation.TraceContext{})
		case "baggage":
			propagators = append(propagators, propagation.Baggage{})
		case "b3":
			// propagators = append(propagators, b3.New()) // Requiere import adicional
		}
	}
	
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagators...))
}

// initInstruments inicializa los instrumentos de métricas
func (p *OpenTelemetryProvider) initInstruments() error {
	if p.meter == nil {
		return nil // Métricas no habilitadas
	}
	
	var err error
	
	// Counters
	p.httpRequestsCounter, err = p.meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create http_requests_counter: %w", err)
	}
	
	p.businessEventsCounter, err = p.meter.Int64Counter(
		"business_events_total",
		metric.WithDescription("Total number of business events"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create business_events_counter: %w", err)
	}
	
	p.errorsCounter, err = p.meter.Int64Counter(
		"errors_total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create errors_counter: %w", err)
	}
	
	// Histograms
	p.httpDurationHistogram, err = p.meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create http_duration_histogram: %w", err)
	}
	
	p.dbDurationHistogram, err = p.meter.Float64Histogram(
		"db_query_duration_seconds",
		metric.WithDescription("Duration of database queries"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create db_duration_histogram: %w", err)
	}
	
	p.cacheDurationHistogram, err = p.meter.Float64Histogram(
		"cache_operation_duration_seconds",
		metric.WithDescription("Duration of cache operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create cache_duration_histogram: %w", err)
	}
	
	// Gauges (UpDownCounters)
	p.activeConnectionsGauge, err = p.meter.Int64UpDownCounter(
		"active_connections",
		metric.WithDescription("Number of active connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create active_connections_gauge: %w", err)
	}
	
	p.goroutinesGauge, err = p.meter.Int64UpDownCounter(
		"goroutines_active",
		metric.WithDescription("Number of active goroutines"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create goroutines_gauge: %w", err)
	}
	
	p.memoryGauge, err = p.meter.Int64UpDownCounter(
		"memory_usage_bytes",
		metric.WithDescription("Memory usage in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return fmt.Errorf("failed to create memory_gauge: %w", err)
	}
	
	return nil
}

// GetTracer retorna el tracer
func (p *OpenTelemetryProvider) GetTracer() trace.Tracer {
	return p.tracer
}

// GetMeter retorna el meter
func (p *OpenTelemetryProvider) GetMeter() metric.Meter {
	return p.meter
}

// StartSpan inicia un nuevo span
func (p *OpenTelemetryProvider) StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if p.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return p.tracer.Start(ctx, spanName, opts...)
}

// RecordHTTPRequest registra una request HTTP
func (p *OpenTelemetryProvider) RecordHTTPRequest(ctx context.Context, method, path, status string, duration time.Duration, tenantID string) {
	if p.httpRequestsCounter != nil {
		p.httpRequestsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
			attribute.String("status", status),
			attribute.String("tenant_id", tenantID),
		))
	}
	
	if p.httpDurationHistogram != nil {
		p.httpDurationHistogram.Record(ctx, duration.Seconds(), metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
			attribute.String("status", status),
		))
	}
}

// RecordBusinessEvent registra un evento de negocio
func (p *OpenTelemetryProvider) RecordBusinessEvent(ctx context.Context, eventType, tenantID string) {
	if p.businessEventsCounter != nil {
		p.businessEventsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("event_type", eventType),
			attribute.String("tenant_id", tenantID),
		))
	}
}

// RecordError registra un error
func (p *OpenTelemetryProvider) RecordError(ctx context.Context, errorType, service string) {
	if p.errorsCounter != nil {
		p.errorsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("error_type", errorType),
			attribute.String("service", service),
		))
	}
}

// RecordDatabaseOperation registra una operación de base de datos
func (p *OpenTelemetryProvider) RecordDatabaseOperation(ctx context.Context, operation, table string, duration time.Duration) {
	if p.dbDurationHistogram != nil {
		p.dbDurationHistogram.Record(ctx, duration.Seconds(), metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.String("table", table),
		))
	}
}

// RecordCacheOperation registra una operación de cache
func (p *OpenTelemetryProvider) RecordCacheOperation(ctx context.Context, operation string, duration time.Duration, hit bool) {
	if p.cacheDurationHistogram != nil {
		p.cacheDurationHistogram.Record(ctx, duration.Seconds(), metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.Bool("hit", hit),
		))
	}
}

// SetActiveConnections actualiza el gauge de conexiones activas
func (p *OpenTelemetryProvider) SetActiveConnections(ctx context.Context, count int64) {
	if p.activeConnectionsGauge != nil {
		p.activeConnectionsGauge.Add(ctx, count)
	}
}

// SetGoroutineCount actualiza el contador de goroutines
func (p *OpenTelemetryProvider) SetGoroutineCount(ctx context.Context, count int64) {
	if p.goroutinesGauge != nil {
		p.goroutinesGauge.Add(ctx, count)
	}
}

// SetMemoryUsage actualiza el gauge de uso de memoria
func (p *OpenTelemetryProvider) SetMemoryUsage(ctx context.Context, bytes int64) {
	if p.memoryGauge != nil {
		p.memoryGauge.Add(ctx, bytes)
	}
}

// Shutdown cierra el proveedor de OpenTelemetry
func (p *OpenTelemetryProvider) Shutdown(ctx context.Context) error {
	var errors []error
	
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}
	
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}
	
	p.logger.LogBusinessEvent(context.Background(), "otel_provider_shutdown", map[string]interface{}{
		"service": p.config.ServiceName,
	})
	
	return nil
}

// CreateSpanAttributes crea atributos comunes para spans
func CreateSpanAttributes(attributes map[string]interface{}) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(attributes))
	
	for key, value := range attributes {
		switch v := value.(type) {
		case string:
			attrs = append(attrs, attribute.String(key, v))
		case int:
			attrs = append(attrs, attribute.Int(key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(key, v))
		case bool:
			attrs = append(attrs, attribute.Bool(key, v))
		default:
			attrs = append(attrs, attribute.String(key, fmt.Sprintf("%v", v)))
		}
	}
	
	return attrs
}

// AddSpanAttributes añade atributos a un span existente
func AddSpanAttributes(span trace.Span, attributes map[string]interface{}) {
	if span != nil && attributes != nil {
		span.SetAttributes(CreateSpanAttributes(attributes)...)
	}
}