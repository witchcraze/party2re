package timer

import (
	"context"
	"errors"
	"os"
	"strconv"
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

func TestMemoryTimer_SetLockWithClaim(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken: %v", err)
	}

	// Wrong token returns ErrClaimLost and does not set the lock
	wrongCtx := WithClaim(ctx, CategoryWaking, "wrong-tok")
	if err := svc.SetLock(wrongCtx, CategorySleep, "c1", time.Minute); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("SetLock wrong token err = %v, want ErrClaimLost", err)
	}
	if locked, _ := svc.IsLocked(ctx, CategorySleep, "c1"); locked {
		t.Fatal("CategorySleep must not be set on claim mismatch")
	}

	// Matching token sets the lock
	validCtx := WithClaim(ctx, CategoryWaking, "tok-1")
	if err := svc.SetLock(validCtx, CategorySleep, "c1", time.Minute); err != nil {
		t.Fatalf("SetLock matching token err = %v", err)
	}
	if locked, _ := svc.IsLocked(ctx, CategorySleep, "c1"); !locked {
		t.Fatal("CategorySleep must be set on matching claim")
	}
}

func TestValkeyTimer_SetLockWithClaim(t *testing.T) {
	ctx := context.Background()
	claimKey := PrefixTimer + CategoryWaking + ":c1"
	targetKey := PrefixTimer + CategorySleep + ":c1"

	var lastCmd []string
	allowed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeIntResult(1)
	}))
	claimCtx := WithClaim(ctx, CategoryWaking, "tok-1")
	if err := NewService(allowed).SetLock(claimCtx, CategorySleep, "c1", 60*time.Second); err != nil {
		t.Fatalf("SetLock with claim err = %v", err)
	}
	if len(lastCmd) < 7 || (lastCmd[0] != "EVALSHA" && lastCmd[0] != "EVAL") || lastCmd[3] != claimKey || lastCmd[4] != targetKey || lastCmd[5] != "tok-1" || lastCmd[6] != "60" {
		t.Fatalf("unexpected script command: %v", lastCmd)
	}

	denied := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	if err := NewService(denied).SetLock(claimCtx, CategorySleep, "c1", 60*time.Second); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("SetLock denied claim err = %v, want ErrClaimLost", err)
	}
}

func TestMemoryTimer_TryLockNoExpiry(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	// TryLock with duration <= 0 acquires persistent lock without TTL
	ok, err := svc.TryLock(ctx, CategoryDungeonOnce, "c1", 0)
	if err != nil || !ok {
		t.Fatalf("first TryLock = %v, %v; want true", ok, err)
	}

	// Contended TryLock must fail
	ok, err = svc.TryLock(ctx, CategoryDungeonOnce, "c1", 0)
	if err != nil || ok {
		t.Fatalf("second TryLock = %v, %v; want false", ok, err)
	}

	// IsLocked remains true indefinitely
	locked, err := svc.IsLocked(ctx, CategoryDungeonOnce, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked = %v, %v; want true", locked, err)
	}

	// Release clears the lock
	if err := svc.ReleaseLock(ctx, CategoryDungeonOnce, "c1"); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}

	locked, err = svc.IsLocked(ctx, CategoryDungeonOnce, "c1")
	if err != nil || locked {
		t.Fatalf("IsLocked after release = %v, %v; want false", locked, err)
	}
}

func TestValkeyTimer_TryLockNoExpiry(t *testing.T) {
	ctx := context.Background()
	var lastCmd []string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeOKResult()
	}))

	svc := NewService(client)
	ok, err := svc.TryLock(ctx, CategoryDungeonOnce, "c1", 0)
	if err != nil || !ok {
		t.Fatalf("TryLock with 0 duration: %v, %v; want true", ok, err)
	}

	// Must be SET key value NX without EX
	if len(lastCmd) != 4 || lastCmd[0] != "SET" || lastCmd[1] != PrefixTimer+CategoryDungeonOnce+":c1" || lastCmd[2] != "1" || lastCmd[3] != "NX" {
		t.Fatalf("expected SET key 1 NX without EX, got: %v", lastCmd)
	}
}

