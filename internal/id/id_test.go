package id_test

import (
	"encoding/hex"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/id"
)

func TestNew(t *testing.T) {
	t.Parallel()

	val := id.New()
	if len(val) != 32 {
		t.Fatalf("expected 32 hex chars, got %d (%s)", len(val), val)
	}

	decoded, err := hex.DecodeString(val)
	if err != nil {
		t.Fatalf("expected valid hex string, got error: %v", err)
	}
	if len(decoded) != 16 {
		t.Fatalf("expected 16 bytes decoded, got %d", len(decoded))
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	val, err := id.Generate()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(val) != 32 {
		t.Fatalf("expected 32 hex chars, got %d (%s)", len(val), val)
	}

	decoded, err := hex.DecodeString(val)
	if err != nil {
		t.Fatalf("expected valid hex string, got error: %v", err)
	}
	if len(decoded) != 16 {
		t.Fatalf("expected 16 bytes decoded, got %d", len(decoded))
	}
}

func TestNewLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		byteLength int
		wantLength int
		wantErr    bool
	}{
		{name: "invalid zero", byteLength: 0, wantErr: true},
		{name: "invalid negative", byteLength: -5, wantErr: true},
		{name: "8 bytes", byteLength: 8, wantLength: 16, wantErr: false},
		{name: "16 bytes", byteLength: 16, wantLength: 32, wantErr: false},
		{name: "32 bytes", byteLength: 32, wantLength: 64, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := id.NewLength(tt.byteLength)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewLength(%d) err = %v, wantErr = %v", tt.byteLength, err, tt.wantErr)
			}
			if !tt.wantErr && len(got) != tt.wantLength {
				t.Fatalf("NewLength(%d) len = %d, want = %d", tt.byteLength, len(got), tt.wantLength)
			}
		})
	}
}

func TestNew_UniquenessAndConcurrency(t *testing.T) {
	t.Parallel()

	const iterations = 5000
	const goroutines = 50

	var (
		mu  sync.Mutex
		ids = make(map[string]struct{}, iterations*goroutines)
		wg  sync.WaitGroup
	)

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			localIDs := make([]string, iterations)
			for i := 0; i < iterations; i++ {
				localIDs[i] = id.New()
			}

			mu.Lock()
			for _, item := range localIDs {
				ids[item] = struct{}{}
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	expectedTotal := iterations * goroutines
	if len(ids) != expectedTotal {
		t.Fatalf("expected %d unique IDs, got %d (collision detected)", expectedTotal, len(ids))
	}
}
