package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/domain/service"
)

// OrganizationTemplateService handles organization template operations
type OrganizationTemplateService struct {
	serviceRegistry service.ServiceRegistryInterface
	logger          *logrus.Logger
}

// NewOrganizationTemplateService creates a new organization template service
func NewOrganizationTemplateService(serviceRegistry service.ServiceRegistryInterface, logger *logrus.Logger) *OrganizationTemplateService {
	return &OrganizationTemplateService{
		serviceRegistry: serviceRegistry,
		logger:          logger,
	}
}

// CreateOrganizationRequest represents a request to create an organization
type CreateOrganizationRequest struct {
	OrganizationID   string                  `json:"organization_id" validate:"required,uuid"`
	TemplateType     entity.TemplateType     `json:"template_type" validate:"required"`
	Customizations   map[string]interface{}  `json:"customizations,omitempty"`
	AdminUserID      string                  `json:"admin_user_id" validate:"required,uuid"`
	OrganizationName string                  `json:"organization_name" validate:"required,min=2,max=100"`
}

// CreateOrganizationResponse represents the response from creating an organization
type CreateOrganizationResponse struct {
	OrganizationConfig *entity.OrganizationConfig `json:"organization_config"`
	MonthlyPrice       float64                    `json:"monthly_price"`
	EnabledServices    []string                   `json:"enabled_services"`
	TemplateInfo       *service.TemplateInfo      `json:"template_info"`
	OnboardingSteps    []OnboardingStep           `json:"onboarding_steps"`
}

