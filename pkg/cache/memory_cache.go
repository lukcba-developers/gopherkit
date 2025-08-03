package cache

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// MemoryCacheItem representa un elemento en el cache de memoria
type MemoryCacheItem struct {
	Value      interface{}
	Expiration time.Time
	CreatedAt  time.Time
}

// IsExpired verifica si el elemento ha expirado
func (item *MemoryCacheItem) IsExpired() bool {
	if item.Expiration.IsZero() {
		return false
	}
	return time.Now().After(item.Expiration)
}

// MemoryCache implementa un cache en memoria con TTL
type MemoryCache struct {
	items    map[string]*MemoryCacheItem
	mutex    sync.RWMutex
	maxSize  int
	stats    *MemoryCacheStats
	stopChan chan struct{}
}

// MemoryCacheStats estadísticas del cache de memoria
type MemoryCacheStats struct {
	Hits        int64
	Misses      int64
	Sets        int64
	Deletes     int64
	Evictions   int64
	Size        int64
	mutex       sync.RWMutex
}

// NewMemoryCache crea un nuevo cache de memoria
func NewMemoryCache(maxSize int) *MemoryCache {
	cache := &MemoryCache{
		items:    make(map[string]*MemoryCacheItem),
		maxSize:  maxSize,
		stats:    &MemoryCacheStats{},
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Get obtiene un valor del cache de memoria
func (c *MemoryCache) Get(key string, dest interface{}) error {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		c.stats.recordMiss()
		return ErrCacheMiss
	}

	if item.IsExpired() {
		c.mutex.Lock()
		delete(c.items, key)
		c.mutex.Unlock()
		c.stats.recordMiss()
		return ErrCacheMiss
	}

	// Convert value using JSON marshaling/unmarshaling for type safety
	jsonData, err := json.Marshal(item.Value)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonData, dest); err != nil {
		return err
	}

	c.stats.recordHit()
	return nil
}

// Set guarda un valor en el cache de memoria
func (c *MemoryCache) Set(key string, value interface{}, expiration time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if we need to evict items
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	var expirationTime time.Time
	if expiration > 0 {
		expirationTime = time.Now().Add(expiration)
	}

	c.items[key] = &MemoryCacheItem{
		Value:      value,
		Expiration: expirationTime,
		CreatedAt:  time.Now(),
	}

	c.stats.recordSet()
	return nil
}

// Delete elimina una clave del cache de memoria
func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.items[key]; exists {
		delete(c.items, key)
		c.stats.recordDelete()
	}
}

// Exists verifica si una clave existe en el cache de memoria
func (c *MemoryCache) Exists(key string) bool {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		return false
	}

	if item.IsExpired() {
		c.mutex.Lock()
		delete(c.items, key)
		c.mutex.Unlock()
		return false
	}

	return true
}

// SetNX sets a key only if it doesn't exist
func (c *MemoryCache) SetNX(key string, value interface{}, expiration time.Duration) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if key exists and is not expired
	if item, exists := c.items[key]; exists && !item.IsExpired() {
		return false
	}

	// Set the value
	var expirationTime time.Time
	if expiration > 0 {
		expirationTime = time.Now().Add(expiration)
	}

	c.items[key] = &MemoryCacheItem{
		Value:      value,
		Expiration: expirationTime,
		CreatedAt:  time.Now(),
	}

	c.stats.recordSet()
	return true
}

// Increment atomically increments a numeric value
func (c *MemoryCache) Increment(key string, delta int64) int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, exists := c.items[key]
	if !exists || item.IsExpired() {
		// Create new item with delta as initial value
		c.items[key] = &MemoryCacheItem{
			Value:     delta,
			CreatedAt: time.Now(),
		}
		c.stats.recordSet()
		return delta
	}

	// Try to increment existing value
	switch v := item.Value.(type) {
	case int64:
		newValue := v + delta
		item.Value = newValue
		return newValue
	case int:
		newValue := int64(v) + delta
		item.Value = newValue
		return newValue
	case float64:
		newValue := int64(v) + delta
		item.Value = newValue
		return newValue
	default:
		// If value is not numeric, replace with delta
		item.Value = delta
		return delta
	}
}

