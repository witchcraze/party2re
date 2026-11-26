package ranking_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestNextMidnightJST(t *testing.T) {
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)

	midday := time.Date(2026, 11, 26, 15, 0, 0, 0, jst)
	wantNext := time.Date(2026, 11, 27, 0, 0, 0, 0, jst).UTC()
	if got := ranking.NextMidnightJST(midday); !got.Equal(wantNext) {
		t.Fatalf("midday next = %v, want %v", got, wantNext)
	}

	atMidnight := time.Date(2026, 11, 26, 0, 0, 0, 0, jst)
	if got := ranking.NextMidnightJST(atMidnight); !got.Equal(wantNext) {
		t.Fatalf("midnight next = %v, want %v", got, wantNext)
	}

	beforeMidnight := time.Date(2026, 11, 26, 23, 59, 59, 0, jst)
	if got := ranking.NextMidnightJST(beforeMidnight); !got.Equal(wantNext) {
		t.Fatalf("23:59:59 next = %v, want %v", got, wantNext)
	}
}

func TestDailyRefreshActionIDUsesJSTDate(t *testing.T) {
	utc := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC) // 2026-11-27 00:00 JST
	for _, instant := range []time.Time{utc, utc.In(time.FixedZone("other", -5*60*60))} {
		if got := ranking.DailyRefreshActionID(instant); got != "ranking:daily_refresh:2026-11-27" {
			t.Fatalf("ID = %q", got)
		}
	}
}

type countingSnapshotSaveRepo struct {
	*mockRankingRepository
	saveSnapshotCalls int
}

func (r *countingSnapshotSaveRepo) SaveSnapshot(ctx context.Context, snapshot ranking.RankingSnapshot) error {
	r.saveSnapshotCalls++
	return r.mockRankingRepository.SaveSnapshot(ctx, snapshot)
}

func TestRefreshHandler_HandleAll(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	now := time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC) // 15:00 JST
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	sched := &weeklyScheduler{}
	handler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:         ranking.DailyRefreshActionID(now) + ":1",
		ActionType: ranking.RankingActionTypeRefresh,
		ActorID:    "ranking:daily",
		Params:     map[string]string{"ranking_type": "ALL"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle(ALL): %v", err)
	}

	snapshots, err := svc.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllSnapshots: %v", err)
	}
	if len(snapshots) != 8 {
		t.Errorf("expected 8 snapshots, got %d", len(snapshots))
	}
	if sched.writes != 1 || len(sched.actions) != 1 {
		t.Fatalf("expected 1 scheduled next daily refresh, got writes=%d len=%d", sched.writes, len(sched.actions))
	}
	if !sched.actions[0].ExecuteAt.Equal(ranking.NextMidnightJST(now)) || sched.actions[0].ActorID != "ranking:daily" {
		t.Fatalf("unexpected scheduled action: %+v", sched.actions[0])
	}
}

func TestRefreshHandler_ReconcileThenDueHandleDoesNotDoubleRefresh(t *testing.T) {
	ctx := context.Background()
	repo := &countingSnapshotSaveRepo{mockRankingRepository: newMockRepo()}

	// Yesterday's snapshot was published at 2026-11-25 15:00 JST.
	yesterday := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	now := yesterday
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	dueToday := ranking.NextMidnightJST(yesterday) // 2026-11-26 00:00 JST
	sched := &weeklyScheduler{
		actions: []core_scheduling.ScheduledAction{
			{
				ID:         ranking.DailyRefreshActionID(dueToday) + ":due",
				ActionType: ranking.RankingActionTypeRefresh,
				ActorID:    "ranking:daily",
				Params:     map[string]string{"ranking_type": "all"},
				ExecuteAt:  dueToday,
				State:      core_scheduling.StatePending,
			},
			{
				ID:         ranking.DailyRefreshActionID(yesterday) + ":stale",
				ActionType: ranking.RankingActionTypeRefresh,
				ActorID:    "ranking:daily",
				Params:     map[string]string{"ranking_type": "all"},
				ExecuteAt:  yesterday,
				State:      core_scheduling.StatePending,
			},
		},
	}
	handler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(sched))
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	repo.saveSnapshotCalls = 0

	// Worker tick crosses 00:00 JST: Reconcile runs before FetchDue.
	now = dueToday.Add(5 * time.Second)
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile at 00:00 JST: %v", err)
	}
	if repo.saveSnapshotCalls != 8 {
		t.Fatalf("expected 8 snapshot saves during Reconcile, got %d", repo.saveSnapshotCalls)
	}
	calcAt := repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt

	// Due action (and a stale downtime action) execute right after Reconcile: neither may refresh again.
	now = now.Add(time.Second)
	for _, act := range sched.actions[:2] {
		if err := handler.Handle(ctx, act); err != nil {
			t.Fatalf("Handle(%s): %v", act.ID, err)
		}
	}
	if repo.saveSnapshotCalls != 8 {
		t.Fatalf("expected no duplicate refresh after Reconcile, got saveSnapshotCalls=%d", repo.saveSnapshotCalls)
	}
	if !repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt.Equal(calcAt) {
		t.Fatalf("CalculatedAt changed after Handle: got %v, want %v", repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt, calcAt)
	}

	// Cross-instance Handle (fresh handler with unverified boundary) also skips once DB snapshots are fresh for today.
	freshHandler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(sched))
	if err := freshHandler.Handle(ctx, sched.actions[0]); err != nil {
		t.Fatalf("freshHandler.Handle: %v", err)
	}
	if repo.saveSnapshotCalls != 8 {
		t.Fatalf("expected cross-instance Handle to see today's persisted snapshots and skip, got saveSnapshotCalls=%d", repo.saveSnapshotCalls)
	}

	// Explicit RefreshAllSnapshots (admin path) and non-daily scheduled actions still refresh unconditionally.
	now = now.Add(time.Minute)
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("explicit RefreshAllSnapshots: %v", err)
	}
	if repo.saveSnapshotCalls != 16 {
		t.Fatalf("expected explicit RefreshAllSnapshots to save 8 snapshots, got %d", repo.saveSnapshotCalls)
	}
	now = now.Add(time.Minute)
	if err := handler.Handle(ctx, core_scheduling.ScheduledAction{
		ID:         "admin-scheduled",
		ActionType: ranking.RankingActionTypeRefresh,
		ActorID:    "admin",
		Params:     map[string]string{"ranking_type": "all"},
	}); err != nil {
		t.Fatalf("explicit non-daily Handle: %v", err)
	}
	if repo.saveSnapshotCalls != 24 {
		t.Fatalf("expected non-daily Handle to save 8 snapshots, got %d", repo.saveSnapshotCalls)
	}
}

