package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/tavern"
)

type mockSchedRepo struct {
	actions     []core_scheduling.ScheduledAction
	scheduleErr error
}

func (m *mockSchedRepo) Schedule(_ context.Context, a core_scheduling.ScheduledAction) error {
	if m.scheduleErr != nil {
		return m.scheduleErr
	}
	m.actions = append(m.actions, a)
	return nil
}

func (m *mockSchedRepo) FetchDue(_ context.Context, _ time.Time, _ int) ([]core_scheduling.ScheduledAction, error) {
	return nil, nil
}

func (m *mockSchedRepo) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	var actions []core_scheduling.ScheduledAction
	for _, action := range m.actions {
		if action.ActorID == actorID && (action.State == core_scheduling.StatePending || action.State == core_scheduling.StateProcessing) {
			actions = append(actions, action)
		}
	}
	return actions, nil
}

func (m *mockSchedRepo) AcquireLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (m *mockSchedRepo) Save(_ context.Context, _ core_scheduling.ScheduledAction) error {
	return nil
}

func (m *mockSchedRepo) CancelByActorID(_ context.Context, _ string) (int, error) {
	return 0, nil
}

type mockChapelRepo struct {
	clearedID string
}

func (m *mockChapelRepo) GetBlessing(_ context.Context, _ string) (chapel.CharacterBlessing, error) {
	return chapel.CharacterBlessing{ActiveBlessing: chapel.BlessingNone}, nil
}

func (m *mockChapelRepo) SelectBlessing(_ context.Context, _ string, _ chapel.BlessingType) (chapel.CharacterBlessing, error) {
	return chapel.CharacterBlessing{}, nil
}

func (m *mockChapelRepo) ClearBlessing(_ context.Context, charID string) error {
	m.clearedID = charID
	return nil
}

func TestRegisterWorkerHandlers_ChapelReset(t *testing.T) {
	repo := &mockSchedRepo{}
	worker := scheduling.NewWorker(repo, time.Second, logging.Nop())
	sched := scheduling.NewService(repo)

	soc := &socServices{
		worker: worker,
		sched:  sched,
	}

	chapelRepo := &mockChapelRepo{}
	chapelService, err := chapel.NewService(chapelRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	soc.registerWorkerHandlers(nil, chapelService, nil, nil)

	// Dispatch residual chapel_reset action via worker: must be a no-op migration
	// that does not clear character blessings.
	ctx := context.Background()
	action := core_scheduling.ScheduledAction{
		ID:          "act-reset-test",
		ActionType:  chapel.ActionTypeChapelReset,
		ActorID:     "system",
		ScheduledAt: time.Now(),
		ExecuteAt:   time.Now(),
		State:       core_scheduling.StatePending,
	}

	worker.ProcessAction(ctx, action)

	if chapelRepo.clearedID != "" {
		t.Errorf("expected residual chapel_reset to not clear blessings, but cleared %s", chapelRepo.clearedID)
	}
}

type mockTavernService struct {
	resetFullnessCalled bool
	resetFullnessErr    error
	claimedCharID       string
	claimResult         tavern.OrderResult
	claimErr            error
}

func (m *mockTavernService) ResetFullness(ctx context.Context, characterID string) error {
	m.resetFullnessCalled = true
	return m.resetFullnessErr
}

func (m *mockTavernService) ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error) {
	m.claimedCharID = characterID
	return m.claimResult, m.claimErr
}

func TestWirePostAdventureHook_ResetsFullnessAndClaimsDelivery(t *testing.T) {
	tavernMock := &mockTavernService{claimErr: tavern.ErrNoActiveDelivery}
	postAdventureHook := buildPostAdventureHook(tavernMock)

	message, err := postAdventureHook(context.Background(), "char-42")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if message != "" {
		t.Fatalf("expected no message without a reservation, got %q", message)
	}
	if !tavernMock.resetFullnessCalled {
		t.Errorf("expected ResetFullness to be called")
	}
	if tavernMock.claimedCharID != "char-42" {
		t.Errorf("expected ClaimDelivery for char-42, got %s", tavernMock.claimedCharID)
	}

	resetErr := errors.New("tavern fullness reset db error")
	failingTavern := &mockTavernService{resetFullnessErr: resetErr}
	failingHook := buildPostAdventureHook(failingTavern)
	if _, err := failingHook(context.Background(), "char-42"); !errors.Is(err, resetErr) {
		t.Fatalf("expected ResetFullness error %v, got %v", resetErr, err)
	}
	if failingTavern.claimedCharID != "" {
		t.Errorf("expected ClaimDelivery not to run when ResetFullness fails, got %s", failingTavern.claimedCharID)
	}
}

