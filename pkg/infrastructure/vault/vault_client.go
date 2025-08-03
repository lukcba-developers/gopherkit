package vault

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
)

// VaultClient wraps HashiCorp Vault client with additional functionality
type VaultClient struct {
	client *api.Client
	logger *logrus.Logger
	config VaultConfig
}

// VaultConfig holds Vault client configuration
type VaultConfig struct {
	Address     string        `json:"address"`
	Token       string        `json:"token"`
	MountPath   string        `json:"mount_path"`
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"max_retries"`
	EnableTLS   bool          `json:"enable_tls"`
	TLSInsecure bool          `json:"tls_insecure"`
}

// SecretData represents a secret with metadata
type SecretData struct {
	Data        map[string]interface{} `json:"data"`
	Version     int                    `json:"version"`
	CreatedTime time.Time              `json:"created_time"`
	UpdatedTime time.Time              `json:"updated_time"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewVaultClient creates a new Vault client
func NewVaultClient(config VaultConfig, logger *logrus.Logger) (*VaultClient, error) {
	// Create Vault config
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = config.Address
	vaultConfig.Timeout = config.Timeout

	// Configure TLS
	if config.EnableTLS {
		tlsConfig := &api.TLSConfig{
			Insecure: config.TLSInsecure,
		}
		vaultConfig.ConfigureTLS(tlsConfig)
	}

	// Create client
	client, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Set token if provided
	if config.Token != "" {
		client.SetToken(config.Token)
	}

	// Set max retries
	client.SetMaxRetries(config.MaxRetries)

	vaultClient := &VaultClient{
		client: client,
		logger: logger,
		config: config,
	}

	// Test connection
	if err := vaultClient.HealthCheck(context.Background()); err != nil {
		logger.WithError(err).Warn("Vault health check failed during initialization")
	}

	logger.WithFields(logrus.Fields{
		"address":    config.Address,
		"mount_path": config.MountPath,
		"tls":        config.EnableTLS,
	}).Info("Vault client initialized successfully")

	return vaultClient, nil
}

// HealthCheck verifies Vault connection and status
func (vc *VaultClient) HealthCheck(ctx context.Context) error {
	health, err := vc.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}

	if health.Sealed {
		return fmt.Errorf("vault is sealed")
	}

	vc.logger.WithFields(logrus.Fields{
		"version":     health.Version,
		"cluster_id":  health.ClusterID,
		"cluster_name": health.ClusterName,
	}).Debug("Vault health check successful")

	return nil
}

// WriteSecret stores a secret at the given path
func (vc *VaultClient) WriteSecret(ctx context.Context, path string, data map[string]interface{}) error {
	secretPath := fmt.Sprintf("%s/data/%s", vc.config.MountPath, path)

	// Wrap data in required format for KV v2
	secretData := map[string]interface{}{
		"data": data,
	}

	_, err := vc.client.Logical().WriteWithContext(ctx, secretPath, secretData)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to write secret to Vault")
		return fmt.Errorf("failed to write secret to path %s: %w", path, err)
	}

	vc.logger.WithField("path", path).Info("Secret written to Vault successfully")
	return nil
}

// ReadSecret retrieves a secret from the given path
func (vc *VaultClient) ReadSecret(ctx context.Context, path string) (*SecretData, error) {
	secretPath := fmt.Sprintf("%s/data/%s", vc.config.MountPath, path)

	secret, err := vc.client.Logical().ReadWithContext(ctx, secretPath)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to read secret from Vault")
		return nil, fmt.Errorf("failed to read secret from path %s: %w", path, err)
	}

	if secret == nil {
		return nil, fmt.Errorf("secret not found at path %s", path)
	}

	// Parse secret data
	secretData := &SecretData{}

	if data, ok := secret.Data["data"].(map[string]interface{}); ok {
		secretData.Data = data
	}

	if metadata, ok := secret.Data["metadata"].(map[string]interface{}); ok {
		secretData.Metadata = metadata

		if version, ok := metadata["version"].(float64); ok {
			secretData.Version = int(version)
		}

		if createdTime, ok := metadata["created_time"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdTime); err == nil {
				secretData.CreatedTime = t
			}
		}

		if updatedTime, ok := metadata["updated_time"].(string); ok {
			if t, err := time.Parse(time.RFC3339, updatedTime); err == nil {
				secretData.UpdatedTime = t
			}
		}
	}

	vc.logger.WithFields(logrus.Fields{
		"path":    path,
		"version": secretData.Version,
	}).Debug("Secret read from Vault successfully")

	return secretData, nil
}

// DeleteSecret removes a secret at the given path
func (vc *VaultClient) DeleteSecret(ctx context.Context, path string) error {
	// Delete metadata (soft delete)
	metadataPath := fmt.Sprintf("%s/metadata/%s", vc.config.MountPath, path)

	_, err := vc.client.Logical().DeleteWithContext(ctx, metadataPath)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to delete secret from Vault")
		return fmt.Errorf("failed to delete secret at path %s: %w", path, err)
	}

	vc.logger.WithField("path", path).Info("Secret deleted from Vault successfully")
	return nil
}

