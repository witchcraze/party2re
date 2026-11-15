package scheduling

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestRequeuePending(t *testing.T) {
	storeErr := errors.New("queue unavailable")
	for _, state := range []core.State{core.StatePending, core.StateProcessing, core.StateFailed, core.StateCompleted} {
		t.Run(string(state), func(t *testing.T) {
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(context.Context, valkey.Completed) valkey.ValkeyResult {
				return valkeytest.MakeErrorResult(storeErr)
			}))
			action := core.ScheduledAction{ID: "round", ActorID: "system", ActionType: "draw", ExecuteAt: time.Now().UTC(), State: state}
			err := NewService(NewValkeyRepository(client)).RequeuePending(context.Background(), action)
			if state != core.StatePending {
				if err != nil || len(client.RecordedCommandStrings()) != 0 {
					t.Fatalf("non-Pending repair: %v, %v", err, client.RecordedCommandStrings())
				}
				return
			}
			if !errors.Is(err, storeErr) {
				t.Fatalf("error = %v", err)
			}
			want := [][]string{{"ZADD", pendingQueueKey, "NX", strconv.FormatInt(action.ExecuteAt.Unix(), 10), action.ID}}
			if got := client.RecordedCommandStrings(); !reflect.DeepEqual(got, want) {
				t.Fatalf("commands = %v, want %v", got, want)
			}
		})
	}
	client := valkeytest.NewMockClient()
	if err := NewValkeyRepository(client).RequeuePending(context.Background(), core.ScheduledAction{}); !errors.Is(err, core.ErrInvalidAction) {
		t.Fatalf("invalid action: %v", err)
	}
	if len(client.RecordedCommandStrings()) != 0 {
		t.Fatal("invalid action wrote queue")
	}
}