func TestWirePostAdventureHook_Messages(t *testing.T) {
	claimErr := errors.New("delivery storage unavailable")
	for _, tt := range []struct {
		name    string
		result  tavern.OrderResult
		err     error
		want    string
		wantErr error
	}{
		{"delivered", tavern.OrderResult{Message: "酒場からスープが届きました！ HPが20、MPが10回復した！"}, nil, "酒場からスープが届きました！ HPが20、MPが10回復した！", nil},
		{"insufficient funds", tavern.OrderResult{}, fmt.Errorf("claim: %w", tavern.ErrInsufficientFunds), "酒場から出前が届きましたが、お金が足りませんでした", nil},
		{"no reservation", tavern.OrderResult{}, tavern.ErrNoActiveDelivery, "", nil},
		{"storage failure", tavern.OrderResult{Message: "uncommitted delivery"}, claimErr, "", claimErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tavernMock := &mockTavernService{claimResult: tt.result, claimErr: tt.err}
			got, err := buildPostAdventureHook(tavernMock)(context.Background(), "hero")
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("message=%q err=%v, want %q err=%v", got, err, tt.want, tt.wantErr)
			}
			if !tavernMock.resetFullnessCalled || tavernMock.claimedCharID != "hero" {
				t.Fatalf("fullness reset or delivery claim missing: %+v", tavernMock)
			}
		})
	}
}

type victoryCrowdTracker struct {
	eventplaza.PresenceTracker
	recordedContext context.Context
	calls           int
	crowd           []eventplaza.Presence
	err             error
}

func (m *victoryCrowdTracker) AppendCrowd(ctx context.Context, crowd []eventplaza.Presence) error {
	m.recordedContext = ctx
	m.calls++
	m.crowd = crowd
	return m.err
}

func TestWireBossVictoryBanquetHook(t *testing.T) {
	tracker := &victoryCrowdTracker{}
	// Embedded nil interfaces fail on any SQL, character or inventory access.
	plaza, err := eventplaza.NewService(
		&struct{ eventplaza.Repository }{}, &struct{ eventplaza.CharacterRepository }{},
		&struct{ eventplaza.InventoryRepository }{}, eventplaza.WithPresenceTracker(tracker))
	if err != nil {
		t.Fatal(err)
	}
	hook := buildVictoryBanquetHook(plaza)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, tier := range []int{1, 2, 3} {
		if err := hook(ctx, "king", "King", "hero", "Hero", tier); err != nil {
			t.Fatal(err)
		}
		if tracker.calls != tier || tracker.recordedContext != ctx || len(tracker.crowd) < 3 || len(tracker.crowd) > 7 {
			t.Fatalf("hook lost crowd call or context: %+v", tracker)
		}
	}
	tracker.err = errors.New("plaza unavailable")
	if err := hook(context.Background(), "king", "King", "hero", "Hero", 1); !errors.Is(err, tracker.err) {
		t.Fatalf("hook suppressed error: %v", err)
	}
}

type stubCharRepo struct{}

