package timer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
)

func TestImposeLockUntil(t *testing.T) {
	ctx := context.Background()
	until := time.Now().Add(time.Minute).Truncate(time.Millisecond)

	t.Run("missing creates imposed lock", func(t *testing.T) {
		s := NewService(nil).(*service)
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "actor", until); err != nil {
			t.Fatal(err)
		}
		key, _ := timerKey(CategoryActionWait, "actor")
		if got := s.memory[key]; got.value != "1" || !got.expiresAt.Equal(until) {
			t.Fatalf("entry=%+v, want value=1 expiresAt=%v", got, until)
		}
	})

	t.Run("shorter claim token is superseded and release cannot delete imposed wait", func(t *testing.T) {
		s := NewService(nil)
		ok, err := s.TryLockWithToken(ctx, CategoryActionWait, "actor", "claim-token", 18*time.Second)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken: ok=%v err=%v", ok, err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "actor", until); err != nil {
			t.Fatal(err)
		}
		released, err := s.ReleaseLockWithToken(ctx, CategoryActionWait, "actor", "claim-token")
		if err != nil || released {
			t.Fatalf("superseded claim released wait: released=%v err=%v", released, err)
		}
		if rem, err := s.GetRemainingLock(ctx, CategoryActionWait, "actor"); err != nil || rem <= 50*time.Second {
			t.Fatalf("imposed wait lost: rem=%v err=%v", rem, err)
		}
	})

	t.Run("longer active claim preserves expiry while superseding token", func(t *testing.T) {
		s := NewService(nil)
		ok, err := s.TryLockWithToken(ctx, CategoryActionWait, "actor", "long-token", 2*time.Minute)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken: ok=%v err=%v", ok, err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "actor", until); err != nil {
			t.Fatal(err)
		}
		released, err := s.ReleaseLockWithToken(ctx, CategoryActionWait, "actor", "long-token")
		if err != nil || released {
			t.Fatalf("longer superseded claim released wait: released=%v err=%v", released, err)
		}
		if rem, err := s.GetRemainingLock(ctx, CategoryActionWait, "actor"); err != nil || rem <= time.Minute {
			t.Fatalf("longer wait shortened or lost: rem=%v err=%v", rem, err)
		}
	})

	t.Run("longer imposed wait, persistent lock, and expired deadline are preserved", func(t *testing.T) {
		s := NewService(nil).(*service)
		longer := until.Add(time.Hour)
		if err := s.SetLockUntil(ctx, CategoryActionWait, "longer", longer); err != nil {
			t.Fatal(err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "longer", until); err != nil {
			t.Fatal(err)
		}
		keyLonger, _ := timerKey(CategoryActionWait, "longer")
		if !s.memory[keyLonger].expiresAt.Equal(longer) {
			t.Fatalf("longer wait changed: %v", s.memory[keyLonger].expiresAt)
		}

		if err := s.SetLock(ctx, CategoryActionWait, "persistent", 0); err != nil {
			t.Fatal(err)
		}
		keyPersist, _ := timerKey(CategoryActionWait, "persistent")
		s.memory[keyPersist] = memTimerEntry{value: "token"}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "persistent", until); err != nil {
			t.Fatal(err)
		}
		if got := s.memory[keyPersist]; got.value != "token" || !got.expiresAt.IsZero() {
			t.Fatalf("persistent lock modified: %+v", got)
		}

		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "expired-missing", time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		if rem, err := s.GetRemainingLock(ctx, CategoryActionWait, "expired-missing"); err != nil || rem != 0 {
			t.Fatalf("expired deadline revived timer: rem=%v err=%v", rem, err)
		}

		if ok, err := s.TryLockWithToken(ctx, CategoryActionWait, "expired-claim", "keep-token", 18*time.Second); err != nil || !ok {
			t.Fatalf("TryLockWithToken: ok=%v err=%v", ok, err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, "expired-claim", time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		released, err := s.ReleaseLockWithToken(ctx, CategoryActionWait, "expired-claim", "keep-token")
		if err != nil || !released {
			t.Fatalf("expired impose broke active claim: released=%v err=%v", released, err)
		}
	})

	t.Run("validation and context errors", func(t *testing.T) {
		s := NewService(nil)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		for _, tc := range []struct {
			ctx          context.Context
			category, id string
			want         error
		}{
			{cancelled, CategoryActionWait, "actor", context.Canceled},
			{ctx, "", "actor", ErrInvalidCategory},
			{ctx, CategoryActionWait, "", ErrEmptyID},
			{WithClaim(ctx, CategoryWaking, "token"), CategoryActionWait, "actor", ErrInvalidAction},
		} {
			if err := s.ImposeLockUntil(tc.ctx, tc.category, tc.id, until); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v, want %v", err, tc.want)
			}
		}
	})
}

