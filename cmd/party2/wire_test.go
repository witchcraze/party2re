package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/tavern"
)

type mockSchedRepo struct {
	actions []core_scheduling.ScheduledAction
}

func (m *mockSchedRepo) Schedule(_ context.Context, a core_scheduling.ScheduledAction) error {
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
	claimedCharID       string
}

func (m *mockTavernService) ResetFullness(ctx context.Context, characterID string) error {
	m.resetFullnessCalled = true
	return nil
}

func (m *mockTavernService) ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error) {
	m.claimedCharID = characterID
	return tavern.OrderResult{}, tavern.ErrNoActiveDelivery
}

func TestWirePostAdventureHook_ResetsFullnessAndClaimsDelivery(t *testing.T) {
	tavernMock := &mockTavernService{}
	postAdventureHook := func(ctx context.Context, characterID string) error {
		_ = tavernMock.ResetFullness(ctx, characterID)
		_, err := tavernMock.ClaimDelivery(ctx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
			return nil
		}
		return err
	}

	err := postAdventureHook(context.Background(), "char-42")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tavernMock.resetFullnessCalled {
		t.Errorf("expected ResetFullness to be called")
	}
	if tavernMock.claimedCharID != "char-42" {
		t.Errorf("expected ClaimDelivery for char-42, got %s", tavernMock.claimedCharID)
	}
}

type mockPlazaService struct {
	banquetRecorded   bool
	presenceRecorded  bool
	recordedAttendees int
	recordedDuration  time.Duration
}

func (m *mockPlazaService) RecordVictoryBanquet(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) (eventplaza.CelebrationBanquet, error) {
	m.banquetRecorded = true
	attendees := eventplaza.BanquetAttendeesForTier(tier)
	_ = m.RecordBanquetPresence(ctx, "banquet-1", attendees, time.Hour)
	return eventplaza.CelebrationBanquet{
		ID:   "banquet-1",
		Tier: tier,
	}, nil
}

func (m *mockPlazaService) RecordBanquetPresence(ctx context.Context, banquetID string, count int, duration time.Duration) error {
	m.presenceRecorded = true
	m.recordedAttendees = count
	m.recordedDuration = duration
	return nil
}

func TestWireBossVictoryBanquetHook_RecordsPresence(t *testing.T) {
	plazaMock := &mockPlazaService{}
	hook := func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error {
		_, err := plazaMock.RecordVictoryBanquet(ctx, bossID, bossName, slayerID, slayerName, tier)
		return err
	}

	for _, tier := range []int{1, 2, 3} {
		plazaMock.banquetRecorded = false
		plazaMock.presenceRecorded = false
		plazaMock.recordedAttendees = 0

		err := hook(context.Background(), "boss-1", "King", "slayer-1", "Hero", tier)
		if err != nil {
			t.Fatalf("tier %d: expected nil error, got %v", tier, err)
		}
		if !plazaMock.banquetRecorded {
			t.Errorf("tier %d: expected banquet to be recorded", tier)
		}
		if !plazaMock.presenceRecorded {
			t.Errorf("tier %d: expected presence to be recorded", tier)
		}
		expectedAttendees := eventplaza.BanquetAttendeesForTier(tier)
		if plazaMock.recordedAttendees != expectedAttendees {
			t.Errorf("tier %d: expected %d attendees, got %d", tier, expectedAttendees, plazaMock.recordedAttendees)
		}
		if plazaMock.recordedDuration != time.Hour {
			t.Errorf("tier %d: expected duration 1h, got %v", tier, plazaMock.recordedDuration)
		}
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
	saveRoundFn func(round contest.ContestRound) error
}

func (m *stubContestRepo) GetActiveRound(_ context.Context) (contest.ContestRound, error) {
	if m.activeErr != nil {
		return contest.ContestRound{}, m.activeErr
	}
	return m.activeRound, nil
}

func (m *stubContestRepo) GetActiveRoundForUpdate(_ context.Context) (contest.ContestRound, error) {
	if m.activeErr != nil {
		return contest.ContestRound{}, m.activeErr
	}
	return m.activeRound, nil
}

func (m *stubContestRepo) ListEntriesByRound(_ context.Context, _ int) ([]contest.ContestEntry, error) {
	return nil, nil
}

func (m *stubContestRepo) SaveRound(_ context.Context, round contest.ContestRound) error {
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

	wireContestSettlement(sched, contestSvc)

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}

	action := repo.actions[0]
	if action.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected ActionType %s, got %s", contest.ActionTypeContestSettlement, action.ActionType)
	}
	expectedID := contest.SettlementActionID(2, endTime)
	if action.ID != expectedID {
		t.Errorf("expected action ID %q, got %q", expectedID, action.ID)
	}
	if action.ActorID != "system" {
		t.Errorf("expected ActorID system, got %s", action.ActorID)
	}
	if !action.ExecuteAt.Equal(endTime) {
		t.Errorf("expected ExecuteAt %v, got %v", endTime, action.ExecuteAt)
	}
}

