package party

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type memoryAdventureRepoForPartyTest struct {
	records []adventure.Adventure
	saved   map[string]adventure.Adventure
	saveErr error
}

func newMemoryAdventureRepoForPartyTest() *memoryAdventureRepoForPartyTest {
	return &memoryAdventureRepoForPartyTest{
		saved: make(map[string]adventure.Adventure),
	}
}

func (m *memoryAdventureRepoForPartyTest) Save(_ context.Context, adv adventure.Adventure) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved[adv.ID] = adv
	m.records = append(m.records, adv)
	return nil
}

func (m *memoryAdventureRepoForPartyTest) ClearCountForCharacterAndStage(characterID, stageID string, seasonalSubID int) int {
	count := 0
	for _, a := range m.saved {
		if a.CharacterID == characterID && a.Type == stageID && a.SeasonalSubID == seasonalSubID && a.IsCleared {
			count++
		}
	}
	return count
}

func (m *memoryAdventureRepoForPartyTest) TotalVictoriesForCharacter(characterID string) int {
	victories := 0
	for _, a := range m.saved {
		if a.CharacterID == characterID && a.Resolved && (a.IsCleared || (a.BattleResult.Outcome == corebattle.OutcomeWin && a.BattleResult.WinnerID == characterID)) {
			victories++
		}
	}
	return victories
}

func TestParty_CoopClearRecordsInAdventureRepo(t *testing.T) {
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	leader := corecharacter.Character{
		ID:    "lead-1",
		Name:  "Leader",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	member := corecharacter.Character{
		ID:    "mem-1",
		Name:  "Member",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	svc, _, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo))
	partyID := createReadyParty(ctx, t, svc, nil, charRepo, leader, member)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != string(corebattle.OutcomeWin) {
		t.Fatalf("expected party adventure outcome win, got %s", res.Outcome)
	}

	// Verify both leader and member have adventure records persisted in advRepo
	if len(advRepo.saved) != 2 {
		t.Fatalf("expected 2 adventure records in advRepo, got %d", len(advRepo.saved))
	}

	leadCleared := advRepo.ClearCountForCharacterAndStage(leader.ID, "stage-01", 0)
	if leadCleared != 1 {
		t.Errorf("leader stage-01 clear count = %d, want 1", leadCleared)
	}

	memCleared := advRepo.ClearCountForCharacterAndStage(member.ID, "stage-01", 0)
	if memCleared != 1 {
		t.Errorf("member stage-01 clear count = %d, want 1", memCleared)
	}
}

func TestParty_Solo1PlusParty1Equals2(t *testing.T) {
	// Requirements check: "solo1+party1=2"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	hero := corecharacter.Character{
		ID:    "hero-common",
		Name:  "Hero",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	ally := corecharacter.Character{
		ID:    "ally-common",
		Name:  "Ally",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	// 1. Solo clear: Record 1 solo adventure clear for hero
	soloAdv := adventure.Adventure{
		ID:            "solo-run-1-hero",
		CharacterID:   hero.ID,
		Type:          "stage-01",
		StageID:       "stage-01",
		FloorsCleared: 10,
		IsCleared:     true,
		PartySize:     1,
		Resolved:      true,
		BattleResult:  corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: hero.ID},
	}
	if err := advRepo.Save(ctx, soloAdv); err != nil {
		t.Fatalf("Save solo: %v", err)
	}

	// 2. Party clear: Hero participates in a 2-person party adventure and clears it
	svc, _, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo))
	partyID := createReadyParty(ctx, t, svc, nil, charRepo, hero, ally)

	_, err := svc.StartPartyAdventure(ctx, partyID, hero.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}

	// Verify: solo 1 + party 1 = 2 clears for hero
	totalVictories := advRepo.TotalVictoriesForCharacter(hero.ID)
	if totalVictories != 2 {
		t.Errorf("hero total victories = %d, want 2 (solo1+party1=2)", totalVictories)
	}

	stageClears := advRepo.ClearCountForCharacterAndStage(hero.ID, "stage-01", 0)
	if stageClears != 2 {
		t.Errorf("hero stage-01 clears = %d, want 2 (solo1+party1=2)", stageClears)
	}
}

