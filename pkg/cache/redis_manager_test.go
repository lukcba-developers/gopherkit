package cache

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedisManager_Encrypt_Decrypt(t *testing.T) {
	t.Run("encryption disabled", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: false,
		}
		
		rm := &RedisManager{config: cfg}
		
		testData := []byte("test data")
		
		// With encryption disabled, data should pass through unchanged
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		assert.Equal(t, testData, encrypted)
		
		decrypted, err := rm.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, testData, decrypted)
	})

	t.Run("encryption enabled with valid key", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "test-encryption-key-32-bytes!!",
		}
		
		rm := &RedisManager{config: cfg}
		
		testData := []byte("sensitive test data")
		
		// Encrypt the data
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		assert.NotEqual(t, testData, encrypted) // Should be different
		assert.Greater(t, len(encrypted), len(testData)) // Should be longer due to nonce
		
		// Decrypt the data
		decrypted, err := rm.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, testData, decrypted) // Should match original
	})

	t.Run("encryption enabled with short key", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "short", // Will be padded to 32 bytes
		}
		
		rm := &RedisManager{config: cfg}
		
		testData := []byte("test data with short key")
		
		// Should still work with key padding
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		
		decrypted, err := rm.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, testData, decrypted)
	})

	t.Run("decrypt with invalid data", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "test-encryption-key-32-bytes!!",
		}
		
		rm := &RedisManager{config: cfg}
		
		// Try to decrypt data that's too short
		invalidData := []byte("short")
		_, err := rm.decrypt(invalidData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ciphertext too short")
	})

	t.Run("decrypt with corrupted data", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "test-encryption-key-32-bytes!!",
		}
		
		rm := &RedisManager{config: cfg}
		
		// Create valid encrypted data first
		testData := []byte("test data")
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		
		// Corrupt the encrypted data
		corrupted := make([]byte, len(encrypted))
		copy(corrupted, encrypted)
		corrupted[len(corrupted)-1] ^= 0xFF // Flip last byte
		
		// Decryption should fail
		_, err = rm.decrypt(corrupted)
		assert.Error(t, err)
	})

	t.Run("encryption with empty key", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "", // Empty key should disable encryption
		}
		
		rm := &RedisManager{config: cfg}
		
		testData := []byte("test data")
		
		// Should pass through unchanged when encryption key is empty
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		assert.Equal(t, testData, encrypted)
		
		decrypted, err := rm.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, testData, decrypted)
	})

	t.Run("encryption nil config", func(t *testing.T) {
		rm := &RedisManager{config: &Config{}} // No encryption config
		
		testData := []byte("test data")
		
		// Should pass through unchanged when encryption is disabled
		encrypted, err := rm.encrypt(testData)
		assert.NoError(t, err)
		assert.Equal(t, testData, encrypted)
		
		decrypted, err := rm.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, testData, decrypted)
	})

	t.Run("round trip with different data sizes", func(t *testing.T) {
		cfg := &Config{
			EnableEncryption: true,
			EncryptionKey:    "test-encryption-key-32-bytes!!",
		}
		
		rm := &RedisManager{config: cfg}
		
		testCases := [][]byte{
			[]byte(""), // Empty data
			[]byte("a"), // Single byte
			[]byte("small data"),
			[]byte("medium length test data with some content"),
			make([]byte, 1024), // Large data (1KB of zeros)
		}
		
		for i, testData := range testCases {
			t.Run(fmt.Sprintf("case_%d_len_%d", i, len(testData)), func(t *testing.T) {
				encrypted, err := rm.encrypt(testData)
				assert.NoError(t, err)
				
				decrypted, err := rm.decrypt(encrypted)
				assert.NoError(t, err)
				assert.Equal(t, testData, decrypted)
			})
		}
	})
}