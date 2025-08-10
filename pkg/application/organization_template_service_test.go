package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lukcba-developers/gopherkit/pkg/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/domain/service"
)

// MockServiceRegistry implementa service.ServiceRegistry para tests
type MockServiceRegistry struct {
	mock.Mock
}

func (m *MockServiceRegistry) GetOrganizationConfig(ctx context.Context, orgID string) (*entity.OrganizationConfig, error) {
	args := m.Called(ctx, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.OrganizationConfig), args.Error(1)
}

func (m *MockServiceRegistry) CreateOrganizationConfig(ctx context.Context, orgID string, templateType entity.TemplateType) (*entity.OrganizationConfig, error) {
	args := m.Called(ctx, orgID, templateType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.OrganizationConfig), args.Error(1)
}

func (m *MockServiceRegistry) UpdateOrganizationConfig(ctx context.Context, config *entity.OrganizationConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockServiceRegistry) EnableService(ctx context.Context, orgID, serviceName string) error {
	args := m.Called(ctx, orgID, serviceName)
	return args.Error(0)
}

func (m *MockServiceRegistry) DisableService(ctx context.Context, orgID, serviceName string) error {
	args := m.Called(ctx, orgID, serviceName)
	return args.Error(0)
}

func (m *MockServiceRegistry) GetAvailableTemplates() []service.TemplateInfo {
	args := m.Called()
	if args.Get(0) == nil {
		return []service.TemplateInfo{}
	}
	return args.Get(0).([]service.TemplateInfo)
}

func setupOrganizationTemplateService() (*OrganizationTemplateService, *MockServiceRegistry) {
	mockRegistry := &MockServiceRegistry{}
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce log noise in tests
	
	// Setup default templates for the mock
	defaultTemplates := []service.TemplateInfo{
		{
			Type:        entity.TemplateTypeStartup,
			Name:        "Startup Plan",
			Description: "Basic startup plan",
			Features:    []string{"feature1", "feature2"},
			Price:       99.99,
		},
		{
			Type:        entity.TemplateTypeEnterprise,
			Name:        "Enterprise Plan", 
			Description: "Full enterprise plan",
			Features:    []string{"all_features"},
			Price:       499.99,
		},
	}
	mockRegistry.On("GetAvailableTemplates").Return(defaultTemplates).Maybe()
	
	service := NewOrganizationTemplateService(mockRegistry, logger)
	return service, mockRegistry
}

func createValidOrganizationConfig() *entity.OrganizationConfig {
	// Use the proper constructor that applies template defaults
	config := entity.NewOrganizationConfig(uuid.New().String(), entity.TemplateTypeStartup)
	return config
}

func TestNewOrganizationTemplateService(t *testing.T) {
	mockRegistry := &MockServiceRegistry{}
	logger := logrus.New()
	
	service := NewOrganizationTemplateService(mockRegistry, logger)
	
	assert.NotNil(t, service)
	assert.Equal(t, mockRegistry, service.serviceRegistry)
	assert.Equal(t, logger, service.logger)
}

