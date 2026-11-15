package testutil_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestOpenValkeyUnconfigured(t *testing.T) {
	t.Setenv("PARTY2_VALKEY_ADDR", "")
	// The production loader's fallback must not enable an unconfigured test.
	t.Setenv("VALKEY_ADDR", "127.0.0.1:1")
	var skipped bool
	if !t.Run("unconfigured", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		testutil.OpenValkey(t)
		t.Error("returned from an unconfigured fixture")
	}) || !skipped {
		t.Fatal("unconfigured fixture must skip")
	}
}

func TestOpenValkeyUnreachable(t *testing.T) {
	if os.Getenv("PARTY2_TEST_VALKEY_FAILURE_CHILD") == "1" {
		testutil.OpenValkey(t)
		return
	}
	t.Setenv("PARTY2_TEST_VALKEY_FAILURE_CHILD", "1")
	t.Setenv("PARTY2_VALKEY_ADDR", "127.0.0.1:1")
	t.Setenv("PARTY2_VALKEY_DIAL_TIMEOUT", "50ms")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestOpenValkeyUnreachable$", "-test.v")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "open valkey client:") || strings.Contains(string(out), "--- SKIP:") {
		t.Fatalf("configured connection failure must fail, not skip: err=%v\n%s", err, out)
	}
}

func TestOpenValkeyLifecycle(t *testing.T) {
	var client valkeygo.Client
	if !t.Run("connected", func(t *testing.T) {
		client = testutil.OpenValkey(t)
		t.Cleanup(func() {
			if err := client.Do(context.Background(), client.B().Ping().Build()).Error(); err != nil {
				t.Errorf("client closed before fixture cleanup: %v", err)
			}
		})
		if err := client.Do(t.Context(), client.B().Ping().Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}) {
		return
	}
	if client == nil {
		t.Skip("Valkey integration environment is not configured")
	}
	if err := client.Do(t.Context(), client.B().Ping().Build()).Error(); !errors.Is(err, valkeygo.ErrClosing) {
		t.Fatalf("client not closed after test cleanup: %v", err)
	}
}
