package timer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestMemoryTimer_TryLock(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	ok, err := svc.TryLock(ctx, CategoryWaking, "c1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first TryLock = %v, %v; want acquired", ok, err)
	}
	ok, err = svc.TryLock(ctx, CategoryWaking, "c1", time.Minute)
	if err != nil || ok {
		t.Fatalf("second TryLock = %v, %v; want contended", ok, err)
	}
	if ok, _ = svc.TryLock(ctx, CategoryWaking, "c2", time.Minute); !ok {
		t.Fatal("different target must not contend")
	}
	if err := svc.ReleaseLock(ctx, CategoryWaking, "c1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ = svc.TryLock(ctx, CategoryWaking, "c1", time.Minute); !ok {
		t.Fatal("released claim must be acquirable")
	}
}

func TestMemoryTimer_TryLockExpiredClaimIsReacquirable(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	if ok, _ := svc.TryLock(ctx, CategoryWaking, "c1", 10*time.Millisecond); !ok {
		t.Fatal("expected acquire")
	}
	time.Sleep(30 * time.Millisecond)
	if ok, _ := svc.TryLock(ctx, CategoryWaking, "c1", time.Minute); !ok {
		t.Fatal("expired claim must be reacquirable")
	}
}

func TestMemoryTimer_TryLockValidation(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.TryLock(context.Background(), "", "c1", time.Second); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("err = %v, want ErrInvalidCategory", err)
	}
	if _, err := svc.TryLock(context.Background(), CategoryWaking, "", time.Second); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("err = %v, want ErrEmptyID", err)
	}
}

func TestValkeyTimer_TryLock(t *testing.T) {
	ctx := context.Background()
	key := PrefixTimer + CategoryWaking + ":c1"

	var lastCmd []string
	acquired := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeOKResult()
	}))
	ok, err := NewService(acquired).TryLock(ctx, CategoryWaking, "c1", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("TryLock = %v, %v; want acquired", ok, err)
	}
	// SET party2:timer:waking:c1 1 NX EX 30
	if len(lastCmd) != 6 || lastCmd[0] != "SET" || lastCmd[1] != key || lastCmd[3] != "NX" || lastCmd[4] != "EX" || lastCmd[5] != "30" {
		t.Fatalf("unexpected command: %v", lastCmd)
	}

	contended := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	if ok, err := NewService(contended).TryLock(ctx, CategoryWaking, "c1", time.Second); err != nil || ok {
		t.Fatalf("TryLock = %v, %v; want contended", ok, err)
	}

	boom := errors.New("valkey down")
	failing := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(boom)
	}))
	if _, err := NewService(failing).TryLock(ctx, CategoryWaking, "c1", time.Second); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want valkey error", err)
	}
}
