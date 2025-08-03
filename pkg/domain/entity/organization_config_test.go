package entity

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestOrganizationConfig_Creation(t *testing.T) {
	// Arrange
	orgID := uuid.New().String()
	
	// Act
	config := NewOrganizationConfig(orgID, TEMPLATE_PADEL_ONLY)
	
	// Assert
	assert.NotEmpty(t, config.ID)
	assert.Equal(t, orgID, config.OrganizationID)
	assert.Equal(t, TEMPLATE_PADEL_ONLY, config.TemplateType)
	assert.True(t, config.IsActive)
	assert.True(t, config.ServicesEnabled.AuthEnabled)
	assert.True(t, config.ServicesEnabled.UserEnabled)
	assert.True(t, config.ServicesEnabled.BookingEnabled)
	assert.False(t, config.ServicesEnabled.ChampionshipEnabled) // Padel template doesn't include championships by default
}

func TestOrganizationConfig_EnableService(t *testing.T) {
	// Arrange
	config := NewOrganizationConfig(uuid.New().String(), TEMPLATE_PADEL_ONLY)
	
	// Act
	err := config.EnableService("championship")
	
	// Assert
	assert.NoError(t, err)
	assert.True(t, config.ServicesEnabled.ChampionshipEnabled)
}

func TestOrganizationConfig_DisableService(t *testing.T) {
	// Arrange
	config := NewOrganizationConfig(uuid.New().String(), TEMPLATE_SOCIAL_CLUB)
	
	// Act
	err := config.DisableService("championship")
	
	// Assert
	assert.NoError(t, err)
	assert.False(t, config.ServicesEnabled.ChampionshipEnabled)
}

func TestOrganizationConfig_CannotDisableCoreServices(t *testing.T) {
	// Arrange
	config := NewOrganizationConfig(uuid.New().String(), TEMPLATE_PADEL_ONLY)
	
	// Act & Assert
	err := config.DisableService("auth")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot disable core service")
	
	err = config.DisableService("user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot disable core service")
}

func TestOrganizationConfig_CalculateMonthlyPrice(t *testing.T) {
	tests := []struct {
		name     string
		template TemplateType
		expected float64
	}{
		{
			name:     "Padel Only Template",
			template: TEMPLATE_PADEL_ONLY,
			expected: 149.0, // With 15% discount
		},
		{
			name:     "Social Club Template",
			template: TEMPLATE_SOCIAL_CLUB,
			expected: 399.0, // No discount, full price
		},
		{
			name:     "Football Club Template",
			template: TEMPLATE_FOOTBALL_CLUB,
			expected: 199.0, // With 20% discount
		},
		{
			name:     "Gym Fitness Template",
			template: TEMPLATE_GYM_FITNESS,
			expected: 249.0, // With 10% discount
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			config := NewOrganizationConfig(uuid.New().String(), tt.template)
			
			// Act
			price := config.CalculateMonthlyPrice()
			
			// Assert
			assert.Equal(t, tt.expected, price)
		})
	}
}

func TestOrganizationConfig_IsServiceEnabled(t *testing.T) {
	// Arrange
	config := NewOrganizationConfig(uuid.New().String(), TEMPLATE_PADEL_ONLY)
	
	// Act & Assert
	assert.True(t, config.IsServiceEnabled("booking"))
	assert.False(t, config.IsServiceEnabled("championship"))
	assert.False(t, config.IsServiceEnabled("nonexistent"))
}

func TestOrganizationConfig_GetEnabledServices(t *testing.T) {
	// Arrange
	config := NewOrganizationConfig(uuid.New().String(), TEMPLATE_PADEL_ONLY)
	
	// Act
	services := config.GetEnabledServices()
	
	// Assert
	assert.Contains(t, services, "auth")
	assert.Contains(t, services, "user")
	assert.Contains(t, services, "booking")
	assert.Contains(t, services, "gamification")
	assert.NotContains(t, services, "championship")
}

func TestOrganizationConfig_Validation(t *testing.T) {
	// Arrange
	config := &OrganizationConfig{
		ID:             uuid.New().String(),
		OrganizationID: "", // Invalid: empty
		TemplateType:   "INVALID_TEMPLATE",
		IsActive:       true,
	}
	
	// Act
	err := config.Validate()
	
	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization_id is required")
	assert.Contains(t, err.Error(), "invalid template type")
}