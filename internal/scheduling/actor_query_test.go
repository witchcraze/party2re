package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestFindPendingByActorID(t *testing.T) {
	actions := []core.ScheduledAction{
		{ID: "pending", ActorID: "actor", ActionType: "test", State: core.StatePending, ExecuteAt: time.Now().UTC().Add(-time.Hour)},
		{ID: "processing", ActorID: "actor", ActionType: "test", State: core.StateProcessing, ExecuteAt: time.Now().UTC()},
		{ID: "completed", ActorID: "actor", ActionType: "test", State: core.StateCompleted, ExecuteAt: time.Now().UTC()},
		{ID: "failed", ActorID: "actor", ActionType: "test", State: core.StateFailed, ExecuteAt: time.Now().UTC()},
	}
	ids := make([]string, len(actions))
	payloads := make([]string, len(actions))
	for i, action := range actions {
		ids[i] = action.ID
		data, err := json.Marshal(action)
		if err != nil {
			t.Fatal(err)
		}
		payloads[i] = string(data)
	}
	ctx := context.WithValue(context.Background(), struct{}{}, "query")
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(gotCtx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if gotCtx != ctx {
			t.Error("query context was not propagated")
		}
		switch cmd.Commands()[0] {
		case "SMEMBERS":
			return valkeytest.MakeStringSliceResult(ids)
		case "MGET":
			return valkeytest.MakeStringSliceResult(payloads)
		default:
			t.Fatalf("unexpected command %v", cmd.Commands())
			return valkeytest.MakeOKResult()
		}
	}))
	got, err := NewValkeyRepository(client).FindPendingByActorID(ctx, "actor")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, actions[:2]) {
		t.Fatalf("got %+v, want unfinished %+v", got, actions[:2])
	}
	wantCommands := [][]string{
		{"SMEMBERS", actorKeyPrefix + "actor"},
		{"MGET", actionKeyPrefix + "pending", actionKeyPrefix + "processing", actionKeyPrefix + "completed", actionKeyPrefix + "failed"},
	}
	if got := client.RecordedCommandStrings(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("commands = %v, want %v", got, wantCommands)
	}
}

func TestFindPendingByActorID_Empty(t *testing.T) {
	for _, actor := range []string{"", "absent"} {
		t.Run(actor, func(t *testing.T) {
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				if cmd.Commands()[0] != "SMEMBERS" {
					t.Fatalf("unexpected command %v", cmd.Commands())
				}
				return valkeytest.MakeStringSliceResult(nil)
			}))
			got, err := NewValkeyRepository(client).FindPendingByActorID(context.Background(), actor)
			if err != nil || len(got) != 0 {
				t.Fatalf("got %v, %v", got, err)
			}
			if actor == "" && len(client.RecordedCommandStrings()) != 0 {
				t.Error("empty actor must not read Valkey")
			}
		})
	}
}

func TestFindPendingByActorID_Errors(t *testing.T) {
	readErr := errors.New("read unavailable")
	valid := core.ScheduledAction{ID: "action", ActorID: "actor", ActionType: "test", State: core.StatePending, ExecuteAt: time.Now()}
	payload := func(change func(*core.ScheduledAction)) string {
		action := valid
		change(&action)
		data, err := json.Marshal(action)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	goodPayload := payload(func(a *core.ScheduledAction) { a.ID = "good" })
	for _, tc := range []struct {
		name    string
		command string
		data    string
		wantErr error
	}{
		{name: "actor read", command: "SMEMBERS", wantErr: readErr},
		{name: "payload read", command: "MGET", wantErr: readErr},
		{name: "invalid JSON", data: "{broken"},
		{name: "invalid domain", data: payload(func(a *core.ScheduledAction) { a.State = "unknown" }), wantErr: core.ErrInvalidAction},
		{name: "other actor", data: payload(func(a *core.ScheduledAction) { a.ActorID = "other" }), wantErr: core.ErrInvalidAction},
		{name: "other ID", data: payload(func(a *core.ScheduledAction) { a.ID = "other" }), wantErr: core.ErrInvalidAction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				if cmd.Commands()[0] == tc.command {
					return valkeytest.MakeErrorResult(readErr)
				}
				if cmd.Commands()[0] == "SMEMBERS" {
					return valkeytest.MakeStringSliceResult([]string{"good", "action"})
				}
				return valkeytest.MakeStringSliceResult([]string{goodPayload, tc.data})
			}))
			got, err := NewValkeyRepository(client).FindPendingByActorID(context.Background(), "actor")
			if err == nil || got != nil {
				t.Fatalf("want error with no partial results; got %v, %v", got, err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}
