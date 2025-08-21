package entity

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// SkillLevel representa los niveles de habilidad disponibles
type SkillLevel string

const (
	SkillLevelBeginner     SkillLevel = "BEGINNER"
	SkillLevelIntermediate SkillLevel = "INTERMEDIATE"
	SkillLevelAdvanced     SkillLevel = "ADVANCED"
	SkillLevelExpert       SkillLevel = "EXPERT"
)

// User representa un usuario en el sistema migrado con GopherKit
type User struct {
	ID                string          `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Email             string          `json:"email" gorm:"uniqueIndex;not null;size:255"`
	Name              string          `json:"name" gorm:"not null;size:255"`
	Phone             *string         `json:"phone,omitempty" gorm:"size:20"`
	DateOfBirth       *time.Time      `json:"date_of_birth,omitempty"`
	ProfilePictureURL *string         `json:"profile_picture_url,omitempty" gorm:"size:500"`
	SportsPreferences json.RawMessage `json:"sports_preferences,omitempty" gorm:"type:jsonb"`
	SkillLevel        *SkillLevel     `json:"skill_level,omitempty" gorm:"type:varchar(20)"`
	IsActive          bool            `json:"is_active" gorm:"default:true"`
	TenantID          string          `json:"tenant_id" gorm:"not null;size:255;index"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	DeletedAt         gorm.DeletedAt  `json:"-" gorm:"index"`
}

// TableName especifica el nombre de la tabla en la base de datos
func (User) TableName() string {
	return "users"
}

// NewUser crea una nueva instancia de User
func NewUser(email, name, tenantID string) *User {
	return &User{
		Email:     email,
		Name:      name,
		IsActive:  true,
		TenantID:  tenantID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// BeforeCreate hook de GORM ejecutado antes de crear el registro
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = time.Now()
	}
	return
}

// BeforeUpdate hook de GORM ejecutado antes de actualizar el registro
func (u *User) BeforeUpdate(tx *gorm.DB) (err error) {
	u.UpdatedAt = time.Now()
	return
}

// IsValidSkillLevel verifica si el nivel de habilidad es válido
func (u *User) IsValidSkillLevel() bool {
	if u.SkillLevel == nil {
		return true // null is valid
	}
	
	validLevels := []SkillLevel{
		SkillLevelBeginner,
		SkillLevelIntermediate,
		SkillLevelAdvanced,
		SkillLevelExpert,
	}
	
	for _, level := range validLevels {
		if *u.SkillLevel == level {
			return true
		}
	}
	return false
}

// GetSportsPreferences deserializa las preferencias deportivas
func (u *User) GetSportsPreferences() (map[string]interface{}, error) {
	if len(u.SportsPreferences) == 0 {
		return make(map[string]interface{}), nil
	}
	
	var prefs map[string]interface{}
	err := json.Unmarshal(u.SportsPreferences, &prefs)
	return prefs, err
}

// SetSportsPreferences serializa las preferencias deportivas
func (u *User) SetSportsPreferences(prefs map[string]interface{}) error {
	if prefs == nil {
		u.SportsPreferences = nil
		return nil
	}
	
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	u.SportsPreferences = data
	return nil
}

// UserStats representa las estadísticas básicas de un usuario
type UserStats struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           string    `json:"user_id" gorm:"not null;size:255;index"`
	TotalMatches     int       `json:"total_matches" gorm:"default:0"`
	TotalWins        int       `json:"total_wins" gorm:"default:0"`
	TotalLosses      int       `json:"total_losses" gorm:"default:0"`
	WinRate          float64   `json:"win_rate" gorm:"default:0"`
	Points           int       `json:"points" gorm:"default:0"`
	Level            int       `json:"level" gorm:"default:1"`
	LastMatchDate    *time.Time `json:"last_match_date,omitempty"`
	CurrentStreak    int       `json:"current_streak" gorm:"default:0"`
	LongestStreak    int       `json:"longest_streak" gorm:"default:0"`
	TenantID         string    `json:"tenant_id" gorm:"not null;size:255;index"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	
	// Relación con User
	User User `json:"user,omitempty" gorm:"foreignKey:UserID;references:ID"`
}

// TableName especifica el nombre de la tabla para UserStats
func (UserStats) TableName() string {
	return "user_stats"
}

// UpdateWinRate calcula y actualiza la tasa de victorias
func (us *UserStats) UpdateWinRate() {
	if us.TotalMatches > 0 {
		us.WinRate = float64(us.TotalWins) / float64(us.TotalMatches)
	} else {
		us.WinRate = 0
	}
}

// AddWin registra una victoria
func (us *UserStats) AddWin() {
	us.TotalMatches++
	us.TotalWins++
	us.CurrentStreak++
	if us.CurrentStreak > us.LongestStreak {
		us.LongestStreak = us.CurrentStreak
	}
	us.UpdateWinRate()
	now := time.Now()
	us.LastMatchDate = &now
}

// AddLoss registra una derrota
func (us *UserStats) AddLoss() {
	us.TotalMatches++
	us.TotalLosses++
	us.CurrentStreak = 0 // Reset streak on loss
	us.UpdateWinRate()
	now := time.Now()
	us.LastMatchDate = &now
}