func TestCreateOrganizationRequest_Validation(t *testing.T) {
	service, _ := setupOrganizationTemplateService()
	
	tests := []struct {
		name        string
		request     CreateOrganizationRequest
		expectError bool
	}{
		{
			name: "valid request",
			request: CreateOrganizationRequest{
				OrganizationID:   uuid.New().String(),
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      uuid.New().String(),
				OrganizationName: "Test Organization",
			},
			expectError: false,
		},
		{
			name: "empty organization ID",
			request: CreateOrganizationRequest{
				OrganizationID:   "",
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      uuid.New().String(),
				OrganizationName: "Test Organization",
			},
			expectError: true,
		},
		{
			name: "invalid organization ID",
			request: CreateOrganizationRequest{
				OrganizationID:   "invalid-uuid",
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      uuid.New().String(),
				OrganizationName: "Test Organization",
			},
			expectError: true,
		},
		{
			name: "empty admin user ID",
			request: CreateOrganizationRequest{
				OrganizationID:   uuid.New().String(),
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      "",
				OrganizationName: "Test Organization",
			},
			expectError: true,
		},
		{
			name: "empty organization name",
			request: CreateOrganizationRequest{
				OrganizationID:   uuid.New().String(),
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      uuid.New().String(),
				OrganizationName: "",
			},
			expectError: true,
		},
		{
			name: "organization name too short",
			request: CreateOrganizationRequest{
				OrganizationID:   uuid.New().String(),
				TemplateType:     entity.TemplateTypeStartup,
				AdminUserID:      uuid.New().String(),
				OrganizationName: "A",
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateCreateRequest(tt.request)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateOrganization_Success(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	defer mockRegistry.AssertExpectations(t)
	ctx := context.Background()
	
	orgID := uuid.New().String()
	adminID := uuid.New().String()
	templateType := entity.TemplateTypeStartup
	
	request := CreateOrganizationRequest{
		OrganizationID:   orgID,
		TemplateType:     templateType,
		AdminUserID:      adminID,
		OrganizationName: "Test Organization",
		Customizations: map[string]interface{}{
			"theme": "dark",
			"features": []string{"analytics", "reports"},
		},
	}
	
	config := createValidOrganizationConfig()
	config.OrganizationID = orgID
	config.TemplateType = templateType
	
	
	// Mock expectations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return((*entity.OrganizationConfig)(nil), nil)
	mockRegistry.On("CreateOrganizationConfig", ctx, orgID, templateType).Return(config, nil)
	mockRegistry.On("UpdateOrganizationConfig", ctx, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)
	
	// Execute
	response, err := service.CreateOrganization(ctx, request)
	
	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, config, response.OrganizationConfig)
	assert.NotZero(t, response.MonthlyPrice)
	assert.NotNil(t, response.EnabledServices)
	assert.NotEmpty(t, response.OnboardingSteps)
	
	mockRegistry.AssertExpectations(t)
}

func TestCreateOrganization_OrganizationAlreadyExists(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	existingConfig := createValidOrganizationConfig()
	
	request := CreateOrganizationRequest{
		OrganizationID:   orgID,
		TemplateType:     entity.TemplateTypeStartup,
		AdminUserID:      uuid.New().String(),
		OrganizationName: "Test Organization",
	}
	
	// Mock expectations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return(existingConfig, nil)
	
	// Execute
	response, err := service.CreateOrganization(ctx, request)
	
	// Assertions
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "organization already exists")
	
	mockRegistry.AssertExpectations(t)
}

func TestCreateOrganization_InvalidRequest(t *testing.T) {
	service, _ := setupOrganizationTemplateService()
	ctx := context.Background()
	
	request := CreateOrganizationRequest{
		OrganizationID:   "invalid-uuid",
		TemplateType:     entity.TemplateTypeStartup,
		AdminUserID:      uuid.New().String(),
		OrganizationName: "Test Organization",
	}
	
	// Execute
	response, err := service.CreateOrganization(ctx, request)
	
	// Assertions
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestCreateOrganization_CreateConfigError(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	request := CreateOrganizationRequest{
		OrganizationID:   orgID,
		TemplateType:     entity.TemplateTypeStartup,
		AdminUserID:      uuid.New().String(),
		OrganizationName: "Test Organization",
	}
	
	// Mock expectations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return((*entity.OrganizationConfig)(nil), nil)
	mockRegistry.On("CreateOrganizationConfig", ctx, orgID, entity.TemplateTypeStartup).Return((*entity.OrganizationConfig)(nil), errors.New("creation failed"))
	
	// Execute
	response, err := service.CreateOrganization(ctx, request)
	
	// Assertions
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to create organization config")
	
	mockRegistry.AssertExpectations(t)
}

func TestUpdateOrganizationServices_Success(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	userID := uuid.New().String()
	
	request := UpdateOrganizationServicesRequest{
		OrganizationID:    orgID,
		ServicesToEnable:  []string{"analytics", "reports"},
		ServicesToDisable: []string{"beta_feature"},
		UpdatedByUserID:   userID,
		Customizations: map[string]interface{}{
			"theme": "light",
		},
	}
	
	config := createValidOrganizationConfig()
	config.OrganizationID = orgID
	
	// Mock expectations - UpdateOrganizationServices calls GetOrganizationConfig twice
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return(config, nil).Times(2)
	mockRegistry.On("EnableService", ctx, orgID, "analytics").Return(nil)
	mockRegistry.On("EnableService", ctx, orgID, "reports").Return(nil)
	mockRegistry.On("DisableService", ctx, orgID, "beta_feature").Return(nil)
	mockRegistry.On("UpdateOrganizationConfig", ctx, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)
	
	// Execute
	response, err := service.UpdateOrganizationServices(ctx, request)
	
	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, config, response.OrganizationConfig)
	assert.NotNil(t, response.EnabledServices)
	
	mockRegistry.AssertExpectations(t)
}

func TestUpdateOrganizationServices_OrganizationNotFound(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	request := UpdateOrganizationServicesRequest{
		OrganizationID:  orgID,
		UpdatedByUserID: uuid.New().String(),
	}
	
	// Mock expectations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return((*entity.OrganizationConfig)(nil), nil)
	
	// Execute
	response, err := service.UpdateOrganizationServices(ctx, request)
	
	// Assertions
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "organization not found")
	
	mockRegistry.AssertExpectations(t)
}

func TestUpdateOrganizationServices_GetConfigError(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	request := UpdateOrganizationServicesRequest{
		OrganizationID:  orgID,
		UpdatedByUserID: uuid.New().String(),
	}
	
	// Mock expectations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return((*entity.OrganizationConfig)(nil), errors.New("database error"))
	
	// Execute
	response, err := service.UpdateOrganizationServices(ctx, request)
	
	// Assertions
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to get organization config")
	
	mockRegistry.AssertExpectations(t)
}

func TestUpdateOrganizationServices_EnableServiceError(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	request := UpdateOrganizationServicesRequest{
		OrganizationID:   orgID,
		ServicesToEnable: []string{"failing_service"},
		UpdatedByUserID:  uuid.New().String(),
	}
	
	config := createValidOrganizationConfig()
	config.OrganizationID = orgID
	
	
	// Mock expectations - service enabling fails but process continues
	// UpdateOrganizationServices calls GetOrganizationConfig twice
	// Note: UpdateOrganizationConfig is not called because there are no customizations
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return(config, nil).Times(2)
	mockRegistry.On("EnableService", ctx, orgID, "failing_service").Return(errors.New("service enable failed"))
	
	// Execute - should not fail even if individual service enabling fails
	response, err := service.UpdateOrganizationServices(ctx, request)
	
	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, response)
	
	mockRegistry.AssertExpectations(t)
}