func TestImposeLockUntilRealValkey(t *testing.T) {
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s := NewService(client)
	until := time.Now().Add(time.Minute).Truncate(time.Millisecond)

	newActor := func(t *testing.T) (string, string) {
		t.Helper()
		actor := id.New()
		key, err := timerKey(CategoryActionWait, actor)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := s.ReleaseLock(context.Background(), CategoryActionWait, actor); err != nil {
				t.Error(err)
			}
		})
		return actor, key
	}

	t.Run("shorter claim token superseded and protected from ReleaseLockWithToken", func(t *testing.T) {
		actor, key := newActor(t)
		ok, err := s.TryLockWithToken(ctx, CategoryActionWait, actor, "claim-token", 18*time.Second)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken: ok=%v err=%v", ok, err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, actor, until); err != nil {
			t.Fatal(err)
		}
		released, err := s.ReleaseLockWithToken(ctx, CategoryActionWait, actor, "claim-token")
		if err != nil || released {
			t.Fatalf("superseded claim deleted imposed wait: released=%v err=%v", released, err)
		}
		actual, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
		if err != nil || actual != until.UnixMilli() {
			t.Fatalf("deadline=%d err=%v, want %d", actual, err, until.UnixMilli())
		}
		val, err := client.Do(ctx, client.B().Get().Key(key).Build()).ToString()
		if err != nil || val != "1" {
			t.Fatalf("value=%q err=%v, want 1", val, err)
		}
	})

	t.Run("longer claim token superseded without shortening expiry", func(t *testing.T) {
		actor, key := newActor(t)
		ok, err := s.TryLockWithToken(ctx, CategoryActionWait, actor, "long-token", 2*time.Minute)
		if err != nil || !ok {
			t.Fatalf("TryLockWithToken: ok=%v err=%v", ok, err)
		}
		beforeExpiry, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
		if err != nil || beforeExpiry <= until.UnixMilli() {
			t.Fatalf("beforeExpiry=%d err=%v", beforeExpiry, err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, actor, until); err != nil {
			t.Fatal(err)
		}
		released, err := s.ReleaseLockWithToken(ctx, CategoryActionWait, actor, "long-token")
		if err != nil || released {
			t.Fatalf("longer claim deleted wait: released=%v err=%v", released, err)
		}
		afterExpiry, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
		if err != nil || afterExpiry != beforeExpiry {
			t.Fatalf("afterExpiry=%d, want %d (err=%v)", afterExpiry, beforeExpiry, err)
		}
	})

	t.Run("missing, longer imposed, persistent, and expired", func(t *testing.T) {
		aMissing, kMissing := newActor(t)
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, aMissing, until); err != nil {
			t.Fatal(err)
		}
		if p, err := client.Do(ctx, client.B().Pexpiretime().Key(kMissing).Build()).AsInt64(); err != nil || p != until.UnixMilli() {
			t.Fatalf("missing=%d err=%v", p, err)
		}

		aLonger, kLonger := newActor(t)
		longer := until.Add(time.Hour)
		if err := s.SetLockUntil(ctx, CategoryActionWait, aLonger, longer); err != nil {
			t.Fatal(err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, aLonger, until); err != nil {
			t.Fatal(err)
		}
		if p, err := client.Do(ctx, client.B().Pexpiretime().Key(kLonger).Build()).AsInt64(); err != nil || p != longer.UnixMilli() {
			t.Fatalf("longer=%d err=%v, want %d", p, err, longer.UnixMilli())
		}

		aPersist, kPersist := newActor(t)
		if err := s.SetLock(ctx, CategoryActionWait, aPersist, 0); err != nil {
			t.Fatal(err)
		}
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, aPersist, until); err != nil {
			t.Fatal(err)
		}
		if p, err := client.Do(ctx, client.B().Pexpiretime().Key(kPersist).Build()).AsInt64(); err != nil || p != -1 {
			t.Fatalf("persistent=%d err=%v, want -1", p, err)
		}

		aExpired, kExpired := newActor(t)
		if err := s.ImposeLockUntil(ctx, CategoryActionWait, aExpired, time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if p, err := client.Do(ctx, client.B().Pexpiretime().Key(kExpired).Build()).AsInt64(); err != nil || p != -2 {
			t.Fatalf("expired=%d err=%v, want -2", p, err)
		}
	})
}
