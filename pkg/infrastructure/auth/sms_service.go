package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/sirupsen/logrus"
)

// SMSService handles SMS-based two-factor authentication
type SMSService struct {
	logger   *logrus.Logger
	provider SMSProvider
	config   SMSConfig
}

// SMSConfig holds SMS service configuration
type SMSConfig struct {
	CodeLength     int           `json:"code_length"`
	CodeTTL        time.Duration `json:"code_ttl"`
	MaxAttempts    int           `json:"max_attempts"`
	RateLimit      time.Duration `json:"rate_limit"`
	DefaultMessage string        `json:"default_message"`
}

// SMSProvider interface for different SMS providers
type SMSProvider interface {
	SendSMS(phoneNumber, message string) error
	GetProviderName() string
	IsHealthy() bool
}

// SMSCode represents an SMS verification code
type SMSCode struct {
	Code        string    `json:"code"`
	PhoneNumber string    `json:"phone_number"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Attempts    int       `json:"attempts"`
	IsUsed      bool      `json:"is_used"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
}

// TwilioProvider implements SMS provider using Twilio
type TwilioProvider struct {
	accountSID string
	authToken  string
	fromNumber string
	logger     *logrus.Logger
}

// AWSProvider implements SMS provider using AWS SNS
type AWSProvider struct {
	region    string
	accessKey string
	secretKey string
	logger    *logrus.Logger
}

// MockProvider implements a mock SMS provider for testing
type MockProvider struct {
	logger     *logrus.Logger
	shouldFail bool
	sentCodes  []string
}

// NewSMSService creates a new SMS service
func NewSMSService(provider SMSProvider, config SMSConfig, logger *logrus.Logger) *SMSService {
	// Set default config values if not provided
	if config.CodeLength == 0 {
		config.CodeLength = 6
	}
	if config.CodeTTL == 0 {
		config.CodeTTL = 5 * time.Minute
	}
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 3
	}
	if config.RateLimit == 0 {
		config.RateLimit = 1 * time.Minute
	}
	if config.DefaultMessage == "" {
		config.DefaultMessage = "Your ClubPulse verification code is: %s. This code expires in 5 minutes."
	}

	return &SMSService{
		logger:   logger,
		provider: provider,
		config:   config,
	}
}

// SendVerificationCode sends an SMS verification code
func (sms *SMSService) SendVerificationCode(phoneNumber string) (*SMSCode, error) {
	// Generate verification code
	code, err := sms.generateVerificationCode()
	if err != nil {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": phoneNumber,
			"error":        err,
		}).Error("Failed to generate verification code")
		return nil, fmt.Errorf("failed to generate verification code: %w", err)
	}

	// Create SMS code record
	now := time.Now()
	smsCode := &SMSCode{
		Code:        code,
		PhoneNumber: phoneNumber,
		CreatedAt:   now,
		ExpiresAt:   now.Add(sms.config.CodeTTL),
		Attempts:    0,
		IsUsed:      false,
	}

	// Format message
	message := fmt.Sprintf(sms.config.DefaultMessage, code)

	// Send SMS
	if err := sms.provider.SendSMS(phoneNumber, message); err != nil {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": phoneNumber,
			"provider":     sms.provider.GetProviderName(),
			"error":        err,
		}).Error("Failed to send SMS")
		return nil, fmt.Errorf("failed to send SMS: %w", err)
	}

	sms.logger.WithFields(logrus.Fields{
		"phone_number": phoneNumber,
		"provider":     sms.provider.GetProviderName(),
		"code_length":  len(code),
		"expires_at":   smsCode.ExpiresAt,
	}).Info("SMS verification code sent successfully")

	return smsCode, nil
}

// VerifyCode verifies an SMS verification code
func (sms *SMSService) VerifyCode(smsCode *SMSCode, inputCode string) (bool, error) {
	now := time.Now()

	// Check if code is already used
	if smsCode.IsUsed {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": smsCode.PhoneNumber,
			"used_at":      smsCode.UsedAt,
		}).Warn("Attempted to verify already used SMS code")
		return false, fmt.Errorf("code has already been used")
	}

	// Check if code is expired
	if now.After(smsCode.ExpiresAt) {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": smsCode.PhoneNumber,
			"expired_at":   smsCode.ExpiresAt,
		}).Warn("Attempted to verify expired SMS code")
		return false, fmt.Errorf("code has expired")
	}

	// Increment attempts
	smsCode.Attempts++

	// Check max attempts
	if smsCode.Attempts > sms.config.MaxAttempts {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": smsCode.PhoneNumber,
			"attempts":     smsCode.Attempts,
			"max_attempts": sms.config.MaxAttempts,
		}).Warn("SMS code verification exceeded max attempts")
		return false, fmt.Errorf("maximum verification attempts exceeded")
	}

	// Verify code
	if smsCode.Code != inputCode {
		sms.logger.WithFields(logrus.Fields{
			"phone_number": smsCode.PhoneNumber,
			"attempts":     smsCode.Attempts,
		}).Warn("SMS code verification failed - invalid code")
		return false, nil // Don't reveal that the code is wrong
	}

	// Mark code as used
	smsCode.IsUsed = true
	smsCode.UsedAt = &now

	sms.logger.WithFields(logrus.Fields{
		"phone_number": smsCode.PhoneNumber,
		"attempts":     smsCode.Attempts,
	}).Info("SMS code verified successfully")

	return true, nil
}

