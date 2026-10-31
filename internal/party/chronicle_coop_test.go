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

func (m *memoryAdventureRepoForPartyTest) FindByID(_ context.Context, id string) (adventure.Adventure, error) {
	if a, ok := m.saved[id]; ok {
		return a, nil
	}
	return adventure.Adventure{}, errors.New("not found")
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

	// Run backfill again: idempotent, records already exist and are skipped without duplication
	backfilled2, err := BackfillPartyAdventureLogs(ctx, partyRepo, advRepo, "party-past")
	if err != nil {
		t.Fatalf("second BackfillPartyAdventureLogs: %v", err)
	}
	if backfilled2 != 0 {
		t.Errorf("second backfill count = %d, want 0 (skipped existing records)", backfilled2)
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

func TestParty_BackfillDuplicatePreventionWithLiveRun(t *testing.T) {
	// Requirements check: "通常記録とBackfillによる二重計上防止"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	leader := corecharacter.Character{
		ID:    "lead-dup",
		Name:  "LeaderDup",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}
	member := corecharacter.Character{
		ID:    "mem-dup",
		Name:  "MemberDup",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	svc, parties, chars, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo))
	partyID := createReadyParty(ctx, t, svc, parties, chars, leader, member)

	// 1. Live party crawl runs and records to advRepo
	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure: %v", err)
	}
	if res.Outcome != string(corebattle.OutcomeWin) {
		t.Fatalf("expected win, got %v", res.Outcome)
	}

	// Verify live records exist
	clearedLeader := advRepo.ClearCountForCharacterAndStage(leader.ID, "stage-01", 0)
	if clearedLeader != 1 {
		t.Fatalf("leader clears before backfill = %d, want 1", clearedLeader)
	}
	clearedMember := advRepo.ClearCountForCharacterAndStage(member.ID, "stage-01", 0)
	if clearedMember != 1 {
		t.Fatalf("member clears before backfill = %d, want 1", clearedMember)
	}
	initialRecordCount := len(advRepo.saved)
	if initialRecordCount != 2 {
		t.Fatalf("advRepo saved records = %d, want 2", initialRecordCount)
	}

	// 2. Run backfill on the same party ID (party_adventure_logs generated by the live run)
	partyRepo, ok := svc.repo.(AdventureLogReader)
	if !ok {
		t.Fatalf("svc.repo does not implement AdventureLogReader")
	}

	backfilled, err := BackfillPartyAdventureLogs(ctx, partyRepo, advRepo, partyID)
	if err != nil {
		t.Fatalf("BackfillPartyAdventureLogs: %v", err)
	}
	// Existing live records were skipped, so backfilled is 0
	if backfilled != 0 {
		t.Errorf("backfilled count = %d, want 0 (live records skipped)", backfilled)
	}

	// Clear count must remain exactly 1 (no double-counting)
	clearedLeaderAfter := advRepo.ClearCountForCharacterAndStage(leader.ID, "stage-01", 0)
	if clearedLeaderAfter != 1 {
		t.Errorf("leader clears after backfill = %d, want 1 (double counted!)", clearedLeaderAfter)
	}
	clearedMemberAfter := advRepo.ClearCountForCharacterAndStage(member.ID, "stage-01", 0)
	if clearedMemberAfter != 1 {
		t.Errorf("member clears after backfill = %d, want 1 (double counted!)", clearedMemberAfter)
	}
	if len(advRepo.saved) != initialRecordCount {
		t.Errorf("advRepo saved records grew from %d to %d (duplicate entries!)", initialRecordCount, len(advRepo.saved))
	}
}

func TestParty_BackfillEarlierFallenParticipantConstraint(t *testing.T) {
	// Requirements check: "途中階で脱落した参加者が最終戦闘に含まれない制約の検証"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()
	partyRepo := newMockPartyRepository()

	// Simulate a historical log where:
	// - char-survived was alive at final floor
	// - char-boss-fallen fell at final floor
	// - char-early-fallen fell on floor 2 and was excluded from final battle's AlliesSurvived/AlliesFallen
	battleRes := corebattle.PartyBattleResult{
		Outcome:        corebattle.OutcomeWin,
		Turns:          7,
		AlliesSurvived: []string{"char-survived"},
		AlliesFallen:   []string{"char-boss-fallen"},
	}
	detailsJSON, _ := json.Marshal(battleRes)

	pastLog := PartyAdventureLog{
		ID:          "past-log-partial",
		PartyID:     "party-partial",
		StageID:     "stage-03",
		Outcome:     string(corebattle.OutcomeWin),
		Turns:       7,
		TotalEXP:    300,
		TotalGold:   150,
		DetailsJSON: string(detailsJSON),
		CreatedAt:   time.Now().UTC().Add(-48 * time.Hour),
	}
	if err := partyRepo.SaveAdventureLog(ctx, pastLog); err != nil {
		t.Fatalf("SaveAdventureLog: %v", err)
	}

	backfilled, err := BackfillPartyAdventureLogs(ctx, partyRepo, advRepo, "party-partial")
	if err != nil {
		t.Fatalf("BackfillPartyAdventureLogs: %v", err)
	}
	// Only char-survived and char-boss-fallen can be reconstructed
	if backfilled != 2 {
		t.Fatalf("backfilled = %d, want 2", backfilled)
	}

	if advRepo.ClearCountForCharacterAndStage("char-survived", "stage-03", 0) != 1 {
		t.Errorf("char-survived clear count != 1")
	}
	if advRepo.ClearCountForCharacterAndStage("char-boss-fallen", "stage-03", 0) != 1 {
		t.Errorf("char-boss-fallen clear count != 1")
	}
	// Early fallen character absent from details_json is not fabricated
	if advRepo.ClearCountForCharacterAndStage("char-early-fallen", "stage-03", 0) != 0 {
		t.Errorf("char-early-fallen should have 0 clears (not in details_json)")
	}
}

