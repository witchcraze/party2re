package rescue_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
)

type rescueFixture struct {
	db      *sql.DB
	char    corecharacter.Character
	chars   *database.CharacterRepository
	records *database.RescueRepository
	work    *scheduling.Service
	timers  timer.Service
	tx      *database.DefaultTransactionProvider
}

func newRescueFixture(t *testing.T) *rescueFixture {
	t.Helper()
	client := testutil.OpenValkey(t)
	f := &rescueFixture{db: database.OpenTestDB(t), timers: timer.NewService(client)}
	var err error
	f.chars, err = database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.records, err = database.NewRescueRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.char, err = database.CreateTestCharacter(context.Background(), f.db, "")
	if err != nil {
		t.Fatal(err)
	}
	f.work = scheduling.NewService(scheduling.NewValkeyRepository(client))
	f.tx = database.NewTransactionProvider(f.db)
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := f.work.CancelByActorID(ctx, f.char.ID); err != nil {
			t.Error(err)
		}
		if err := f.timers.ReleaseLock(ctx, timer.CategorySleep, f.char.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, `DELETE FROM rescue_records WHERE character_id = ?`, f.char.ID); err != nil {
			t.Error(err)
		}
		if err := f.chars.Delete(ctx, f.char.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, `DELETE FROM players WHERE id = ?`, f.char.PlayerID); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *rescueFixture) service() *rescue.Service {
	return rescue.NewService(f.records, f.chars, f.work, f.timers, f.tx)
}

