package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/infrastructure/vault"
)

// SecretsService manages application secrets using HashiCorp Vault
type SecretsService struct {
	vaultClient *vault.VaultClient
	logger      *logrus.Logger
	namespace   string
}

// SecretMetadata contains metadata about a secret
type SecretMetadata struct {
	Path        string                 `json:"path"`
	Version     int                    `json:"version"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Tags        map[string]string      `json:"tags"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// SecretType represents different types of secrets
type SecretType string

const (
	SecretTypeDatabase    SecretType = "database"
	SecretTypeAPI         SecretType = "api"
	SecretTypeJWT         SecretType = "jwt"
	SecretTypeEncryption  SecretType = "encryption"
	SecretTypeOAuth       SecretType = "oauth"
	SecretTypeWebhook     SecretType = "webhook"
	SecretTypeEmail       SecretType = "email"
	SecretTypePayment     SecretType = "payment"
	SecretTypeCustom      SecretType = "custom"
)

// SecretRequest represents a request to create or update a secret
type SecretRequest struct {
	Name        string            `json:"name" validate:"required"`
	Type        SecretType        `json:"type" validate:"required"`
	Data        map[string]string `json:"data" validate:"required"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
	TTL         time.Duration     `json:"ttl"`
}

// SecretResponse represents a secret response
type SecretResponse struct {
	Name        string            `json:"name"`
	Type        SecretType        `json:"type"`
	Data        map[string]string `json:"data,omitempty"`
	Metadata    SecretMetadata    `json:"metadata"`
	HasData     bool              `json:"has_data"`
}

// NewSecretsService creates a new secrets service
func NewSecretsService(vaultClient *vault.VaultClient, namespace string, logger *logrus.Logger) *SecretsService {
	return &SecretsService{
		vaultClient: vaultClient,
		logger:      logger,
		namespace:   namespace,
	}
}

// CreateSecret creates a new secret
func (ss *SecretsService) CreateSecret(ctx context.Context, req SecretRequest) (*SecretResponse, error) {
	if err := ss.validateSecretRequest(req); err != nil {
		return nil, fmt.Errorf("invalid secret request: %w", err)
	}

	secretPath := ss.buildSecretPath(req.Type, req.Name)

	// Check if secret already exists
	existing, err := ss.vaultClient.ReadSecret(ctx, secretPath)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("secret '%s' already exists", req.Name)
	}

	// Prepare secret data
	secretData := make(map[string]interface{})
	for k, v := range req.Data {
		secretData[k] = v
	}

	// Add metadata
	secretData["_type"] = string(req.Type)
	secretData["_description"] = req.Description
	secretData["_created_by"] = "secrets_service"
	secretData["_created_at"] = time.Now().Format(time.RFC3339)

	// Add tags as metadata
	if len(req.Tags) > 0 {
		for k, v := range req.Tags {
			secretData["_tag_"+k] = v
		}
	}

	// Write secret to Vault
	if err := ss.vaultClient.WriteSecret(ctx, secretPath, secretData); err != nil {
		ss.logger.WithFields(logrus.Fields{
			"name":  req.Name,
			"type":  req.Type,
			"error": err,
		}).Error("Failed to create secret")
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	ss.logger.WithFields(logrus.Fields{
		"name": req.Name,
		"type": req.Type,
		"path": secretPath,
	}).Info("Secret created successfully")

	// Return response without sensitive data
	return &SecretResponse{
		Name: req.Name,
		Type: req.Type,
		Metadata: SecretMetadata{
			Path:        secretPath,
			Description: req.Description,
			Tags:        req.Tags,
			CreatedAt:   time.Now(),
		},
		HasData: true,
	}, nil
}

// GetSecret retrieves a secret
func (ss *SecretsService) GetSecret(ctx context.Context, secretType SecretType, name string) (*SecretResponse, error) {
	secretPath := ss.buildSecretPath(secretType, name)

	secretData, err := ss.vaultClient.ReadSecret(ctx, secretPath)
	if err != nil {
		ss.logger.WithFields(logrus.Fields{
			"name":  name,
			"type":  secretType,
			"error": err,
		}).Error("Failed to read secret")
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	// Parse secret data
	response := &SecretResponse{
		Name: name,
		Type: secretType,
		Data: make(map[string]string),
		Metadata: SecretMetadata{
			Path:      secretPath,
			Version:   secretData.Version,
			CreatedAt: secretData.CreatedTime,
			UpdatedAt: secretData.UpdatedTime,
			Tags:      make(map[string]string),
			Metadata:  secretData.Metadata,
		},
		HasData: true,
	}

	// Extract data and metadata
	for k, v := range secretData.Data {
		if strings.HasPrefix(k, "_tag_") {
			tagKey := strings.TrimPrefix(k, "_tag_")
			if tagValue, ok := v.(string); ok {
				response.Metadata.Tags[tagKey] = tagValue
			}
		} else if k == "_description" {
			if desc, ok := v.(string); ok {
				response.Metadata.Description = desc
			}
		} else if !strings.HasPrefix(k, "_") {
			// Regular secret data
			if value, ok := v.(string); ok {
				response.Data[k] = value
			}
		}
	}

	ss.logger.WithFields(logrus.Fields{
		"name":    name,
		"type":    secretType,
		"version": secretData.Version,
	}).Debug("Secret retrieved successfully")

	return response, nil
}

// UpdateSecret updates an existing secret
func (ss *SecretsService) UpdateSecret(ctx context.Context, secretType SecretType, name string, data map[string]string) (*SecretResponse, error) {
	secretPath := ss.buildSecretPath(secretType, name)

	// Read current secret to preserve metadata
	currentSecret, err := ss.vaultClient.ReadSecret(ctx, secretPath)
	if err != nil {
		return nil, fmt.Errorf("secret not found: %w", err)
	}

	// Prepare updated secret data
	secretData := make(map[string]interface{})

	// Preserve metadata
	for k, v := range currentSecret.Data {
		if strings.HasPrefix(k, "_") {
			secretData[k] = v
		}
	}

	// Add new data
	for k, v := range data {
		secretData[k] = v
	}

	// Update timestamps
	secretData["_updated_at"] = time.Now().Format(time.RFC3339)

	// Write updated secret
	if err := ss.vaultClient.WriteSecret(ctx, secretPath, secretData); err != nil {
		ss.logger.WithFields(logrus.Fields{
			"name":  name,
			"type":  secretType,
			"error": err,
		}).Error("Failed to update secret")
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	ss.logger.WithFields(logrus.Fields{
		"name": name,
		"type": secretType,
		"path": secretPath,
	}).Info("Secret updated successfully")

	// Return updated secret (without sensitive data)
	return ss.GetSecretMetadata(ctx, secretType, name)
}

// DeleteSecret deletes a secret
func (ss *SecretsService) DeleteSecret(ctx context.Context, secretType SecretType, name string) error {
	secretPath := ss.buildSecretPath(secretType, name)

	if err := ss.vaultClient.DeleteSecret(ctx, secretPath); err != nil {
		ss.logger.WithFields(logrus.Fields{
			"name":  name,
			"type":  secretType,
			"error": err,
		}).Error("Failed to delete secret")
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	ss.logger.WithFields(logrus.Fields{
		"name": name,
		"type": secretType,
		"path": secretPath,
	}).Info("Secret deleted successfully")

	return nil
}

// ListSecrets lists all secrets of a given type
func (ss *SecretsService) ListSecrets(ctx context.Context, secretType SecretType) ([]SecretResponse, error) {
	typePath := ss.buildTypePath(secretType)

	secretNames, err := ss.vaultClient.ListSecrets(ctx, typePath)
	if err != nil {
		ss.logger.WithFields(logrus.Fields{
			"type":  secretType,
			"error": err,
		}).Error("Failed to list secrets")
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var secrets []SecretResponse
	for _, name := range secretNames {
		// Remove trailing slash if present
		name = strings.TrimSuffix(name, "/")
		
		metadata, err := ss.GetSecretMetadata(ctx, secretType, name)
		if err != nil {
			ss.logger.WithFields(logrus.Fields{
				"name":  name,
				"type":  secretType,
				"error": err,
			}).Warn("Failed to get secret metadata, skipping")
			continue
		}
		secrets = append(secrets, *metadata)
	}

	ss.logger.WithFields(logrus.Fields{
		"type":  secretType,
		"count": len(secrets),
	}).Debug("Secrets listed successfully")

	return secrets, nil
}

// GetSecretMetadata retrieves only metadata about a secret (no sensitive data)
func (ss *SecretsService) GetSecretMetadata(ctx context.Context, secretType SecretType, name string) (*SecretResponse, error) {
	secretPath := ss.buildSecretPath(secretType, name)

	secretData, err := ss.vaultClient.ReadSecret(ctx, secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret metadata: %w", err)
	}

	response := &SecretResponse{
		Name: name,
		Type: secretType,
		Metadata: SecretMetadata{
			Path:      secretPath,
			Version:   secretData.Version,
			CreatedAt: secretData.CreatedTime,
			UpdatedAt: secretData.UpdatedTime,
			Tags:      make(map[string]string),
			Metadata:  secretData.Metadata,
		},
		HasData: len(secretData.Data) > 0,
	}

	// Extract metadata only
	for k, v := range secretData.Data {
		if strings.HasPrefix(k, "_tag_") {
			tagKey := strings.TrimPrefix(k, "_tag_")
			if tagValue, ok := v.(string); ok {
				response.Metadata.Tags[tagKey] = tagValue
			}
		} else if k == "_description" {
			if desc, ok := v.(string); ok {
				response.Metadata.Description = desc
			}
		}
	}

	return response, nil
}

// RotateSecret creates a new version of a secret with new random data
func (ss *SecretsService) RotateSecret(ctx context.Context, secretType SecretType, name string) (*SecretResponse, error) {
	// Generate new random data based on secret type
	newData, err := ss.generateSecretData(secretType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new secret data: %w", err)
	}

	// Update the secret
	return ss.UpdateSecret(ctx, secretType, name, newData)
}

// GenerateAPIKey generates a new API key
func (ss *SecretsService) GenerateAPIKey(ctx context.Context, name string, keyLength int) (*SecretResponse, error) {
	if keyLength <= 0 {
		keyLength = 32
	}

	apiKey, err := ss.generateRandomString(keyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	req := SecretRequest{
		Name: name,
		Type: SecretTypeAPI,
		Data: map[string]string{
			"api_key": apiKey,
		},
		Description: "Generated API key",
	}

	return ss.CreateSecret(ctx, req)
}

// GenerateJWTSecret generates a JWT signing secret
func (ss *SecretsService) GenerateJWTSecret(ctx context.Context, name string) (*SecretResponse, error) {
	secret, err := ss.generateRandomString(64)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	req := SecretRequest{
		Name: name,
		Type: SecretTypeJWT,
		Data: map[string]string{
			"signing_key": secret,
		},
		Description: "JWT signing secret",
	}

	return ss.CreateSecret(ctx, req)
}

// GenerateEncryptionKey generates an encryption key
func (ss *SecretsService) GenerateEncryptionKey(ctx context.Context, name string) (*SecretResponse, error) {
	key, err := ss.generateRandomBytes(32) // 256-bit key
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	req := SecretRequest{
		Name: name,
		Type: SecretTypeEncryption,
		Data: map[string]string{
			"encryption_key": base64.StdEncoding.EncodeToString(key),
		},
		Description: "AES-256 encryption key",
	}

	return ss.CreateSecret(ctx, req)
}

// Private helper methods

func (ss *SecretsService) buildSecretPath(secretType SecretType, name string) string {
	return fmt.Sprintf("%s/%s/%s", ss.namespace, secretType, name)
}

func (ss *SecretsService) buildTypePath(secretType SecretType) string {
	return fmt.Sprintf("%s/%s", ss.namespace, secretType)
}

func (ss *SecretsService) validateSecretRequest(req SecretRequest) error {
	if req.Name == "" {
		return fmt.Errorf("secret name is required")
	}

	if req.Type == "" {
		return fmt.Errorf("secret type is required")
	}

	if len(req.Data) == 0 {
		return fmt.Errorf("secret data is required")
	}

	// Validate secret name (alphanumeric and hyphens only)
	for _, char := range req.Name {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return fmt.Errorf("secret name can only contain alphanumeric characters, hyphens, and underscores")
		}
	}

	return nil
}

func (ss *SecretsService) generateSecretData(secretType SecretType) (map[string]string, error) {
	switch secretType {
	case SecretTypeAPI:
		key, err := ss.generateRandomString(32)
		if err != nil {
			return nil, err
		}
		return map[string]string{"api_key": key}, nil

	case SecretTypeJWT:
		key, err := ss.generateRandomString(64)
		if err != nil {
			return nil, err
		}
		return map[string]string{"signing_key": key}, nil

	case SecretTypeEncryption:
		key, err := ss.generateRandomBytes(32)
		if err != nil {
			return nil, err
		}
		return map[string]string{"encryption_key": base64.StdEncoding.EncodeToString(key)}, nil

	case SecretTypeWebhook:
		secret, err := ss.generateRandomString(48)
		if err != nil {
			return nil, err
		}
		return map[string]string{"webhook_secret": secret}, nil

	default:
		// For custom and other types, generate a generic secret
		secret, err := ss.generateRandomString(32)
		if err != nil {
			return nil, err
		}
		return map[string]string{"secret": secret}, nil
	}
}

func (ss *SecretsService) generateRandomString(length int) (string, error) {
	bytes, err := ss.generateRandomBytes(length)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (ss *SecretsService) generateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}

// GetVaultClient returns the underlying Vault client (for advanced operations)
func (ss *SecretsService) GetVaultClient() *vault.VaultClient {
	return ss.vaultClient
}

// HealthCheck verifies the secrets service is healthy
func (ss *SecretsService) HealthCheck(ctx context.Context) error {
	return ss.vaultClient.HealthCheck(ctx)
}