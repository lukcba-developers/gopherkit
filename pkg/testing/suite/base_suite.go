package suite

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"gorm.io/gorm"
)

// BaseSuite proporciona funcionalidad de testing común para todos los servicios
type BaseSuite struct {
	suite.Suite
	
	// Database
	DB           *sql.DB
	GORMDB       *gorm.DB
	SQLXDB       *sqlx.DB
	
	// HTTP Testing
	Router     *gin.Engine
	TestServer *httptest.Server
	
	// Containers (for integration tests)
	PostgresContainer testcontainers.Container
	RedisContainer    testcontainers.Container
	
	// Configuration
	TestTenantID  string
	TestUserID    string
	TestOrgID     string
	
	// Cleanup
	CleanupFuncs  []func() error
	
	// Logger
	Logger *logrus.Logger
	
	// Context
	Ctx    context.Context
	Cancel context.CancelFunc
}

// SetupSuite configura el entorno de testing base
func (s *BaseSuite) SetupSuite() {
	// Setup logger para tests
	s.Logger = logrus.New()
	s.Logger.SetLevel(logrus.ErrorLevel) // Reducir ruido en tests
	
	// Setup context
	s.Ctx, s.Cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	
	// Setup test data
	s.TestTenantID = "550e8400-e29b-41d4-a716-446655440000"
	s.TestUserID = "550e8400-e29b-41d4-a716-446655440001"
	s.TestOrgID = "550e8400-e29b-41d4-a716-446655440002"
	
	// Initialize cleanup functions
	s.CleanupFuncs = make([]func() error, 0)
	
	s.Logger.Info("Base test suite initialized")
}

// TearDownSuite limpia recursos después de todos los tests
func (s *BaseSuite) TearDownSuite() {
	// Execute cleanup functions in reverse order
	for i := len(s.CleanupFuncs) - 1; i >= 0; i-- {
		if err := s.CleanupFuncs[i](); err != nil {
			s.Logger.WithError(err).Error("Error during cleanup")
		}
	}
	
	// Cancel context
	if s.Cancel != nil {
		s.Cancel()
	}
	
	s.Logger.Info("Base test suite cleaned up")
}

// SetupTest configura cada test individual
func (s *BaseSuite) SetupTest() {
	// Reset Gin to test mode
	gin.SetMode(gin.TestMode)
	
	// Start transaction if database is available
	if s.DB != nil {
		s.beginTransaction()
	}
}

// TearDownTest limpia después de cada test
func (s *BaseSuite) TearDownTest() {
	// Rollback transaction if database is available
	if s.DB != nil {
		s.rollbackTransaction()
	}
	
	// Close test server if running
	if s.TestServer != nil {
		s.TestServer.Close()
		s.TestServer = nil
	}
}

// AddCleanup añade una función de cleanup
func (s *BaseSuite) AddCleanup(cleanup func() error) {
	s.CleanupFuncs = append(s.CleanupFuncs, cleanup)
}

// beginTransaction inicia una transacción para el test
func (s *BaseSuite) beginTransaction() {
	// Implementation depends on database setup
	// This is a placeholder for transaction management
}

// rollbackTransaction revierte la transacción del test
func (s *BaseSuite) rollbackTransaction() {
	// Implementation depends on database setup
	// This is a placeholder for transaction rollback
}

// RequireDB verifica que la base de datos esté disponible
func (s *BaseSuite) RequireDB() {
	s.Require().NotNil(s.DB, "Database connection is required for this test")
}

// RequireRouter verifica que el router esté disponible
func (s *BaseSuite) RequireRouter() {
	s.Require().NotNil(s.Router, "Router is required for this test")
}

// StartTestServer inicia un servidor HTTP de prueba
func (s *BaseSuite) StartTestServer() {
	s.RequireRouter()
	s.TestServer = httptest.NewServer(s.Router)
	s.Logger.WithField("url", s.TestServer.URL).Debug("Test server started")
}

// GetTestServerURL retorna la URL del servidor de prueba
func (s *BaseSuite) GetTestServerURL() string {
	if s.TestServer == nil {
		s.StartTestServer()
	}
	return s.TestServer.URL
}

// WithTenant configura el tenant ID para el test
func (s *BaseSuite) WithTenant(tenantID string) *BaseSuite {
	s.TestTenantID = tenantID
	return s
}

// WithUser configura el user ID para el test
func (s *BaseSuite) WithUser(userID string) *BaseSuite {
	s.TestUserID = userID
	return s
}

// WithOrg configura el organization ID para el test
func (s *BaseSuite) WithOrg(orgID string) *BaseSuite {
	s.TestOrgID = orgID
	return s
}

// AssertNoError verifica que no haya error y proporciona contexto
func (s *BaseSuite) AssertNoError(err error, msgAndArgs ...interface{}) {
	if err != nil {
		s.Logger.WithError(err).Error("Unexpected error in test")
	}
	s.Assert().NoError(err, msgAndArgs...)
}

// AssertError verifica que haya error y proporciona contexto
func (s *BaseSuite) AssertError(err error, msgAndArgs ...interface{}) {
	if err == nil {
		s.Logger.Error("Expected error but got nil")
	}
	s.Assert().Error(err, msgAndArgs...)
}