func TestMemoryTimer_RenewLockNoExpiry(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken = %v, %v; want true", ok, err)
	}

	renewed, err := svc.RenewLock(ctx, CategoryWaking, "c1", "tok-1", 0)
	if err != nil || !renewed {
		t.Fatalf("RenewLock with 0 duration = %v, %v; want true", renewed, err)
	}

	locked, err := svc.IsLocked(ctx, CategoryWaking, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked after 0 duration renew = %v, %v; want true", locked, err)
	}

	rem, err := svc.GetRemainingLock(ctx, CategoryWaking, "c1")
	if err != nil || rem != 0 {
		t.Fatalf("GetRemainingLock = %v, %v; want 0", rem, err)
	}

	// Contended TryLock must fail
	ok, err = svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-2", 0)
	if err != nil || ok {
		t.Fatalf("contended TryLock = %v, %v; want false", ok, err)
	}

	// Wrong token renew fails
	renewed, err = svc.RenewLock(ctx, CategoryWaking, "c1", "wrong-tok", 0)
	if err != nil || renewed {
		t.Fatalf("RenewLock with wrong token = %v, %v; want false", renewed, err)
	}

	// Release with token clears the persistent lock
	released, err := svc.ReleaseLockWithToken(ctx, CategoryWaking, "c1", "tok-1")
	if err != nil || !released {
		t.Fatalf("ReleaseLockWithToken = %v, %v; want true", released, err)
	}

	locked, err = svc.IsLocked(ctx, CategoryWaking, "c1")
	if err != nil || locked {
		t.Fatalf("IsLocked after release = %v, %v; want false", locked, err)
	}

	// Negative duration renew also acquires/maintains persistent lock
	ok, err = svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reacquire TryLockWithToken = %v, %v; want true", ok, err)
	}
	renewed, err = svc.RenewLock(ctx, CategoryWaking, "c1", "tok-1", -5*time.Second)
	if err != nil || !renewed {
		t.Fatalf("RenewLock with negative duration = %v, %v; want true", renewed, err)
	}
	locked, err = svc.IsLocked(ctx, CategoryWaking, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked after negative duration renew = %v, %v; want true", locked, err)
	}
}

func TestMemoryTimer_SetLockWithClaimNoExpiry(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	ok, err := svc.TryLockWithToken(ctx, CategoryWaking, "c1", "tok-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLockWithToken = %v, %v; want true", ok, err)
	}

	claimCtx := WithClaim(ctx, CategoryWaking, "tok-1")
	if err := svc.SetLock(claimCtx, CategoryDungeonOnce, "c1", 0); err != nil {
		t.Fatalf("SetLock with claim and 0 duration err = %v", err)
	}

	locked, err := svc.IsLocked(ctx, CategoryDungeonOnce, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked after claimed 0 duration SetLock = %v, %v; want true", locked, err)
	}

	rem, err := svc.GetRemainingLock(ctx, CategoryDungeonOnce, "c1")
	if err != nil || rem != 0 {
		t.Fatalf("GetRemainingLock = %v, %v; want 0", rem, err)
	}

	// Negative duration claim SetLock also creates persistent lock
	if err := svc.SetLock(claimCtx, CategoryDungeonOnce, "c1", -10*time.Second); err != nil {
		t.Fatalf("SetLock with claim and negative duration err = %v", err)
	}
	locked, err = svc.IsLocked(ctx, CategoryDungeonOnce, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked after claimed negative duration SetLock = %v, %v; want true", locked, err)
	}

	// Wrong claim token fails and does not modify lock
	wrongCtx := WithClaim(ctx, CategoryWaking, "wrong-tok")
	if err := svc.SetLock(wrongCtx, CategoryDungeonOnce, "c1", 0); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("SetLock with wrong claim err = %v; want ErrClaimLost", err)
	}

	locked, err = svc.IsLocked(ctx, CategoryDungeonOnce, "c1")
	if err != nil || !locked {
		t.Fatalf("IsLocked after failed wrong claim = %v, %v; want true", locked, err)
	}
}