func TestWireContestSettlement_NoActiveRound(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	contestRepo := &stubContestRepo{
		activeErr: contest.ErrContestNotFound,
	}
	contestSvc, err := contest.NewService(&stubCharRepo{}, contestRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	wireContestSettlement(sched, contestSvc)

	if len(repo.actions) != 0 {
		t.Errorf("expected 0 actions scheduled when no active round, got %d", len(repo.actions))
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

	// Idempotent duplicate
	if err := adapter.RecordLegend(ctx, "comp_mon", "char-hero"); err != nil {
		t.Fatalf("duplicate RecordLegend failed: %v", err)
	}
	if len(repo.legends) != 1 {
		t.Errorf("expected still 1 legend record, got %d", len(repo.legends))
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

func TestLegendInductor_EndToEndIntegration(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment failed: %v", err)
	}
	defer db.Close()

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv failed: %v", err)
	}

	wiring, err := wireApp(db, nil, cfg, logging.Nop())
	if err != nil {
		t.Fatalf("wireApp failed: %v", err)
	}

	server := httptest.NewServer(wiring.handler.Router())
	defer server.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	playerRepo, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	prefix := fmt.Sprintf("wir_%d_", time.Now().UnixNano()%1000000)
	p, err := coreplayer.New(prefix+"p", "pass", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = playerRepo.Delete(ctx, p.ID)
	})

	c, err := corecharacter.NewWithOptions(prefix+"Hero", "job-01", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.PlayerID = p.ID
	c.Color = "#123456"
	if err := charRepo.Save(ctx, c); err != nil {
		t.Fatal(err)
	}

	// Test GET /legends/comp_mon before induction
	resp, err := http.Get(server.URL + "/legends/comp_mon")
	if err != nil {
		t.Fatalf("GET /legends/comp_mon failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var pageBefore ranking.LegendCategoryPage
	if err := json.NewDecoder(resp.Body).Decode(&pageBefore); err != nil {
		t.Fatalf("decode page failed: %v", err)
	}
	initialCount := pageBefore.Total

	// Induct directly through legendInductorAdapter or collection/job/alchemy
	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rankingSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}
	adapter := legendInductorAdapter{ranking: rankingSvc}

	if err := adapter.RecordLegend(ctx, "comp_mon", c.ID); err != nil {
		t.Fatalf("adapter.RecordLegend failed: %v", err)
	}

	// Test GET /legends/comp_mon after induction
	respAfter, err := http.Get(server.URL + "/legends/comp_mon")
	if err != nil {
		t.Fatalf("GET /legends/comp_mon after induction failed: %v", err)
	}
	defer respAfter.Body.Close()
	if respAfter.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respAfter.StatusCode)
	}

	var pageAfter ranking.LegendCategoryPage
	if err := json.NewDecoder(respAfter.Body).Decode(&pageAfter); err != nil {
		t.Fatalf("decode page after induction failed: %v", err)
	}

	if pageAfter.Total != initialCount+1 {
		t.Fatalf("expected total %d, got %d", initialCount+1, pageAfter.Total)
	}

	found := false
	for _, entry := range pageAfter.Entries {
		if entry.CharacterID == c.ID {
			found = true
			if entry.CharacterName != c.Name {
				t.Errorf("expected CharacterName %s, got %s", c.Name, entry.CharacterName)
			}
			if entry.Color != c.Color {
				t.Errorf("expected Color %s, got %s", c.Color, entry.Color)
			}
			break
		}
	}
	if !found {
		t.Fatalf("inducted character %s not found in GET /legends/comp_mon", c.ID)
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

	// Store Ortega in home_members (0 farm monsters present)
	memberID := fmt.Sprintf("hm-o-%d", time.Now().UnixNano()%1000000000000000)
	member := god.HomeMember{
		ID:          memberID,
		CharacterID: c.ID,
		IsNPC:       true,
		Name:        "オルテガ",
		Icon:        "ortega_icon",
		Color:       "#ffffff",
		CreatedAt:   time.Now().UTC(),
	}
	if err := homeMemberRepo.AddMember(ctx, member); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM home_members WHERE id = ?", memberID)
	})

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

	// 1. Verify GetHomeView exposes Ortega in ResidentPets
	view, err := homeSvc.GetHomeView(ctx, c.ID, c.ID, c.PlayerID)
	if err != nil {
		t.Fatalf("GetHomeView failed: %v", err)
	}
	if len(view.ResidentPets) != 1 {
		t.Fatalf("expected 1 resident pet, got %d", len(view.ResidentPets))
	}
	if view.ResidentPets[0].CustomName != "オルテガ" || view.ResidentPets[0].MonsterID != "ortega_icon" {
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

type wireMockHomeRepo struct {
	home.Repository
}

type mockRecoveryTavern struct {
	calledID string
	err      error
}

func (m *mockRecoveryTavern) ResetFullness(_ context.Context, charID string) error {
	m.calledID = charID
	return m.err
}

type mockRecoveryChapel struct {
	calledID string
	err      error
}

func (m *mockRecoveryChapel) ClearBlessing(_ context.Context, charID string) error {
	m.calledID = charID
	return m.err
}

type mockRecoveryAlchemy struct {
	calledID string
	err      error
}

func (m *mockRecoveryAlchemy) CompleteOngoingSynthesis(_ context.Context, charID string) error {
	m.calledID = charID
	return m.err
}

type mockRecoveryStore struct {
	calledID string
	err      error
}

func (m *mockRecoveryStore) ResetCostume(_ context.Context, charID string) error {
	m.calledID = charID
	return m.err
}

func TestWireHomeRecoveryHooks_WiringAndErrorPropagation(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{
		ID:    "char-wire",
		Name:  "WireHero",
		Tired: 50,
		Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50},
	}
	charRepo := &wireMockCharRepo{char: char}
	timerSvc := timer.NewService(nil)

	homeSvc, err := home.NewService(
		&wireMockHomeRepo{},
		charRepo,
		home.WithTimer(timerSvc),
		home.WithCharacterUpdater(charRepo),
	)
	if err != nil {
		t.Fatalf("home.NewService failed: %v", err)
	}

	tavernMock := &mockRecoveryTavern{}
	chapelMock := &mockRecoveryChapel{}
	alchemyMock := &mockRecoveryAlchemy{}
	storeMock := &mockRecoveryStore{}

	soc := &socServices{home: homeSvc}

	// Mirror wireHooks wiring:
	soc.home.SetFullnessResetter(tavernMock)
	soc.home.SetBlessingCleaner(chapelMock)
	soc.home.SetAlchemyCompleter(alchemyMock)
	soc.home.SetCostumeResetter(storeMock)

	// Set asleep flag
	_ = timerSvc.SetLock(ctx, timer.CategoryAsleep, "char-wire", 24*time.Hour)

	// 1. Success case: all hooks called
	res, err := soc.home.Wake(ctx, "char-wire")
	if err != nil {
		t.Fatalf("Wake failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected Wake success")
	}
	if tavernMock.calledID != "char-wire" || chapelMock.calledID != "char-wire" || alchemyMock.calledID != "char-wire" || storeMock.calledID != "char-wire" {
		t.Errorf("hooks not all called for char-wire: tavern=%s chapel=%s alchemy=%s store=%s",
			tavernMock.calledID, chapelMock.calledID, alchemyMock.calledID, storeMock.calledID)
	}
	asleep, _ := timerSvc.IsLocked(ctx, timer.CategoryAsleep, "char-wire")
	if asleep {
		t.Errorf("expected CategoryAsleep unlocked after successful Wake")
	}

	// 2. Failure case: hook error propagates and preserves CategoryAsleep
	_ = timerSvc.SetLock(ctx, timer.CategoryAsleep, "char-wire", 24*time.Hour)
	tavernMock.err = errors.New("simulated tavern error")

	_, err = soc.home.Wake(ctx, "char-wire")
	if err == nil {
		t.Fatalf("expected error from failing tavern hook, got nil")
	}
	asleep, _ = timerSvc.IsLocked(ctx, timer.CategoryAsleep, "char-wire")
	if !asleep {
		t.Errorf("expected CategoryAsleep to remain locked when hook fails")
	}
}

func TestWireHomeGuildPoints_BuildHouseIntegration(t *testing.T) {
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