func TestRefreshHandler_HandleSingleType(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	handler := ranking.NewRefreshHandler(svc)

	action := core_scheduling.ScheduledAction{
		ID:         "act-2",
		ActionType: ranking.RankingActionTypeRefresh,
		Params:     map[string]string{"ranking_type": "MONSTER_KILLS"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle(MONSTER_KILLS): %v", err)
	}

	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil {
		t.Fatalf("GetSnapshot(MONSTER_KILLS): %v", err)
	}
	if snap.RankingType != ranking.RankingTypeMonsterKills {
		t.Errorf("expected snapshot monster_kills, got %s", snap.RankingType)
	}
}

func TestRefreshHandler_HandleInvalidType(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc, _ := ranking.NewService(repo)
	handler := ranking.NewRefreshHandler(svc)

	action := core_scheduling.ScheduledAction{
		ID:         "act-3",
		ActionType: ranking.RankingActionTypeRefresh,
		Params:     map[string]string{"ranking_type": "INVALID_TYPE"},
	}

	if err := handler.Handle(ctx, action); err == nil {
		t.Errorf("expected error for invalid ranking type, got nil")
	}
}

func TestRefreshHandler_ReconcileLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", CharacterName: "Initial", Score: 10}}

	now := time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC) // 15:00 JST
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	sched := &weeklyScheduler{}
	handler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(sched))

	// 1. Cold start: publishes all 8 boards and schedules next JST midnight.
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatalf("cold Reconcile: %v", err)
	}
	if len(repo.snapshots) != 8 || sched.writes != 1 {
		t.Fatalf("cold start snapshots=%d writes=%d", len(repo.snapshots), sched.writes)
	}
	firstCalc := repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt
	if !firstCalc.Equal(now) {
		t.Fatalf("calculated_at=%v, want %v", firstCalc, now)
	}

	// 2. Same-day repeat/restart: updates to repo do not republish until next JST day, and reservation is kept.
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", CharacterName: "Updated", Score: 99}}
	now = now.Add(2 * time.Hour) // 17:00 JST same day
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatalf("same-day Reconcile: %v", err)
	}
	if sched.writes != 1 || !repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt.Equal(firstCalc) {
		t.Fatalf("same-day reconcile unnecessarily refreshed or scheduled: writes=%d calc=%v", sched.writes, repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt)
	}
	page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Score != 10 {
		t.Fatalf("expected yesterday/earlier same-day published score 10, got %+v, %v", page, err)
	}

	// 3. Lost Valkey reservation on same day: restores schedule without re-querying/re-publishing boards.
	sched.actions = nil
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatalf("lost-reservation Reconcile: %v", err)
	}
	if sched.writes != 2 || !repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt.Equal(firstCalc) {
		t.Fatalf("lost reservation did not restore without republishing: writes=%d calc=%v", sched.writes, repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt)
	}

	// 4. Restart after missing one or more daily boundaries: republishes immediately and schedules next JST midnight.
	now = ranking.NextMidnightJST(now).Add(3 * time.Hour) // next day 03:00 JST
	sched.actions = nil
	if err := handler.Reconcile(ctx); err != nil {
		t.Fatalf("overdue Reconcile: %v", err)
	}
	if sched.writes != 3 || !repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt.Equal(now) {
		t.Fatalf("overdue reconcile did not republish: writes=%d calc=%v", sched.writes, repo.snapshots[ranking.RankingTypeMonsterKills].CalculatedAt)
	}
	page, err = svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Score != 99 {
		t.Fatalf("expected republished score 99, got %+v, %v", page, err)
	}
}

func TestRefreshHandler_ReconcilePreservesUncertainProcessingAndPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	now := time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatal(err)
	}

	next := ranking.NextMidnightJST(now)
	uncertain := core_scheduling.ScheduledAction{
		ID:         ranking.DailyRefreshActionID(next),
		ActionType: ranking.RankingActionTypeRefresh,
		ActorID:    "ranking:daily",
		ExecuteAt:  next,
		State:      core_scheduling.StateProcessing,
	}
	sched := &weeklyScheduler{actions: []core_scheduling.ScheduledAction{uncertain}}
	handler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(sched))

	if err := handler.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sched.actions) != 2 || !reflect.DeepEqual(sched.actions[0], uncertain) || sched.actions[1].ID == uncertain.ID {
		t.Fatalf("reconcile modified uncertain Processing reservation: %+v", sched.actions)
	}

	// Scheduler read failure propagates.
	schedFail := &weeklyScheduler{readErr: errors.New("valkey down")}
	if err := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(schedFail)).Reconcile(ctx); !errors.Is(err, schedFail.readErr) {
		t.Fatalf("expected scheduler read error, got %v", err)
	}
}