func TestValkeyTimer_RenewLockNoExpiry_Mock(t *testing.T) {
	ctx := context.Background()
	key := PrefixTimer + CategoryWaking + ":c1"

	var lastCmd []string
	allowed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeIntResult(1)
	}))

	ok, err := NewService(allowed).RenewLock(ctx, CategoryWaking, "c1", "tok-1", 0)
	if err != nil || !ok {
		t.Fatalf("RenewLock with 0 duration err = %v, ok = %v", err, ok)
	}
	if len(lastCmd) < 6 || (lastCmd[0] != "EVALSHA" && lastCmd[0] != "EVAL") || lastCmd[3] != key || lastCmd[4] != "tok-1" || lastCmd[5] != "0" {
		t.Fatalf("unexpected script command: %v", lastCmd)
	}

	denied := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	ok, err = NewService(denied).RenewLock(ctx, CategoryWaking, "c1", "tok-1", 0)
	if err != nil || ok {
		t.Fatalf("RenewLock denied err = %v, ok = %v", err, ok)
	}
}

func TestValkeyTimer_SetLockWithClaimNoExpiry_Mock(t *testing.T) {
	ctx := context.Background()
	claimKey := PrefixTimer + CategoryWaking + ":c1"
	targetKey := PrefixTimer + CategorySleep + ":c1"

	var lastCmd []string
	allowed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		return valkeytest.MakeIntResult(1)
	}))
	claimCtx := WithClaim(ctx, CategoryWaking, "tok-1")
	if err := NewService(allowed).SetLock(claimCtx, CategorySleep, "c1", 0); err != nil {
		t.Fatalf("SetLock with claim and 0 duration err = %v", err)
	}
	if len(lastCmd) < 7 || (lastCmd[0] != "EVALSHA" && lastCmd[0] != "EVAL") || lastCmd[3] != claimKey || lastCmd[4] != targetKey || lastCmd[5] != "tok-1" || lastCmd[6] != "0" {
		t.Fatalf("unexpected script command: %v", lastCmd)
	}

	denied := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	if err := NewService(denied).SetLock(claimCtx, CategorySleep, "c1", 0); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("SetLock denied claim err = %v, want ErrClaimLost", err)
	}
}

