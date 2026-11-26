package main

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func TestRankingProductionHTTPWiring(t *testing.T) {
	app := newScenarioApp(t)
	app.call("", http.MethodGet, "/legends/comp_mon", "", http.StatusOK, nil)
}

func TestRankingDailyPublicationAndSnapshotOnlyHTTPWiring(t *testing.T) {
	app := newIsolatedScenarioApp(t)
	ctx := context.Background()

	// 1. Startup reconciliation publishes all 8 daily snapshots and schedules ranking:daily.
	repo, err := database.NewRankingRepository(app.db)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := repo.GetAllSnapshots(ctx)
	if err != nil || len(snaps) < 8 {
		t.Fatalf("startup snapshots = %d, err = %v", len(snaps), err)
	}
	schedRepo := scheduling.NewValkeyRepository(app.valkey)
	pending, err := schedRepo.FindPendingByActorID(ctx, "ranking:daily")
	if err != nil || len(pending) != 1 || pending[0].ActionType != ranking.RankingActionTypeRefresh {
		t.Fatalf("pending daily refresh actions = %+v, err = %v", pending, err)
	}
	weeklyPending, err := schedRepo.FindPendingByActorID(ctx, "ranking:weekly")
	if err != nil || len(weeklyPending) != 1 || weeklyPending[0].ActionType != ranking.RankingActionTypeRotateWeekly {
		t.Fatalf("pending weekly rotation actions = %+v, err = %v", weeklyPending, err)
	}
	// Executing the daily scheduled action after Reconcile must not republish today's snapshots.
	calcBefore := snaps[ranking.RankingTypeMonsterKills].CalculatedAt
	app.worker.ProcessAction(ctx, pending[0])
	snapAfter, err := repo.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil || !snapAfter.CalculatedAt.Equal(calcBefore) {
		t.Fatalf("scheduled daily action re-refreshed after Reconcile: before=%v after=%v err=%v", calcBefore, snapAfter.CalculatedAt, err)
	}

	// 2. Live counter mutations do not leak through ?snapshot=false on public boards.
	char, err := database.CreateTestCharacter(ctx, app.db, "LiveHide")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.ExecContext(ctx, `UPDATE characters SET monster_kills = 999 WHERE id = ?`, char.ID); err != nil {
		t.Fatal(err)
	}
	var mkPage ranking.RankingPage[ranking.CharacterRankingEntry]
	app.call("", http.MethodGet, "/rankings/monster-kills?snapshot=false", "", http.StatusOK, &mkPage)
	if !mkPage.IsSnapshot {
		t.Fatalf("expected published snapshot even with snapshot=false, got %+v", mkPage)
	}
	for _, entry := range mkPage.Entries {
		if entry.CharacterID == char.ID {
			t.Fatalf("unpublished monster_kills leaked into public read: %+v", mkPage)
		}
	}

	// 3. Active weekly job changes never leak before first Sunday rotation.
	if err := repo.IncrementJobChangeCount(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	var weeklyPage ranking.RankingPage[ranking.CharacterRankingEntry]
	app.call("", http.MethodGet, "/rankings/weekly-job-change?snapshot=false", "", http.StatusOK, &weeklyPage)
	if weeklyPage.Total != 0 || len(weeklyPage.Entries) != 0 {
		t.Fatalf("active weekly counters leaked before first rotation: %+v", weeklyPage)
	}
}

type wiringWeeklyRankingRepo struct {
	ranking.Repository
	mu        sync.Mutex
	lockCalls atomic.Int32
	due       time.Time
	snapshots map[ranking.RankingType]ranking.RankingSnapshot
}

func (r *wiringWeeklyRankingRepo) LockWeeklyJobChangesForUpdate(context.Context) error {
	r.lockCalls.Add(1)
	return nil
}

func (r *wiringWeeklyRankingRepo) GetWeeklyRotationDue(context.Context) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.due, nil
}

func (r *wiringWeeklyRankingRepo) SaveWeeklyRotationDue(_ context.Context, due time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.due = due
	return nil
}

func (r *wiringWeeklyRankingRepo) GetAllSnapshots(context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[ranking.RankingType]ranking.RankingSnapshot, len(r.snapshots))
	for k, v := range r.snapshots {
		out[k] = v
	}
	return out, nil
}

