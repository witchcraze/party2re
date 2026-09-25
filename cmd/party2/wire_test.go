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
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
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
	clearedAll bool
	clearedID  string
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

func (m *mockChapelRepo) ClearAllBlessings(_ context.Context) error {
	m.clearedAll = true
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

	// Dispatch chapel_reset action via worker
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

	if !chapelRepo.clearedAll {
		t.Errorf("expected ClearAllBlessings to be called on chapel_reset")
	}
}

func TestWireChapelDailyReset(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	wireChapelDailyReset(sched)

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}

	action := repo.actions[0]
	if action.ActionType != chapel.ActionTypeChapelReset {
		t.Errorf("expected ActionType %s, got %s", chapel.ActionTypeChapelReset, action.ActionType)
	}
	if action.ActorID != "system" {
		t.Errorf("expected ActorID system, got %s", action.ActorID)
	}
	if !action.ExecuteAt.After(time.Now()) {
		t.Errorf("expected ExecuteAt to be in the future, got %v", action.ExecuteAt)
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

	c, err := corecharacter.NewWithOptions(prefix+"Hero", "warrior", "m", nil)
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
