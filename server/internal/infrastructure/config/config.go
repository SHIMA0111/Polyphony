package config

import (
	"fmt"
	"os"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	LLMGatewayURL string
}

// Load reads configuration from environment variables.
// DATABASE_URL and JWT_SECRET are required; others have defaults.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	llmURL := os.Getenv("LLM_GATEWAY_URL")
	if llmURL == "" {
		llmURL = "http://localhost:8081"
	}

	return &Config{
		Port:          port,
		DatabaseURL:   dbURL,
		JWTSecret:     jwtSecret,
		LLMGatewayURL: llmURL,
	}, nil
}
