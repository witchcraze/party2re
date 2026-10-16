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

func TestMemoryTimer_TokenLockLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	// 1. Acquire with tok1
	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken tok1 = %v, %v; want true", ok, err)
	}

	// 2. Contention with tok2
	ok, err = svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok2", time.Minute)
	if err != nil || ok {
		t.Fatalf("TryLockWithToken tok2 = %v, %v; want false", ok, err)
	}

	// 3. Renew with wrong token fails
	renewed, err := svc.RenewLock(ctx, CategoryWaking, "c1", "tok2", time.Minute)
	if err != nil || renewed {
		t.Fatalf("RenewLock tok2 = %v, %v; want false", renewed, err)
	}

	// 4. Renew with correct token succeeds
	renewed, err = svc.RenewLock(ctx, CategoryWaking, "c1", "tok1", time.Minute)
	if err != nil || !renewed {
		t.Fatalf("RenewLock tok1 = %v, %v; want true", renewed, err)
	}

	// 5. Release with wrong token fails and preserves lock
	released, err := svc.ReleaseLockWithToken(ctx, CategoryWaking, "c1", "tok2")
	if err != nil || released {
		t.Fatalf("ReleaseLockWithToken tok2 = %v, %v; want false", released, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryWaking, "c1"); !locked {
		t.Fatal("lock should still be held after wrong token release")
	}

	// 6. Release with correct token succeeds
	released, err = svc.ReleaseLockWithToken(ctx, CategoryWaking, "c1", "tok1")
	if err != nil || !released {
		t.Fatalf("ReleaseLockWithToken tok1 = %v, %v; want true", released, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryWaking, "c1"); locked {
		t.Fatal("lock should be released after correct token release")
	}

	// 7. tok2 can now acquire
	ok, err = svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok2", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken tok2 after release = %v, %v; want true", ok, err)
	}
}

func TestMemoryTimer_TokenLockExpiredFailsRenewAndRelease(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok1", 10*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken = %v, %v", ok, err)
	}

	time.Sleep(30 * time.Millisecond)

	// Renew expired returns false
	renewed, err := svc.RenewLock(ctx, CategoryWaking, "c1", "tok1", time.Minute)
	if err != nil || renewed {
		t.Fatalf("RenewLock on expired = %v, %v; want false", renewed, err)
	}

	// Release expired returns false
	released, err := svc.ReleaseLockWithToken(ctx, CategoryWaking, "c1", "tok1")
	if err != nil || released {
		t.Fatalf("ReleaseLockWithToken on expired = %v, %v; want false", released, err)
	}
}

func TestMemoryTimer_TokenLockValidation(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	if _, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "", time.Minute); !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("TryLockWithToken empty token err = %v, want ErrEmptyToken", err)
	}
	if _, err := svc.RenewLock(ctx, CategoryWaking, "c1", "", time.Minute); !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("RenewLock empty token err = %v, want ErrEmptyToken", err)
	}
	if _, err := svc.ReleaseLockWithToken(ctx, CategoryWaking, "c1", ""); !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("ReleaseLockWithToken empty token err = %v, want ErrEmptyToken", err)
	}
}

func TestValkeyTimer_TryLockWithToken(t *testing.T) {
	ctx := context.Background()
	key := PrefixTimer + CategoryWaking + ":c1"

	var lastCmd []string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeOKResult()
	}))

	ok, err := NewService(client).TryLockWithToken(ctx, CategoryWaking, "c1", "my-uuid-token", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken = %v, %v; want true", ok, err)
	}
	if len(lastCmd) != 6 || lastCmd[0] != "SET" || lastCmd[1] != key || lastCmd[2] != "my-uuid-token" || lastCmd[3] != "NX" || lastCmd[4] != "EX" || lastCmd[5] != "30" {
		t.Fatalf("unexpected command: %v", lastCmd)
	}
}

func TestMemoryTimer_ReleaseLockWithClaim(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	// Setup waking claim and asleep lock
	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken: %v", err)
	}
	if err := svc.SetLock(ctx, CategoryAsleep, "c1", time.Hour); err != nil {
		t.Fatalf("SetLock(CategoryAsleep): %v", err)
	}

	// Release with mismatched claim token fails and preserves lock
	released, err := svc.ReleaseLockWithClaim(ctx, CategoryAsleep, "c1", CategoryWaking, "wrong-tok")
	if err != nil || released {
		t.Fatalf("ReleaseLockWithClaim wrong token = %v, %v; want false", released, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryAsleep, "c1"); !locked {
		t.Fatal("CategoryAsleep must be preserved on token mismatch")
	}

	// Release with matching claim token succeeds and deletes lock
	released, err = svc.ReleaseLockWithClaim(ctx, CategoryAsleep, "c1", CategoryWaking, "tok-1")
	if err != nil || !released {
		t.Fatalf("ReleaseLockWithClaim matching token = %v, %v; want true", released, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryAsleep, "c1"); locked {
		t.Fatal("CategoryAsleep must be deleted on matching token")
	}
}

func TestMemoryTimer_ResetDungeonOnceWithClaim(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	// Setup claim, dungeon_once lock, and quota
	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken: %v", err)
	}
	if err := svc.SetLock(ctx, CategoryDungeonOnce, "c1", time.Hour); err != nil {
		t.Fatalf("SetLock: %v", err)
	}
	if _, err := svc.ConsumeDailyQuota(ctx, "dungeon_once", "c1", time.Now()); err != nil {
		t.Fatalf("ConsumeDailyQuota: %v", err)
	}

	// Reset with wrong token fails and preserves both
	reset, err := svc.ResetDungeonOnceWithClaim(ctx, "c1", CategoryWaking, "wrong-tok")
	if err != nil || reset {
		t.Fatalf("ResetDungeonOnceWithClaim wrong token = %v, %v; want false", reset, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryDungeonOnce, "c1"); !locked {
		t.Fatal("dungeon_once lock must be preserved")
	}
	if has, _ := svc.HasUsedDailyQuota(ctx, "dungeon_once", "c1"); !has {
		t.Fatal("dungeon_once quota must be preserved")
	}

	// Reset with correct token succeeds and deletes both atomically
	reset, err = svc.ResetDungeonOnceWithClaim(ctx, "c1", CategoryWaking, "tok-1")
	if err != nil || !reset {
		t.Fatalf("ResetDungeonOnceWithClaim matching token = %v, %v; want true", reset, err)
	}
	if locked, _ := svc.IsLocked(ctx, CategoryDungeonOnce, "c1"); locked {
		t.Fatal("dungeon_once lock must be deleted")
	}
	if has, _ := svc.HasUsedDailyQuota(ctx, "dungeon_once", "c1"); has {
		t.Fatal("dungeon_once quota must be deleted")
	}
}