// OnboardingStep represents a step in the organization onboarding process
type OnboardingStep struct {
	StepID      string `json:"step_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Completed   bool   `json:"completed"`
	Order       int    `json:"order"`
}

// CreateOrganization creates a new organization with the specified template
func (ots *OrganizationTemplateService) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	// Validate request
	if err := ots.validateCreateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Check if organization already exists
	existingConfig, err := ots.serviceRegistry.GetOrganizationConfig(ctx, req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing organization: %w", err)
	}
	if existingConfig != nil {
		return nil, errors.New("organization already exists")
	}

	// Create organization configuration
	config, err := ots.serviceRegistry.CreateOrganizationConfig(ctx, req.OrganizationID, req.TemplateType)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization config: %w", err)
	}

	// Apply customizations if provided
	if len(req.Customizations) > 0 {
		config.Customizations = req.Customizations
		if err := ots.serviceRegistry.UpdateOrganizationConfig(ctx, config); err != nil {
			ots.logger.WithFields(logrus.Fields{
				"organization_id": req.OrganizationID,
				"error":          err,
			}).Warn("Failed to apply customizations")
		}
	}

	// Calculate pricing
	monthlyPrice := config.CalculateMonthlyPrice()

	// Get enabled services
	enabledServices := config.GetEnabledServices()

	// Get template info
	templateInfo := ots.getTemplateInfo(req.TemplateType)

	// Generate onboarding steps
	onboardingSteps := ots.generateOnboardingSteps(config)

	ots.logger.WithFields(logrus.Fields{
		"organization_id":    req.OrganizationID,
		"template_type":      req.TemplateType,
		"monthly_price":      monthlyPrice,
		"enabled_services":   len(enabledServices),
		"admin_user_id":      req.AdminUserID,
		"organization_name":  req.OrganizationName,
	}).Info("Organization created successfully")

	return &CreateOrganizationResponse{
		OrganizationConfig: config,
		MonthlyPrice:       monthlyPrice,
		EnabledServices:    enabledServices,
		TemplateInfo:       templateInfo,
		OnboardingSteps:    onboardingSteps,
	}, nil
}

// UpdateOrganizationServicesRequest represents a request to update organization services
type UpdateOrganizationServicesRequest struct {
	OrganizationID     string            `json:"organization_id" validate:"required,uuid"`
	ServicesToEnable   []string          `json:"services_to_enable,omitempty"`
	ServicesToDisable  []string          `json:"services_to_disable,omitempty"`
	Customizations     map[string]interface{} `json:"customizations,omitempty"`
	UpdatedByUserID    string            `json:"updated_by_user_id" validate:"required,uuid"`
}

// UpdateOrganizationServices updates the enabled services for an organization
func (ots *OrganizationTemplateService) UpdateOrganizationServices(ctx context.Context, req UpdateOrganizationServicesRequest) (*CreateOrganizationResponse, error) {
	// Get current configuration
	config, err := ots.serviceRegistry.GetOrganizationConfig(ctx, req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization config: %w", err)
	}
	if config == nil {
		return nil, errors.New("organization not found")
	}

	// Enable services
	for _, serviceName := range req.ServicesToEnable {
		if err := ots.serviceRegistry.EnableService(ctx, req.OrganizationID, serviceName); err != nil {
			ots.logger.WithFields(logrus.Fields{
				"organization_id": req.OrganizationID,
				"service":         serviceName,
				"error":          err,
			}).Warn("Failed to enable service")
		}
	}

	// Disable services
	for _, serviceName := range req.ServicesToDisable {
		if err := ots.serviceRegistry.DisableService(ctx, req.OrganizationID, serviceName); err != nil {
			ots.logger.WithFields(logrus.Fields{
				"organization_id": req.OrganizationID,
				"service":         serviceName,
				"error":          err,
			}).Warn("Failed to disable service")
		}
	}

	// Apply customizations
	if len(req.Customizations) > 0 {
		config.Customizations = req.Customizations
		if err := ots.serviceRegistry.UpdateOrganizationConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to update customizations: %w", err)
		}
	}

	// Get updated configuration
	updatedConfig, err := ots.serviceRegistry.GetOrganizationConfig(ctx, req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated config: %w", err)
	}

	monthlyPrice := updatedConfig.CalculateMonthlyPrice()
	enabledServices := updatedConfig.GetEnabledServices()
	templateInfo := ots.getTemplateInfo(updatedConfig.TemplateType)

	ots.logger.WithFields(logrus.Fields{
		"organization_id":     req.OrganizationID,
		"services_enabled":    req.ServicesToEnable,
		"services_disabled":   req.ServicesToDisable,
		"updated_by_user_id":  req.UpdatedByUserID,
		"new_monthly_price":   monthlyPrice,
	}).Info("Organization services updated successfully")

	return &CreateOrganizationResponse{
		OrganizationConfig: updatedConfig,
		MonthlyPrice:       monthlyPrice,
		EnabledServices:    enabledServices,
		TemplateInfo:       templateInfo,
		OnboardingSteps:    ots.generateOnboardingSteps(updatedConfig),
	}, nil
}

// GetOrganizationSummary returns a summary of an organization's configuration
func (ots *OrganizationTemplateService) GetOrganizationSummary(ctx context.Context, organizationID string) (*OrganizationSummary, error) {
	config, err := ots.serviceRegistry.GetOrganizationConfig(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization config: %w", err)
	}
	if config == nil {
		return nil, errors.New("organization not found")
	}

	enabledServices := config.GetEnabledServices()
	monthlyPrice := config.CalculateMonthlyPrice()
	templateInfo := ots.getTemplateInfo(config.TemplateType)

	summary := &OrganizationSummary{
		OrganizationID:  organizationID,
		TemplateType:    config.TemplateType,
		IsActive:        config.IsActive,
		EnabledServices: enabledServices,
		MonthlyPrice:    monthlyPrice,
		TemplateInfo:    *templateInfo,
		CreatedAt:       config.CreatedAt,
		UpdatedAt:       config.UpdatedAt,
		ServiceCounts: ServiceCounts{
			TotalServices:   13, // Total available services
			EnabledServices: len(enabledServices),
			CoreServices:    2, // auth, user
			PremiumServices: ots.countPremiumServices(enabledServices),
		},
	}

	return summary, nil
}

// OrganizationSummary represents a summary of an organization
type OrganizationSummary struct {
	OrganizationID  string                `json:"organization_id"`
	TemplateType    entity.TemplateType   `json:"template_type"`
	IsActive        bool                  `json:"is_active"`
	EnabledServices []string              `json:"enabled_services"`
	MonthlyPrice    float64               `json:"monthly_price"`
	TemplateInfo    service.TemplateInfo  `json:"template_info"`
	ServiceCounts   ServiceCounts         `json:"service_counts"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// ServiceCounts represents counts of different service types
type ServiceCounts struct {
	TotalServices   int `json:"total_services"`
	EnabledServices int `json:"enabled_services"`
	CoreServices    int `json:"core_services"`
	PremiumServices int `json:"premium_services"`
}

// GetAvailableTemplates returns all available organization templates
func (ots *OrganizationTemplateService) GetAvailableTemplates() []service.TemplateInfo {
	return ots.serviceRegistry.GetAvailableTemplates()
}

// MigrateOrganization migrates an organization from one template to another
func (ots *OrganizationTemplateService) MigrateOrganization(ctx context.Context, organizationID string, newTemplateType entity.TemplateType, preserveCustomizations bool) (*CreateOrganizationResponse, error) {
	// Get current configuration
	currentConfig, err := ots.serviceRegistry.GetOrganizationConfig(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current config: %w", err)
	}
	if currentConfig == nil {
		return nil, errors.New("organization not found")
	}

	// Store current customizations if needed
	var customizations map[string]interface{}
	if preserveCustomizations {
		customizations = currentConfig.Customizations
	}

	// Create new configuration with new template
	newConfig := entity.NewOrganizationConfig(organizationID, newTemplateType)
	newConfig.ID = currentConfig.ID // Keep the same ID
	
	if preserveCustomizations {
		newConfig.Customizations = customizations
	}

	// Update the configuration
	if err := ots.serviceRegistry.UpdateOrganizationConfig(ctx, newConfig); err != nil {
		return nil, fmt.Errorf("failed to update config with new template: %w", err)
	}

	monthlyPrice := newConfig.CalculateMonthlyPrice()
	enabledServices := newConfig.GetEnabledServices()
	templateInfo := ots.getTemplateInfo(newTemplateType)

	ots.logger.WithFields(logrus.Fields{
		"organization_id":       organizationID,
		"old_template":          currentConfig.TemplateType,
		"new_template":          newTemplateType,
		"preserve_customizations": preserveCustomizations,
		"new_monthly_price":     monthlyPrice,
	}).Info("Organization migrated to new template")

	return &CreateOrganizationResponse{
		OrganizationConfig: newConfig,
		MonthlyPrice:       monthlyPrice,
		EnabledServices:    enabledServices,
		TemplateInfo:       templateInfo,
		OnboardingSteps:    ots.generateOnboardingSteps(newConfig),
	}, nil
}

// Private helper methods

func (ots *OrganizationTemplateService) validateCreateRequest(req CreateOrganizationRequest) error {
	if req.OrganizationID == "" {
		return errors.New("organization_id is required")
	}
	
	if _, err := uuid.Parse(req.OrganizationID); err != nil {
		return errors.New("organization_id must be a valid UUID")
	}

	if req.AdminUserID == "" {
		return errors.New("admin_user_id is required")
	}
	
	if _, err := uuid.Parse(req.AdminUserID); err != nil {
		return errors.New("admin_user_id must be a valid UUID")
	}

	if req.OrganizationName == "" {
		return errors.New("organization_name is required")
	}

	if len(req.OrganizationName) < 3 {
		return errors.New("organization_name must be at least 3 characters long")
	}

	validTemplates := []entity.TemplateType{
		entity.TEMPLATE_PADEL_ONLY,
		entity.TEMPLATE_SOCIAL_CLUB,
		entity.TEMPLATE_FOOTBALL_CLUB,
		entity.TEMPLATE_GYM_FITNESS,
		entity.TEMPLATE_CUSTOM,
	}

	valid := false
	for _, template := range validTemplates {
		if req.TemplateType == template {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("invalid template_type")
	}

	return nil
}

func (ots *OrganizationTemplateService) getTemplateInfo(templateType entity.TemplateType) *service.TemplateInfo {
	templates := ots.serviceRegistry.GetAvailableTemplates()
	for _, template := range templates {
		if template.Type == templateType {
			return &template
		}
	}
	return nil
}

func (ots *OrganizationTemplateService) generateOnboardingSteps(config *entity.OrganizationConfig) []OnboardingStep {
	steps := []OnboardingStep{
		{
			StepID:      "basic_setup",
			Title:       "Configuración Básica",
			Description: "Completar información básica de la organización",
			Required:    true,
			Completed:   true, // Always completed when config is created
			Order:       1,
		},
		{
			StepID:      "admin_user",
			Title:       "Usuario Administrador",
			Description: "Configurar usuario administrador principal",
			Required:    true,
			Completed:   false,
			Order:       2,
		},
	}

	// Add template-specific steps
	switch config.TemplateType {
	case entity.TEMPLATE_PADEL_ONLY:
		steps = append(steps, OnboardingStep{
			StepID:      "padel_courts",
			Title:       "Configurar Pistas de Pádel",
			Description: "Añadir y configurar las pistas de pádel",
			Required:    true,
			Completed:   false,
			Order:       3,
		})
		steps = append(steps, OnboardingStep{
			StepID:      "pricing_setup",
			Title:       "Configurar Precios",
			Description: "Establecer precios base y horarios pico",
			Required:    true,
			Completed:   false,
			Order:       4,
		})

	case entity.TEMPLATE_SOCIAL_CLUB:
		steps = append(steps, OnboardingStep{
			StepID:      "facilities_setup",
			Title:       "Configurar Instalaciones",
			Description: "Añadir todas las instalaciones del club",
			Required:    true,
			Completed:   false,
			Order:       3,
		})
		steps = append(steps, OnboardingStep{
			StepID:      "membership_tiers",
			Title:       "Tipos de Membresía",
			Description: "Configurar diferentes tipos de membresía",
			Required:    true,
			Completed:   false,
			Order:       4,
		})

	case entity.TEMPLATE_FOOTBALL_CLUB:
		steps = append(steps, OnboardingStep{
			StepID:      "teams_setup",
			Title:       "Configurar Equipos",
			Description: "Crear equipos y categorías",
			Required:    true,
			Completed:   false,
			Order:       3,
		})
		steps = append(steps, OnboardingStep{
			StepID:      "training_schedule",
			Title:       "Horarios de Entrenamiento",
			Description: "Configurar horarios de entrenamiento",
			Required:    true,
			Completed:   false,
			Order:       4,
		})

	case entity.TEMPLATE_GYM_FITNESS:
		steps = append(steps, OnboardingStep{
			StepID:      "class_schedule",
			Title:       "Clases Grupales",
			Description: "Configurar horarios de clases grupales",
			Required:    true,
			Completed:   false,
			Order:       3,
		})
		steps = append(steps, OnboardingStep{
			StepID:      "equipment_setup",
			Title:       "Equipamiento",
			Description: "Registrar equipamiento del gimnasio",
			Required:    false,
			Completed:   false,
			Order:       4,
		})
	}

	// Add payment setup for all templates with payments enabled
	if config.IsServiceEnabled("payments") {
		steps = append(steps, OnboardingStep{
			StepID:      "payment_setup",
			Title:       "Configurar Pagos",
			Description: "Configurar métodos de pago y procesadores",
			Required:    true,
			Completed:   false,
			Order:       10,
		})
	}

	return steps
}

func (ots *OrganizationTemplateService) countPremiumServices(enabledServices []string) int {
	premiumServices := []string{"gamification", "wallet", "groupbooking", "dynamicpricing", "insights"}
	count := 0
	
	enabledMap := make(map[string]bool)
	for _, service := range enabledServices {
		enabledMap[service] = true
	}
	
	for _, premium := range premiumServices {
		if enabledMap[premium] {
			count++
		}
	}
	
	return count
}