// ListSecrets lists all secrets under a given path
func (vc *VaultClient) ListSecrets(ctx context.Context, path string) ([]string, error) {
	listPath := fmt.Sprintf("%s/metadata/%s", vc.config.MountPath, path)

	secret, err := vc.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to list secrets from Vault")
		return nil, fmt.Errorf("failed to list secrets at path %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	var secretList []string
	for _, key := range keys {
		if keyStr, ok := key.(string); ok {
			secretList = append(secretList, keyStr)
		}
	}

	vc.logger.WithFields(logrus.Fields{
		"path":  path,
		"count": len(secretList),
	}).Debug("Secrets listed from Vault successfully")

	return secretList, nil
}

// RotateSecret creates a new version of an existing secret
func (vc *VaultClient) RotateSecret(ctx context.Context, path string, newData map[string]interface{}) error {
	// First read the current secret to preserve any existing fields
	currentSecret, err := vc.ReadSecret(ctx, path)
	if err != nil {
		// If secret doesn't exist, create it
		return vc.WriteSecret(ctx, path, newData)
	}

	// Merge current data with new data
	mergedData := make(map[string]interface{})
	for k, v := range currentSecret.Data {
		mergedData[k] = v
	}
	for k, v := range newData {
		mergedData[k] = v
	}

	// Write the updated secret
	return vc.WriteSecret(ctx, path, mergedData)
}

// CreateToken creates a new Vault token with specified policies
func (vc *VaultClient) CreateToken(ctx context.Context, policies []string, ttl time.Duration) (*api.SecretAuth, error) {
	tokenRequest := &api.TokenCreateRequest{
		Policies: policies,
		TTL:      ttl.String(),
		Renewable: true,
	}

	secret, err := vc.client.Auth().Token().CreateWithContext(ctx, tokenRequest)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"policies": policies,
			"ttl":      ttl,
			"error":    err,
		}).Error("Failed to create Vault token")
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	vc.logger.WithFields(logrus.Fields{
		"policies":   policies,
		"ttl":        ttl,
		"token_accessor": secret.Auth.Accessor,
	}).Info("Vault token created successfully")

	return secret.Auth, nil
}

// RevokeToken revokes a Vault token
func (vc *VaultClient) RevokeToken(ctx context.Context, token string) error {
	err := vc.client.Auth().Token().RevokeTokenWithContext(ctx, token)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to revoke Vault token")
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	vc.logger.Info("Vault token revoked successfully")
	return nil
}

// RenewToken renews a Vault token
func (vc *VaultClient) RenewToken(ctx context.Context, token string, increment time.Duration) (*api.SecretAuth, error) {
	secret, err := vc.client.Auth().Token().RenewTokenWithContext(ctx, token, int(increment.Seconds()))
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"increment": increment,
			"error":     err,
		}).Error("Failed to renew Vault token")
		return nil, fmt.Errorf("failed to renew token: %w", err)
	}

	vc.logger.WithFields(logrus.Fields{
		"increment": increment,
		"new_ttl":   secret.Auth.LeaseDuration,
	}).Info("Vault token renewed successfully")

	return secret.Auth, nil
}

// GetTransitKey creates or retrieves a transit encryption key
func (vc *VaultClient) GetTransitKey(ctx context.Context, keyName string) error {
	keyPath := fmt.Sprintf("transit/keys/%s", keyName)

	// Try to read the key first
	_, err := vc.client.Logical().ReadWithContext(ctx, keyPath)
	if err == nil {
		// Key already exists
		return nil
	}

	// Create the key
	createPath := fmt.Sprintf("transit/keys/%s", keyName)
	_, err = vc.client.Logical().WriteWithContext(ctx, createPath, map[string]interface{}{
		"type": "aes256-gcm96",
	})
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"key_name": keyName,
			"error":    err,
		}).Error("Failed to create transit key")
		return fmt.Errorf("failed to create transit key %s: %w", keyName, err)
	}

	vc.logger.WithField("key_name", keyName).Info("Transit key created successfully")
	return nil
}

// Encrypt encrypts data using Vault's transit engine
func (vc *VaultClient) Encrypt(ctx context.Context, keyName string, plaintext []byte) (string, error) {
	encryptPath := fmt.Sprintf("transit/encrypt/%s", keyName)

	data := map[string]interface{}{
		"plaintext": plaintext,
	}

	secret, err := vc.client.Logical().WriteWithContext(ctx, encryptPath, data)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"key_name": keyName,
			"error":    err,
		}).Error("Failed to encrypt data with Vault")
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}

	ciphertext, ok := secret.Data["ciphertext"].(string)
	if !ok {
		return "", fmt.Errorf("invalid ciphertext format returned from Vault")
	}

	return ciphertext, nil
}

// Decrypt decrypts data using Vault's transit engine
func (vc *VaultClient) Decrypt(ctx context.Context, keyName string, ciphertext string) ([]byte, error) {
	decryptPath := fmt.Sprintf("transit/decrypt/%s", keyName)

	data := map[string]interface{}{
		"ciphertext": ciphertext,
	}

	secret, err := vc.client.Logical().WriteWithContext(ctx, decryptPath, data)
	if err != nil {
		vc.logger.WithFields(logrus.Fields{
			"key_name": keyName,
			"error":    err,
		}).Error("Failed to decrypt data with Vault")
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	plaintext, ok := secret.Data["plaintext"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid plaintext format returned from Vault")
	}

	return []byte(plaintext), nil
}

// Close closes the Vault client connection
func (vc *VaultClient) Close() error {
	// Vault client doesn't have explicit close method
	vc.logger.Info("Vault client connection closed")
	return nil
}

// GetClient returns the underlying Vault API client
func (vc *VaultClient) GetClient() *api.Client {
	return vc.client
}

// IsInitialized checks if Vault is initialized
func (vc *VaultClient) IsInitialized(ctx context.Context) (bool, error) {
	initStatus, err := vc.client.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check init status: %w", err)
	}
	return initStatus, nil
}

// IsSealed checks if Vault is sealed
func (vc *VaultClient) IsSealed(ctx context.Context) (bool, error) {
	sealStatus, err := vc.client.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check seal status: %w", err)
	}
	return sealStatus.Sealed, nil
}