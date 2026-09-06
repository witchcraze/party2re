package valkey

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
)

const (
	DefaultAddress     = "localhost:6379"
	DefaultDialTimeout = 5 * time.Second
)

// Config holds Valkey connection and client parameters.
type Config struct {
	Address     string
	Password    string
	DB          int
	DialTimeout time.Duration
}

// DefaultConfig returns a Config initialized with baseline defaults.
func DefaultConfig(address string) Config {
	if strings.TrimSpace(address) == "" {
		address = DefaultAddress
	}
	return Config{
		Address:     address,
		Password:    "",
		DB:          0,
		DialTimeout: DefaultDialTimeout,
	}
}

// ConfigFromEnvironment loads Valkey configuration from environment variables.
// Supported environment variables:
//   - PARTY2_VALKEY_ADDR or VALKEY_ADDR: Address of Valkey server (e.g. "localhost:6379")
//   - PARTY2_VALKEY_PASSWORD: Optional password for Valkey AUTH
//   - PARTY2_VALKEY_DB: Database index (integer, default: 0)
//   - PARTY2_VALKEY_DIAL_TIMEOUT: Connection dial timeout (duration, default: 5s)
func ConfigFromEnvironment() Config {
	addr := strings.TrimSpace(os.Getenv("PARTY2_VALKEY_ADDR"))
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("VALKEY_ADDR"))
	}
	cfg := DefaultConfig(addr)

	if pass := os.Getenv("PARTY2_VALKEY_PASSWORD"); pass != "" {
		cfg.Password = pass
	}

	if dbStr := strings.TrimSpace(os.Getenv("PARTY2_VALKEY_DB")); dbStr != "" {
		if dbNum, err := strconv.Atoi(dbStr); err == nil && dbNum >= 0 {
			cfg.DB = dbNum
		}
	}

	if timeoutStr := strings.TrimSpace(os.Getenv("PARTY2_VALKEY_DIAL_TIMEOUT")); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
			cfg.DialTimeout = d
		}
	}

	return cfg
}

// ToClientOption converts Config to valkeygo.ClientOption.
func (c Config) ToClientOption() valkeygo.ClientOption {
	addr := c.Address
	if strings.TrimSpace(addr) == "" {
		addr = DefaultAddress
	}
	dialTimeout := c.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = DefaultDialTimeout
	}

	return valkeygo.ClientOption{
		InitAddress: []string{addr},
		Password:    c.Password,
		SelectDB:    c.DB,
		Dialer: net.Dialer{
			Timeout: dialTimeout,
		},
	}
}

// GetConfig returns a valkeygo.ClientOption configured from environment variables.
// Kept for backward compatibility.
func GetConfig() valkeygo.ClientOption {
	return ConfigFromEnvironment().ToClientOption()
}

// NewClientWithConfig creates a new Valkey client using the provided Config struct.
func NewClientWithConfig(cfg Config) (valkeygo.Client, error) {
	return valkeygo.NewClient(cfg.ToClientOption())
}

// NewClient creates a new Valkey client using the environment configuration.
func NewClient() (valkeygo.Client, error) {
	return NewClientWithConfig(ConfigFromEnvironment())
}
