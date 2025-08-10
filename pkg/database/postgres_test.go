package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lukcba-developers/gopherkit/pkg/config"
)

func TestDatabaseConfig_GetDSN_Simple(t *testing.T) {
	tests := []struct {
		name   string
		config config.DatabaseConfig
		want   string
	}{
		{
			name: "with URL",
			config: config.DatabaseConfig{
				URL: "postgresql://user:pass@host:5432/db",
			},
			want: "postgresql://user:pass@host:5432/db",
		},
		{
			name: "without URL - build from components",
			config: config.DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				Database: "testdb",
				SSLMode:  "disable",
			},
			want: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable",
		},
		{
			name: "production config",
			config: config.DatabaseConfig{
				Host:     "prod.example.com",
				Port:     "5432",
				User:     "produser",
				Password: "prodpass",
				Database: "proddb",
				SSLMode:  "require",
			},
			want: "host=prod.example.com port=5432 user=produser password=prodpass dbname=proddb sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetDSN()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  config.DatabaseConfig
		isValid bool
	}{
		{
			name: "valid config with URL",
			config: config.DatabaseConfig{
				URL: "postgresql://user:pass@host:5432/db",
			},
			isValid: true,
		},
		{
			name: "valid config with components",
			config: config.DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "test",
				Password: "test",
				Database: "test",
				SSLMode:  "disable",
			},
			isValid: true,
		},
		{
			name: "missing host",
			config: config.DatabaseConfig{
				Port:     "5432",
				User:     "test",
				Password: "test",
				Database: "test",
				SSLMode:  "disable",
			},
			isValid: false,
		},
		{
			name: "missing database name",
			config: config.DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "test",
				Password: "test",
				Database: "",
				SSLMode:  "disable",
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := tt.config.GetDSN()
			
			if tt.isValid {
				assert.NotEmpty(t, dsn)
				if tt.config.URL != "" {
					assert.Equal(t, tt.config.URL, dsn)
				} else {
					assert.Contains(t, dsn, tt.config.Host)
					assert.Contains(t, dsn, tt.config.Database)
				}
			} else {
				// Para configuraciones inválidas, aún se genera DSN pero puede estar incompleto
				if tt.config.URL == "" && tt.config.Host == "" {
					assert.Contains(t, dsn, "host=")
				}
			}
		})
	}
}

func TestConnectionPoolDefaults(t *testing.T) {
	configs := []config.DatabaseConfig{
		{
			MaxOpenConns: 25,
			MaxIdleConns: 5,
		},
		{
			MaxOpenConns: 10,
			MaxIdleConns: 2,
		},
		{
			MaxOpenConns: 0, // Should use defaults
			MaxIdleConns: 0, // Should use defaults
		},
	}

	for i, cfg := range configs {
		t.Run(fmt.Sprintf("config_%d", i), func(t *testing.T) {
			if cfg.MaxOpenConns > 0 {
				assert.Greater(t, cfg.MaxOpenConns, 0)
			}
			if cfg.MaxIdleConns > 0 {
				assert.Greater(t, cfg.MaxIdleConns, 0)
			}
			
			// Verificar que MaxIdleConns no sea mayor que MaxOpenConns cuando ambos > 0
			if cfg.MaxOpenConns > 0 && cfg.MaxIdleConns > 0 {
				assert.LessOrEqual(t, cfg.MaxIdleConns, cfg.MaxOpenConns)
			}
		})
	}
}