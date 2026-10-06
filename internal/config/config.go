package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          int
	DBPath        string
	JWTSecret     []byte
	JWTTTL        time.Duration
	AdminEmail    string
	AdminPassword string
}

func Load() (Config, error) {
	port := 8080
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 65535 {
			return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
		}
		port = value
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "./data/ecommerce.db"
	}
	if dbPath == ":memory:" {
		return Config{}, fmt.Errorf("DB_PATH must be a file path")
	}

	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}

	tokenTTL := 15 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("JWT_TTL")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("JWT_TTL must be a positive duration")
		}
		tokenTTL = value
	}

	adminEmail := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if (adminEmail == "") != (adminPassword == "") {
		return Config{}, fmt.Errorf("ADMIN_EMAIL and ADMIN_PASSWORD must be provided together")
	}
	if adminPassword != "" && len(adminPassword) < 12 {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD must contain at least 12 characters")
	}

	return Config{
		Port:          port,
		DBPath:        dbPath,
		JWTSecret:     secret,
		JWTTTL:        tokenTTL,
		AdminEmail:    adminEmail,
		AdminPassword: adminPassword,
	}, nil
}