func (m *stubCharRepo) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (m *stubCharRepo) FindByIDForUpdate(_ context.Context, _ string) (corecharacter.Character, error) {
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (m *stubCharRepo) Update(_ context.Context, _ corecharacter.Character) error {
	return nil
}

type stubContestRepo struct {
	contest.ContestRepository
	activeRound contest.ContestRound
	activeErr   error
	prepRound   contest.ContestRound
	prepErr     error
	settledErr  error
	saveRoundFn func(round contest.ContestRound) error
}

func (m *stubContestRepo) GetActiveRound(_ context.Context) (contest.ContestRound, error) {
	if m.activeErr != nil {
		return contest.ContestRound{}, m.activeErr
	}
	if m.activeRound.Round == 0 {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return m.activeRound, nil
}

func (m *stubContestRepo) GetActiveRoundForUpdate(_ context.Context) (contest.ContestRound, error) {
	if m.activeErr != nil {
		return contest.ContestRound{}, m.activeErr
	}
	if m.activeRound.Round == 0 {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return m.activeRound, nil
}

func (m *stubContestRepo) GetPreparingRound(_ context.Context) (contest.ContestRound, error) {
	if m.prepErr != nil {
		return contest.ContestRound{}, m.prepErr
	}
	if m.prepRound.Round == 0 {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return m.prepRound, nil
}

func (m *stubContestRepo) GetPreparingRoundForUpdate(_ context.Context) (contest.ContestRound, error) {
	if m.prepErr != nil {
		return contest.ContestRound{}, m.prepErr
	}
	if m.prepRound.Round == 0 {
		return contest.ContestRound{}, contest.ErrContestNotFound
	}
	return m.prepRound, nil
}

func (m *stubContestRepo) GetLatestSettledRound(_ context.Context) (contest.ContestRound, error) {
	if m.settledErr != nil {
		return contest.ContestRound{}, m.settledErr
	}
	return contest.ContestRound{}, contest.ErrContestNotFound
}

func (m *stubContestRepo) ListEntriesByRound(_ context.Context, _ int) ([]contest.ContestEntry, error) {
	return nil, nil
}

func (m *stubContestRepo) SaveRound(_ context.Context, round contest.ContestRound) error {
	m.prepRound = round
	m.prepErr = nil
	if m.saveRoundFn != nil {
		return m.saveRoundFn(round)
	}
	return nil
}

func TestRegisterWorkerHandlers_ContestSettlement(t *testing.T) {
	repo := &mockSchedRepo{}
	worker := scheduling.NewWorker(repo, time.Second, logging.Nop())
	sched := scheduling.NewService(repo)

	soc := &socServices{
		worker: worker,
		sched:  sched,
	}

	endTime := time.Now().Add(-time.Minute)
	savedRound := contest.ContestRound{}
	contestRepo := &stubContestRepo{
		activeRound: contest.ContestRound{
			Round:     1,
			Status:    contest.StatusActive,
			StartTime: endTime.Add(-10 * 24 * time.Hour),
			EndTime:   endTime,
		},
		saveRoundFn: func(r contest.ContestRound) error {
			savedRound = r
			return nil
		},
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	soc.registerWorkerHandlers(nil, nil, nil, contestSvc)

	// Dispatch contest_settlement action via worker
	ctx := context.Background()
	action := core_scheduling.ScheduledAction{
		ID:          "act-contest-settle-test",
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: endTime,
		ExecuteAt:   endTime,
		State:       core_scheduling.StatePending,
	}

	worker.ProcessAction(ctx, action)

	if savedRound.Round != 1 {
		t.Errorf("expected round 1 to be saved, got %d", savedRound.Round)
	}
	if !savedRound.EndTime.After(endTime) {
		t.Errorf("expected extended EndTime after %v, got %v", endTime, savedRound.EndTime)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}
	scheduled := repo.actions[0]
	if scheduled.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected ActionType %s, got %s", contest.ActionTypeContestSettlement, scheduled.ActionType)
	}
}

func TestWireContestSettlement(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	endTime := time.Now().Add(10 * 24 * time.Hour)
	contestRepo := &stubContestRepo{
		activeRound: contest.ContestRound{
			Round:     2,
			Status:    contest.StatusActive,
			StartTime: time.Now(),
			EndTime:   endTime,
		},
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if err := wireContestSettlement(sched, contestSvc); err != nil {
		t.Fatalf("wireContestSettlement failed: %v", err)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}

	action := repo.actions[0]
	if action.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected ActionType %s, got %s", contest.ActionTypeContestSettlement, action.ActionType)
	}
	expectedPrefix := contest.SettlementActionID(2, endTime)
	if !strings.HasPrefix(action.ID, expectedPrefix) {
		t.Errorf("expected action ID with prefix %q, got %q", expectedPrefix, action.ID)
	}
	if action.ActorID != contest.ContestSettlementActorID {
		t.Errorf("expected ActorID %s, got %s", contest.ContestSettlementActorID, action.ActorID)
	}
	if !action.ExecuteAt.Equal(endTime) {
		t.Errorf("expected ExecuteAt %v, got %v", endTime, action.ExecuteAt)
	}
}

func TestWireContestSettlement_NoActiveRound_SchedulesPreparingRound(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	endTime := time.Now().Add(10 * 24 * time.Hour)
	contestRepo := &stubContestRepo{
		activeErr: contest.ErrContestNotFound,
		prepRound: contest.ContestRound{
			Round:     1,
			Status:    contest.StatusPreparing,
			StartTime: time.Now(),
			EndTime:   endTime,
		},
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if err := wireContestSettlement(sched, contestSvc); err != nil {
		t.Fatalf("wireContestSettlement failed: %v", err)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}
	action := repo.actions[0]
	if action.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected ActionType %s, got %s", contest.ActionTypeContestSettlement, action.ActionType)
	}
	expectedPrefix := contest.SettlementActionID(1, endTime)
	if !strings.HasPrefix(action.ID, expectedPrefix) {

		t.Errorf("expected action ID with prefix %q, got %q", expectedPrefix, action.ID)
	}
	if action.ActorID != contest.ContestSettlementActorID {
		t.Errorf("expected ActorID %s, got %s", contest.ContestSettlementActorID, action.ActorID)
	}
}

func TestWireContestSettlement_FreshRepository_InitializesAndSchedules(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	var saved contest.ContestRound
	contestRepo := &stubContestRepo{
		activeErr:  contest.ErrContestNotFound,
		prepErr:    contest.ErrContestNotFound,
		settledErr: contest.ErrContestNotFound,
		saveRoundFn: func(r contest.ContestRound) error {
			saved = r
			return nil
		},
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if err := wireContestSettlement(sched, contestSvc); err != nil {
		t.Fatalf("wireContestSettlement failed: %v", err)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 action scheduled for fresh repository, got %d", len(repo.actions))
	}
	if saved.Round != 1 || saved.Status != contest.StatusPreparing {
		t.Errorf("expected round 1 preparing saved, got %+v", saved)
	}
}

func TestWireContestSettlement_ActiveRoundStorageError_PropagatesError(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	dbErr := errors.New("mariaDB connection timeout")
	contestRepo := &stubContestRepo{
		activeErr: dbErr,
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	err = wireContestSettlement(sched, contestSvc)
	if err == nil {
		t.Fatal("expected storage error, got nil")
	}
	if !strings.Contains(err.Error(), dbErr.Error()) {
		t.Fatalf("expected error containing dbErr, got %v", err)
	}
	if len(repo.actions) != 0 {
		t.Errorf("expected no actions scheduled on storage error, got %d", len(repo.actions))
	}
}

func TestWireContestSettlement_PreparingRoundStorageError_PropagatesError(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	dbErr := errors.New("read preparing round disk I/O error")
	contestRepo := &stubContestRepo{
		activeErr: contest.ErrContestNotFound,
		prepErr:   dbErr,
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	err = wireContestSettlement(sched, contestSvc)
	if err == nil {
		t.Fatal("expected storage error when preparing round read fails, got nil")
	}
	if !strings.Contains(err.Error(), dbErr.Error()) {
		t.Fatalf("expected error containing dbErr, got %v", err)
	}
	if len(repo.actions) != 0 {
		t.Errorf("expected no actions scheduled on storage error, got %d", len(repo.actions))
	}
}

func TestWireContestSettlement_ScheduleError_PropagatesError(t *testing.T) {
	queueErr := errors.New("valkey queue unavailable")
	repo := &mockSchedRepo{
		scheduleErr: queueErr,
	}
	sched := scheduling.NewService(repo)

	endTime := time.Now().Add(10 * 24 * time.Hour)
	contestRepo := &stubContestRepo{
		activeRound: contest.ContestRound{
			Round:     2,
			Status:    contest.StatusActive,
			StartTime: time.Now(),
			EndTime:   endTime,
		},
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	err = wireContestSettlement(sched, contestSvc)
	if err == nil {
		t.Fatal("expected scheduling error, got nil")
	}
	if !strings.Contains(err.Error(), queueErr.Error()) {
		t.Fatalf("expected error containing queueErr, got %v", err)
	}
}

type stubRankingRepo struct {
	ranking.Repository
	legends []ranking.LegendEntry
}

func (s *stubRankingRepo) RecordLegend(_ context.Context, entry ranking.LegendEntry) (bool, error) {
	for _, l := range s.legends {
		if l.Category == entry.Category && l.CharacterID == entry.CharacterID {
			return false, nil
		}
	}
	s.legends = append(s.legends, entry)
	return true, nil
}

func (s *stubRankingRepo) GetLegendInductees(_ context.Context, category ranking.LegendCategory) ([]ranking.LegendEntry, error) {
	var result []ranking.LegendEntry
	for _, l := range s.legends {
		if l.Category == category {
			result = append(result, l)
		}
	}
	return result, nil
}

func TestLegendInductorAdapter(t *testing.T) {
	repo := &stubRankingRepo{}
	rankingSvc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	adapter := legendInductorAdapter{ranking: rankingSvc}

	ctx := context.Background()
	// Record monster completion
	if err := adapter.RecordLegend(ctx, "comp_mon", "char-hero"); err != nil {
		t.Fatalf("RecordLegend failed: %v", err)
	}

	if len(repo.legends) != 1 {
		t.Fatalf("expected 1 legend record, got %d", len(repo.legends))
	}
	if repo.legends[0].Category != "comp_mon" || repo.legends[0].CharacterID != "char-hero" {
		t.Errorf("unexpected legend record: %+v", repo.legends[0])
	}

	// Record weapon completion
	if err := adapter.RecordLegend(ctx, "comp_wea", "char-hero"); err != nil {
		t.Fatalf("RecordLegend comp_wea failed: %v", err)
	}
	if len(repo.legends) != 2 || repo.legends[1].Category != "comp_wea" {
		t.Errorf("expected comp_wea legend record, got %+v", repo.legends)
	}

	// Record armor completion
	if err := adapter.RecordLegend(ctx, "comp_arm", "char-hero"); err != nil {
		t.Fatalf("RecordLegend comp_arm failed: %v", err)
	}
	if len(repo.legends) != 3 || repo.legends[2].Category != "comp_arm" {
		t.Errorf("expected comp_arm legend record, got %+v", repo.legends)
	}

	// Nil ranking check
	nilAdapter := legendInductorAdapter{ranking: nil}
	if err := nilAdapter.RecordLegend(ctx, "comp_job", "char-hero"); err != nil {
		t.Errorf("expected nil error on nil ranking, got %v", err)
	}
}

func TestHomePetAdapter_HeavenWishCompanionIntegration(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	monsterRepo, err := database.NewMonsterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homeMemberRepo, err := database.NewHomeMemberRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	c, err := database.CreateTestCharacter(ctx, db, "HeavenCompanionHero")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM home_members WHERE character_id = ?", c.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM character_monsters WHERE character_id = ?", c.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", c.ID)
	})

	c.GodVisit = string(god.RealmHeaven)
	if err := charRepo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	godSvc, err := god.NewService(
		charRepo,
		god.WithTransactionProvider(txProvider),
		god.WithHomeMemberRepository(homeMemberRepo),
	)
	if err != nil {
		t.Fatalf("god.NewService failed: %v", err)
	}
	if _, err := godSvc.GrantWish(ctx, c.ID, god.WishOrtega); err != nil {
		t.Fatalf("GrantWish WishOrtega failed: %v", err)
	}

	adapter := &homePetAdapter{
		monsterRepo:    monsterRepo,
		homeMemberRepo: homeMemberRepo,
	}

	homeSvc, err := home.NewService(
		homeRepo,
		charRepo,
		home.WithHomePetReader(adapter),
	)
	if err != nil {
		t.Fatalf("home.NewService failed: %v", err)
	}

	monsterSvc := monster.NewService(
		charRepo,
		monsterRepo,
		monster.WithTransactionProvider(txProvider),
		monster.WithHomeCompanionStore(homeMemberRepo),
	)

	// 1. Verify GetHomeView exposes Ortega in ResidentPets
	view, err := homeSvc.GetHomeView(ctx, c.ID, c.ID, c.PlayerID)
	if err != nil {
		t.Fatalf("GetHomeView failed: %v", err)
	}
	if len(view.ResidentPets) != 1 {
		t.Fatalf("expected 1 resident pet, got %d", len(view.ResidentPets))
	}
	memberID := view.ResidentPets[0].ID
	if view.ResidentPets[0].CustomName != "オルテガ" || view.ResidentPets[0].MonsterID != "chr/029.gif" {
		t.Errorf("unexpected resident pet: %+v", view.ResidentPets[0])
	}

	// 2. Verify TalkToCompanion uses Ortega for default dialogue
	talkRes, err := homeSvc.TalkToCompanion(ctx, c.ID)
	if err != nil {
		t.Fatalf("TalkToCompanion failed: %v", err)
	}
	if talkRes.PetName != "オルテガ" {
		t.Errorf("expected pet name オルテガ, got %s", talkRes.PetName)
	}

	// 3. Teach a phrase and verify Ortega speaks taught phrase
	phraseText := "父さんは生きている！"
	_, err = homeSvc.TeachCompanionPhrase(ctx, c.ID, phraseText)
	if err != nil {
		t.Fatalf("TeachCompanionPhrase failed: %v", err)
	}

	talkRes2, err := homeSvc.TalkToCompanion(ctx, c.ID)
	if err != nil {
		t.Fatalf("TalkToCompanion after teach failed: %v", err)
	}
	if talkRes2.PetName != "オルテガ" || talkRes2.Phrase != phraseText {
		t.Errorf("expected オルテガ speaking %q, got %+v", phraseText, talkRes2)
	}

	// 4. Deposit Ortega to ranch box via monster.Service and verify HomeView + ranch summary
	boxInst, err := monsterSvc.DepositToBox(ctx, c.ID, memberID)
	if err != nil {
		t.Fatalf("DepositToBox Ortega failed: %v", err)
	}
	if boxInst.ID != memberID || boxInst.CustomName != "オルテガ" || boxInst.MonsterID != "chr/029.gif" || boxInst.Location != monster.LocationBox {
		t.Fatalf("unexpected deposited Ortega: %+v", boxInst)
	}
	viewAfterDeposit, err := homeSvc.GetHomeView(ctx, c.ID, c.ID, c.PlayerID)
	if err != nil {
		t.Fatalf("GetHomeView after deposit failed: %v", err)
	}
	if len(viewAfterDeposit.ResidentPets) != 0 {
		t.Fatalf("expected 0 resident pets after deposit, got %+v", viewAfterDeposit.ResidentPets)
	}

	// 5. Bring Ortega back to Home and verify identity/icon/phrase are preserved in HomeView
	homeInst, err := monsterSvc.BringToHome(ctx, c.ID, memberID)
	if err != nil {
		t.Fatalf("BringToHome Ortega failed: %v", err)
	}
	if homeInst.ID != memberID || homeInst.CustomName != "オルテガ" || homeInst.MonsterID != "chr/029.gif" || homeInst.Location != monster.LocationHome {
		t.Fatalf("unexpected brought-home Ortega: %+v", homeInst)
	}
	viewAfterBringBack, err := homeSvc.GetHomeView(ctx, c.ID, c.ID, c.PlayerID)
	if err != nil {
		t.Fatalf("GetHomeView after bring back failed: %v", err)
	}
	if len(viewAfterBringBack.ResidentPets) != 1 || viewAfterBringBack.ResidentPets[0].ID != memberID || viewAfterBringBack.ResidentPets[0].CustomName != "オルテガ" || viewAfterBringBack.ResidentPets[0].MonsterID != "chr/029.gif" {
		t.Fatalf("unexpected resident pet after bring back: %+v", viewAfterBringBack.ResidentPets)
	}
}

type wireMockCharRepo struct {
	char corecharacter.Character
}

func (m *wireMockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if id == m.char.ID {
		return m.char, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (m *wireMockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *wireMockCharRepo) Update(_ context.Context, char corecharacter.Character) error {
	m.char = char
	return nil
}

var wireHomeGuildPointsRequiredTables = []string{
	"players",
	"characters",
	"character_depots",
	"character_homes",
	"house_town_capacity_lock",
	"guilds",
	"guild_members",
}

// wireHomeGuildPointsIsolatedDB creates a unique dedicated database holding only copied table
// definitions (no data). BuildHouse checks town capacity, so tests must not run
// against the shared test DB. Cleanup drops only this database; missing admin
// configuration fails the test rather than falling back to the shared DB.
func wireHomeGuildPointsIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})

	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated wire home tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})

	name := fmt.Sprintf("party2_wire_home_%06x_%s", time.Now().UnixNano()%0x1000000, id.New())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error(err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	copyTableDefinitions(t, source, db, wireHomeGuildPointsRequiredTables)
	return db
}

func TestWireHomeGuildPoints_BuildHouseIntegration(t *testing.T) {
	db := wireHomeGuildPointsIsolatedDB(t)
	ctx := context.Background()

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatalf("NewHomeRepository failed: %v", err)
	}

	// Verify town capacity before building in the dedicated database.
	now := time.Now().UTC()
	for _, townID := range []string{"town1", "town2", "town3", "town4"} {
		count, err := homeRepo.CountActiveTownHouses(ctx, townID, now)
		if err != nil {
			t.Fatalf("CountActiveTownHouses for %s failed: %v", townID, err)
		}
		if count != 0 {
			t.Fatalf("expected 0 active houses in isolated town %s, got %d", townID, count)
		}
	}

	// Instantiate production wiring
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatalf("newCoreServices failed: %v", err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatalf("newEconServices failed: %v", err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatalf("newSocServices failed: %v", err)
	}

	// 1. Create a test guild with leader character
	runID := fmt.Sprintf("%06x", time.Now().UnixNano()%0x1000000)
	testGuild, leader, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("WGP_%s", runID), 50000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	guildRepo := soc.guildRepo

	// Verify initial guild points = 0
	g, _, err := guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if g.Points != 0 {
		t.Fatalf("expected initial guild points 0, got %d", g.Points)
	}

	// 2. Test 4 towns: cycle_days * 10 GP awarded to guild
	// town1 (5d -> 50GP), town2 (10d -> 100GP), town3 (15d -> 150GP), town4 (20d -> 200GP)
	townCases := []struct {
		townID    string
		style     string
		price     int
		cycleDays int
		wantGP    int
	}{
		{"town1", "001", 500, 5, 50},
		{"town2", "005", 1500, 10, 100},
		{"town3", "013", 3000, 15, 150},
		{"town4", "021", 5000, 20, 200},
	}

	expectedTotalGP := 0

	for i, tc := range townCases {
		var memberChar corecharacter.Character
		if i == 0 {
			// Leader builds in town1
			memberChar = leader
		} else {
			// Create additional guild members for town2..town4 (due to 1-house per character rule)
			memberChar, err = database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("W%s_M%d", runID, i), 50000)
			if err != nil {
				t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
			}
			m := guild.Member{
				GuildID:     testGuild.ID,
				CharacterID: memberChar.ID,
				Role:        guild.RoleMember,
				Title:       "Member",
				IsPending:   false,
				JoinedAt:    time.Now().UTC(),
			}
			if _, err := guildRepo.AddMember(ctx, m); err != nil {
				t.Fatalf("AddMember failed: %v", err)
			}
		}

		res, err := soc.home.BuildHouse(ctx, memberChar.ID, tc.townID, tc.style)
		if err != nil {
			t.Fatalf("BuildHouse in %s failed: %v", tc.townID, err)
		}
		if res.TownID != tc.townID {
			t.Errorf("expected town %s, got %s", tc.townID, res.TownID)
		}

		expectedTotalGP += tc.wantGP
		g, _, err = guildRepo.GetGuild(ctx, testGuild.ID)
		if err != nil {
			t.Fatalf("GetGuild failed: %v", err)
		}
		if g.Points != int64(expectedTotalGP) {
			t.Errorf("after building in %s: expected %d total guild points, got %d", tc.townID, expectedTotalGP, g.Points)
		}
	}

	// 3. Unaffiliated character can build a house successfully without error, and does NOT award GP to any guild
	soloChar, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("W%s_Sol", runID), 50000)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}
	soloRes, err := soc.home.BuildHouse(ctx, soloChar.ID, "town1", "002")
	if err != nil {
		t.Fatalf("BuildHouse for unaffiliated character failed: %v", err)
	}
	if soloRes.TownID != "town1" {
		t.Errorf("expected town1, got %s", soloRes.TownID)
	}
	// Guild points must remain unchanged
	g, _, err = guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if g.Points != int64(expectedTotalGP) {
		t.Errorf("guild points should not change when unaffiliated builds house: expected %d, got %d", expectedTotalGP, g.Points)
	}

	// 4. Insufficient funds: construction fails, guild points unchanged
	poorChar, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("W%s_Por", runID), 100)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}
	mPoor := guild.Member{
		GuildID:     testGuild.ID,
		CharacterID: poorChar.ID,
		Role:        guild.RoleMember,
		Title:       "Member",
		IsPending:   false,
		JoinedAt:    time.Now().UTC(),
	}
	if _, err := guildRepo.AddMember(ctx, mPoor); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	_, err = soc.home.BuildHouse(ctx, poorChar.ID, "town1", "003")
	if !errors.Is(err, home.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
	// Guild points unchanged
	g, _, err = guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if g.Points != int64(expectedTotalGP) {
		t.Errorf("guild points should not change on insufficient funds failure: expected %d, got %d", expectedTotalGP, g.Points)
	}

	// 5. Retry after failure: give character funds and retry -> succeeds, awards GP exactly once
	poorChar.Money = 5000
	if err := core.charRepo.Update(ctx, poorChar); err != nil {
		t.Fatalf("Update poorChar failed: %v", err)
	}
	retryRes, err := soc.home.BuildHouse(ctx, poorChar.ID, "town1", "003")
	if err != nil {
		t.Fatalf("retry BuildHouse failed: %v", err)
	}
	if retryRes.TownID != "town1" {
		t.Errorf("expected town1, got %s", retryRes.TownID)
	}
	expectedTotalGP += 50
	g, _, err = guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if g.Points != int64(expectedTotalGP) {
		t.Errorf("after successful retry: expected %d total guild points, got %d", expectedTotalGP, g.Points)
	}

	// 6. Already owns house: second build fails, guild points unchanged
	_, err = soc.home.BuildHouse(ctx, poorChar.ID, "town2", "005")
	if !errors.Is(err, home.ErrAlreadyOwnsHouse) {
		t.Errorf("expected ErrAlreadyOwnsHouse, got %v", err)
	}
	g, _, err = guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if g.Points != int64(expectedTotalGP) {
		t.Errorf("guild points should not change on ErrAlreadyOwnsHouse: expected %d, got %d", expectedTotalGP, g.Points)
	}
}