// generateVerificationCode generates a random numeric verification code
func (sms *SMSService) generateVerificationCode() (string, error) {
	// Calculate the maximum value for the code length
	max := big.NewInt(1)
	for i := 0; i < sms.config.CodeLength; i++ {
		max.Mul(max, big.NewInt(10))
	}

	// Generate random number
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate random number: %w", err)
	}

	// Format with leading zeros
	format := fmt.Sprintf("%%0%dd", sms.config.CodeLength)
	code := fmt.Sprintf(format, n.Int64())

	return code, nil
}

// ValidatePhoneNumber validates a phone number format
func (sms *SMSService) ValidatePhoneNumber(phoneNumber string) error {
	// Basic validation - check if it starts with + and contains only digits
	if len(phoneNumber) < 10 {
		return fmt.Errorf("phone number too short")
	}

	if phoneNumber[0] != '+' {
		return fmt.Errorf("phone number must start with country code (+)")
	}

	// Check if the rest are digits
	for i := 1; i < len(phoneNumber); i++ {
		if phoneNumber[i] < '0' || phoneNumber[i] > '9' {
			return fmt.Errorf("phone number contains invalid characters")
		}
	}

	return nil
}

// IsCodeExpired checks if a code is expired
func (sms *SMSService) IsCodeExpired(smsCode *SMSCode) bool {
	return time.Now().After(smsCode.ExpiresAt)
}

// GetRemainingAttempts returns remaining verification attempts
func (sms *SMSService) GetRemainingAttempts(smsCode *SMSCode) int {
	remaining := sms.config.MaxAttempts - smsCode.Attempts
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetConfig returns the SMS service configuration
func (sms *SMSService) GetConfig() SMSConfig {
	return sms.config
}

// GetProviderStatus returns the status of the SMS provider
func (sms *SMSService) GetProviderStatus() bool {
	return sms.provider.IsHealthy()
}

// Twilio Provider Implementation

// NewTwilioProvider creates a new Twilio SMS provider
func NewTwilioProvider(accountSID, authToken, fromNumber string, logger *logrus.Logger) *TwilioProvider {
	return &TwilioProvider{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		logger:     logger,
	}
}

func (tp *TwilioProvider) SendSMS(phoneNumber, message string) error {
	// In a real implementation, this would use the Twilio SDK
	// For now, we'll simulate the SMS sending
	tp.logger.WithFields(logrus.Fields{
		"provider":     "twilio",
		"to":           phoneNumber,
		"from":         tp.fromNumber,
		"message_len":  len(message),
	}).Info("SMS sent via Twilio (simulated)")

	// Simulate potential failure
	if tp.accountSID == "" || tp.authToken == "" {
		return fmt.Errorf("twilio credentials not configured")
	}

	return nil
}

func (tp *TwilioProvider) GetProviderName() string {
	return "twilio"
}

func (tp *TwilioProvider) IsHealthy() bool {
	return tp.accountSID != "" && tp.authToken != "" && tp.fromNumber != ""
}

// AWS Provider Implementation

// NewAWSProvider creates a new AWS SNS SMS provider
func NewAWSProvider(region, accessKey, secretKey string, logger *logrus.Logger) *AWSProvider {
	return &AWSProvider{
		region:    region,
		accessKey: accessKey,
		secretKey: secretKey,
		logger:    logger,
	}
}

func (ap *AWSProvider) SendSMS(phoneNumber, message string) error {
	// In a real implementation, this would use the AWS SDK
	ap.logger.WithFields(logrus.Fields{
		"provider":    "aws_sns",
		"region":      ap.region,
		"to":          phoneNumber,
		"message_len": len(message),
	}).Info("SMS sent via AWS SNS (simulated)")

	// Simulate potential failure
	if ap.accessKey == "" || ap.secretKey == "" {
		return fmt.Errorf("aws credentials not configured")
	}

	return nil
}

func (ap *AWSProvider) GetProviderName() string {
	return "aws_sns"
}

func (ap *AWSProvider) IsHealthy() bool {
	return ap.region != "" && ap.accessKey != "" && ap.secretKey != ""
}

// Mock Provider Implementation (for testing)

// NewMockProvider creates a new mock SMS provider
func NewMockProvider(shouldFail bool, logger *logrus.Logger) *MockProvider {
	return &MockProvider{
		logger:     logger,
		shouldFail: shouldFail,
		sentCodes:  make([]string, 0),
	}
}

func (mp *MockProvider) SendSMS(phoneNumber, message string) error {
	if mp.shouldFail {
		return fmt.Errorf("mock SMS provider configured to fail")
	}

	mp.logger.WithFields(logrus.Fields{
		"provider":    "mock",
		"to":          phoneNumber,
		"message":     message,
	}).Info("SMS sent via mock provider")

	mp.sentCodes = append(mp.sentCodes, message)
	return nil
}

func (mp *MockProvider) GetProviderName() string {
	return "mock"
}

func (mp *MockProvider) IsHealthy() bool {
	return !mp.shouldFail
}

func (mp *MockProvider) GetSentCodes() []string {
	return mp.sentCodes
}

func (mp *MockProvider) SetShouldFail(shouldFail bool) {
	mp.shouldFail = shouldFail
}

// Close closes the SMS service
func (sms *SMSService) Close() error {
	sms.logger.Info("SMS service closed")
	return nil
}