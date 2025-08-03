package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"image/png"
	"io"
	"net/url"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/sirupsen/logrus"
)

// TOTPService handles Time-based One-Time Password operations
type TOTPService struct {
	logger *logrus.Logger
	issuer string
}

// TOTPConfig holds TOTP configuration
type TOTPConfig struct {
	Secret      string    `json:"secret"`
	URL         string    `json:"url"`
	QRCode      []byte    `json:"qr_code,omitempty"`
	BackupCodes []string  `json:"backup_codes"`
	CreatedAt   time.Time `json:"created_at"`
	IsVerified  bool      `json:"is_verified"`
}

// BackupCode represents a backup recovery code
type BackupCode struct {
	Code      string    `json:"code"`
	Used      bool      `json:"used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NewTOTPService creates a new TOTP service
func NewTOTPService(issuer string, logger *logrus.Logger) *TOTPService {
	return &TOTPService{
		logger: logger,
		issuer: issuer,
	}
}

// GenerateSecret generates a new TOTP secret for a user
func (ts *TOTPService) GenerateSecret(userEmail, accountName string) (*TOTPConfig, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      ts.issuer,
		AccountName: userEmail,
		SecretSize:  32,
	})
	if err != nil {
		ts.logger.WithFields(logrus.Fields{
			"user_email": userEmail,
			"error":      err,
		}).Error("Failed to generate TOTP secret")
		return nil, fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	// Generate backup codes
	backupCodes, err := ts.generateBackupCodes(10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Generate QR code
	qrCode, err := ts.generateQRCode(key.URL())
	if err != nil {
		ts.logger.WithFields(logrus.Fields{
			"user_email": userEmail,
			"error":      err,
		}).Warn("Failed to generate QR code, continuing without it")
		// Continue without QR code - not critical
	}

	config := &TOTPConfig{
		Secret:      key.Secret(),
		URL:         key.URL(),
		QRCode:      qrCode,
		BackupCodes: backupCodes,
		CreatedAt:   time.Now(),
		IsVerified:  false,
	}

	ts.logger.WithFields(logrus.Fields{
		"user_email": userEmail,
		"issuer":     ts.issuer,
	}).Info("TOTP secret generated successfully")

	return config, nil
}

// ValidateToken validates a TOTP token
func (ts *TOTPService) ValidateToken(secret, token string) bool {
	valid := totp.Validate(token, secret)
	
	ts.logger.WithFields(logrus.Fields{
		"token_valid": valid,
		"token_length": len(token),
	}).Debug("TOTP token validation attempted")

	return valid
}

// ValidateTokenWithWindow validates a TOTP token with a time window
func (ts *TOTPService) ValidateTokenWithWindow(secret, token string, windowSize uint) bool {
	now := time.Now()
	
	// Check current time and previous/next windows
	for i := -int(windowSize); i <= int(windowSize); i++ {
		checkTime := now.Add(time.Duration(i) * 30 * time.Second)
		expectedToken, err := totp.GenerateCode(secret, checkTime)
		if err != nil {
			continue
		}
		
		if expectedToken == token {
			ts.logger.WithFields(logrus.Fields{
				"window_offset": i,
				"token_valid":   true,
			}).Debug("TOTP token validated with time window")
			return true
		}
	}

	ts.logger.Debug("TOTP token validation failed with time window")
	return false
}

// GenerateCurrentToken generates the current TOTP token for testing
func (ts *TOTPService) GenerateCurrentToken(secret string) (string, error) {
	token, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		ts.logger.WithError(err).Error("Failed to generate current TOTP token")
		return "", fmt.Errorf("failed to generate current token: %w", err)
	}
	return token, nil
}

// ValidateSetupToken validates a token during initial setup
func (ts *TOTPService) ValidateSetupToken(secret, token string) bool {
	// Use a larger window during setup to account for clock drift
	return ts.ValidateTokenWithWindow(secret, token, 2)
}

// generateBackupCodes generates recovery backup codes
func (ts *TOTPService) generateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	
	for i := 0; i < count; i++ {
		code, err := ts.generateBackupCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate backup code %d: %w", i, err)
		}
		codes[i] = code
	}

	ts.logger.WithField("count", count).Debug("Backup codes generated successfully")
	return codes, nil
}

// generateBackupCode generates a single backup code
func (ts *TOTPService) generateBackupCode() (string, error) {
	// Generate 8 random bytes
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert to base32 and format
	code := base32.StdEncoding.EncodeToString(bytes)
	
	// Format as XXXX-XXXX for better readability
	if len(code) >= 8 {
		formatted := code[:4] + "-" + code[4:8]
		return formatted, nil
	}
	
	return code, nil
}

// ValidateBackupCode validates a backup recovery code
func (ts *TOTPService) ValidateBackupCode(backupCodes []BackupCode, inputCode string) (bool, int) {
	normalizedInput := ts.normalizeBackupCode(inputCode)
	
	for i, code := range backupCodes {
		if !code.Used && ts.normalizeBackupCode(code.Code) == normalizedInput {
			ts.logger.WithFields(logrus.Fields{
				"code_index": i,
				"code_valid": true,
			}).Info("Backup code validated successfully")
			return true, i
		}
	}

	ts.logger.WithField("input_code", inputCode).Warn("Invalid backup code provided")
	return false, -1
}

// normalizeBackupCode removes hyphens and converts to uppercase
func (ts *TOTPService) normalizeBackupCode(code string) string {
	// Remove hyphens and convert to uppercase
	normalized := ""
	for _, char := range code {
		if char != '-' {
			normalized += string(char)
		}
	}
	return normalized
}

// generateQRCode generates a QR code image for the TOTP URL
func (ts *TOTPService) generateQRCode(url string) ([]byte, error) {
	// Parse the TOTP URL
	totpURL, err := otp.NewKeyFromURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TOTP URL: %w", err)
	}

	// Generate QR code
	img, err := totpURL.Image(256, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code image: %w", err)
	}

	// Encode as PNG
	var buf []byte
	writer := &byteWriter{data: &buf}
	err = png.Encode(writer, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QR code as PNG: %w", err)
	}

	return buf, nil
}

// byteWriter implements io.Writer for byte slice
type byteWriter struct {
	data *[]byte
}

func (bw *byteWriter) Write(p []byte) (n int, err error) {
	*bw.data = append(*bw.data, p...)
	return len(p), nil
}

// VerifySecretFormat validates that a secret is properly formatted
func (ts *TOTPService) VerifySecretFormat(secret string) error {
	// Decode base32 secret to verify format
	_, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return fmt.Errorf("invalid secret format: %w", err)
	}

	// Check minimum length (should be at least 16 characters for good security)
	if len(secret) < 16 {
		return fmt.Errorf("secret too short: minimum 16 characters required")
	}

	return nil
}

// GetTOTPURL constructs a TOTP URL for manual entry
func (ts *TOTPService) GetTOTPURL(secret, userEmail, accountName string) string {
	params := url.Values{}
	params.Add("secret", secret)
	params.Add("issuer", ts.issuer)
	
	if accountName != "" {
		params.Add("algorithm", "SHA1")
		params.Add("digits", "6")
		params.Add("period", "30")
	}

	totpURL := fmt.Sprintf("otpauth://totp/%s:%s?%s", 
		url.QueryEscape(ts.issuer),
		url.QueryEscape(userEmail),
		params.Encode(),
	)

	return totpURL
}

// GenerateRecoveryCodes generates new backup codes (for rotation)
func (ts *TOTPService) GenerateRecoveryCodes(count int) ([]BackupCode, error) {
	codes := make([]BackupCode, count)
	now := time.Now()
	
	for i := 0; i < count; i++ {
		codeStr, err := ts.generateBackupCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate recovery code %d: %w", i, err)
		}
		
		codes[i] = BackupCode{
			Code:      codeStr,
			Used:      false,
			CreatedAt: now,
		}
	}

	ts.logger.WithField("count", count).Info("Recovery codes generated successfully")
	return codes, nil
}

// MarkBackupCodeUsed marks a backup code as used
func (ts *TOTPService) MarkBackupCodeUsed(backupCodes []BackupCode, codeIndex int) error {
	if codeIndex < 0 || codeIndex >= len(backupCodes) {
		return fmt.Errorf("invalid backup code index: %d", codeIndex)
	}

	now := time.Now()
	backupCodes[codeIndex].Used = true
	backupCodes[codeIndex].UsedAt = &now

	ts.logger.WithFields(logrus.Fields{
		"code_index": codeIndex,
		"used_at":    now,
	}).Info("Backup code marked as used")

	return nil
}

// GetRemainingBackupCodes returns the count of unused backup codes
func (ts *TOTPService) GetRemainingBackupCodes(backupCodes []BackupCode) int {
	remaining := 0
	for _, code := range backupCodes {
		if !code.Used {
			remaining++
		}
	}
	return remaining
}

// Close closes the TOTP service
func (ts *TOTPService) Close() error {
	ts.logger.Info("TOTP service closed")
	return nil
}