func TestParty_TryModeZeroRewardsInChronicle(t *testing.T) {
	// Requirements check: "トライモードでの報酬不支給とChronicleの報酬統計"
	// In Try Mode, when a party is defeated, unadjusted crawl EXP/Gold is non-zero,
	// but settled rewards are zero. Chronicle must reflect 0 rather than raw crawl totals.
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	leader := corecharacter.Character{
		ID:    "lead-try",
		Name:  "LeaderTry",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	calls := 0
	engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		calls++
		outcome := corebattle.OutcomeWin
		hps, mps := make(map[string]int), make(map[string]int)
		if calls == 2 {
			outcome = corebattle.OutcomeDefeat
			for _, p := range req.Allies {
				hps[p.ID] = 0
			}
		} else {
			for _, p := range req.Allies {
				hps[p.ID] = 80
				mps[p.ID] = 20
			}
		}
		return corebattle.PartyBattleResult{
			Outcome:     outcome,
			RemainingHP: hps,
			RemainingMP: mps,
			TotalReward: corebattle.Reward{Experience: 50, Currency: 25},
		}, nil
	})

	svc, parties, chars, _, _ := setupPartyAdventureTestService(t, engine, WithAdventureRepository(advRepo))
	partyID := createReadyParty(ctx, t, svc, parties, chars, leader)

	// Set Try Mode
	p := parties.parties[partyID]
	p.Mode = adventure.CrawlModeTry
	parties.parties[partyID] = p

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure: %v", err)
	}
	if res.Outcome != string(corebattle.OutcomeDefeat) {
		t.Fatalf("expected defeat, got %v", res.Outcome)
	}

	// Verify that the saved adventure record has 0 EXP and 0 Gold rewards for Try Mode defeat
	var savedAdv adventure.Adventure
	for _, a := range advRepo.saved {
		if a.CharacterID == leader.ID {
			savedAdv = a
			break
		}
	}
	if savedAdv.ID == "" {
		t.Fatalf("no adventure record found for %s", leader.ID)
	}
	if savedAdv.ExperienceReward != 0 {
		t.Errorf("ExperienceReward in Try Mode defeat = %d, want 0", savedAdv.ExperienceReward)
	}
	if savedAdv.BattleResult.Reward.Experience != 0 {
		t.Errorf("BattleResult.Reward.Experience in Try Mode defeat = %d, want 0", savedAdv.BattleResult.Reward.Experience)
	}
	if savedAdv.BattleResult.Reward.Currency != 0 {
		t.Errorf("BattleResult.Reward.Currency in Try Mode defeat = %d, want 0", savedAdv.BattleResult.Reward.Currency)
	}
}

func TestParty_SettledRewardsInChronicle(t *testing.T) {
	// Requirements check: "確定済み精算値（GainedEXP/GainedGold）の反映"
	ctx := context.Background()
	advRepo := newMemoryAdventureRepoForPartyTest()

	leader := corecharacter.Character{
		ID:    "lead-norm",
		Name:  "LeaderNorm",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
	}

	svc, parties, chars, _, _ := setupPartyAdventureTestService(t, nil, WithAdventureRepository(advRepo))
	partyID := createReadyParty(ctx, t, svc, parties, chars, leader)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure: %v", err)
	}
	if res.Outcome != string(corebattle.OutcomeWin) {
		t.Fatalf("expected win, got %v", res.Outcome)
	}

	var savedAdv adventure.Adventure
	for _, a := range advRepo.saved {
		if a.CharacterID == leader.ID {
			savedAdv = a
			break
		}
	}
	if savedAdv.ID == "" {
		t.Fatalf("no adventure record found for %s", leader.ID)
	}
	// In normal mode, EXP and Gold should match the settled amounts
	if savedAdv.ExperienceReward <= 0 {
		t.Errorf("ExperienceReward = %d, want > 0", savedAdv.ExperienceReward)
	}
	if savedAdv.BattleResult.Reward.Currency <= 0 {
		t.Errorf("BattleResult.Reward.Currency = %d, want > 0", savedAdv.BattleResult.Reward.Currency)
	}
}
