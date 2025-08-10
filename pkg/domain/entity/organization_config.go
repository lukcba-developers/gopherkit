package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TemplateType represents the organization template types
type TemplateType string

const (
	TEMPLATE_PADEL_ONLY    TemplateType = "PADEL_ONLY"
	TEMPLATE_SOCIAL_CLUB   TemplateType = "SOCIAL_CLUB"
	TEMPLATE_FOOTBALL_CLUB TemplateType = "FOOTBALL_CLUB"
	TEMPLATE_GYM_FITNESS   TemplateType = "GYM_FITNESS"
	TEMPLATE_CUSTOM        TemplateType = "CUSTOM"
	
	// Alias for backwards compatibility
	TemplateTypeStartup    = TEMPLATE_PADEL_ONLY
	TemplateTypeEnterprise = TEMPLATE_CUSTOM
)

// OrganizationConfig represents the configuration for an organization
type OrganizationConfig struct {
	ID              string                 `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrganizationID  string                 `gorm:"type:uuid;not null;unique" json:"organization_id"`
	TemplateType    TemplateType           `gorm:"type:varchar(50);not null" json:"template_type"`
	IsActive        bool                   `gorm:"default:true" json:"is_active"`
	
	// Service configuration
	ServicesEnabled ServicesConfig         `gorm:"type:jsonb;not null" json:"services_enabled"`
	
	// Template-specific configuration
	TemplateConfig  map[string]interface{} `gorm:"type:jsonb" json:"template_config"`
	
	// Additional customizations
	Customizations  map[string]interface{} `gorm:"type:jsonb" json:"customizations"`
	
	CreatedAt       time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
}

// ServicesConfig defines which services are enabled for an organization
type ServicesConfig struct {
	// Core services (always enabled)
	AuthEnabled         bool `json:"auth_enabled"`
	UserEnabled         bool `json:"user_enabled"`
	
	// Optional services
	CalendarEnabled     bool `json:"calendar_enabled"`
	BookingEnabled      bool `json:"booking_enabled"`
	ChampionshipEnabled bool `json:"championship_enabled"`
	MembershipEnabled   bool `json:"membership_enabled"`
	FacilitiesEnabled   bool `json:"facilities_enabled"`
	NotificationEnabled bool `json:"notification_enabled"`
	PaymentsEnabled     bool `json:"payments_enabled"`
	
	// Premium features
	GamificationEnabled bool `json:"gamification_enabled"`
	WalletEnabled       bool `json:"wallet_enabled"`
	GroupBookingEnabled bool `json:"group_booking_enabled"`
	DynamicPricingEnabled bool `json:"dynamic_pricing_enabled"`
	InsightsEnabled     bool `json:"insights_enabled"`
}

// PricingModel defines the pricing structure for services
type PricingModel struct {
	Base     float64            `json:"base"`
	Services map[string]float64 `json:"services"`
	Features map[string]float64 `json:"features"`
	Discounts map[TemplateType]float64 `json:"template_discounts"`
}

// GetPricingModel returns the current pricing model
func GetPricingModel() *PricingModel {
	return &PricingModel{
		Base: 49.0,
		Services: map[string]float64{
			"calendar":     20.0,
			"booking":      30.0,
			"championship": 25.0,
			"membership":   20.0,
			"facilities":   15.0,
			"notification": 10.0,
			"payments":     25.0,
		},
		Features: map[string]float64{
			"gamification":    30.0,
			"wallet":          40.0,
			"groupBooking":    20.0,
			"dynamicPricing":  50.0,
			"insights":        40.0,
		},
		Discounts: map[TemplateType]float64{
			TEMPLATE_PADEL_ONLY:    0.15, // 15% discount
			TEMPLATE_SOCIAL_CLUB:   0.0,  // No discount
			TEMPLATE_FOOTBALL_CLUB: 0.20, // 20% discount
			TEMPLATE_GYM_FITNESS:   0.10, // 10% discount
			TEMPLATE_CUSTOM:        0.0,  // No discount
		},
	}
}

// NewOrganizationConfig creates a new organization configuration with template defaults
func NewOrganizationConfig(organizationID string, templateType TemplateType) *OrganizationConfig {
	config := &OrganizationConfig{
		ID:             uuid.New().String(),
		OrganizationID: organizationID,
		TemplateType:   templateType,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	config.applyTemplate(templateType)
	return config
}

// applyTemplate applies the default configuration for a template type
func (oc *OrganizationConfig) applyTemplate(templateType TemplateType) {
	// Always enable core services
	oc.ServicesEnabled.AuthEnabled = true
	oc.ServicesEnabled.UserEnabled = true
	
	switch templateType {
	case TEMPLATE_PADEL_ONLY:
		oc.ServicesEnabled = ServicesConfig{
			AuthEnabled:         true,
			UserEnabled:         true,
			CalendarEnabled:     true,
			BookingEnabled:      true,
			FacilitiesEnabled:   true,
			PaymentsEnabled:     true,
			NotificationEnabled: true,
			ChampionshipEnabled: false,
			MembershipEnabled:   false,
			GamificationEnabled: true,
			WalletEnabled:       true,
			GroupBookingEnabled: true,
			DynamicPricingEnabled: true,
			InsightsEnabled:     true,
		}
		oc.TemplateConfig = map[string]interface{}{
			"sports":     []string{"padel"},
			"facilities": []string{"padel_court_indoor", "padel_court_outdoor"},
			"courtConfig": map[string]interface{}{
				"defaultDuration":     90,
				"maxAdvanceBooking":   30,
				"cancellationPolicy":  24,
				"maxPlayersPerCourt":  4,
			},
			"pricing": map[string]interface{}{
				"basePrice":         20,
				"peakHours":         []string{"17:00-21:00"},
				"peakMultiplier":    1.5,
				"weekendMultiplier": 1.2,
			},
		}
		
	case TEMPLATE_SOCIAL_CLUB:
		// Enable all services for social club
		oc.ServicesEnabled = ServicesConfig{
			AuthEnabled:         true,
			UserEnabled:         true,
			CalendarEnabled:     true,
			BookingEnabled:      true,
			ChampionshipEnabled: true,
			MembershipEnabled:   true,
			FacilitiesEnabled:   true,
			NotificationEnabled: true,
			PaymentsEnabled:     true,
			GamificationEnabled: true,
			WalletEnabled:       true,
			GroupBookingEnabled: true,
			DynamicPricingEnabled: true,
			InsightsEnabled:     true,
		}
		oc.TemplateConfig = map[string]interface{}{
			"sports":     []string{"padel", "tennis", "swimming", "gym", "football"},
			"facilities": []string{"padel_court", "tennis_court", "swimming_pool", "gym", "football_field", "restaurant", "parking"},
			"membershipTypes": []map[string]interface{}{
				{"name": "Individual", "price": 50, "duration": "monthly"},
				{"name": "Familiar", "price": 120, "duration": "monthly"},
				{"name": "Senior", "price": 35, "duration": "monthly"},
				{"name": "Junior", "price": 30, "duration": "monthly"},
			},
		}
		
	case TEMPLATE_FOOTBALL_CLUB:
		oc.ServicesEnabled = ServicesConfig{
			AuthEnabled:         true,
			UserEnabled:         true,
			CalendarEnabled:     true,
			BookingEnabled:      true,
			ChampionshipEnabled: true,
			MembershipEnabled:   true,
			FacilitiesEnabled:   true,
			NotificationEnabled: true,
			PaymentsEnabled:     true,
			GamificationEnabled: false,
			WalletEnabled:       false,
			GroupBookingEnabled: true,
			DynamicPricingEnabled: false,
			InsightsEnabled:     true,
		}
		oc.TemplateConfig = map[string]interface{}{
			"sports":     []string{"football", "football_7", "football_sala"},
			"facilities": []string{"football_field", "football_7_field", "gym", "changing_room"},
			"teamCategories": []string{"prebenjamin", "benjamin", "alevin", "infantil", "cadete", "juvenil", "senior"},
			"trainingSchedule": map[string]interface{}{
				"sessionsPerWeek":   3,
				"sessionDuration":   90,
				"matchDay":          "saturday",
			},
		}
		
	case TEMPLATE_GYM_FITNESS:
		oc.ServicesEnabled = ServicesConfig{
			AuthEnabled:         true,
			UserEnabled:         true,
			CalendarEnabled:     true,
			BookingEnabled:      true,
			ChampionshipEnabled: false,
			MembershipEnabled:   true,
			FacilitiesEnabled:   true,
			NotificationEnabled: true,
			PaymentsEnabled:     true,
			GamificationEnabled: true,
			WalletEnabled:       true,
			GroupBookingEnabled: true,
			DynamicPricingEnabled: true,
			InsightsEnabled:     true,
		}
		oc.TemplateConfig = map[string]interface{}{
			"sports":     []string{"fitness", "crossfit", "yoga", "pilates", "spinning"},
			"facilities": []string{"gym_floor", "group_class_room", "spinning_room", "changing_room"},
			"classes": []map[string]interface{}{
				{"name": "Spinning", "duration": 45, "maxCapacity": 20},
				{"name": "Yoga", "duration": 60, "maxCapacity": 15},
				{"name": "CrossFit", "duration": 60, "maxCapacity": 12},
			},
		}
		
	case TEMPLATE_CUSTOM:
		// Only core services enabled for custom template
		oc.ServicesEnabled = ServicesConfig{
			AuthEnabled: true,
			UserEnabled: true,
		}
		oc.TemplateConfig = map[string]interface{}{}
	}
}

// EnableService enables a specific service
func (oc *OrganizationConfig) EnableService(serviceName string) error {
	if err := oc.validateServiceName(serviceName); err != nil {
		return err
	}
	
	switch strings.ToLower(serviceName) {
	case "calendar":
		oc.ServicesEnabled.CalendarEnabled = true
	case "booking":
		oc.ServicesEnabled.BookingEnabled = true
	case "championship":
		oc.ServicesEnabled.ChampionshipEnabled = true
	case "membership":
		oc.ServicesEnabled.MembershipEnabled = true
	case "facilities":
		oc.ServicesEnabled.FacilitiesEnabled = true
	case "notification":
		oc.ServicesEnabled.NotificationEnabled = true
	case "payments":
		oc.ServicesEnabled.PaymentsEnabled = true
	case "gamification":
		oc.ServicesEnabled.GamificationEnabled = true
	case "wallet":
		oc.ServicesEnabled.WalletEnabled = true
	case "groupbooking":
		oc.ServicesEnabled.GroupBookingEnabled = true
	case "dynamicpricing":
		oc.ServicesEnabled.DynamicPricingEnabled = true
	case "insights":
		oc.ServicesEnabled.InsightsEnabled = true
	default:
		return errors.New("unknown service: " + serviceName)
	}
	
	oc.UpdatedAt = time.Now()
	return nil
}

// DisableService disables a specific service
func (oc *OrganizationConfig) DisableService(serviceName string) error {
	// Cannot disable core services
	if serviceName == "auth" || serviceName == "user" {
		return errors.New("cannot disable core service: " + serviceName)
	}
	
	if err := oc.validateServiceName(serviceName); err != nil {
		return err
	}
	
	switch strings.ToLower(serviceName) {
	case "calendar":
		oc.ServicesEnabled.CalendarEnabled = false
	case "booking":
		oc.ServicesEnabled.BookingEnabled = false
	case "championship":
		oc.ServicesEnabled.ChampionshipEnabled = false
	case "membership":
		oc.ServicesEnabled.MembershipEnabled = false
	case "facilities":
		oc.ServicesEnabled.FacilitiesEnabled = false
	case "notification":
		oc.ServicesEnabled.NotificationEnabled = false
	case "payments":
		oc.ServicesEnabled.PaymentsEnabled = false
	case "gamification":
		oc.ServicesEnabled.GamificationEnabled = false
	case "wallet":
		oc.ServicesEnabled.WalletEnabled = false
	case "groupbooking":
		oc.ServicesEnabled.GroupBookingEnabled = false
	case "dynamicpricing":
		oc.ServicesEnabled.DynamicPricingEnabled = false
	case "insights":
		oc.ServicesEnabled.InsightsEnabled = false
	default:
		return errors.New("unknown service: " + serviceName)
	}
	
	oc.UpdatedAt = time.Now()
	return nil
}

// IsServiceEnabled checks if a service is enabled
func (oc *OrganizationConfig) IsServiceEnabled(serviceName string) bool {
	switch strings.ToLower(serviceName) {
	case "auth":
		return oc.ServicesEnabled.AuthEnabled
	case "user":
		return oc.ServicesEnabled.UserEnabled
	case "calendar":
		return oc.ServicesEnabled.CalendarEnabled
	case "booking":
		return oc.ServicesEnabled.BookingEnabled
	case "championship":
		return oc.ServicesEnabled.ChampionshipEnabled
	case "membership":
		return oc.ServicesEnabled.MembershipEnabled
	case "facilities":
		return oc.ServicesEnabled.FacilitiesEnabled
	case "notification":
		return oc.ServicesEnabled.NotificationEnabled
	case "payments":
		return oc.ServicesEnabled.PaymentsEnabled
	case "gamification":
		return oc.ServicesEnabled.GamificationEnabled
	case "wallet":
		return oc.ServicesEnabled.WalletEnabled
	case "groupbooking":
		return oc.ServicesEnabled.GroupBookingEnabled
	case "dynamicpricing":
		return oc.ServicesEnabled.DynamicPricingEnabled
	case "insights":
		return oc.ServicesEnabled.InsightsEnabled
	default:
		return false
	}
}

// GetEnabledServices returns a list of enabled services
func (oc *OrganizationConfig) GetEnabledServices() []string {
	var services []string
	
	if oc.ServicesEnabled.AuthEnabled {
		services = append(services, "auth")
	}
	if oc.ServicesEnabled.UserEnabled {
		services = append(services, "user")
	}
	if oc.ServicesEnabled.CalendarEnabled {
		services = append(services, "calendar")
	}
	if oc.ServicesEnabled.BookingEnabled {
		services = append(services, "booking")
	}
	if oc.ServicesEnabled.ChampionshipEnabled {
		services = append(services, "championship")
	}
	if oc.ServicesEnabled.MembershipEnabled {
		services = append(services, "membership")
	}
	if oc.ServicesEnabled.FacilitiesEnabled {
		services = append(services, "facilities")
	}
	if oc.ServicesEnabled.NotificationEnabled {
		services = append(services, "notification")
	}
	if oc.ServicesEnabled.PaymentsEnabled {
		services = append(services, "payments")
	}
	if oc.ServicesEnabled.GamificationEnabled {
		services = append(services, "gamification")
	}
	if oc.ServicesEnabled.WalletEnabled {
		services = append(services, "wallet")
	}
	if oc.ServicesEnabled.GroupBookingEnabled {
		services = append(services, "groupbooking")
	}
	if oc.ServicesEnabled.DynamicPricingEnabled {
		services = append(services, "dynamicpricing")
	}
	if oc.ServicesEnabled.InsightsEnabled {
		services = append(services, "insights")
	}
	
	return services
}

// CalculateMonthlyPrice calculates the monthly price based on enabled services
func (oc *OrganizationConfig) CalculateMonthlyPrice() float64 {
	pricing := GetPricingModel()
	price := pricing.Base
	
	// Add service costs
	if oc.ServicesEnabled.CalendarEnabled {
		price += pricing.Services["calendar"]
	}
	if oc.ServicesEnabled.BookingEnabled {
		price += pricing.Services["booking"]
	}
	if oc.ServicesEnabled.ChampionshipEnabled {
		price += pricing.Services["championship"]
	}
	if oc.ServicesEnabled.MembershipEnabled {
		price += pricing.Services["membership"]
	}
	if oc.ServicesEnabled.FacilitiesEnabled {
		price += pricing.Services["facilities"]
	}
	if oc.ServicesEnabled.NotificationEnabled {
		price += pricing.Services["notification"]
	}
	if oc.ServicesEnabled.PaymentsEnabled {
		price += pricing.Services["payments"]
	}
	
	// Add feature costs
	if oc.ServicesEnabled.GamificationEnabled {
		price += pricing.Features["gamification"]
	}
	if oc.ServicesEnabled.WalletEnabled {
		price += pricing.Features["wallet"]
	}
	if oc.ServicesEnabled.GroupBookingEnabled {
		price += pricing.Features["groupBooking"]
	}
	if oc.ServicesEnabled.DynamicPricingEnabled {
		price += pricing.Features["dynamicPricing"]
	}
	if oc.ServicesEnabled.InsightsEnabled {
		price += pricing.Features["insights"]
	}
	
	// Apply template discount
	if discount, exists := pricing.Discounts[oc.TemplateType]; exists {
		price = price * (1 - discount)
	}
	
	return price
}

// Validate validates the organization configuration
func (oc *OrganizationConfig) Validate() error {
	var errors []string
	
	if oc.OrganizationID == "" {
		errors = append(errors, "organization_id is required")
	}
	
	if !oc.isValidTemplateType() {
		errors = append(errors, "invalid template type: "+string(oc.TemplateType))
	}
	
	// Core services must always be enabled
	if !oc.ServicesEnabled.AuthEnabled {
		errors = append(errors, "auth service must be enabled")
	}
	if !oc.ServicesEnabled.UserEnabled {
		errors = append(errors, "user service must be enabled")
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("%s", errors[0]) // Return first error for simplicity
	}
	
	return nil
}

// validateServiceName validates if a service name is valid
func (oc *OrganizationConfig) validateServiceName(serviceName string) error {
	validServices := []string{
		"auth", "user", "calendar", "booking", "championship", 
		"membership", "facilities", "notification", "payments",
		"gamification", "wallet", "groupbooking", "dynamicpricing", "insights",
	}
	
	serviceName = strings.ToLower(serviceName)
	for _, valid := range validServices {
		if serviceName == valid {
			return nil
		}
	}
	
	return errors.New("invalid service name: " + serviceName)
}

// isValidTemplateType checks if the template type is valid
func (oc *OrganizationConfig) isValidTemplateType() bool {
	validTypes := []TemplateType{
		TEMPLATE_PADEL_ONLY,
		TEMPLATE_SOCIAL_CLUB,
		TEMPLATE_FOOTBALL_CLUB,
		TEMPLATE_GYM_FITNESS,
		TEMPLATE_CUSTOM,
	}
	
	for _, valid := range validTypes {
		if oc.TemplateType == valid {
			return true
		}
	}
	
	return false
}