func TestParty_SameRunRetryIdempotent(t *testing.T) {
	// Requirements check: "同じrun retryで1"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	hero := corecharacter.Character{
		ID:    "hero-retry",
		Name:  "HeroRetry",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	// Simulate same run saved twice (retry)
	adv := adventure.Adventure{
		ID:            "det-run-same-id",
		CharacterID:   hero.ID,
		Type:          "stage-01",
		StageID:       "stage-01",
		FloorsCleared: 10,
		IsCleared:     true,
		PartySize:     1,
		Resolved:      true,
		BattleResult:  corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: hero.ID},
	}

	if err := advRepo.Save(ctx, adv); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	// Retry save of identical run
	if err := advRepo.Save(ctx, adv); err != nil {
		t.Fatalf("retry save failed: %v", err)
	}

	totalVictories := advRepo.TotalVictoriesForCharacter(hero.ID)
	if totalVictories != 1 {
		t.Errorf("TotalVictories on retry = %d, want 1 (not duplicated)", totalVictories)
	}
	clears := advRepo.ClearCountForCharacterAndStage(hero.ID, "stage-01", 0)
	if clears != 1 {
		t.Errorf("Stage-01 clears on retry = %d, want 1 (not duplicated)", clears)
	}
}

func TestParty_SaveAdventureFailureRollback(t *testing.T) {
	// Requirements check: "保存失敗rollback"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()
	injectedErr := errors.New("simulated database failure on adventure save")
	advRepo.saveErr = injectedErr

	leader := corecharacter.Character{
		ID:    "lead-fail",
		Name:  "LeaderFail",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	txProvider := &mockTransactionProvider{}
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo), WithTransactionProvider(txProvider))
	partyID := createReadyParty(ctx, t, svc, nil, charRepo, leader)

	_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err == nil {
		t.Fatal("expected StartPartyAdventure to fail when advRepo.Save fails")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected error to wrap injectedErr, got %v", err)
	}

	// Verify rollback: transaction rollback executed, party status reset to recruiting
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback, got 0 rollbacks")
	}

	p, err := partyRepo.GetParty(ctx, partyID)
	if err != nil {
		t.Fatalf("GetParty: %v", err)
	}
	if p.Status != StatusRecruiting {
		t.Errorf("party status = %s, want %s after rollback", p.Status, StatusRecruiting)
	}

	if len(advRepo.saved) != 0 {
		t.Errorf("expected 0 adventure records in advRepo after rollback, got %d", len(advRepo.saved))
	}
}