func TestOnboardingStep_Fields(t *testing.T) {
	step := OnboardingStep{
		StepID:      "step_1",
		Title:       "Setup Profile",
		Description: "Complete your organization profile",
		Required:    true,
		Completed:   false,
		Order:       1,
	}
	
	assert.Equal(t, "step_1", step.StepID)
	assert.Equal(t, "Setup Profile", step.Title)
	assert.Equal(t, "Complete your organization profile", step.Description)
	assert.True(t, step.Required)
	assert.False(t, step.Completed)
	assert.Equal(t, 1, step.Order)
}

func TestCreateOrganizationResponse_Fields(t *testing.T) {
	config := createValidOrganizationConfig()
	onboardingSteps := []OnboardingStep{
		{StepID: "step_1", Title: "Step 1", Order: 1},
	}
	
	response := CreateOrganizationResponse{
		OrganizationConfig: config,
		MonthlyPrice:       99.99,
		EnabledServices:    []string{"analytics", "reports"},
		OnboardingSteps:    onboardingSteps,
	}
	
	assert.Equal(t, config, response.OrganizationConfig)
	assert.Equal(t, 99.99, response.MonthlyPrice)
	assert.Len(t, response.EnabledServices, 2)
	assert.Contains(t, response.EnabledServices, "analytics")
	assert.Contains(t, response.EnabledServices, "reports")
	assert.Len(t, response.OnboardingSteps, 1)
}

func TestUpdateOrganizationServicesRequest_Fields(t *testing.T) {
	orgID := uuid.New().String()
	userID := uuid.New().String()
	
	request := UpdateOrganizationServicesRequest{
		OrganizationID:    orgID,
		ServicesToEnable:  []string{"service1", "service2"},
		ServicesToDisable: []string{"service3"},
		Customizations: map[string]interface{}{
			"setting1": "value1",
			"setting2": 42,
		},
		UpdatedByUserID: userID,
	}
	
	assert.Equal(t, orgID, request.OrganizationID)
	assert.Len(t, request.ServicesToEnable, 2)
	assert.Len(t, request.ServicesToDisable, 1)
	assert.Len(t, request.Customizations, 2)
	assert.Equal(t, userID, request.UpdatedByUserID)
}