// GetTTL obtiene el tiempo de vida restante de una clave
func (c *MemoryCache) GetTTL(key string) time.Duration {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		return 0
	}

	if item.Expiration.IsZero() {
		return -1 // No expiration
	}

	if item.IsExpired() {
		return 0
	}

	return time.Until(item.Expiration)
}

// InvalidatePattern elimina todas las claves que coinciden con un patrón
func (c *MemoryCache) InvalidatePattern(pattern string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	keysToDelete := make([]string, 0)
	
	for key := range c.items {
		if c.matchesPattern(key, pattern) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.items, key)
		c.stats.recordDelete()
	}
}

// matchesPattern verifica si una clave coincide con un patrón
func (c *MemoryCache) matchesPattern(key, pattern string) bool {
	// Simple pattern matching - supports * as wildcard
	if !strings.Contains(pattern, "*") {
		return key == pattern
	}

	// Replace * with regex equivalent and match
	// This is a simplified implementation
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(key, prefix)
	}

	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(key, suffix)
	}

	// For patterns with * in the middle, use basic contains check
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(key, parts[0]) && strings.HasSuffix(key, parts[1])
	}

	return false
}

// GetStats retorna estadísticas del cache
func (c *MemoryCache) GetStats() map[string]interface{} {
	c.stats.mutex.RLock()
	stats := map[string]interface{}{
		"hits":       c.stats.Hits,
		"misses":     c.stats.Misses,
		"sets":       c.stats.Sets,
		"deletes":    c.stats.Deletes,
		"evictions":  c.stats.Evictions,
		"size":       c.getCurrentSize(),
		"max_size":   c.maxSize,
		"hit_ratio":  c.getHitRatio(),
	}
	c.stats.mutex.RUnlock()

	return stats
}

// getCurrentSize retorna el tamaño actual del cache
func (c *MemoryCache) getCurrentSize() int {
	c.mutex.RLock()
	size := len(c.items)
	c.mutex.RUnlock()
	return size
}

// getHitRatio calcula el ratio de hits
func (c *MemoryCache) getHitRatio() float64 {
	totalRequests := c.stats.Hits + c.stats.Misses
	if totalRequests == 0 {
		return 0.0
	}
	return float64(c.stats.Hits) / float64(totalRequests)
}

// evictOldest elimina el elemento más antiguo
func (c *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
		c.stats.recordEviction()
	}
}

// cleanupExpired limpia elementos expirados periódicamente
func (c *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mutex.Lock()
			expiredKeys := make([]string, 0)
			
			for key, item := range c.items {
				if item.IsExpired() {
					expiredKeys = append(expiredKeys, key)
				}
			}

			for _, key := range expiredKeys {
				delete(c.items, key)
			}
			c.mutex.Unlock()

		case <-c.stopChan:
			return
		}
	}
}

// Close detiene el cache de memoria
func (c *MemoryCache) Close() {
	close(c.stopChan)
}

// Clear limpia todo el cache
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items = make(map[string]*MemoryCacheItem)
}

// Stats methods

func (s *MemoryCacheStats) recordHit() {
	s.mutex.Lock()
	s.Hits++
	s.mutex.Unlock()
}

func (s *MemoryCacheStats) recordMiss() {
	s.mutex.Lock()
	s.Misses++
	s.mutex.Unlock()
}

func (s *MemoryCacheStats) recordSet() {
	s.mutex.Lock()
	s.Sets++
	s.mutex.Unlock()
}

func (s *MemoryCacheStats) recordDelete() {
	s.mutex.Lock()
	s.Deletes++
	s.mutex.Unlock()
}

func (s *MemoryCacheStats) recordEviction() {
	s.mutex.Lock()
	s.Evictions++
	s.mutex.Unlock()
}