type wireMockMonsterRepo struct {
	monsters map[string]monster.MonsterInstance
}

func (m *wireMockMonsterRepo) ListByCharacterID(_ context.Context, _ string) ([]monster.MonsterInstance, error) {
	return nil, nil
}

func (m *wireMockMonsterRepo) ListByCharacterIDAndLocation(_ context.Context, _, _ string) ([]monster.MonsterInstance, error) {
	return nil, nil
}

func (m *wireMockMonsterRepo) FindByID(_ context.Context, _ string) (monster.MonsterInstance, error) {
	return monster.MonsterInstance{}, nil
}

func (m *wireMockMonsterRepo) FindByIDForUpdate(_ context.Context, _ string) (monster.MonsterInstance, error) {
	return monster.MonsterInstance{}, nil
}

func (m *wireMockMonsterRepo) Save(_ context.Context, inst monster.MonsterInstance) error {
	if m.monsters == nil {
		m.monsters = make(map[string]monster.MonsterInstance)
	}
	m.monsters[inst.ID] = inst
	return nil
}

func (m *wireMockMonsterRepo) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *wireMockMonsterRepo) CountByLocation(_ context.Context, _ string, _ string) (int, error) {
	return len(m.monsters), nil
}

func TestMonsterTamerAdapter(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{
		ID:       "char-tamer-1",
		Name:     "TamerHero",
		JobLevel: 1,
	}
	charRepo := &wireMockCharRepo{char: char}
	monRepo := &wireMockMonsterRepo{monsters: make(map[string]monster.MonsterInstance)}
	svc := monster.NewService(charRepo, monRepo)
	adapter := monsterTamerAdapter{svc: svc}

	// 1. Successful recruitment via battle adapter
	err := adapter.TameMonster(ctx, "char-tamer-1", "slime", "スラりん")
	if err != nil {
		t.Fatalf("expected successful tame, got %v", err)
	}
	if len(monRepo.monsters) != 1 {
		t.Fatalf("expected 1 monster saved, got %d", len(monRepo.monsters))
	}

	// 2. Box capacity full translates to battle.ErrMonsterBoxFull
	for i := 1; i < 50; i++ {
		monRepo.monsters[fmt.Sprintf("m-%d", i)] = monster.MonsterInstance{
			ID:          fmt.Sprintf("m-%d", i),
			CharacterID: "char-tamer-1",
			Location:    monster.LocationBox,
		}
	}
	err = adapter.TameMonster(ctx, "char-tamer-1", "dragon", "ドラごん")
	if !errors.Is(err, battle.ErrMonsterBoxFull) {
		t.Fatalf("expected battle.ErrMonsterBoxFull, got %v", err)
	}

	// 3. Other errors (e.g. character not found) are propagated directly
	err = adapter.TameMonster(ctx, "unknown-char", "slime", "スラりん")
	if !errors.Is(err, monster.ErrCharacterNotFound) {
		t.Fatalf("expected ErrCharacterNotFound for missing character, got %v", err)
	}
}

