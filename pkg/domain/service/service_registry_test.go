package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/lukcba-developers/gopherkit/pkg/domain/entity"
)

// MockOrganizationConfigRepository is a mock implementation of OrganizationConfigRepository
type MockOrganizationConfigRepository struct {
	mock.Mock
}

func (m *MockOrganizationConfigRepository) GetByOrganizationID(ctx context.Context, organizationID string) (*entity.OrganizationConfig, error) {
	args := m.Called(ctx, organizationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.OrganizationConfig), args.Error(1)
}

func (m *MockOrganizationConfigRepository) Create(ctx context.Context, config *entity.OrganizationConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockOrganizationConfigRepository) Update(ctx context.Context, config *entity.OrganizationConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockOrganizationConfigRepository) Delete(ctx context.Context, organizationID string) error {
	args := m.Called(ctx, organizationID)
	return args.Error(0)
}

func (m *MockOrganizationConfigRepository) List(ctx context.Context, limit, offset int) ([]*entity.OrganizationConfig, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.OrganizationConfig), args.Error(1)
}

func TestServiceRegistry_IsServiceEnabled(t *testing.T) {
	tests := []struct {
		name           string
		organizationID string
		serviceName    string
		mockConfig     *entity.OrganizationConfig
		mockError      error
		expectedResult bool
		expectedError  string
	}{
		{
			name:           "Service enabled in padel template",
			organizationID: uuid.New().String(),
			serviceName:    "booking",
			mockConfig:     entity.NewOrganizationConfig(uuid.New().String(), entity.TEMPLATE_PADEL_ONLY),
			expectedResult: true,
		},
		{
			name:           "Service disabled in padel template",
			organizationID: uuid.New().String(),
			serviceName:    "championship",
			mockConfig:     entity.NewOrganizationConfig(uuid.New().String(), entity.TEMPLATE_PADEL_ONLY),
			expectedResult: false,
		},
		{
			name:           "Core service enabled when no config",
			organizationID: uuid.New().String(),
			serviceName:    "auth",
			mockConfig:     nil,
			expectedResult: true,
		},
		{
			name:           "Non-core service disabled when no config",
			organizationID: uuid.New().String(),
			serviceName:    "booking",
			mockConfig:     nil,
			expectedResult: false,
		},
		{
			name:           "Empty organization ID",
			organizationID: "",
			serviceName:    "booking",
			expectedError:  "organization ID is required",
		},
		{
			name:           "Repository error",
			organizationID: uuid.New().String(),
			serviceName:    "booking",
			mockError:      errors.New("database error"),
			expectedError:  "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockRepo := new(MockOrganizationConfigRepository)
			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel) // Reduce log noise in tests
			
			registry := NewServiceRegistry(mockRepo, logger)

			if tt.organizationID != "" {
				mockRepo.On("GetByOrganizationID", mock.Anything, tt.organizationID).Return(tt.mockConfig, tt.mockError)
			}

			// Act
			result, err := registry.IsServiceEnabled(context.Background(), tt.organizationID, tt.serviceName)

			// Assert
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestServiceRegistry_CreateOrganizationConfig(t *testing.T) {
	tests := []struct {
		name           string
		organizationID string
		templateType   entity.TemplateType
		existingConfig *entity.OrganizationConfig
		createError    error
		expectedError  string
	}{
		{
			name:           "Successful creation",
			organizationID: uuid.New().String(),
			templateType:   entity.TEMPLATE_PADEL_ONLY,
		},
		{
			name:           "Config already exists",
			organizationID: uuid.New().String(),
			templateType:   entity.TEMPLATE_PADEL_ONLY,
			existingConfig: entity.NewOrganizationConfig(uuid.New().String(), entity.TEMPLATE_PADEL_ONLY),
			expectedError:  "organization config already exists",
		},
		{
			name:           "Database error on creation",
			organizationID: uuid.New().String(),
			templateType:   entity.TEMPLATE_PADEL_ONLY,
			createError:    errors.New("database error"),
			expectedError:  "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockRepo := new(MockOrganizationConfigRepository)
			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			
			registry := NewServiceRegistry(mockRepo, logger)

			mockRepo.On("GetByOrganizationID", mock.Anything, tt.organizationID).Return(tt.existingConfig, nil)
			
			if tt.existingConfig == nil {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*entity.OrganizationConfig")).Return(tt.createError)
			}

			// Act
			config, err := registry.CreateOrganizationConfig(context.Background(), tt.organizationID, tt.templateType)

			// Assert
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
				assert.Equal(t, tt.organizationID, config.OrganizationID)
				assert.Equal(t, tt.templateType, config.TemplateType)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestServiceRegistry_EnableService(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)
	// Disable championship initially
	mockConfig.DisableService("championship")

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil)
	mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)

	// Act
	err := registry.EnableService(context.Background(), organizationID, "championship")

	// Assert
	assert.NoError(t, err)
	assert.True(t, mockConfig.IsServiceEnabled("championship"))
	mockRepo.AssertExpectations(t)
}

func TestServiceRegistry_DisableService(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil)
	mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)

	// Act
	err := registry.DisableService(context.Background(), organizationID, "gamification")

	// Assert
	assert.NoError(t, err)
	assert.False(t, mockConfig.IsServiceEnabled("gamification"))
	mockRepo.AssertExpectations(t)
}