// Integration test que simula un flujo completo
func TestOrganizationTemplateService_Integration(t *testing.T) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	orgID := uuid.New().String()
	adminID := uuid.New().String()
	
	// 1. Crear organización
	createRequest := CreateOrganizationRequest{
		OrganizationID:   orgID,
		TemplateType:     entity.TemplateTypeEnterprise,
		AdminUserID:      adminID,
		OrganizationName: "Enterprise Test Org",
		Customizations: map[string]interface{}{
			"branding": map[string]string{
				"logo":  "https://example.com/logo.png",
				"color": "#ff0000",
			},
		},
	}
	
	config := createValidOrganizationConfig()
	config.OrganizationID = orgID
	config.TemplateType = entity.TemplateTypeEnterprise
	
	
	// Mock para creación
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return((*entity.OrganizationConfig)(nil), nil).Once()
	mockRegistry.On("CreateOrganizationConfig", ctx, orgID, entity.TemplateTypeEnterprise).Return(config, nil)
	mockRegistry.On("UpdateOrganizationConfig", ctx, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil).Once()
	
	// Crear organización
	createResponse, err := service.CreateOrganization(ctx, createRequest)
	require.NoError(t, err)
	assert.NotNil(t, createResponse)
	assert.Equal(t, config.OrganizationID, createResponse.OrganizationConfig.OrganizationID)
	
	// 2. Actualizar servicios
	updateRequest := UpdateOrganizationServicesRequest{
		OrganizationID:    orgID,
		ServicesToEnable:  []string{"premium_support", "advanced_reporting"},
		ServicesToDisable: []string{"basic_analytics"},
		UpdatedByUserID:   adminID,
	}
	
	// Mock para actualización - UpdateOrganizationServices calls GetOrganizationConfig twice
	mockRegistry.On("GetOrganizationConfig", ctx, orgID).Return(config, nil).Times(2)
	mockRegistry.On("EnableService", ctx, orgID, "premium_support").Return(nil)
	mockRegistry.On("EnableService", ctx, orgID, "advanced_reporting").Return(nil)
	mockRegistry.On("DisableService", ctx, orgID, "basic_analytics").Return(nil)
	mockRegistry.On("UpdateOrganizationConfig", ctx, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)
	
	// Actualizar servicios
	updateResponse, err := service.UpdateOrganizationServices(ctx, updateRequest)
	require.NoError(t, err)
	assert.NotNil(t, updateResponse)
	assert.Equal(t, config.OrganizationID, updateResponse.OrganizationConfig.OrganizationID)
	
	mockRegistry.AssertExpectations(t)
}

// Benchmark tests
func BenchmarkCreateOrganization(b *testing.B) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	config := createValidOrganizationConfig()
	
	request := CreateOrganizationRequest{
		OrganizationID:   uuid.New().String(),
		TemplateType:     entity.TemplateTypeStartup,
		AdminUserID:      uuid.New().String(),
		OrganizationName: "Benchmark Org",
	}
	
	// Setup mocks for benchmark
	mockRegistry.On("GetOrganizationConfig", ctx, mock.AnythingOfType("string")).Return((*entity.OrganizationConfig)(nil), nil)
	mockRegistry.On("CreateOrganizationConfig", ctx, mock.AnythingOfType("string"), entity.TemplateTypeStartup).Return(config, nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use different org ID for each iteration
		request.OrganizationID = uuid.New().String()
		service.CreateOrganization(ctx, request)
	}
}

func BenchmarkUpdateOrganizationServices(b *testing.B) {
	service, mockRegistry := setupOrganizationTemplateService()
	ctx := context.Background()
	
	config := createValidOrganizationConfig()
	
	request := UpdateOrganizationServicesRequest{
		OrganizationID:   uuid.New().String(),
		ServicesToEnable: []string{"service1"},
		UpdatedByUserID:  uuid.New().String(),
	}
	
	// Setup mocks for benchmark
	mockRegistry.On("GetOrganizationConfig", ctx, mock.AnythingOfType("string")).Return(config, nil)
	mockRegistry.On("EnableService", ctx, mock.AnythingOfType("string"), "service1").Return(nil)
	mockRegistry.On("UpdateOrganizationConfig", ctx, mock.AnythingOfType("*entity.OrganizationConfig")).Return(nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use different org ID for each iteration
		request.OrganizationID = uuid.New().String()
		service.UpdateOrganizationServices(ctx, request)
	}
}
