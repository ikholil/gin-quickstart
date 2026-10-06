package config

import (
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "DB_PATH", "JWT_SECRET", "JWT_TTL", "ADMIN_EMAIL", "ADMIN_PASSWORD"} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaultsAndRequiredSecret(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 || cfg.DBPath != "./data/ecommerce.db" || cfg.JWTTTL != 15*time.Minute {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.AdminEmail != "" || cfg.AdminPassword != "" {
		t.Fatalf("unexpected admin bootstrap config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		set  func(*testing.T)
	}{
		{"short JWT secret", func(t *testing.T) { t.Setenv("JWT_SECRET", "too-short") }},
		{"invalid port", func(t *testing.T) {
			t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
			t.Setenv("PORT", "70000")
		}},
		{"invalid token duration", func(t *testing.T) {
			t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
			t.Setenv("JWT_TTL", "0s")
		}},
		{"partial admin config", func(t *testing.T) {
			t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
			t.Setenv("ADMIN_EMAIL", "admin@example.com")
		}},
		{"weak admin password", func(t *testing.T) {
			t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
			t.Setenv("ADMIN_EMAIL", "admin@example.com")
			t.Setenv("ADMIN_PASSWORD", "short")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearConfigEnv(t)
			test.set(t)
			if _, err := Load(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