func TestValkeyTimer_PersistentDurationSemantics_Integration(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not set, skipping real Valkey test")
	}

	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(client.Close)

	ctx := context.Background()
	svc := NewService(client)

	var wakingKey string
	t.Run("RenewLock", func(t *testing.T) {
		targetID := "integ-renew-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		wakingKey = PrefixTimer + CategoryWaking + ":" + targetID
		token := "tok-persist-1"

		t.Cleanup(func() {
			if err := client.Do(ctx, client.B().Del().Key(wakingKey).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
				t.Errorf("cleanup %s: %v", wakingKey, err)
			}
		})

		ok, err := svc.TryLockWithToken(ctx, CategoryWaking, targetID, token, 30*time.Second)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken: %v, %v", ok, err)
		}

		// Verify it initially has a positive TTL
		ttl, err := client.Do(ctx, client.B().Ttl().Key(wakingKey).Build()).AsInt64()
		if err != nil || ttl <= 0 {
			t.Fatalf("initial TTL = %v, %v; want > 0", ttl, err)
		}

		// Renew with 0 duration
		renewed, err := svc.RenewLock(ctx, CategoryWaking, targetID, token, 0)
		if err != nil || !renewed {
			t.Fatalf("RenewLock 0 duration: %v, %v", renewed, err)
		}

		// Valkey TTL must now be -1 (persistent, no expiration)
		ttl, err = client.Do(ctx, client.B().Ttl().Key(wakingKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("TTL after 0 duration renew = %v, %v; want -1", ttl, err)
		}

		locked, err := svc.IsLocked(ctx, CategoryWaking, targetID)
		if err != nil || !locked {
			t.Fatalf("IsLocked = %v, %v; want true", locked, err)
		}

		// Mismatched token does not renew and does not change TTL
		renewed, err = svc.RenewLock(ctx, CategoryWaking, targetID, "wrong-token", 10*time.Second)
		if err != nil || renewed {
			t.Fatalf("RenewLock wrong token = %v, %v; want false", renewed, err)
		}
		ttl, err = client.Do(ctx, client.B().Ttl().Key(wakingKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("TTL after rejected renew = %v, %v; want -1", ttl, err)
		}

		// Renew back to positive TTL works
		renewed, err = svc.RenewLock(ctx, CategoryWaking, targetID, token, 60*time.Second)
		if err != nil || !renewed {
			t.Fatalf("RenewLock positive duration: %v, %v", renewed, err)
		}
		ttl, err = client.Do(ctx, client.B().Ttl().Key(wakingKey).Build()).AsInt64()
		if err != nil || ttl <= 0 {
			t.Fatalf("TTL after positive renew = %v, %v; want > 0", ttl, err)
		}

		// Renew with negative duration also makes lock persistent
		renewed, err = svc.RenewLock(ctx, CategoryWaking, targetID, token, -5*time.Second)
		if err != nil || !renewed {
			t.Fatalf("RenewLock negative duration: %v, %v", renewed, err)
		}
		ttl, err = client.Do(ctx, client.B().Ttl().Key(wakingKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("TTL after negative renew = %v, %v; want -1", ttl, err)
		}
	})

	// Subtest cleanup has completed; verify key is deleted while client remains open
	if wakingKey != "" {
		count, err := client.Do(ctx, client.B().Exists().Key(wakingKey).Build()).AsInt64()
		if err != nil || count != 0 {
			t.Fatalf("expected %s to be deleted after subtest, exists = %d, err = %v", wakingKey, count, err)
		}
	}

	var claimKey, targetKey string
	t.Run("SetLockWithClaim", func(t *testing.T) {
		claimTargetID := "integ-claim-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		claimKey = PrefixTimer + CategoryWaking + ":" + claimTargetID
		targetKey = PrefixTimer + CategoryDungeonOnce + ":" + claimTargetID
		claimToken := "tok-claim-1"

		t.Cleanup(func() {
			if err := client.Do(ctx, client.B().Del().Key(claimKey, targetKey).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
				t.Errorf("cleanup %s, %s: %v", claimKey, targetKey, err)
			}
		})

		ok, err := svc.TryLockWithToken(ctx, CategoryWaking, claimTargetID, claimToken, 30*time.Second)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken claim: %v, %v", ok, err)
		}

		claimCtx := WithClaim(ctx, CategoryWaking, claimToken)
		if err := svc.SetLock(claimCtx, CategoryDungeonOnce, claimTargetID, 0); err != nil {
			t.Fatalf("SetLock with claim and 0 duration: %v", err)
		}

		// Target key must have TTL == -1 (persistent)
		ttl, err := client.Do(ctx, client.B().Ttl().Key(targetKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("target key TTL = %v, %v; want -1", ttl, err)
		}

		locked, err := svc.IsLocked(ctx, CategoryDungeonOnce, claimTargetID)
		if err != nil || !locked {
			t.Fatalf("IsLocked target key = %v, %v; want true", locked, err)
		}

		// SetLock with claim and negative duration also creates/maintains persistent lock
		if err := svc.SetLock(claimCtx, CategoryDungeonOnce, claimTargetID, -10*time.Second); err != nil {
			t.Fatalf("SetLock with claim and negative duration: %v", err)
		}
		ttl, err = client.Do(ctx, client.B().Ttl().Key(targetKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("target key TTL after negative SetLock = %v, %v; want -1", ttl, err)
		}

		// Wrong claim token fails and preserves lock
		wrongCtx := WithClaim(ctx, CategoryWaking, "wrong-claim-token")
		if err := svc.SetLock(wrongCtx, CategoryDungeonOnce, claimTargetID, 10*time.Second); !errors.Is(err, ErrClaimLost) {
			t.Fatalf("SetLock with wrong claim err = %v; want ErrClaimLost", err)
		}
		ttl, err = client.Do(ctx, client.B().Ttl().Key(targetKey).Build()).AsInt64()
		if err != nil || ttl != -1 {
			t.Fatalf("target key TTL after rejected SetLock = %v, %v; want -1", ttl, err)
		}
	})

	// Subtest cleanup has completed; verify keys are deleted while client remains open
	if claimKey != "" && targetKey != "" {
		count, err := client.Do(ctx, client.B().Exists().Key(claimKey, targetKey).Build()).AsInt64()
		if err != nil || count != 0 {
			t.Fatalf("expected %s and %s to be deleted after subtest, exists = %d, err = %v", claimKey, targetKey, count, err)
		}
	}
}
