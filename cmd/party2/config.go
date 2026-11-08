package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
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
	Player struct {
		MaxPlayers int
	}
	Home struct {
		CompanionPhraseLimit int
		LetterLogLimit       int
		LetterMaxLength      int
	}
	Park struct {
		PostLogLimit  int
		PostMaxLength int
	}
	PlaceLog placelog.Config
	// ActionWait is the legacy $act_time applied after successful commands.
	ActionWait time.Duration
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

	const defaultMaxPlayers = 999
	maxPlayers := defaultMaxPlayers
	if val := strings.TrimSpace(os.Getenv("PARTY2_MAX_PLAYERS")); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PARTY2_MAX_PLAYERS: %w", err)
		}
		maxPlayers = parsed
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
		Player: struct{ MaxPlayers int }{
			MaxPlayers: maxPlayers,
		},
		Home: struct {
			CompanionPhraseLimit int
			LetterLogLimit       int
			LetterMaxLength      int
		}{
			CompanionPhraseLimit: resolvePositiveInt("PARTY2_HOME_COMPANION_PHRASE_LIMIT", os.Getenv("PARTY2_HOME_COMPANION_PHRASE_LIMIT"), home.DefaultCompanionPhraseLimit),
			LetterLogLimit:       resolvePositiveInt("PARTY2_HOME_LETTER_LOG_LIMIT", os.Getenv("PARTY2_HOME_LETTER_LOG_LIMIT"), home.DefaultLetterLogLimit),
			LetterMaxLength:      resolvePositiveInt("PARTY2_HOME_LETTER_MAX_LENGTH", os.Getenv("PARTY2_HOME_LETTER_MAX_LENGTH"), home.DefaultLetterMaxLength),
		},
		Park: struct {
			PostLogLimit  int
			PostMaxLength int
		}{
			PostLogLimit:  resolvePositiveInt("PARTY2_PARK_POST_LOG_LIMIT", os.Getenv("PARTY2_PARK_POST_LOG_LIMIT"), park.DefaultLogLimit),
			PostMaxLength: resolvePositiveInt("PARTY2_PARK_POST_MAX_LENGTH", os.Getenv("PARTY2_PARK_POST_MAX_LENGTH"), park.DefaultMaxContentLength),
		},
		ActionWait: time.Duration(resolvePositiveInt("PARTY2_ACTION_WAIT_SECONDS", os.Getenv("PARTY2_ACTION_WAIT_SECONDS"), int(playercontext.DefaultActionWait/time.Second))) * time.Second,
		PlaceLog: placelog.Config{
			Limit: resolvePositiveInt("PARTY2_PLACE_LOG_LIMIT", os.Getenv("PARTY2_PLACE_LOG_LIMIT"), placelog.DefaultLimit),
			TTL:   time.Duration(resolvePositiveInt("PARTY2_PLACE_LOG_TTL_HOURS", os.Getenv("PARTY2_PLACE_LOG_TTL_HOURS"), int(placelog.DefaultTTL/time.Hour))) * time.Hour,
		},
	}, nil
}

// resolvePositiveInt falls back to the default instead of failing startup, as approved in #1125 and #1126.
func resolvePositiveInt(name, raw string, def int) int {
	val := strings.TrimSpace(raw)
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		slog.Warn("invalid "+name+"; using default", slog.String("value", val), slog.Int("default", def))
		return def
	}
	return parsed
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
