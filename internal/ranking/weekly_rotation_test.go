package ranking_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corescheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestWeeklyRotation_FirstStartPreservesCounters(t *testing.T) {
	repo := newMockRepo()
	repo.activeWeeklyJobChanges["char-1"] = 3
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "char-1", Score: 3}}
	repo.weeklyJobChangeTotal = 1
	now := time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RotateWeeklyJobChangeRanking(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.activeWeeklyJobChanges["char-1"] != 3 {
		t.Fatal("initial reconciliation consumed existing counters before the first future boundary")
	}
	if !repo.weeklyRotationDue.Equal(ranking.NextSundayMidnightJST(now)) || len(repo.snapshots) != 0 {
		t.Fatalf("first start must only establish a future deadline: due=%v snapshots=%v", repo.weeklyRotationDue, repo.snapshots)
	}
}

type weeklyScheduler struct {
	actions    []corescheduling.ScheduledAction
	enqueueErr error
	readErr    error
	writes     int
}

func (s *weeklyScheduler) FindPendingByActorID(ctx context.Context, actor string) ([]corescheduling.ScheduledAction, error) {
	return s.actions, s.readErr
}

func (s *weeklyScheduler) ScheduleWithID(ctx context.Context, id, typ, actor string, params map[string]string, at time.Time) error {
	if s.enqueueErr != nil {
		return s.enqueueErr
	}
	s.writes++
	s.actions = append(s.actions, corescheduling.ScheduledAction{
		ID: id, ActionType: typ, ActorID: actor, Params: params, ExecuteAt: at, State: corescheduling.StatePending,
	})
	return nil
}

func TestWeeklyRotation_RecurringRecoveryAndReplay(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC) // Sunday 00:00 JST
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &weeklyScheduler{}
	handler := ranking.NewRotateWeeklyHandler(svc, scheduler)
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	due := repo.weeklyRotationDue
	if !due.Equal(now.AddDate(0, 0, 7)) || scheduler.writes != 1 {
		t.Fatalf("initial deadline: %v", due)
	}
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if scheduler.writes != 1 {
		t.Fatal("restart replaced an existing future reservation")
	}

	for week := 0; week < 2; week++ {
		repo.activeWeeklyJobChanges["char-1"] = week + 2
		repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "char-1", Score: int64(week + 2)}}
		repo.weeklyJobChangeTotal = 1
		now = due.Add(-time.Nanosecond)
		if err := handler.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		if repo.activeWeeklyJobChanges["char-1"] != week+2 {
			t.Fatal("rotated before deadline")
		}
		now = due
		if err := handler.Handle(ctx, corescheduling.ScheduledAction{ExecuteAt: due}); err != nil {
			t.Fatal(err)
		}
		snap := repo.snapshots[ranking.RankingTypeWeeklyJobChange]
		if len(repo.activeWeeklyJobChanges) != 0 || !snap.CalculatedAt.Equal(now) {
			t.Fatal("due rotation did not settle")
		}
		due = repo.weeklyRotationDue
		if !due.Equal(now.AddDate(0, 0, 7)) {
			t.Fatalf("next deadline: %v", due)
		}
		repo.activeWeeklyJobChanges["new-week"] = 1
		if err := handler.Handle(ctx, corescheduling.ScheduledAction{ExecuteAt: now.AddDate(0, 0, -7)}); err != nil {
			t.Fatal(err)
		}
		if repo.activeWeeklyJobChanges["new-week"] != 1 || repo.snapshots[ranking.RankingTypeWeeklyJobChange].SnapshotData != snap.SnapshotData {
			t.Fatal("stale delivery consumed new counters")
		}
	}

	// Simulate several missed boundaries and complete loss of queue reservations.
	now = due.AddDate(0, 0, 22).Add(4 * time.Hour)
	scheduler.actions = nil
	repo.activeWeeklyJobChanges = map[string]int{"char-1": 9}
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "char-1", Score: 9}}
	svc, err = ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	handler = ranking.NewRotateWeeklyHandler(svc, scheduler)
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if !repo.weeklyRotationDue.Equal(ranking.NextSundayMidnightJST(now)) || len(repo.activeWeeklyJobChanges) != 0 || len(scheduler.actions) != 1 {
		t.Fatal("missed weeks were not consolidated into one recovery")
	}
	repo.activeWeeklyJobChanges["after-recovery"] = 1
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if repo.activeWeeklyJobChanges["after-recovery"] != 1 {
		t.Fatal("recovery replay consumed fresh counter")
	}
	// Losing only the next wake-up must restore it without settling again.
	scheduler.actions = nil
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.actions) != 1 || repo.activeWeeklyJobChanges["after-recovery"] != 1 {
		t.Fatal("future reservation repair repeated settlement")
	}
}

