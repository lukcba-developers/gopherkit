package service

import (
	"context"
	"errors"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/domain/entity"
)

// ServiceRegistry manages which services are enabled for each organization
type ServiceRegistry struct {
	configRepo   OrganizationConfigRepository
	cache        map[string]*entity.OrganizationConfig // organizationID -> config
	cacheMutex   sync.RWMutex
	logger       *logrus.Logger
}

// OrganizationConfigRepository interface for persistence
type OrganizationConfigRepository interface {
	GetByOrganizationID(ctx context.Context, organizationID string) (*entity.OrganizationConfig, error)
	Create(ctx context.Context, config *entity.OrganizationConfig) error
	Update(ctx context.Context, config *entity.OrganizationConfig) error
	Delete(ctx context.Context, organizationID string) error
	List(ctx context.Context, limit, offset int) ([]*entity.OrganizationConfig, error)
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(configRepo OrganizationConfigRepository, logger *logrus.Logger) *ServiceRegistry {
	return &ServiceRegistry{
		configRepo: configRepo,
		cache:      make(map[string]*entity.OrganizationConfig),
		logger:     logger,
	}
}

// IsServiceEnabled checks if a service is enabled for an organization
func (sr *ServiceRegistry) IsServiceEnabled(ctx context.Context, organizationID, serviceName string) (bool, error) {
	if organizationID == "" {
		return false, errors.New("organization ID is required")
	}

	config, err := sr.getOrganizationConfig(ctx, organizationID)
	if err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"service":         serviceName,
			"error":          err,
		}).Error("Failed to get organization config")
		return false, err
	}

	if config == nil {
		// If no config exists, default to false (except core services)
		return serviceName == "auth" || serviceName == "user", nil
	}

	enabled := config.IsServiceEnabled(serviceName)
	sr.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"service":         serviceName,
		"enabled":         enabled,
	}).Debug("Service enablement check")

	return enabled, nil
}

// GetEnabledServices returns all enabled services for an organization
func (sr *ServiceRegistry) GetEnabledServices(ctx context.Context, organizationID string) ([]string, error) {
	config, err := sr.getOrganizationConfig(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	if config == nil {
		// Return only core services if no config exists
		return []string{"auth", "user"}, nil
	}

	return config.GetEnabledServices(), nil
}

// EnableService enables a service for an organization
func (sr *ServiceRegistry) EnableService(ctx context.Context, organizationID, serviceName string) error {
	config, err := sr.getOrganizationConfig(ctx, organizationID)
	if err != nil {
		return err
	}

	if config == nil {
		return errors.New("organization config not found")
	}

	if err := config.EnableService(serviceName); err != nil {
		return err
	}

	// Update in database
	if err := sr.configRepo.Update(ctx, config); err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"service":         serviceName,
			"error":          err,
		}).Error("Failed to update organization config")
		return err
	}

	// Update cache
	sr.updateCache(organizationID, config)

	sr.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"service":         serviceName,
	}).Info("Service enabled successfully")

	return nil
}

// DisableService disables a service for an organization
func (sr *ServiceRegistry) DisableService(ctx context.Context, organizationID, serviceName string) error {
	config, err := sr.getOrganizationConfig(ctx, organizationID)
	if err != nil {
		return err
	}

	if config == nil {
		return errors.New("organization config not found")
	}

	if err := config.DisableService(serviceName); err != nil {
		return err
	}

	// Update in database
	if err := sr.configRepo.Update(ctx, config); err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"service":         serviceName,
			"error":          err,
		}).Error("Failed to update organization config")
		return err
	}

	// Update cache
	sr.updateCache(organizationID, config)

	sr.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"service":         serviceName,
	}).Info("Service disabled successfully")

	return nil
}

// CreateOrganizationConfig creates a new organization configuration
func (sr *ServiceRegistry) CreateOrganizationConfig(ctx context.Context, organizationID string, templateType entity.TemplateType) (*entity.OrganizationConfig, error) {
	// Check if config already exists
	existing, _ := sr.configRepo.GetByOrganizationID(ctx, organizationID)
	if existing != nil {
		return nil, errors.New("organization config already exists")
	}

	config := entity.NewOrganizationConfig(organizationID, templateType)

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if err := sr.configRepo.Create(ctx, config); err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"template_type":   templateType,
			"error":          err,
		}).Error("Failed to create organization config")
		return nil, err
	}

	// Update cache
	sr.updateCache(organizationID, config)

	sr.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"template_type":   templateType,
		"config_id":       config.ID,
	}).Info("Organization config created successfully")

	return config, nil
}

// GetOrganizationConfig returns the configuration for an organization
func (sr *ServiceRegistry) GetOrganizationConfig(ctx context.Context, organizationID string) (*entity.OrganizationConfig, error) {
	return sr.getOrganizationConfig(ctx, organizationID)
}

