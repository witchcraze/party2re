package lottery_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// Keep each test's actor isolated while exercising the real queue and indexes.
type recoveryQueue struct {
	core.ScheduledActionRepository
	actor           string
	fail, committed bool
	ids             []string
}

var errRecoverySchedule = errors.New("schedule response lost")

type partialScheduleClient struct {
	valkey.Client
	fail       bool
	originalID string
}

func (c *partialScheduleClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	args := cmd.Commands()
	if c.fail && (args[0] == "ZADD" || args[0] == "SREM" || args[0] == "DEL") {
		original := false
		for _, arg := range args[1:] {
			if arg == c.originalID || arg == "party2:scheduled:action:"+c.originalID || arg == "party2:scheduled:lock:"+c.originalID {
				original = true
			}
		}
		if !original {
			return valkeytest.MakeErrorResult(errRecoverySchedule)
		}
	}
	return c.Client.Do(ctx, cmd)
}

func (q *recoveryQueue) Schedule(ctx context.Context, a core.ScheduledAction) error {
	a.ActorID = q.actor
	q.ids = append(q.ids, a.ID)
	if q.fail && !q.committed {
		return errRecoverySchedule
	}
	if err := q.ScheduledActionRepository.Schedule(ctx, a); err != nil {
		return err
	}
	if q.fail {
		return errRecoverySchedule
	}
	return nil
}

func (q *recoveryQueue) FindPendingByActorID(ctx context.Context, _ string) ([]core.ScheduledAction, error) {
	return q.ScheduledActionRepository.FindPendingByActorID(ctx, q.actor)
}

func (q *recoveryQueue) FetchDue(ctx context.Context, now time.Time, limit int) ([]core.ScheduledAction, error) {
	// Discover only owned work; the global queue may contain other tests' due jobs.
	actions, err := q.FindPendingByActorID(ctx, q.actor)
	if err != nil {
		return nil, err
	}
	var own []core.ScheduledAction
	for _, a := range actions {
		if a.State == core.StatePending && !a.ExecuteAt.After(now) {
			own = append(own, a)
		}
	}
	return own, nil
}

type recoveryNotifier struct {
	lottery.DeliveryNotifier
	fail bool
}

func (n *recoveryNotifier) AddDeliveryNotice(ctx context.Context, characterID, noticeType, message string) error {
	if err := n.DeliveryNotifier.AddDeliveryNotice(ctx, characterID, noticeType, message); err != nil {
		return err
	}
	if n.fail {
		return errors.New("notice transaction failure")
	}
	return nil
}

