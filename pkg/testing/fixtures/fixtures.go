package fixtures

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserFixture representa un usuario de prueba
type UserFixture struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Sport     string    `json:"sport"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrganizationFixture representa una organización de prueba
type OrganizationFixture struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Settings  string    `json:"settings"` // JSON string
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BookingFixture representa una reserva de prueba
type BookingFixture struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id"`
	FacilityID string    `json:"facility_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Status     string    `json:"status"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PaymentFixture representa un pago de prueba
type PaymentFixture struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	UserID        string    `json:"user_id"`
	BookingID     string    `json:"booking_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	PaymentMethod string    `json:"payment_method"`
	ExternalID    string    `json:"external_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NotificationFixture representa una notificación de prueba
type NotificationFixture struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Status    string    `json:"status"`
	Channel   string    `json:"channel"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FixtureManager maneja la creación de datos de prueba
type FixtureManager struct {
	db       *sql.DB
	gormDB   *gorm.DB
	tenantID string
}

// NewFixtureManager crea un nuevo manager de fixtures
func NewFixtureManager(db *sql.DB, gormDB *gorm.DB, tenantID string) *FixtureManager {
	return &FixtureManager{
		db:       db,
		gormDB:   gormDB,
		tenantID: tenantID,
	}
}

// CreateUser crea un usuario de prueba
func (fm *FixtureManager) CreateUser(overrides ...*UserFixture) *UserFixture {
	user := &UserFixture{
		ID:        uuid.New().String(),
		TenantID:  fm.tenantID,
		Name:      "Test User",
		Email:     fmt.Sprintf("user_%s@test.com", uuid.New().String()[:8]),
		Sport:     "tennis",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Apply overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.ID != "" {
			user.ID = override.ID
		}
		if override.TenantID != "" {
			user.TenantID = override.TenantID
		}
		if override.Name != "" {
			user.Name = override.Name
		}
		if override.Email != "" {
			user.Email = override.Email
		}
		if override.Sport != "" {
			user.Sport = override.Sport
		}
		user.IsActive = override.IsActive
		if !override.CreatedAt.IsZero() {
			user.CreatedAt = override.CreatedAt
		}
		if !override.UpdatedAt.IsZero() {
			user.UpdatedAt = override.UpdatedAt
		}
	}

	return user
}

// CreateOrganization crea una organización de prueba
func (fm *FixtureManager) CreateOrganization(overrides ...*OrganizationFixture) *OrganizationFixture {
	org := &OrganizationFixture{
		ID:        uuid.New().String(),
		TenantID:  fm.tenantID,
		Name:      "Test Organization",
		Type:      "club",
		Settings:  `{"theme": "default", "language": "es"}`,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Apply overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.ID != "" {
			org.ID = override.ID
		}
		if override.TenantID != "" {
			org.TenantID = override.TenantID
		}
		if override.Name != "" {
			org.Name = override.Name
		}
		if override.Type != "" {
			org.Type = override.Type
		}
		if override.Settings != "" {
			org.Settings = override.Settings
		}
		org.IsActive = override.IsActive
		if !override.CreatedAt.IsZero() {
			org.CreatedAt = override.CreatedAt
		}
		if !override.UpdatedAt.IsZero() {
			org.UpdatedAt = override.UpdatedAt
		}
	}

	return org
}

// CreateBooking crea una reserva de prueba
func (fm *FixtureManager) CreateBooking(userID, facilityID string, overrides ...*BookingFixture) *BookingFixture {
	startTime := time.Now().Add(24 * time.Hour) // Tomorrow
	booking := &BookingFixture{
		ID:         uuid.New().String(),
		TenantID:   fm.tenantID,
		UserID:     userID,
		FacilityID: facilityID,
		StartTime:  startTime,
		EndTime:    startTime.Add(time.Hour),
		Status:     "confirmed",
		Amount:     50.00,
		Currency:   "EUR",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Apply overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.ID != "" {
			booking.ID = override.ID
		}
		if override.TenantID != "" {
			booking.TenantID = override.TenantID
		}
		if override.UserID != "" {
			booking.UserID = override.UserID
		}
		if override.FacilityID != "" {
			booking.FacilityID = override.FacilityID
		}
		if !override.StartTime.IsZero() {
			booking.StartTime = override.StartTime
		}
		if !override.EndTime.IsZero() {
			booking.EndTime = override.EndTime
		}
		if override.Status != "" {
			booking.Status = override.Status
		}
		if override.Amount != 0 {
			booking.Amount = override.Amount
		}
		if override.Currency != "" {
			booking.Currency = override.Currency
		}
		if !override.CreatedAt.IsZero() {
			booking.CreatedAt = override.CreatedAt
		}
		if !override.UpdatedAt.IsZero() {
			booking.UpdatedAt = override.UpdatedAt
		}
	}

	return booking
}

// CreatePayment crea un pago de prueba
func (fm *FixtureManager) CreatePayment(userID, bookingID string, overrides ...*PaymentFixture) *PaymentFixture {
	payment := &PaymentFixture{
		ID:            uuid.New().String(),
		TenantID:      fm.tenantID,
		UserID:        userID,
		BookingID:     bookingID,
		Amount:        50.00,
		Currency:      "EUR",
		Status:        "completed",
		PaymentMethod: "credit_card",
		ExternalID:    fmt.Sprintf("ext_%s", uuid.New().String()[:8]),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Apply overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.ID != "" {
			payment.ID = override.ID
		}
		if override.TenantID != "" {
			payment.TenantID = override.TenantID
		}
		if override.UserID != "" {
			payment.UserID = override.UserID
		}
		if override.BookingID != "" {
			payment.BookingID = override.BookingID
		}
		if override.Amount != 0 {
			payment.Amount = override.Amount
		}
		if override.Currency != "" {
			payment.Currency = override.Currency
		}
		if override.Status != "" {
			payment.Status = override.Status
		}
		if override.PaymentMethod != "" {
			payment.PaymentMethod = override.PaymentMethod
		}
		if override.ExternalID != "" {
			payment.ExternalID = override.ExternalID
		}
		if !override.CreatedAt.IsZero() {
			payment.CreatedAt = override.CreatedAt
		}
		if !override.UpdatedAt.IsZero() {
			payment.UpdatedAt = override.UpdatedAt
		}
	}

	return payment
}

// CreateNotification crea una notificación de prueba
func (fm *FixtureManager) CreateNotification(userID string, overrides ...*NotificationFixture) *NotificationFixture {
	notification := &NotificationFixture{
		ID:        uuid.New().String(),
		TenantID:  fm.tenantID,
		UserID:    userID,
		Type:      "booking_confirmation",
		Title:     "Booking Confirmed",
		Message:   "Your booking has been confirmed successfully",
		Status:    "sent",
		Channel:   "email",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Apply overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.ID != "" {
			notification.ID = override.ID
		}
		if override.TenantID != "" {
			notification.TenantID = override.TenantID
		}
		if override.UserID != "" {
			notification.UserID = override.UserID
		}
		if override.Type != "" {
			notification.Type = override.Type
		}
		if override.Title != "" {
			notification.Title = override.Title
		}
		if override.Message != "" {
			notification.Message = override.Message
		}
		if override.Status != "" {
			notification.Status = override.Status
		}
		if override.Channel != "" {
			notification.Channel = override.Channel
		}
		if !override.CreatedAt.IsZero() {
			notification.CreatedAt = override.CreatedAt
		}
		if !override.UpdatedAt.IsZero() {
			notification.UpdatedAt = override.UpdatedAt
		}
	}

	return notification
}

// CreateTestScenario crea un escenario completo de prueba
func (fm *FixtureManager) CreateTestScenario() *TestScenario {
	// Create basic entities
	user := fm.CreateUser()
	org := fm.CreateOrganization()
	facilityID := uuid.New().String()
	booking := fm.CreateBooking(user.ID, facilityID)
	payment := fm.CreatePayment(user.ID, booking.ID)
	notification := fm.CreateNotification(user.ID)

	return &TestScenario{
		User:         user,
		Organization: org,
		FacilityID:   facilityID,
		Booking:      booking,
		Payment:      payment,
		Notification: notification,
	}
}

// TestScenario representa un escenario completo de prueba
type TestScenario struct {
	User         *UserFixture         `json:"user"`
	Organization *OrganizationFixture `json:"organization"`
	FacilityID   string               `json:"facility_id"`
	Booking      *BookingFixture      `json:"booking"`
	Payment      *PaymentFixture      `json:"payment"`
	Notification *NotificationFixture `json:"notification"`
}

// CleanupFixtures limpia todos los fixtures creados
func (fm *FixtureManager) CleanupFixtures() error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// List of tables to clean (in dependency order)
	tables := []string{
		"notifications",
		"payments", 
		"bookings",
		"users",
		"organizations",
		"facilities",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE tenant_id = $1", table)
		if _, err := fm.db.Exec(query, fm.tenantID); err != nil {
			// Log error but continue with cleanup
			continue
		}
	}

	return nil
}

// InsertUser inserta un usuario en la base de datos
func (fm *FixtureManager) InsertUser(user *UserFixture) error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO users (id, tenant_id, name, email, sport, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := fm.db.Exec(query, user.ID, user.TenantID, user.Name, user.Email, 
		user.Sport, user.IsActive, user.CreatedAt, user.UpdatedAt)
	return err
}

// InsertOrganization inserta una organización en la base de datos
func (fm *FixtureManager) InsertOrganization(org *OrganizationFixture) error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO organizations (id, tenant_id, name, type, settings, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := fm.db.Exec(query, org.ID, org.TenantID, org.Name, org.Type,
		org.Settings, org.IsActive, org.CreatedAt, org.UpdatedAt)
	return err
}

// InsertBooking inserta una reserva en la base de datos
func (fm *FixtureManager) InsertBooking(booking *BookingFixture) error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO bookings (id, tenant_id, user_id, facility_id, start_time, end_time, status, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := fm.db.Exec(query, booking.ID, booking.TenantID, booking.UserID, booking.FacilityID,
		booking.StartTime, booking.EndTime, booking.Status, booking.Amount, booking.Currency, 
		booking.CreatedAt, booking.UpdatedAt)
	return err
}

// InsertPayment inserta un pago en la base de datos
func (fm *FixtureManager) InsertPayment(payment *PaymentFixture) error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO payments (id, tenant_id, user_id, booking_id, amount, currency, status, payment_method, external_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := fm.db.Exec(query, payment.ID, payment.TenantID, payment.UserID, payment.BookingID,
		payment.Amount, payment.Currency, payment.Status, payment.PaymentMethod, payment.ExternalID,
		payment.CreatedAt, payment.UpdatedAt)
	return err
}

// InsertNotification inserta una notificación en la base de datos
func (fm *FixtureManager) InsertNotification(notification *NotificationFixture) error {
	if fm.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO notifications (id, tenant_id, user_id, type, title, message, status, channel, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := fm.db.Exec(query, notification.ID, notification.TenantID, notification.UserID, notification.Type,
		notification.Title, notification.Message, notification.Status, notification.Channel,
		notification.CreatedAt, notification.UpdatedAt)
	return err
}

// CreateAndInsertUser crea e inserta un usuario en la base de datos
func (fm *FixtureManager) CreateAndInsertUser(overrides ...*UserFixture) (*UserFixture, error) {
	user := fm.CreateUser(overrides...)
	err := fm.InsertUser(user)
	return user, err
}

// CreateAndInsertOrganization crea e inserta una organización en la base de datos
func (fm *FixtureManager) CreateAndInsertOrganization(overrides ...*OrganizationFixture) (*OrganizationFixture, error) {
	org := fm.CreateOrganization(overrides...)
	err := fm.InsertOrganization(org)
	return org, err
}

// CreateAndInsertBooking crea e inserta una reserva en la base de datos
func (fm *FixtureManager) CreateAndInsertBooking(userID, facilityID string, overrides ...*BookingFixture) (*BookingFixture, error) {
	booking := fm.CreateBooking(userID, facilityID, overrides...)
	err := fm.InsertBooking(booking)
	return booking, err
}

// CreateAndInsertPayment crea e inserta un pago en la base de datos
func (fm *FixtureManager) CreateAndInsertPayment(userID, bookingID string, overrides ...*PaymentFixture) (*PaymentFixture, error) {
	payment := fm.CreatePayment(userID, bookingID, overrides...)
	err := fm.InsertPayment(payment)
	return payment, err
}

// CreateAndInsertNotification crea e inserta una notificación en la base de datos
func (fm *FixtureManager) CreateAndInsertNotification(userID string, overrides ...*NotificationFixture) (*NotificationFixture, error) {
	notification := fm.CreateNotification(userID, overrides...)
	err := fm.InsertNotification(notification)
	return notification, err
}