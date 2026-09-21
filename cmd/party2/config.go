package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/valkey"
)

// Config centralizes all application configuration and environment parameter parsing.
type Config struct {
	DB     database.Config
	Valkey valkey.Config
	Server struct {
		Addr           string
		TrustedProxies string
	}
	Admin struct {
		APIKey string
	}
	CORS struct {
		AllowedOrigins string
	}
}

// ConfigFromEnv loads the complete configuration from environment variables.
func ConfigFromEnv() (Config, error) {
	dbCfg, err := database.ConfigFromEnvironment()
	if err != nil {
		return Config{}, err
	}

	adminKey := os.Getenv("PARTY2_ADMIN_API_KEY")
	if adminKey == "" {
		adminKey = os.Getenv("ADMIN_API_KEY")
	}

	trustedProxies := strings.TrimSpace(os.Getenv("PARTY2_TRUSTED_PROXIES"))
	if trustedProxies != "" {
		if _, err := http.ParseTrustedProxies(trustedProxies); err != nil {
			return Config{}, fmt.Errorf("invalid PARTY2_TRUSTED_PROXIES: %w", err)
		}
	}

	return Config{
		DB:     dbCfg,
		Valkey: valkey.ConfigFromEnvironment(),
		Server: struct {
			Addr           string
			TrustedProxies string
		}{
			Addr:           resolveServerAddr(),
			TrustedProxies: trustedProxies,
		},
		Admin: struct{ APIKey string }{
			APIKey: strings.TrimSpace(adminKey),
		},
		CORS: struct{ AllowedOrigins string }{
			AllowedOrigins: os.Getenv("PARTY2_CORS_ORIGINS"),
		},
	}, nil
}

func resolveServerAddr() string {
	if addr := os.Getenv("PARTY2_ADDR"); addr != "" {
		return addr
	}
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	port := os.Getenv("PARTY2_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		return ":" + port
	}
	return port
}