type wireMockAlchemyRepo struct {
	discovered map[string][]alchemy.DiscoveredRecipe
}

func (m *wireMockAlchemyRepo) GetSynthesisState(_ context.Context, _ string) (alchemy.Synthesis, error) {
	return alchemy.Synthesis{}, nil
}
func (m *wireMockAlchemyRepo) GetSynthesisStateForUpdate(_ context.Context, _ string) (alchemy.Synthesis, error) {
	return alchemy.Synthesis{}, nil
}
func (m *wireMockAlchemyRepo) SaveSynthesisState(_ context.Context, _ alchemy.Synthesis) error {
	return nil
}
func (m *wireMockAlchemyRepo) CompleteOngoingSynthesis(_ context.Context, _ string) error {
	return nil
}
func (m *wireMockAlchemyRepo) GetDiscoveredRecipes(_ context.Context, characterID string) ([]alchemy.DiscoveredRecipe, error) {
	return m.discovered[characterID], nil
}
func (m *wireMockAlchemyRepo) SaveDiscoveredRecipe(_ context.Context, characterID, recipeID string, isCrafted bool) error {
	if m.discovered == nil {
		m.discovered = make(map[string][]alchemy.DiscoveredRecipe)
	}
	m.discovered[characterID] = append(m.discovered[characterID], alchemy.DiscoveredRecipe{RecipeID: recipeID, IsCrafted: isCrafted})
	return nil
}
func (m *wireMockAlchemyRepo) MarkRecipeCrafted(_ context.Context, _, _ string) error {
	return nil
}
func (m *wireMockAlchemyRepo) CountCraftedRecipes(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *wireMockAlchemyRepo) SetCompAlcTitle(_ context.Context, _ string) error {
	return nil
}