func TestWeeklyRotation_PostCommitQueueFailure(t *testing.T) {
	for _, discovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "enqueue", true: "discovery"}[discovery], func(t *testing.T) {
			ctx := context.Background()
			repo := newMockRepo()
			now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
			repo.weeklyRotationDue = now
			repo.activeWeeklyJobChanges["old"] = 2
			repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "old", Score: 2}}
			repo.weeklyJobChangeTotal = 1
			svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			queueErr := errors.New("queue unavailable")
			scheduler := &weeklyScheduler{}
			if discovery {
				scheduler.readErr = queueErr
			} else {
				scheduler.enqueueErr = queueErr
			}
			handler := ranking.NewRotateWeeklyHandler(svc, scheduler)
			if err := handler.Reconcile(ctx); !errors.Is(err, queueErr) {
				t.Fatalf("queue error: %v", err)
			}
			snap := repo.snapshots[ranking.RankingTypeWeeklyJobChange]
			if !repo.weeklyRotationDue.After(now) || len(repo.activeWeeklyJobChanges) != 0 || snap.TotalCount != 1 {
				t.Fatal("SQL settlement did not commit before queue failure")
			}
			repo.activeWeeklyJobChanges["new"] = 1
			scheduler.readErr, scheduler.enqueueErr = nil, nil
			// A fresh service represents recovery after an uncertain worker outcome or restart.
			svc, err = ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			if err := ranking.NewRotateWeeklyHandler(svc, scheduler).Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			if repo.activeWeeklyJobChanges["new"] != 1 || repo.snapshots[ranking.RankingTypeWeeklyJobChange] != snap || len(scheduler.actions) != 1 {
				t.Fatal("retry repeated SQL settlement")
			}
		})
	}
}

func TestWeeklyRotation_ProgressFailuresRollback(t *testing.T) {
	for _, stage := range []string{"lock", "read", "advance", "snapshot", "reset"} {
		t.Run(stage, func(t *testing.T) {
			repo := newMockRepo()
			now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
			repo.weeklyRotationDue = now
			repo.activeWeeklyJobChanges["char"] = 1
			repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "char", Score: 1}}
			repo.weeklyJobChangeTotal = 1
			failure := errors.New(stage)
			switch stage {
			case "lock":
				repo.lockWeeklyJobChangesErr = failure
			case "read":
				repo.getWeeklyRotationDueErr = failure
			case "advance":
				repo.saveWeeklyRotationDueErr = failure
			case "snapshot":
				repo.saveSnapshotErr = failure
			case "reset":
				repo.resetWeeklyJobChangesErr = failure
			}
			cache := newMockSnapshotCache()
			svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }), ranking.WithSnapshotCache(cache))
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &weeklyScheduler{}
			err = ranking.NewRotateWeeklyHandler(svc, scheduler).Reconcile(context.Background())
			if !errors.Is(err, failure) {
				t.Fatalf("expected %v, got %v", failure, err)
			}
			if !repo.weeklyRotationDue.Equal(now) || repo.activeWeeklyJobChanges["char"] != 1 || len(repo.snapshots) != 0 || scheduler.writes != 0 {
				t.Fatal("failed transaction changed progress, snapshot, counters or queue")
			}
			_, found, err := cache.Get(context.Background(), ranking.RankingTypeWeeklyJobChange)
			if err != nil || found {
				t.Fatal("failed SQL transaction published cache")
			}
		})
	}
}

func TestWeeklyRotation_ActionIDUsesJSTBoundary(t *testing.T) {
	utc := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	for _, instant := range []time.Time{utc, utc.In(time.FixedZone("other", -5*60*60))} {
		if got := ranking.WeeklyRotateActionID(instant); got != "ranking:weekly_rotate:2026-10-11" {
			t.Fatalf("ID = %q", got)
		}
	}
}

func TestWeeklyRotation_DelayedCutoff(t *testing.T) {
	for _, delay := range []time.Duration{2 * time.Hour, 22 * 24 * time.Hour} {
		t.Run(delay.String(), func(t *testing.T) {
			repo := newMockRepo()
			due := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
			now := due.Add(delay)
			repo.weeklyRotationDue = due
			repo.activeWeeklyJobChanges["char"] = 4
			repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{{CharacterID: "char", Score: 4}}
			repo.weeklyJobChangeTotal = 1
			svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			next, err := svc.RotateWeeklyJobChangeRanking(context.Background())
			if err != nil || !next.Equal(ranking.NextSundayMidnightJST(now)) {
				t.Fatalf("delayed rotation: next=%v err=%v", next, err)
			}
			snap := repo.snapshots[ranking.RankingTypeWeeklyJobChange]
			if !snap.CalculatedAt.Equal(now) || len(repo.activeWeeklyJobChanges) != 0 {
				t.Fatal("delayed rotation did not use recovery cutoff")
			}
		})
	}
}

func TestWeeklyRotation_PreservesUncertainProcessing(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	due := now.AddDate(0, 0, 7)
	repo.weeklyRotationDue = due
	repo.activeWeeklyJobChanges["new-week"] = 1
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	uncertain := corescheduling.ScheduledAction{ID: ranking.WeeklyRotateActionID(due), ActionType: ranking.RankingActionTypeRotateWeekly, ExecuteAt: due, State: corescheduling.StateProcessing}
	scheduler := &weeklyScheduler{actions: []corescheduling.ScheduledAction{uncertain}}
	handler := ranking.NewRotateWeeklyHandler(svc, scheduler)
	if err := handler.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.actions) != 2 || !reflect.DeepEqual(scheduler.actions[0], uncertain) || scheduler.actions[1].ID == uncertain.ID || repo.activeWeeklyJobChanges["new-week"] != 1 {
		t.Fatal("reconciliation replaced uncertain metadata or consumed future counters")
	}
	if err := handler.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if scheduler.writes != 1 {
		t.Fatal("reconciliation did not preserve the recovered Pending reservation")
	}
}
