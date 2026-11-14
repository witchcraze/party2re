package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/home"
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
	if err := hook(ctx, "king", "King", "hero", "Hero", 1); err != nil {
		t.Fatal(err)
	}
	if tracker.calls != 1 || tracker.recordedContext != ctx {
		t.Fatalf("hook lost crowd call or context: %+v", tracker)
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

func TestWireHomeGuildPoints_BuildHouseIntegration(t *testing.T) {
	db := wiringIsolatedDB(t, []string{"players", "characters", "character_depots", "character_homes", "house_town_capacity_lock", "guilds", "guild_members"})
	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	g, leader, err := database.CreateTestGuildWithLeader(ctx, db, "HouseWiring", 50000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := soc.home.BuildHouse(ctx, leader.ID, "town1", "001"); err != nil {
		t.Fatal(err)
	}
	updated, _, err := soc.guildRepo.GetGuild(ctx, g.ID)
	if err != nil || updated.Points == g.Points {
		t.Fatalf("house guild connection = %+v, %v", updated, err)
	}
}

type wireMockMonsterRepo struct {
	monster.MonsterRepository
	monsters map[string]monster.MonsterInstance
	countErr error
}

func (m *wireMockMonsterRepo) Save(_ context.Context, inst monster.MonsterInstance) error {
	m.monsters[inst.ID] = inst
	return nil
}
func (m *wireMockMonsterRepo) CountByLocation(_ context.Context, _, _ string) (int, error) {
	return 0, m.countErr
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

	monRepo.countErr = monster.ErrBoxFull
	err = adapter.TameMonster(ctx, "char-tamer-1", "dragon", "Dragon")
	if !errors.Is(err, battle.ErrMonsterBoxFull) {
		t.Fatalf("expected battle.ErrMonsterBoxFull, got %v", err)
	}
	monRepo.countErr = nil

	// 3. Other errors (e.g. character not found) are propagated directly
	err = adapter.TameMonster(ctx, "unknown-char", "slime", "スラりん")
	if !errors.Is(err, monster.ErrCharacterNotFound) {
		t.Fatalf("expected ErrCharacterNotFound for missing character, got %v", err)
	}
}

type wireMockAlchemyRepo struct {
	alchemy.Repository
	readErr error
}

func (m *wireMockAlchemyRepo) GetDiscoveredRecipes(context.Context, string) ([]alchemy.DiscoveredRecipe, error) {
	return nil, m.readErr
}
func (m *wireMockAlchemyRepo) SaveDiscoveredRecipe(context.Context, string, string, bool) error {
	return nil
}

func TestAlchemyRecipeLearnerAdapter(t *testing.T) {
	ctx := context.Background()
	if _, err := (alchemyRecipeLearnerAdapter{}).LearnRecipe(ctx, "hero", nil); !errors.Is(err, home.ErrRecipeLearnerUnavailable) {
		t.Fatalf("missing learner: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		ingredients []alchemy.Ingredient
		items       coreitem.DefinitionProvider
		want        home.LearnedRecipe
	}{
		{"catalog names", []alchemy.Ingredient{{DefinitionID: "weapon-01", Quantity: 1}, {DefinitionID: "item-006", Quantity: 1}}, items, home.LearnedRecipe{BaseName: "ヒノキの棒", MaterialName: "霊樹の葉"}},
		{"single ingredient", []alchemy.Ingredient{{DefinitionID: "item-002", Quantity: 2}}, items, home.LearnedRecipe{BaseName: "上薬草", MaterialName: "上薬草"}},
		{"missing catalog entries", []alchemy.Ingredient{{DefinitionID: "unknown-base", Quantity: 1}, {DefinitionID: "unknown-material", Quantity: 1}}, items, home.LearnedRecipe{BaseName: "unknown-base", MaterialName: "unknown-material"}},
		{"nil catalog", []alchemy.Ingredient{{DefinitionID: "weapon-18", Quantity: 1}}, nil, home.LearnedRecipe{BaseName: "weapon-18", MaterialName: "weapon-18"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipes, err := alchemy.NewRecipeCatalog([]alchemy.Recipe{{ID: "recipe", Name: "Test", ResultItemDefinitionID: "item-003", ResultQuantity: 1, Ingredients: tc.ingredients}})
			if err != nil {
				t.Fatal(err)
			}
			repo := &wireMockAlchemyRepo{}
			svc, err := alchemy.NewService(&wireMockCharRepo{}, &struct{ alchemy.DepotRepository }{}, repo, recipes, items)
			if err != nil {
				t.Fatal(err)
			}
			adapter := alchemyRecipeLearnerAdapter{alchemy: svc, items: tc.items}
			got, err := adapter.LearnRecipe(ctx, "hero", nil)
			if err != nil || got != tc.want {
				t.Fatalf("conversion = %+v, %v; want %+v", got, err, tc.want)
			}
			for _, e := range []error{alchemy.ErrNoRecipesToLearn, errors.New("recipe read failed")} {
				repo.readErr = e
				want := e
				if errors.Is(e, alchemy.ErrNoRecipesToLearn) {
					want = home.ErrNoRecipesToLearn
				}
				if _, err := adapter.LearnRecipe(ctx, "hero", nil); !errors.Is(err, want) {
					t.Fatalf("error conversion = %v, want %v", err, want)
				}
			}
		})
	}
}
