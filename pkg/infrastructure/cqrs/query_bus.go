package cqrs

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// QueryBus handles query dispatching and execution
type QueryBus struct {
	handlers map[string]QueryHandler
	logger   *logrus.Logger
	mu       sync.RWMutex
	config   QueryBusConfig
	cache    QueryCache
}

// QueryBusConfig holds query bus configuration
type QueryBusConfig struct {
	EnableCache      bool          `json:"enable_cache"`
	CacheTTL         time.Duration `json:"cache_ttl"`
	EnableMetrics    bool          `json:"enable_metrics"`
	EnableTracing    bool          `json:"enable_tracing"`
	MaxConcurrent    int           `json:"max_concurrent"`
	TimeoutDuration  time.Duration `json:"timeout_duration"`
}

// Query represents a query that can be executed
type Query interface {
	GetQueryType() string
	GetParameters() map[string]interface{}
	GetCacheKey() string
	IsCacheable() bool
}

// QueryHandler defines the interface for query handlers
type QueryHandler interface {
	Handle(ctx context.Context, query Query) (*QueryResult, error)
	GetQueryType() string
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	Data      interface{}            `json:"data"`
	Success   bool                   `json:"success"`
	Message   string                 `json:"message,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Count     int                    `json:"count,omitempty"`
	FromCache bool                   `json:"from_cache"`
	Timestamp time.Time              `json:"timestamp"`
}

// BaseQuery provides a base implementation for queries
type BaseQuery struct {
	QueryType   string                 `json:"query_type"`
	Parameters  map[string]interface{} `json:"parameters"`
	CacheKey    string                 `json:"cache_key"`
	Cacheable   bool                   `json:"cacheable"`
	UserID      string                 `json:"user_id"`
	TenantID    string                 `json:"tenant_id"`
	CorrelationID string               `json:"correlation_id"`
}

// GetQueryType returns the query type
func (bq *BaseQuery) GetQueryType() string {
	return bq.QueryType
}

// GetParameters returns the query parameters
func (bq *BaseQuery) GetParameters() map[string]interface{} {
	return bq.Parameters
}

// GetCacheKey returns the cache key
func (bq *BaseQuery) GetCacheKey() string {
	if bq.CacheKey != "" {
		return bq.CacheKey
	}
	// Generate default cache key based on query type and parameters
	return fmt.Sprintf("%s:%s", bq.QueryType, bq.generateParameterHash())
}

// IsCacheable returns whether the query is cacheable
func (bq *BaseQuery) IsCacheable() bool {
	return bq.Cacheable
}

// generateParameterHash generates a hash from parameters for cache key
func (bq *BaseQuery) generateParameterHash() string {
	// Simple implementation - in production, use a proper hash function
	hash := ""
	for k, v := range bq.Parameters {
		hash += fmt.Sprintf("%s:%v;", k, v)
	}
	return hash
}

// QueryCache interface for caching query results
type QueryCache interface {
	Get(key string) (*QueryResult, bool)
	Set(key string, result *QueryResult, ttl time.Duration)
	Delete(key string)
	Clear()
	GetStats() CacheStats
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits     int64 `json:"hits"`
	Misses   int64 `json:"misses"`
	Entries  int   `json:"entries"`
	HitRatio float64 `json:"hit_ratio"`
}

// InMemoryQueryCache provides an in-memory implementation of QueryCache
type InMemoryQueryCache struct {
	cache map[string]*cacheEntry
	mu    sync.RWMutex
	stats CacheStats
}

type cacheEntry struct {
	result    *QueryResult
	expiresAt time.Time
}

// NewInMemoryQueryCache creates a new in-memory query cache
func NewInMemoryQueryCache() *InMemoryQueryCache {
	return &InMemoryQueryCache{
		cache: make(map[string]*cacheEntry),
		stats: CacheStats{},
	}
}

// Get retrieves a cached query result
func (imc *InMemoryQueryCache) Get(key string) (*QueryResult, bool) {
	imc.mu.RLock()
	defer imc.mu.RUnlock()

	entry, exists := imc.cache[key]
	if !exists {
		imc.stats.Misses++
		imc.updateHitRatio()
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Entry expired
		delete(imc.cache, key)
		imc.stats.Misses++
		imc.updateHitRatio()
		return nil, false
	}

	imc.stats.Hits++
	imc.updateHitRatio()
	
	// Mark result as from cache
	result := *entry.result
	result.FromCache = true
	return &result, true
}

// Set stores a query result in cache
func (imc *InMemoryQueryCache) Set(key string, result *QueryResult, ttl time.Duration) {
	imc.mu.Lock()
	defer imc.mu.Unlock()

	imc.cache[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(ttl),
	}
	imc.stats.Entries = len(imc.cache)
}

// Delete removes a cached entry
func (imc *InMemoryQueryCache) Delete(key string) {
	imc.mu.Lock()
	defer imc.mu.Unlock()

	delete(imc.cache, key)
	imc.stats.Entries = len(imc.cache)
}

// Clear removes all cached entries
func (imc *InMemoryQueryCache) Clear() {
	imc.mu.Lock()
	defer imc.mu.Unlock()

	imc.cache = make(map[string]*cacheEntry)
	imc.stats.Entries = 0
}

// GetStats returns cache statistics
func (imc *InMemoryQueryCache) GetStats() CacheStats {
	imc.mu.RLock()
	defer imc.mu.RUnlock()
	return imc.stats
}

func (imc *InMemoryQueryCache) updateHitRatio() {
	total := imc.stats.Hits + imc.stats.Misses
	if total > 0 {
		imc.stats.HitRatio = float64(imc.stats.Hits) / float64(total)
	}
}

// NewQueryBus creates a new query bus
func NewQueryBus(config QueryBusConfig, logger *logrus.Logger) *QueryBus {
	// Set default config values
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 100
	}
	if config.TimeoutDuration == 0 {
		config.TimeoutDuration = 30 * time.Second
	}

	var cache QueryCache
	if config.EnableCache {
		cache = NewInMemoryQueryCache()
	}

	return &QueryBus{
		handlers: make(map[string]QueryHandler),
		logger:   logger,
		config:   config,
		cache:    cache,
	}
}

// RegisterHandler registers a query handler
func (qb *QueryBus) RegisterHandler(handler QueryHandler) error {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	queryType := handler.GetQueryType()
	if queryType == "" {
		return fmt.Errorf("query type cannot be empty")
	}

	if _, exists := qb.handlers[queryType]; exists {
		return fmt.Errorf("handler for query type %s already registered", queryType)
	}

	qb.handlers[queryType] = handler

	qb.logger.WithFields(logrus.Fields{
		"query_type": queryType,
		"handler":    reflect.TypeOf(handler).String(),
	}).Info("Query handler registered successfully")

	return nil
}

// Execute executes a query
func (qb *QueryBus) Execute(ctx context.Context, query Query) (*QueryResult, error) {
	queryType := query.GetQueryType()
	if queryType == "" {
		return nil, fmt.Errorf("query type cannot be empty")
	}

	// Check cache first if enabled and query is cacheable
	if qb.config.EnableCache && qb.cache != nil && query.IsCacheable() {
		if cachedResult, found := qb.cache.Get(query.GetCacheKey()); found {
			qb.logger.WithFields(logrus.Fields{
				"query_type": queryType,
				"cache_key":  query.GetCacheKey(),
			}).Debug("Query result served from cache")
			return cachedResult, nil
		}
	}

	// Get handler
	qb.mu.RLock()
	handler, exists := qb.handlers[queryType]
	qb.mu.RUnlock()

	if !exists {
		qb.logger.WithFields(logrus.Fields{
			"query_type": queryType,
		}).Error("No handler found for query type")
		return nil, fmt.Errorf("no handler registered for query type: %s", queryType)
	}

	// Log query execution start
	qb.logger.WithFields(logrus.Fields{
		"query_type": queryType,
		"handler":    reflect.TypeOf(handler).String(),
	}).Debug("Executing query")

	// Execute query
	result, err := handler.Handle(ctx, query)
	if err != nil {
		qb.logger.WithFields(logrus.Fields{
			"query_type": queryType,
			"error":      err,
		}).Error("Query execution failed")
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	// Set timestamp
	result.Timestamp = time.Now()

	// Cache result if enabled and query is cacheable
	if qb.config.EnableCache && qb.cache != nil && query.IsCacheable() && result.Success {
		qb.cache.Set(query.GetCacheKey(), result, qb.config.CacheTTL)
		qb.logger.WithFields(logrus.Fields{
			"query_type": queryType,
			"cache_key":  query.GetCacheKey(),
			"cache_ttl":  qb.config.CacheTTL,
		}).Debug("Query result cached")
	}

	// Log successful execution
	qb.logger.WithFields(logrus.Fields{
		"query_type": queryType,
		"success":    result.Success,
		"from_cache": result.FromCache,
		"count":      result.Count,
	}).Info("Query executed successfully")

	return result, nil
}

// ExecuteAsync executes a query asynchronously
func (qb *QueryBus) ExecuteAsync(ctx context.Context, query Query) <-chan *AsyncQueryResult {
	resultChan := make(chan *AsyncQueryResult, 1)

	go func() {
		defer close(resultChan)

		result, err := qb.Execute(ctx, query)
		asyncResult := &AsyncQueryResult{
			Result: result,
			Error:  err,
		}

		select {
		case resultChan <- asyncResult:
		case <-ctx.Done():
			qb.logger.WithFields(logrus.Fields{
				"query_type": query.GetQueryType(),
			}).Warn("Async query execution cancelled")
		}
	}()

	return resultChan
}

// AsyncQueryResult represents the result of an async query execution
type AsyncQueryResult struct {
	Result *QueryResult
	Error  error
}

// GetRegisteredQueries returns a list of registered query types
func (qb *QueryBus) GetRegisteredQueries() []string {
	qb.mu.RLock()
	defer qb.mu.RUnlock()

	queries := make([]string, 0, len(qb.handlers))
	for queryType := range qb.handlers {
		queries = append(queries, queryType)
	}

	return queries
}

// UnregisterHandler removes a query handler
func (qb *QueryBus) UnregisterHandler(queryType string) error {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	if _, exists := qb.handlers[queryType]; !exists {
		return fmt.Errorf("no handler registered for query type: %s", queryType)
	}

	delete(qb.handlers, queryType)

	qb.logger.WithField("query_type", queryType).Info("Query handler unregistered")

	return nil
}

// InvalidateCache invalidates cached results for a specific pattern
func (qb *QueryBus) InvalidateCache(pattern string) {
	if qb.cache == nil {
		return
	}

	// For simplicity, clear all cache
	// In production, implement pattern-based invalidation
	qb.cache.Clear()

	qb.logger.WithField("pattern", pattern).Info("Cache invalidated")
}

// GetCacheStats returns cache statistics
func (qb *QueryBus) GetCacheStats() *CacheStats {
	if qb.cache == nil {
		return nil
	}

	stats := qb.cache.GetStats()
	return &stats
}

// GetHandlerCount returns the number of registered handlers
func (qb *QueryBus) GetHandlerCount() int {
	qb.mu.RLock()
	defer qb.mu.RUnlock()
	return len(qb.handlers)
}

// ValidateQuery validates a query before execution
func (qb *QueryBus) ValidateQuery(query Query) error {
	if query == nil {
		return fmt.Errorf("query cannot be nil")
	}

	if query.GetQueryType() == "" {
		return fmt.Errorf("query type cannot be empty")
	}

	return nil
}

// HealthCheck performs a health check on the query bus
func (qb *QueryBus) HealthCheck(ctx context.Context) error {
	qb.mu.RLock()
	handlerCount := len(qb.handlers)
	qb.mu.RUnlock()

	if handlerCount == 0 {
		return fmt.Errorf("no query handlers registered")
	}

	qb.logger.WithField("handler_count", handlerCount).Debug("Query bus health check passed")
	return nil
}

// Close closes the query bus
func (qb *QueryBus) Close() error {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	// Clear handlers
	qb.handlers = make(map[string]QueryHandler)

	// Clear cache
	if qb.cache != nil {
		qb.cache.Clear()
	}

	qb.logger.Info("Query bus closed")
	return nil
}

// Pagination support

// PaginatedQuery represents a query with pagination parameters
type PaginatedQuery struct {
	*BaseQuery
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	SortBy   string `json:"sort_by,omitempty"`
	SortDir  string `json:"sort_dir,omitempty"` // asc, desc
}

// PaginatedResult represents a paginated query result
type PaginatedResult struct {
	*QueryResult
	Page        int  `json:"page"`
	PageSize    int  `json:"page_size"`
	TotalCount  int  `json:"total_count"`
	TotalPages  int  `json:"total_pages"`
	HasNext     bool `json:"has_next"`
	HasPrevious bool `json:"has_previous"`
}

// NewPaginatedQuery creates a new paginated query
func NewPaginatedQuery(queryType string, page, pageSize int) *PaginatedQuery {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100 // Max page size limit
	}

	return &PaginatedQuery{
		BaseQuery: &BaseQuery{
			QueryType:  queryType,
			Parameters: make(map[string]interface{}),
			Cacheable:  true,
		},
		Page:     page,
		PageSize: pageSize,
		SortDir:  "asc",
	}
}

// CalculatePagination calculates pagination metadata
func CalculatePagination(page, pageSize, totalCount int) (int, bool, bool) {
	totalPages := (totalCount + pageSize - 1) / pageSize
	hasNext := page < totalPages
	hasPrevious := page > 1
	
	return totalPages, hasNext, hasPrevious
}