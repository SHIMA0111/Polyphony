package config

import (
	"fmt"
	"os"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	// Port is the HTTP server listen port (default "8080").
	Port string
	// DatabaseURL is the PostgreSQL connection string (required).
	DatabaseURL string
	// JWTSecret is the HMAC secret used for signing and verifying JWT tokens (required).
	JWTSecret string
	// LLMGatewayURL is the base URL of the LLM Gateway service (default "http://localhost:8081").
	LLMGatewayURL string
}

// Load reads configuration from environment variables and returns a Config.
// DATABASE_URL and JWT_SECRET are required; PORT defaults to "8080" and LLM_GATEWAY_URL
// defaults to "http://localhost:8081". It returns an error if any required variable is missing.
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