// UpdateOrganizationConfig updates an organization configuration
func (sr *ServiceRegistry) UpdateOrganizationConfig(ctx context.Context, config *entity.OrganizationConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	if err := sr.configRepo.Update(ctx, config); err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": config.OrganizationID,
			"config_id":       config.ID,
			"error":          err,
		}).Error("Failed to update organization config")
		return err
	}

	// Update cache
	sr.updateCache(config.OrganizationID, config)

	sr.logger.WithFields(logrus.Fields{
		"organization_id": config.OrganizationID,
		"config_id":       config.ID,
	}).Info("Organization config updated successfully")

	return nil
}

// DeleteOrganizationConfig deletes an organization configuration
func (sr *ServiceRegistry) DeleteOrganizationConfig(ctx context.Context, organizationID string) error {
	if err := sr.configRepo.Delete(ctx, organizationID); err != nil {
		sr.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"error":          err,
		}).Error("Failed to delete organization config")
		return err
	}

	// Remove from cache
	sr.removeFromCache(organizationID)

	sr.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
	}).Info("Organization config deleted successfully")

	return nil
}

// CalculateMonthlyPrice calculates the monthly price for an organization
func (sr *ServiceRegistry) CalculateMonthlyPrice(ctx context.Context, organizationID string) (float64, error) {
	config, err := sr.getOrganizationConfig(ctx, organizationID)
	if err != nil {
		return 0, err
	}

	if config == nil {
		// Default pricing for core services only
		pricing := entity.GetPricingModel()
		return pricing.Base, nil
	}

	return config.CalculateMonthlyPrice(), nil
}

// ListOrganizationConfigs returns a paginated list of organization configurations
func (sr *ServiceRegistry) ListOrganizationConfigs(ctx context.Context, limit, offset int) ([]*entity.OrganizationConfig, error) {
	return sr.configRepo.List(ctx, limit, offset)
}

// ClearCache clears the internal cache
func (sr *ServiceRegistry) ClearCache() {
	sr.cacheMutex.Lock()
	defer sr.cacheMutex.Unlock()
	sr.cache = make(map[string]*entity.OrganizationConfig)
	sr.logger.Info("Service registry cache cleared")
}

// Private helper methods

func (sr *ServiceRegistry) getOrganizationConfig(ctx context.Context, organizationID string) (*entity.OrganizationConfig, error) {
	// Check cache first
	sr.cacheMutex.RLock()
	if config, exists := sr.cache[organizationID]; exists {
		sr.cacheMutex.RUnlock()
		return config, nil
	}
	sr.cacheMutex.RUnlock()

	// Load from database
	config, err := sr.configRepo.GetByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Update cache if found
	if config != nil {
		sr.updateCache(organizationID, config)
	}

	return config, nil
}

func (sr *ServiceRegistry) updateCache(organizationID string, config *entity.OrganizationConfig) {
	sr.cacheMutex.Lock()
	defer sr.cacheMutex.Unlock()
	sr.cache[organizationID] = config
}

func (sr *ServiceRegistry) removeFromCache(organizationID string) {
	sr.cacheMutex.Lock()
	defer sr.cacheMutex.Unlock()
	delete(sr.cache, organizationID)
}

// GetAvailableTemplates returns all available organization templates
func (sr *ServiceRegistry) GetAvailableTemplates() []TemplateInfo {
	return []TemplateInfo{
		{
			Type:        entity.TEMPLATE_PADEL_ONLY,
			Name:        "Centro de Pádel",
			Description: "Configuración especializada para centros de pádel con funcionalidades específicas",
			Features:    []string{"Reservas", "Pagos", "Notificaciones", "Gamificación", "Wallet", "Pricing Dinámico", "Insights"},
			Price:       149.0,
		},
		{
			Type:        entity.TEMPLATE_SOCIAL_CLUB,
			Name:        "Club Social Multideporte",
			Description: "Configuración completa para clubs sociales con múltiples deportes y actividades",
			Features:    []string{"Todas las funcionalidades", "Membresías", "Campeonatos", "Gestión de instalaciones"},
			Price:       399.0,
		},
		{
			Type:        entity.TEMPLATE_FOOTBALL_CLUB,
			Name:        "Club de Fútbol",
			Description: "Configuración específica para clubs de fútbol con gestión de equipos y entrenamientos",
			Features:    []string{"Gestión de equipos", "Calendarios", "Reservas", "Membresías", "Insights"},
			Price:       199.0,
		},
		{
			Type:        entity.TEMPLATE_GYM_FITNESS,
			Name:        "Gimnasio / Centro Fitness",
			Description: "Configuración para gimnasios y centros fitness con clases grupales",
			Features:    []string{"Clases grupales", "Membresías", "Gamificación", "Wallet", "Pricing Dinámico"},
			Price:       249.0,
		},
		{
			Type:        entity.TEMPLATE_CUSTOM,
			Name:        "Configuración Personalizada",
			Description: "Template base que permite configuración manual de todas las funcionalidades",
			Features:    []string{"Configuración manual", "Servicios básicos"},
			Price:       49.0, // Base price
		},
	}
}

// TemplateInfo contains information about an organization template
type TemplateInfo struct {
	Type        entity.TemplateType `json:"type"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Features    []string            `json:"features"`
	Price       float64             `json:"price"`
}