func (r *wiringWeeklyRankingRepo) SaveSnapshot(_ context.Context, snap ranking.RankingSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snapshots == nil {
		r.snapshots = make(map[ranking.RankingType]ranking.RankingSnapshot)
	}
	r.snapshots[snap.RankingType] = snap
	return nil
}

func (r *wiringWeeklyRankingRepo) GetPvPVictoryRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetMonsterKillsRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetMaoCountRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetHeroCountRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetHelperRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetCasinoWinsRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetAlchemyRanking(context.Context, int, int) ([]ranking.CharacterRankingEntry, int, error) {
	return nil, 0, nil
}
func (r *wiringWeeklyRankingRepo) GetJobPopularityRanking(context.Context) ([]ranking.JobPopularityEntry, error) {
	return nil, nil
}

type threadSafeRankingSchedRepo struct {
	mu      sync.Mutex
	actions []core_scheduling.ScheduledAction
	onSched chan string
}

func (m *threadSafeRankingSchedRepo) Schedule(_ context.Context, a core_scheduling.ScheduledAction) error {
	m.mu.Lock()
	m.actions = append(m.actions, a)
	m.mu.Unlock()
	if m.onSched != nil {
		select {
		case m.onSched <- a.ActorID:
		default:
		}
	}
	return nil
}

func (m *threadSafeRankingSchedRepo) FetchDue(context.Context, time.Time, int) ([]core_scheduling.ScheduledAction, error) {
	return nil, nil
}

func (m *threadSafeRankingSchedRepo) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core_scheduling.ScheduledAction
	for _, a := range m.actions {
		if a.ActorID == actorID && (a.State == core_scheduling.StatePending || a.State == core_scheduling.StateProcessing) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *threadSafeRankingSchedRepo) RequeuePending(context.Context, core_scheduling.ScheduledAction) error {
	return nil
}

func (m *threadSafeRankingSchedRepo) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (m *threadSafeRankingSchedRepo) Save(context.Context, core_scheduling.ScheduledAction) error {
	return nil
}

func (m *threadSafeRankingSchedRepo) CancelByActorID(_ context.Context, actorID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []core_scheduling.ScheduledAction
	removed := 0
	for _, a := range m.actions {
		if a.ActorID == actorID {
			removed++
			continue
		}
		kept = append(kept, a)
	}
	m.actions = kept
	return removed, nil
}

func TestRankingWeeklyPeriodicReconciliationWired(t *testing.T) {
	now := time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC)
	repo := &wiringWeeklyRankingRepo{due: ranking.NextSundayMidnightJST(now)}
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}

	onSched := make(chan string, 8)
	queue := &threadSafeRankingSchedRepo{onSched: onSched}
	soc := &socServices{
		ranking: svc,
		sched:   scheduling.NewService(queue),
		worker:  scheduling.NewWorker(queue, time.Millisecond, logging.Nop()),
	}
	if err := registerRankingWorker(soc); err != nil {
		t.Fatalf("registerRankingWorker: %v", err)
	}
	if got := repo.lockCalls.Load(); got != 1 {
		t.Fatalf("startup lockCalls = %d, want 1", got)
	}

	// Simulate mid-run Valkey reservation loss for ranking:weekly only.
	if _, err := queue.CancelByActorID(context.Background(), "ranking:weekly"); err != nil {
		t.Fatal(err)
	}
	for len(onSched) > 0 {
		<-onSched
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		soc.worker.Run(ctx)
	}()

	restored := false
	deadline := time.After(5 * time.Second)
	for !restored {
		select {
		case actor := <-onSched:
			if actor == "ranking:weekly" {
				restored = true
			}
		case <-deadline:
			cancel()
			<-done
			t.Fatal("weekly ranking reconciler not wired to worker")
		}
	}
	cancel()
	<-done

	pending, err := queue.FindPendingByActorID(context.Background(), "ranking:weekly")
	if err != nil || len(pending) != 1 || pending[0].ActionType != ranking.RankingActionTypeRotateWeekly {
		t.Fatalf("restored weekly pending = %+v, err = %v", pending, err)
	}
	if got := repo.lockCalls.Load(); got != 1 {
		t.Fatalf("worker recovery re-entered SQL lock: lockCalls = %d, want 1", got)
	}
}