type wireMockDepotRepo struct{}

func (wireMockDepotRepo) FindByCharacterID(_ context.Context, _ string) (depot.Depot, error) {
	return depot.Depot{}, nil
}
func (wireMockDepotRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (depot.Depot, error) {
	return depot.Depot{}, nil
}
func (wireMockDepotRepo) Save(_ context.Context, _ depot.Depot) error {
	return nil
}

func TestAlchemyRecipeLearnerAdapter(t *testing.T) {
	ctx := context.Background()

	// 1. Nil alchemy service returns ErrRecipeLearnerUnavailable
	nilAdapter := alchemyRecipeLearnerAdapter{alchemy: nil}
	_, err := nilAdapter.LearnRecipe(ctx, "char-1", []string{"weapon-01"})
	if !errors.Is(err, home.ErrRecipeLearnerUnavailable) {
		t.Fatalf("expected ErrRecipeLearnerUnavailable, got %v", err)
	}

	// 2. Real alchemy service with item catalog
	char := corecharacter.Character{ID: "char-1", Name: "Hero"}
	charRepo := &wireMockCharRepo{char: char}
	depotRepo := wireMockDepotRepo{}
	alchemyRepo := &wireMockAlchemyRepo{}

	recipes, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	alcSvc, err := alchemy.NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("alchemy.NewService failed: %v", err)
	}

	adapter := alchemyRecipeLearnerAdapter{
		alchemy: alcSvc,
		items:   items,
	}

	// Learn weapon-01: clean-room name "ヒノキの棒" must be returned, NOT legacy "ひのきの棒"
	res, err := adapter.LearnRecipe(ctx, "char-1", []string{"weapon-01"})
	if err != nil {
		t.Fatalf("unexpected error learning weapon-01: %v", err)
	}
	if res.BaseName != "ヒノキの棒" {
		t.Errorf("expected clean-room BaseName 'ヒノキの棒', got %q", res.BaseName)
	}
	if strings.Contains(res.BaseName, "ひのき") {
		t.Errorf("legacy proprietary/old name leaked into BaseName: %q", res.BaseName)
	}

	// Learn item-006: clean-room name "霊樹の葉" must be returned, NOT legacy "世界樹の葉"
	res, err = adapter.LearnRecipe(ctx, "char-1", []string{"item-006"})
	if err != nil {
		t.Fatalf("unexpected error learning item-006: %v", err)
	}
	if res.BaseName != "霊樹の葉" {
		t.Errorf("expected clean-room BaseName '霊樹の葉', got %q", res.BaseName)
	}
	if strings.Contains(res.BaseName, "世界樹") {
		t.Errorf("legacy proprietary/old name leaked into BaseName: %q", res.BaseName)
	}

	// Learn single-ingredient recipe (recipe-003: item-002 x2 -> item-003)
	res, err = adapter.LearnRecipe(ctx, "char-1", []string{"item-002"})
	if err != nil {
		t.Fatalf("unexpected error learning item-002: %v", err)
	}
	if res.BaseName != "上薬草" || res.MaterialName != "上薬草" {
		t.Errorf("expected 上薬草 x 上薬草, got %s x %s", res.BaseName, res.MaterialName)
	}

	// Exhaustion translates alchemy.ErrNoRecipesToLearn to home.ErrNoRecipesToLearn
	// Discover remaining weapon-01 recipe (it has 2 recipes total: recipe-040 and recipe-041)
	_, err = adapter.LearnRecipe(ctx, "char-1", []string{"weapon-01"})
	if err != nil {
		t.Fatalf("unexpected error learning second weapon-01 recipe: %v", err)
	}
	// Third attempt must return home.ErrNoRecipesToLearn
	_, err = adapter.LearnRecipe(ctx, "char-1", []string{"weapon-01"})
	if !errors.Is(err, home.ErrNoRecipesToLearn) {
		t.Fatalf("expected home.ErrNoRecipesToLearn, got %v", err)
	}

	// 3. Fallback when items is nil
	nilItemsAdapter := alchemyRecipeLearnerAdapter{
		alchemy: alcSvc,
		items:   nil,
	}
	resFallback, err := nilItemsAdapter.LearnRecipe(ctx, "char-1", []string{"weapon-18"})
	if err != nil {
		t.Fatalf("unexpected error with nil items: %v", err)
	}
	if resFallback.BaseName != "weapon-18" {
		t.Errorf("expected fallback to definition ID 'weapon-18', got %q", resFallback.BaseName)
	}
}
