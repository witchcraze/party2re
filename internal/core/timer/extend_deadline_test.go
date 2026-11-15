package timer

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestExtendAbsoluteDeadline(t *testing.T) {
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	for _, tc := range []struct {
		name    string
		initial *memTimerEntry
		until   time.Time
		want    time.Time
	}{
		{"missing", nil, until, until},
		{"shorter", &memTimerEntry{expiresAt: until.Add(-time.Minute), value: "1"}, until, until},
		{"longer", &memTimerEntry{expiresAt: until.Add(time.Hour), value: "1"}, until, until.Add(time.Hour)},
		{"persistent", &memTimerEntry{value: "token"}, until, time.Time{}},
		{"expired missing", nil, time.Now().Add(-time.Minute), time.Time{}},
		{"expired preserves later", &memTimerEntry{expiresAt: until, value: "1"}, time.Now().Add(-time.Minute), until},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewService(nil).(*service)
			key := PrefixTimer + CategorySleep + ":actor"
			if tc.initial != nil {
				s.memory[key] = *tc.initial
			}
			for range 2 {
				if err := s.ExtendLockUntil(ctx, CategorySleep, "actor", tc.until); err != nil {
					t.Fatal(err)
				}
				if got := s.memory[key].expiresAt; !got.Equal(tc.want) {
					t.Fatalf("deadline=%s, want %s", got, tc.want)
				}
			}
			if tc.name == "persistent" && s.memory[key].value != "token" {
				t.Fatal("persistent value replaced")
			}
			if tc.name == "expired missing" && len(s.memory) != 0 {
				t.Fatal("expired obligation created a timer")
			}
		})
	}
	s := NewService(nil)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, tc := range []struct {
		ctx          context.Context
		category, id string
		want         error
	}{
		{cancelled, CategorySleep, "actor", context.Canceled},
		{ctx, "", "actor", ErrInvalidCategory},
		{ctx, CategorySleep, "", ErrEmptyID},
		{WithClaim(ctx, CategoryWaking, "token"), CategorySleep, "actor", ErrInvalidAction},
	} {
		if err := s.ExtendLockUntil(tc.ctx, tc.category, tc.id, until); !errors.Is(err, tc.want) {
			t.Fatalf("error=%v, want %v", err, tc.want)
		}
	}
}

func TestExtendAbsoluteDeadlineRealValkey(t *testing.T) {
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
	until := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	for _, tc := range []struct {
		name           string
		initial, until time.Time
		persistent     bool
		want           int64
	}{
		{"missing", time.Time{}, until, false, until.UnixMilli()},
		{"shorter", until.Add(-time.Minute), until, false, until.UnixMilli()},
		{"longer", until.Add(time.Hour), until, false, until.Add(time.Hour).UnixMilli()},
		{"persistent", time.Time{}, until, true, -1},
		{"expired missing", time.Time{}, until.Add(-2 * time.Hour), false, -2},
		{"expired preserves later", until, until.Add(-2 * time.Hour), false, until.UnixMilli()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := id.New()
			t.Cleanup(func() {
				if err := s.ReleaseLock(context.Background(), CategorySleep, actor); err != nil {
					t.Error(err)
				}
			})
			if tc.persistent {
				if err := s.SetLock(ctx, CategorySleep, actor, 0); err != nil {
					t.Fatal(err)
				}
			} else if !tc.initial.IsZero() {
				if err := s.SetLockUntil(ctx, CategorySleep, actor, tc.initial); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := s.ExtendLockUntil(ctx, CategorySleep, actor, tc.until); err != nil {
					t.Fatal(err)
				}
				key, err := timerKey(CategorySleep, actor)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
				if err != nil || actual != tc.want {
					t.Fatalf("deadline=%d, %v; want %d", actual, err, tc.want)
				}
			}
		})
	}
}

func TestExtendAbsoluteDeadlineValkeyFailures(t *testing.T) {
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	injected := errors.New("Valkey unavailable")
	for _, stage := range []string{"missing", "existing", "create failure", "extend failure"} {
		t.Run(stage, func(t *testing.T) {
			var commands [][]string
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				commands = append(commands, append([]string(nil), cmd.Commands()...))
				if cmd.Commands()[0] == "SET" {
					if stage == "create failure" {
						return valkeytest.MakeErrorResult(injected)
					}
					if stage == "existing" {
						return valkeytest.MakeNilResult()
					}
					return valkeytest.MakeOKResult()
				}
				if stage == "extend failure" {
					return valkeytest.MakeErrorResult(injected)
				}
				return valkeytest.MakeIntResult(1)
			}))
			err := NewService(client).ExtendLockUntil(ctx, CategorySleep, "actor", until)
			if failed := stage == "create failure" || stage == "extend failure"; (failed && !errors.Is(err, injected)) || (!failed && err != nil) {
				t.Fatal(err)
			}
			key, timestamp := PrefixTimer+CategorySleep+":actor", strconv.FormatInt(until.UnixMilli(), 10)
			want := [][]string{{"SET", key, "1", "NX", "PXAT", timestamp}}
			if stage != "create failure" {
				want = append(want, []string{"PEXPIREAT", key, timestamp, "GT"})
			}
			if !reflect.DeepEqual(commands, want) {
				t.Fatalf("commands=%v, want %v", commands, want)
			}
		})
	}
}
