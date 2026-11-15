package testutil

import (
	"os"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/valkey"
)

// OpenValkey opens an environment-configured client and closes it after test
// cleanup. It skips only when PARTY2_VALKEY_ADDR is unset; connection errors fail.
func OpenValkey(t testing.TB) valkeygo.Client {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := valkey.NewClient()
	if err != nil {
		t.Fatalf("open valkey client: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}