func TestParty_BackfillPastAdventureLogs(t *testing.T) {
	// Requirements check: "既存recordsとbackfill"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()
	partyRepo := newMockPartyRepository()

	// Simulate past party adventure log with survivors
	battleRes := corebattle.PartyBattleResult{
		Outcome:        corebattle.OutcomeWin,
		Turns:          5,
		AlliesSurvived: []string{"char-a", "char-b"},
	}
	detailsJSON, _ := json.Marshal(battleRes)

	pastLog := PartyAdventureLog{
		ID:          "past-log-1",
		PartyID:     "party-past",
		StageID:     "stage-02",
		Outcome:     string(corebattle.OutcomeWin),
		Turns:       5,
		TotalEXP:    200,
		TotalGold:   100,
		DetailsJSON: string(detailsJSON),
		CreatedAt:   time.Now().UTC().Add(-24 * time.Hour),
	}
	if err := partyRepo.SaveAdventureLog(ctx, pastLog); err != nil {
		t.Fatalf("SaveAdventureLog: %v", err)
	}

	// Run backfill
	backfilled, err := BackfillPartyAdventureLogs(ctx, partyRepo, advRepo, "party-past")
	if err != nil {
		t.Fatalf("BackfillPartyAdventureLogs: %v", err)
	}
	if backfilled != 2 {
		t.Errorf("backfilled count = %d, want 2", backfilled)
	}

	// Verify records in advRepo
	clearedA := advRepo.ClearCountForCharacterAndStage("char-a", "stage-02", 0)
	if clearedA != 1 {
		t.Errorf("char-a stage-02 clears = %d, want 1", clearedA)
	}
	clearedB := advRepo.ClearCountForCharacterAndStage("char-b", "stage-02", 0)
	if clearedB != 1 {
		t.Errorf("char-b stage-02 clears = %d, want 1", clearedB)
	}

	// Run backfill again: idempotent, records are not duplicated
	backfilled2, err := BackfillPartyAdventureLogs(ctx, partyRepo, advRepo, "party-past")
	if err != nil {
		t.Fatalf("second BackfillPartyAdventureLogs: %v", err)
	}
	if backfilled2 != 2 {
		t.Errorf("second backfill count = %d, want 2", backfilled2)
	}

	clearedAAfter := advRepo.ClearCountForCharacterAndStage("char-a", "stage-02", 0)
	if clearedAAfter != 1 {
		t.Errorf("char-a stage-02 clears after re-backfill = %d, want 1", clearedAAfter)
	}
}

func TestParty_Stage26SeasonalClear(t *testing.T) {
	// Requirements check: "四季の個別達成"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	stages, _ := adventure.InitialStageCatalog()
	stage26, _ := stages.FindByID("stage-26")

	leader := corecharacter.Character{
		ID:    "hero-seasons",
		Name:  "HeroSeasons",
		Level: 90,
		Stats: corecharacter.Stats{HP: 300, MaxHP: 300, MP: 100, MaxMP: 100},
	}

	svc, _, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo))
	charRepo.chars[leader.ID] = leader

	// 1. Create party with Spring season
	pDetailSpring, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:       "SpringParty",
		StageID:    stage26.ID,
		MaxMembers: 1,
	})
	if err != nil {
		t.Fatalf("CreateParty spring: %v", err)
	}
	// Manually set spring season if random was used
	pSpring := pDetailSpring.Party
	pSpring.Season = "spring"
	_ = svc.repo.SaveParty(ctx, pSpring)

	_, err = svc.StartPartyAdventure(ctx, pSpring.ID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure spring: %v", err)
	}

	// Verify spring clear recorded
	springClears := advRepo.ClearCountForCharacterAndStage(leader.ID, "stage-26", 1)
	if springClears != 1 {
		t.Errorf("stage-26 spring clears = %d, want 1", springClears)
	}

	// Disband spring party so leader can start summer party
	if err := svc.DisbandParty(ctx, pSpring.ID, leader.ID); err != nil {
		t.Fatalf("DisbandParty spring: %v", err)
	}

	// 2. Create party with Summer season
	pDetailSummer, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:       "SummerParty",
		StageID:    stage26.ID,
		MaxMembers: 1,
	})
	if err != nil {
		t.Fatalf("CreateParty summer: %v", err)
	}
	pSummer := pDetailSummer.Party
	pSummer.Season = "summer"
	_ = svc.repo.SaveParty(ctx, pSummer)

	_, err = svc.StartPartyAdventure(ctx, pSummer.ID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure summer: %v", err)
	}

	// Verify summer clear recorded
	summerClears := advRepo.ClearCountForCharacterAndStage(leader.ID, "stage-26", 2)
	if summerClears != 1 {
		t.Errorf("stage-26 summer clears = %d, want 1", summerClears)
	}

	// Verify both spring and summer are distinct
	if springClears != 1 || summerClears != 1 {
		t.Errorf("expected 1 spring and 1 summer clear, got spring=%d summer=%d", springClears, summerClears)
	}
}
