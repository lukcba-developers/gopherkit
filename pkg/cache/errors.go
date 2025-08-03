package cache

import "errors"

// Cache errors
var (
	// ErrCacheMiss indicates that a key was not found in the cache
	ErrCacheMiss = errors.New("cache miss")
	
	// ErrCacheConnectionFailed indicates that the cache connection failed
	ErrCacheConnectionFailed = errors.New("cache connection failed")
	
	// ErrInvalidCacheKey indicates that the provided cache key is invalid
	ErrInvalidCacheKey = errors.New("invalid cache key")
	
	// ErrCacheFull indicates that the cache is full and cannot accept more items
	ErrCacheFull = errors.New("cache is full")
	
	// ErrSerializationFailed indicates that serialization of the value failed
	ErrSerializationFailed = errors.New("serialization failed")
	
	// ErrDeserializationFailed indicates that deserialization of the value failed
	ErrDeserializationFailed = errors.New("deserialization failed")
	
	// ErrCacheTimeout indicates that a cache operation timed out
	ErrCacheTimeout = errors.New("cache operation timeout")
)