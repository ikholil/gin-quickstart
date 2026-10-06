package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ENV", "PORT", "DB_PATH", "JWT_SECRET", "JWT_TTL", "ADMIN_EMAIL", "ADMIN_PASSWORD"} {
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
	if cfg.AppEnv != "production" {
		t.Fatalf("APP_ENV = %q, want production by default", cfg.AppEnv)
	}
	if cfg.AdminEmail != "" || cfg.AdminPassword != "" {
		t.Fatalf("unexpected admin bootstrap config: %+v", cfg)
	}
}

func unsetConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ENV", "PORT", "DB_PATH", "JWT_SECRET", "JWT_TTL", "ADMIN_EMAIL", "ADMIN_PASSWORD"} {
		value, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				if err := os.Setenv(key, value); err != nil {
					t.Errorf("restore %s: %v", key, err)
				}
			} else if err := os.Unsetenv(key); err != nil {
				t.Errorf("unset %s: %v", key, err)
			}
		})
	}
}

func TestLoadOptionalDotenvAndProcessEnvironmentPrecedence(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(
		"PORT=8091\nJWT_SECRET=dotenv-secret-that-is-long-enough-to-pass\nDB_PATH=./dotenv.db\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	unsetConfigEnv(t)
	t.Setenv("JWT_SECRET", "process-secret-that-is-long-enough-to-pass")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8091 || cfg.DBPath != "./dotenv.db" {
		t.Fatalf("dotenv values not loaded: %+v", cfg)
	}
	if string(cfg.JWTSecret) != "process-secret-that-is-long-enough-to-pass" {
		t.Fatal("process environment should take precedence over .env")
	}
}

func TestLoadAllowsMissingDotenv(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetConfigEnv(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err != nil {
		t.Fatalf("missing .env should be allowed: %v", err)
	}
}

func TestLoadRejectsMalformedDotenv(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("INVALID@NAME=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsetConfigEnv(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Fatal("expected malformed .env error")
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		set  func(*testing.T)
	}{
		{"invalid application environment", func(t *testing.T) {
			t.Setenv("APP_ENV", "staging")
			t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
		}},
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
