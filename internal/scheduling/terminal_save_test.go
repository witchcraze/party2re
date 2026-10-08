package scheduling_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

var terminalWriteError = errors.New("injected terminal write failure")

type terminalFaultClient struct {
	valkey.Client
	fail func([]string) bool
}

func (c *terminalFaultClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	if c.fail != nil && c.fail(cmd.Commands()) {
		return valkeytest.MakeErrorResult(terminalWriteError)
	}
	return c.Client.Do(ctx, cmd)
}

func terminalFixture(t *testing.T) (valkey.Client, *terminalFaultClient, *scheduling.ValkeyRepository, core.ScheduledAction) {
	t.Helper()
	client := openValkeyClient(t)
	t.Cleanup(client.Close)
	action := validTestAction(id.New(), time.Now().Add(-time.Minute))
	action.ActorID = id.New()
	t.Cleanup(func() {
		cleanupKeys(t, client, action.ID)
		if err := client.Do(context.Background(), client.B().Del().Key("party2:scheduled:actor:"+action.ActorID).Build()).Error(); err != nil {
			t.Error(err)
		}
	})
	fault := &terminalFaultClient{Client: client}
	repo := scheduling.NewValkeyRepository(fault)
	if err := repo.Schedule(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	return client, fault, repo, action
}

func assertTerminalStorage(t *testing.T, client valkey.Client, action core.ScheduledAction, state core.State, ttlMode string, queued, indexed, locked bool) {
	t.Helper()
	ctx := context.Background()
	key := "party2:scheduled:action:" + action.ID
	data, err := client.Do(ctx, client.B().Get().Key(key).Build()).ToString()
	if ttlMode == "absent" {
		if !valkey.IsValkeyNil(err) {
			t.Fatalf("expected absent payload, got %q, %v", data, err)
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		var stored core.ScheduledAction
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			t.Fatal(err)
		}
		if stored.State != state || !stored.RetainUntil.Equal(action.RetainUntil) && state != core.StateProcessing {
			t.Fatalf("unexpected stored outcome: %+v", stored)
		}
		if state != core.StateProcessing && action.CompletedAt != nil && (stored.CompletedAt == nil || !stored.CompletedAt.Equal(*action.CompletedAt)) {
			t.Fatalf("completion timestamp changed: %+v", stored)
		}
		ttl, err := client.Do(ctx, client.B().Ttl().Key(key).Build()).AsInt64()
		if err != nil || (ttlMode == "persistent" && ttl != -1) || (ttlMode == "retained" && (ttl <= 0 || ttl > 86400)) {
			t.Fatalf("unexpected TTL %d (%s): %v", ttl, ttlMode, err)
		}
	}
	_, err = client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(action.ID).Build()).ToString()
	if queued && err != nil || !queued && !valkey.IsValkeyNil(err) {
		t.Fatalf("queue membership want %v: %v", queued, err)
	}
	membership, err := client.Do(ctx, client.B().Sismember().Key("party2:scheduled:actor:"+action.ActorID).Member(action.ID).Build()).AsInt64()
	if err != nil || (membership == 1) != indexed {
		t.Fatalf("actor membership want %v, got %d: %v", indexed, membership, err)
	}
	lock, err := client.Do(ctx, client.B().Exists().Key("party2:scheduled:lock:"+action.ID).Build()).AsInt64()
	if err != nil || (lock == 1) != locked {
		t.Fatalf("lock want %v, got %d: %v", locked, lock, err)
	}
}

func TestTerminalSaveFailureRecovery(t *testing.T) {
	for _, state := range []core.State{core.StateCompleted, core.StateFailed} {
		for _, retention := range []string{"future", "zero", "expired", "boundary", "subsecond"} {
			for _, failure := range []string{"SET", "DEL_LOCK", "SREM", "ZREM", "DEL_ACTION", "healthy"} {
				if failure == "DEL_ACTION" && retention != "expired" && retention != "boundary" && retention != "subsecond" {
					continue
				}
				t.Run(string(state)+"/"+retention+"/"+failure, func(t *testing.T) {
					client, fault, repo, action := terminalFixture(t)
					ctx := context.Background()
					if err := action.MarkProcessing(); err != nil {
						t.Fatal(err)
					}
					if err := repo.Save(ctx, action); err != nil {
						t.Fatal(err)
					}
					if acquired, err := repo.AcquireLock(ctx, action.ID, time.Minute); err != nil || !acquired {
						t.Fatalf("lock: %v, %v", acquired, err)
					}
					if state == core.StateCompleted {
						if err := action.MarkCompleted(24 * time.Hour); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := action.MarkFailed(24 * time.Hour); err != nil {
							t.Fatal(err)
						}
					}
					switch retention {
					case "future":
						action.RetainUntil = time.Now().Add(24 * time.Hour)
					case "expired":
						action.RetainUntil = time.Now().Add(-time.Second)
					case "boundary":
						action.RetainUntil = time.Now()
					case "zero":
						action.RetainUntil = time.Time{}
					case "subsecond":
						action.RetainUntil = time.Now().Add(500 * time.Millisecond)
					}
					fault.fail = func(cmd []string) bool {
						switch failure {
						case "SET", "SREM", "ZREM":
							return cmd[0] == failure
						case "DEL_LOCK":
							return cmd[0] == "DEL" && cmd[1] == "party2:scheduled:lock:"+action.ID
						case "DEL_ACTION":
							return cmd[0] == "DEL" && cmd[1] == "party2:scheduled:action:"+action.ID
						}
						return false
					}
					err := repo.Save(ctx, action)
					if failure != "healthy" && !errors.Is(err, terminalWriteError) || failure == "healthy" && err != nil {
						t.Fatalf("Save: %v", err)
					}
					ttl := "persistent"
					if retention == "future" {
						ttl = "retained"
					}
					expired := retention == "expired" || retention == "boundary" || retention == "subsecond"
					if expired && (failure == "healthy" || failure == "ZREM") {
						ttl = "absent"
					}
					storedState := state
					if failure == "SET" {
						storedState, ttl = core.StateProcessing, "persistent"
					}
					assertTerminalStorage(t, client, action, storedState, ttl, failure != "healthy", failure == "SET" || failure == "DEL_LOCK" || failure == "SREM", failure == "SET" || failure == "DEL_LOCK")
					active, err := repo.FindPendingByActorID(ctx, action.ActorID)
					if err != nil || (len(active) == 1) != (failure == "SET") {
						t.Fatalf("actor observation: %+v, %v", active, err)
					}
					fault.fail = nil
					if err := repo.Save(ctx, action); err != nil {
						t.Fatal(err)
					}
					if expired {
						ttl = "absent"
					} else if retention == "future" {
						ttl = "retained"
					} else {
						ttl = "persistent"
					}
					assertTerminalStorage(t, client, action, state, ttl, false, false, false)
				})
			}
		}
	}
}