func TestServiceRegistry_DisableCoreService(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil)

	// Act
	err := registry.DisableService(context.Background(), organizationID, "auth")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot disable core service")
	mockRepo.AssertExpectations(t)
}

func TestServiceRegistry_GetEnabledServices(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil)

	// Act
	services, err := registry.GetEnabledServices(context.Background(), organizationID)

	// Assert
	assert.NoError(t, err)
	assert.Contains(t, services, "auth")
	assert.Contains(t, services, "user")
	assert.Contains(t, services, "booking")
	assert.Contains(t, services, "gamification")
	assert.NotContains(t, services, "championship") // Not enabled in padel template
	mockRepo.AssertExpectations(t)
}

func TestServiceRegistry_CalculateMonthlyPrice(t *testing.T) {
	tests := []struct {
		name           string
		templateType   entity.TemplateType
		expectedPrice  float64
	}{
		{
			name:           "Padel template with discount",
			templateType:   entity.TEMPLATE_PADEL_ONLY,
			expectedPrice:  214.0, // Based on enabled services and 15% discount
		},
		{
			name:           "Custom template base price",
			templateType:   entity.TEMPLATE_CUSTOM,
			expectedPrice:  49.0, // Only auth + user
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			organizationID := uuid.New().String()
			mockConfig := entity.NewOrganizationConfig(organizationID, tt.templateType)

			mockRepo := new(MockOrganizationConfigRepository)
			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			
			registry := NewServiceRegistry(mockRepo, logger)

			mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil)

			// Act
			price, err := registry.CalculateMonthlyPrice(context.Background(), organizationID)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPrice, price)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestServiceRegistry_GetAvailableTemplates(t *testing.T) {
	// Arrange
	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	registry := NewServiceRegistry(mockRepo, logger)

	// Act
	templates := registry.GetAvailableTemplates()

	// Assert
	assert.Len(t, templates, 5)
	
	// Check that all expected templates are present
	templateTypes := make(map[entity.TemplateType]bool)
	for _, template := range templates {
		templateTypes[template.Type] = true
	}
	
	assert.True(t, templateTypes[entity.TEMPLATE_PADEL_ONLY])
	assert.True(t, templateTypes[entity.TEMPLATE_SOCIAL_CLUB])
	assert.True(t, templateTypes[entity.TEMPLATE_FOOTBALL_CLUB])
	assert.True(t, templateTypes[entity.TEMPLATE_GYM_FITNESS])
	assert.True(t, templateTypes[entity.TEMPLATE_CUSTOM])

	// Check specific template details
	for _, template := range templates {
		if template.Type == entity.TEMPLATE_PADEL_ONLY {
			assert.Equal(t, "Centro de Pádel", template.Name)
			assert.Equal(t, 149.0, template.Price)
			assert.Contains(t, template.Features, "Reservas")
		}
	}
}

func TestServiceRegistry_Cache(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	// First call should hit the database
	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil).Once()

	// Act - First call
	result1, err1 := registry.IsServiceEnabled(context.Background(), organizationID, "booking")
	
	// Act - Second call (should use cache)
	result2, err2 := registry.IsServiceEnabled(context.Background(), organizationID, "booking")

	// Assert
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, result1, result2)
	assert.True(t, result1) // booking is enabled in padel template

	// Verify repository was called only once (second call used cache)
	mockRepo.AssertExpectations(t)
}

func TestServiceRegistry_ClearCache(t *testing.T) {
	// Arrange
	organizationID := uuid.New().String()
	mockConfig := entity.NewOrganizationConfig(organizationID, entity.TEMPLATE_PADEL_ONLY)

	mockRepo := new(MockOrganizationConfigRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	registry := NewServiceRegistry(mockRepo, logger)

	// Setup expectations for two database calls (before and after cache clear)
	mockRepo.On("GetByOrganizationID", mock.Anything, organizationID).Return(mockConfig, nil).Twice()

	// Act - First call (loads cache)
	_, err1 := registry.IsServiceEnabled(context.Background(), organizationID, "booking")
	
	// Clear cache
	registry.ClearCache()
	
	// Second call (should hit database again)
	_, err2 := registry.IsServiceEnabled(context.Background(), organizationID, "booking")

	// Assert
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	mockRepo.AssertExpectations(t)
}