func (f *rescueFixture) schedule(t *testing.T, ctx context.Context) string {
	t.Helper()
	id, err := f.work.Schedule(ctx, "training:complete", f.char.ID, nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *rescueFixture) state(t *testing.T, ctx context.Context, applied bool, ids ...string) rescue.RescueRecord {
	t.Helper()
	rec, err := f.records.FindLatestByCharacterID(ctx, f.char.ID)
	if err != nil || !rec.AppliedAt.IsZero() != applied {
		t.Fatalf("receipt applied=%v: %+v, %v", applied, rec, err)
	}
	actions, err := f.work.FindPendingByActorID(ctx, f.char.ID)
	if err != nil || len(actions) != len(ids) {
		t.Fatalf("pending=%+v, %v; want %v", actions, err, ids)
	}
	for i, id := range ids {
		if actions[i].ID != id {
			t.Fatalf("pending=%+v, want %v", actions, ids)
		}
	}
	return rec
}

type afterCommitTx struct {
	rescue.TransactionProvider
	after func(context.Context) error
}

func (p *afterCommitTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if err := p.TransactionProvider.RunInTx(ctx, fn); err != nil {
		return err
	}
	if p.after != nil {
		after := p.after
		p.after = nil
		return after(ctx)
	}
	return nil
}

type activityFacts struct{ kinds []string }

func (a *activityFacts) ActivityKinds(context.Context, string, string) ([]string, error) {
	return a.kinds, nil
}

func TestRescueIntegration_DelayedDuplicatePreservesLaterWork(t *testing.T) {
	f := newRescueFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	original := f.schedule(t, ctx)
	committed, resume := make(chan struct{}), make(chan struct{})
	gate := &afterCommitTx{TransactionProvider: f.tx, after: func(ctx context.Context) error {
		close(committed)
		select {
		case <-resume:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	activity := &activityFacts{}
	slow := rescue.NewService(f.records, f.chars, f.work, f.timers, gate)
	slow.SetActivityReader(activity)
	result := make(chan error, 1)
	go func() { _, err := slow.EmergencyRescue(ctx, f.char.ID, "slow", time.Now().UTC()); result <- err }()
	joined := false
	defer func() {
		cancel()
		if !joined {
			<-result
		}
	}()
	select {
	case <-committed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pending := f.state(t, ctx, false, original)
	fast, err := f.service().EmergencyRescue(ctx, f.char.ID, "fast", time.Now().UTC())
	if err != nil || fast.ID != pending.ID {
		t.Fatalf("concurrent rescue=%+v, %v", fast, err)
	}
	confirmed := f.state(t, ctx, true)
	if remaining, err := f.timers.GetRemainingLock(ctx, timer.CategorySleep, f.char.ID); err != nil || remaining < 590*time.Second {
		t.Fatalf("healthy sleep=%s, %v", remaining, err)
	}
	if err := f.timers.SetLockUntil(ctx, timer.CategorySleep, f.char.ID, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	newWork := f.schedule(t, ctx)
	activity.kinds = []string{"dungeon"}
	close(resume)
	err = <-result
	joined = true
	if err != nil {
		t.Fatal(err)
	}
	after := f.state(t, ctx, true, newWork)
	if !after.AppliedAt.Equal(confirmed.AppliedAt) {
		t.Fatal("confirmation was replayed")
	}
	if locked, err := f.timers.IsLocked(ctx, timer.CategorySleep, f.char.ID); err != nil || locked {
		t.Fatalf("expired sleep recreated: %v, %v", locked, err)
	}
}

type interceptedCleaner struct {
	*scheduling.Service
	before func(context.Context) error
}

func TestRescueIntegration_RevalidatesActivityAfterRecordCommit(t *testing.T) {
	for _, cleared := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncleared work rejects", true: "cleared work owes sleep"}[cleared], func(t *testing.T) {
			f := newRescueFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			work := f.schedule(t, ctx)
			activity := &activityFacts{}
			gate := &afterCommitTx{TransactionProvider: f.tx, after: func(ctx context.Context) error {
				if cleared {
					if _, err := f.work.CancelByActorID(ctx, f.char.ID); err != nil {
						return err
					}
				}
				activity.kinds = []string{"dungeon"}
				return nil
			}}
			svc := rescue.NewService(f.records, f.chars, f.work, f.timers, gate)
			svc.SetActivityReader(activity)
			_, err := svc.EmergencyRescue(ctx, f.char.ID, "stuck", time.Now().UTC())
			if cleared {
				if err != nil {
					t.Fatal(err)
				}
				f.state(t, ctx, true)
			} else {
				if !errors.Is(err, rescue.ErrActivityRemaining) {
					t.Fatalf("error=%v", err)
				}
				f.state(t, ctx, false, work)
			}
			locked, err := f.timers.IsLocked(ctx, timer.CategorySleep, f.char.ID)
			if err != nil || locked != cleared {
				t.Fatalf("sleep=%v, %v; want %v", locked, err, cleared)
			}
			if len(activity.kinds) != 1 || activity.kinds[0] != "dungeon" {
				t.Fatal("activity changed")
			}
		})
	}
}

func (c interceptedCleaner) CancelByActorID(ctx context.Context, actor string) (int, error) {
	if err := c.before(ctx); err != nil {
		return 0, err
	}
	return c.Service.CancelByActorID(ctx, actor)
}

func TestRescueIntegration_ApplicationLockAndCancellation(t *testing.T) {
	f := newRescueFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	work := f.schedule(t, ctx)
	applying := make(chan struct{})
	cleaner := interceptedCleaner{Service: f.work, before: func(ctx context.Context) error {
		close(applying)
		<-ctx.Done()
		return ctx.Err()
	}}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		_, err := rescue.NewService(f.records, f.chars, cleaner, f.timers, f.tx).EmergencyRescue(requestCtx, f.char.ID, "stuck", time.Now().UTC())
		result <- err
	}()
	joined := false
	defer func() {
		cancelRequest()
		if !joined {
			<-result
		}
	}()
	select {
	case <-applying:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	f.state(t, ctx, false, work)
	err := f.tx.RunInTx(ctx, func(ctx context.Context) error {
		var id string
		return database.ExecutorFromContext(ctx, f.db).QueryRowContext(ctx,
			`SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, f.char.ID).Scan(&id)
	})
	var sqlErr *mysql.MySQLError
	if !errors.As(err, &sqlErr) || sqlErr.Number != 1205 {
		t.Fatalf("application did not hold character lock: %v", err)
	}
	cancelRequest()
	err = <-result
	joined = true
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled rescue=%v", err)
	}
	f.state(t, ctx, false, work)
	if _, err := f.service().EmergencyRescue(ctx, f.char.ID, "retry", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.state(t, ctx, true)
}

type failedConfirmation struct{ rescue.RescueRepository }

var errRescueStore = errors.New("injected rescue store failure")

func (r failedConfirmation) MarkApplied(ctx context.Context, id string, at time.Time) error {
	if err := r.RescueRepository.MarkApplied(ctx, id, at); err != nil {
		return err
	}
	return errRescueStore
}

type failedDeadline struct{ timer.Service }

func (failedDeadline) ExtendLockUntil(context.Context, string, string, time.Time) error {
	return errRescueStore
}

func TestRescueIntegration_FailureRecoveryKeepsReceiptAndDeadline(t *testing.T) {
	for _, stage := range []string{"clear", "timer", "confirmation"} {
		t.Run(stage, func(t *testing.T) {
			f := newRescueFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			work := f.schedule(t, ctx)
			var cleaner rescue.ActionCleaner = f.work
			var timers rescue.TimerService = f.timers
			var records rescue.RescueRepository = f.records
			switch stage {
			case "clear":
				cleaner = interceptedCleaner{Service: f.work, before: func(context.Context) error { return errRescueStore }}
			case "timer":
				timers = failedDeadline{f.timers}
			case "confirmation":
				records = failedConfirmation{f.records}
			}
			if err := f.timers.SetLock(ctx, timer.CategorySleep, f.char.ID, time.Hour); err != nil {
				t.Fatal(err)
			}
			_, err := rescue.NewService(records, f.chars, cleaner, timers, f.tx).EmergencyRescue(ctx, f.char.ID, "stuck", time.Now().UTC())
			if !errors.Is(err, errRescueStore) {
				t.Fatalf("failure=%v", err)
			}
			var ids []string
			if stage == "clear" {
				ids = []string{work}
			}
			before := f.state(t, ctx, false, ids...)
			want := time.Hour
			if stage == "confirmation" {
				want += 600 * time.Second
			}
			if remaining, err := f.timers.GetRemainingLock(ctx, timer.CategorySleep, f.char.ID); err != nil || remaining < want-10*time.Second || remaining > want {
				t.Fatalf("sleep after failure=%s, %v; want %s", remaining, err, want)
			}
			rec, err := f.service().EmergencyRescue(ctx, f.char.ID, "retry", time.Now().UTC())
			if err != nil || rec.ID != before.ID {
				t.Fatalf("retry=%+v, %v", rec, err)
			}
			after := f.state(t, ctx, true)
			if !after.SleepUntil.Equal(before.SleepUntil) {
				t.Fatal("retry changed deadline")
			}
			var count int
			if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rescue_records WHERE character_id = ?`, f.char.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("records=%d, %v", count, err)
			}
			if remaining, err := f.timers.GetRemainingLock(ctx, timer.CategorySleep, f.char.ID); err != nil || remaining < time.Hour+590*time.Second || remaining > time.Hour+600*time.Second {
				t.Fatalf("sleep after retry=%s, %v", remaining, err)
			}
		})
	}
}
