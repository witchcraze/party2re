package valkey

import (
	"os"

	"github.com/valkey-io/valkey-go"
)

// GetConfig returns a valkey.ClientOption configured from environment variables.
func GetConfig() valkey.ClientOption {
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	return valkey.ClientOption{
		InitAddress: []string{addr},
	}
}

// NewClient creates a new Valkey client using the environment configuration.
func NewClient() (valkey.Client, error) {
	return valkey.NewClient(GetConfig())
}
