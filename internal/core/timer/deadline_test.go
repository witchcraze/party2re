package timer

import (
	"context"
	"errors"
	valkey "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestAbsoluteLockDeadline(t *testing.T) {
	s := NewService(nil)
	until := time.Now().Add(time.Hour)
	for range 2 {
		if err := s.SetLockUntil(context.Background(), CategorySleep, "actor", until); err != nil {
			t.Fatal(err)
		}
		actual := s.(*service).memory[PrefixTimer+CategorySleep+":actor"].expiresAt
		if !actual.Equal(until) {
			t.Fatal("deadline changed", actual)
		}
	}
	if err := s.SetLockUntil(context.Background(), CategorySleep, "actor", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if locked, err := s.IsLocked(context.Background(), CategorySleep, "actor"); err != nil || locked {
		t.Fatal(locked, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.SetLockUntil(ctx, CategorySleep, "actor", until); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAbsoluteLockDeadlineValkey(t *testing.T) {
	until := time.Now().Add(time.Hour)
	var commands []string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		commands = append([]string(nil), cmd.Commands()...)
		return valkeytest.MakeOKResult()
	}))
	if err := NewService(client).SetLockUntil(context.Background(), CategorySleep, "actor", until); err != nil {
		t.Fatal(err)
	}
	want := []string{"SET", PrefixTimer + CategorySleep + ":actor", "1", "PXAT", strconv.FormatInt(until.UnixMilli(), 10)}
	if !reflect.DeepEqual(commands, want) {
		t.Fatal(commands)
	}
}