type terminalCountingHandler struct {
	calls int
	err   error
}

func (h *terminalCountingHandler) Handle(context.Context, core.ScheduledAction) error {
	h.calls++
	return h.err
}

func TestWorkerTerminalRecoveryWithoutReplay(t *testing.T) {
	for _, failure := range []string{"SET", "ZREM", "SREM", "DEL"} {
		for _, handlerFails := range []bool{false, true} {
			t.Run(failure+"/"+map[bool]string{false: "completed", true: "failed"}[handlerFails], func(t *testing.T) {
				client, fault, repo, action := terminalFixture(t)
				ctx := context.Background()
				handler := &terminalCountingHandler{}
				if handlerFails {
					handler.err = errors.New("feature failed")
				}
				worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
				worker.RegisterHandler(action.ActionType, handler)
				fault.fail = func(cmd []string) bool {
					if cmd[0] != failure {
						return false
					}
					if failure == "SET" {
						if cmd[1] != "party2:scheduled:action:"+action.ID {
							return false
						}
						var stored core.ScheduledAction
						if err := json.Unmarshal([]byte(cmd[2]), &stored); err != nil {
							t.Fatal(err)
						}
						return stored.State == core.StateCompleted || stored.State == core.StateFailed
					}
					return true
				}
				worker.ProcessAction(ctx, action)
				fault.fail = nil
				// Remove the coordination lock to also prove Processing is never replayed after expiry.
				if failure == "SET" {
					assertTerminalStorage(t, client, action, core.StateProcessing, "persistent", true, true, true)
					if err := client.Do(ctx, client.B().Del().Key("party2:scheduled:lock:"+action.ID).Build()).Error(); err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 2; i++ {
					due, err := repo.FetchDue(ctx, time.Now(), 1000)
					if err != nil {
						t.Fatal(err)
					}
					for _, a := range due {
						if a.ID == action.ID {
							worker.ProcessAction(ctx, a)
						}
					}
				}
				if handler.calls != 1 {
					t.Fatalf("handler invoked %d times", handler.calls)
				}
				if failure == "SET" {
					assertTerminalStorage(t, client, action, core.StateProcessing, "persistent", false, true, false)
				} else {
					data, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+action.ID).Build()).ToString()
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(data), &action); err != nil {
						t.Fatal(err)
					}
					state := core.StateCompleted
					if handlerFails {
						state = core.StateFailed
					}
					assertTerminalStorage(t, client, action, state, "retained", false, false, false)
				}
			})
		}
	}
}

func TestWorkerExpiredTerminalCleanupRecovery(t *testing.T) {
	for _, failure := range []string{"DEL_ACTION", "ZREM"} {
		t.Run(failure, func(t *testing.T) {
			client, fault, repo, action := terminalFixture(t)
			ctx := context.Background()
			if err := action.MarkProcessing(); err != nil {
				t.Fatal(err)
			}
			if err := action.MarkCompleted(-time.Second); err != nil {
				t.Fatal(err)
			}
			fault.fail = func(cmd []string) bool {
				return failure == "ZREM" && cmd[0] == "ZREM" || failure == "DEL_ACTION" && cmd[0] == "DEL" && cmd[1] == "party2:scheduled:action:"+action.ID
			}
			if err := repo.Save(ctx, action); !errors.Is(err, terminalWriteError) {
				t.Fatalf("Save: %v", err)
			}
			ttl := "persistent"
			if failure == "ZREM" {
				ttl = "absent"
			}
			assertTerminalStorage(t, client, action, core.StateCompleted, ttl, true, false, false)
			fault.fail = nil
			handler := &terminalCountingHandler{}
			worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
			worker.RegisterHandler(action.ActionType, handler)
			due, err := repo.FetchDue(ctx, time.Now(), 1000)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, a := range due {
				if a.ID == action.ID {
					found = true
					worker.ProcessAction(ctx, a)
				}
			}
			if found != (failure == "DEL_ACTION") {
				t.Fatalf("terminal discovery: %v", found)
			}
			if handler.calls != 0 {
				t.Fatalf("terminal handler invoked %d times", handler.calls)
			}
			assertTerminalStorage(t, client, action, core.StateCompleted, "absent", false, false, false)
		})
	}
}