func TestTakarakujiScheduleRecoveryIntegration(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	for _, tc := range []struct {
		name                                               string
		buyers, scheduleFail, committed, rollback, partial bool
	}{
		{name: "normal"},
		{name: "unsold_schedule_failure", scheduleFail: true},
		{name: "buyers_schedule_failure", buyers: true, scheduleFail: true},
		{name: "unknown_committed_response", buyers: true, scheduleFail: true, committed: true},
		{name: "draw_rollback", buyers: true, rollback: true},
		{name: "partial_registration", buyers: true, partial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := takarakujiIsolatedDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			client, err := vk.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			faults := &partialScheduleClient{Client: client}
			queue := &recoveryQueue{ScheduledActionRepository: scheduling.NewValkeyRepository(faults), actor: "lottery-test:" + id.New()}
			defer func() {
				faults.fail = false
				if _, err := queue.CancelByActorID(context.Background(), queue.actor); err != nil {
					t.Error(err)
				}
				for _, actionID := range queue.ids {
					if err := client.Do(context.Background(), client.B().Del().Key("party2:scheduled:action:"+actionID, "party2:scheduled:lock:"+actionID).Build()).Error(); err != nil {
						t.Error(err)
					}
				}
			}()
			lotRepo, err := database.NewLotteryRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			charRepo, err := database.NewCharacterRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			depRepo, err := database.NewDepotRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			homeRepo, err := database.NewHomeRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			homeSvc, err := home.NewService(homeRepo, charRepo)
			if err != nil {
				t.Fatal(err)
			}
			notifier := &recoveryNotifier{DeliveryNotifier: homeSvc, fail: tc.rollback}
			svc, err := lottery.NewService(lotRepo, lottery.WithTransactionProvider(database.NewTransactionProvider(db)), lottery.WithCharacterRepository(charRepo), lottery.WithDepotRepository(depRepo), lottery.WithDeliveryNotifier(notifier))
			if err != nil {
				t.Fatal(err)
			}
			round, err := lotRepo.CreateTakarakujiRound(ctx, lottery.TakarakujiRound{DrawDate: time.Now().UTC().Add(-time.Hour), Prize1ItemID: "item-129", Prize1Amount: 1, Prize2ItemID: "item-264", Prize2Amount: 1, Prize3ItemID: "item-126", Prize3Amount: 2})
			if err != nil {
				t.Fatal(err)
			}
			if tc.buyers {
				// All twenty slots are real: exactly four prizes and notices must persist.
				for range lottery.TakarakujiMaxTickets {
					c, err := database.CreateTestCharacterWithFunds(ctx, db, "", lottery.TakarakujiCostGold)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := svc.BuyTakarakujiTicket(ctx, c.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			action := core.ScheduledAction{ID: "recovery-test:" + id.New(), ActorID: queue.actor, ActionType: lottery.ActionTypeTakarakujiDraw, ExecuteAt: round.DrawDate, State: core.StatePending}
			if err := queue.Schedule(ctx, action); err != nil {
				t.Fatal(err)
			}
			queue.fail, queue.committed = tc.scheduleFail, tc.committed
			faults.fail, faults.originalID = tc.partial, action.ID
			handler := lottery.NewDrawHandler(svc, lottery.WithScheduler(scheduling.NewService(queue)))
			worker := scheduling.NewWorker(queue, time.Millisecond, logging.Nop())
			worker.RegisterHandler(lottery.ActionTypeTakarakujiDraw, handler)
			if tc.buyers && !tc.committed && !tc.partial {
				_, competingErr := testutil.RunRace2(func() error {
					worker.ProcessAction(ctx, action)
					return nil
				}, func() error { return handler.Handle(ctx, action) })
				if competingErr != nil && !tc.rollback && !errors.Is(competingErr, errRecoverySchedule) {
					t.Fatal(competingErr)
				}
			} else {
				worker.ProcessAction(ctx, action)
			}
			readAction := func() core.ScheduledAction {
				t.Helper()
				data, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+action.ID).Build()).ToString()
				if err != nil {
					t.Fatal(err)
				}
				var a core.ScheduledAction
				if err := json.Unmarshal([]byte(data), &a); err != nil {
					t.Fatal(err)
				}
				return a
			}
			wantState := core.StateCompleted
			if tc.scheduleFail || tc.rollback || tc.partial {
				wantState = core.StateFailed
			}
			if got := readAction(); got.State != wantState {
				t.Fatalf("original state = %v, want %v", got.State, wantState)
			}
			assertSQL := func(drawn bool) {
				t.Helper()
				var isDrawn bool
				if err := db.QueryRowContext(ctx, "SELECT is_drawn FROM takarakuji_rounds WHERE round_id = ?", round.RoundID).Scan(&isDrawn); err != nil {
					t.Fatal(err)
				}
				if isDrawn != drawn {
					t.Fatalf("drawn = %v, want %v", isDrawn, drawn)
				}
				want := 0
				if tc.buyers && drawn {
					want = 4
				}
				for _, query := range []string{"SELECT COALESCE(SUM(quantity),0) FROM depot_items", "SELECT COUNT(*) FROM character_delivery_notices", "SELECT COUNT(*) FROM takarakuji_tickets WHERE won_rank > 0"} {
					var n int
					if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
						t.Fatal(err)
					}
					if n != want {
						t.Fatalf("%s = %d, want %d", query, n, want)
					}
				}
				var rounds int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM takarakuji_rounds").Scan(&rounds); err != nil {
					t.Fatal(err)
				}
				wantRounds := 1
				if drawn {
					wantRounds = 2
				}
				if rounds != wantRounds {
					t.Fatalf("rounds = %d, want %d", rounds, wantRounds)
				}
			}
			assertSQL(!tc.rollback)
			var partialActionID, partialPayload string
			if tc.partial {
				leftovers, err := queue.FindPendingByActorID(ctx, "system")
				if err != nil || len(leftovers) != 1 {
					t.Fatalf("partial reservations = %+v, %v", leftovers, err)
				}
				partialActionID = leftovers[0].ID
				partialPayload, err = client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+partialActionID).Build()).ToString()
				if err != nil {
					t.Fatal(err)
				}
				if err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(partialActionID).Build()).Error(); !valkey.IsValkeyNil(err) {
					t.Fatalf("expected missing queue member, got %v", err)
				}
			}
			if (tc.scheduleFail && !tc.committed) || tc.partial {
				if err := handler.Reconcile(ctx); !errors.Is(err, errRecoverySchedule) {
					t.Fatalf("reconcile error = %v", err)
				}
			}
			queue.fail, notifier.fail = false, false
			faults.fail = false
			// Recovery runs on the existing periodic worker even when no job is queued.
			reconciled := make(chan struct{}, 1)
			worker.RegisterReconciler(lottery.ActionTypeTakarakujiDraw, func(ctx context.Context) error {
				if err := handler.Reconcile(ctx); err != nil {
					return err
				}
				current, err := lotRepo.GetActiveTakarakujiRound(ctx)
				if err != nil {
					return err
				}
				if current.RoundID == round.RoundID {
					return nil
				}
				select {
				case reconciled <- struct{}{}:
				default:
				}
				return nil
			})
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); worker.Run(workerCtx) }()
			select {
			case <-reconciled:
			case <-ctx.Done():
				stop()
				<-done
				t.Fatal(ctx.Err())
			}
			stop()
			<-done
			assertSQL(true)
			// Paired recovery and stale deliveries must conserve already committed awards.
			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(workerID, op int) error {
				if workerID%2 == 0 {
					return handler.Reconcile(ctx)
				}
				return handler.Handle(ctx, action)
			})
			if res.Failures != 0 {
				t.Fatalf("recovery failures: %+v", res)
			}
			assertSQL(true)
			pending, err := queue.FindPendingByActorID(ctx, "system")
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 {
				t.Fatalf("unfinished = %+v", pending)
			}
			if tc.partial {
				if pending[0].ID != partialActionID {
					t.Fatalf("replaced partial reservation: %s", pending[0].ID)
				}
				payload, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+partialActionID).Build()).ToString()
				if err != nil || payload != partialPayload {
					t.Fatalf("payload changed during repair: %v", err)
				}
			}
			active, err := lotRepo.GetActiveTakarakujiRound(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !pending[0].ExecuteAt.Equal(active.DrawDate) {
				t.Fatalf("reservation date = %v, SQL = %v", pending[0].ExecuteAt, active.DrawDate)
			}
			score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(pending[0].ID).Build()).AsFloat64()
			if err != nil || score != float64(active.DrawDate.Unix()) {
				t.Fatalf("queue score = %v, %v", score, err)
			}
			if err := pending[0].Validate(); err != nil {
				t.Fatal(fmt.Errorf("recovered action: %w", err))
			}
		})
	}
}
