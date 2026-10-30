package adventure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func createTestCharacters() []corecharacter.Character {
	return []corecharacter.Character{
		{
			ID:       "char-leader",
			Name:     "勇者",
			Level:    10,
			JobLevel: 5,
			Stats: corecharacter.Stats{
				HP:      150,
				MaxHP:   150,
				MP:      50,
				MaxMP:   50,
				Attack:  60,
				Defense: 30,
				Agility: 30,
			},
		},
		{
			ID:       "char-merchant",
			Name:     "商人",
			JobID:    "merchant",
			Level:    10,
			JobLevel: 5,
			Stats: corecharacter.Stats{
				HP:      120,
				MaxHP:   120,
				MP:      30,
				MaxMP:   30,
				Attack:  50,
				Defense: 25,
				Agility: 25,
			},
		},
		{
			ID:       "char-thief",
			Name:     "盗賊",
			JobID:    "treasure_hunter",
			Level:    10,
			JobLevel: 5,
			Stats: corecharacter.Stats{
				HP:      130,
				MaxHP:   130,
				MP:      40,
				MaxMP:   40,
				Attack:  55,
				Defense: 25,
				Agility: 35,
			},
		},
	}
}

func setupTestCatalogs(t *testing.T) (*adventure.StageCatalog, *adventure.MonsterCatalog) {
	t.Helper()
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog: %v", err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatalf("InitialMonsterCatalog: %v", err)
	}
	return stages, monsters
}

func TestCrawlSession_Full10FloorsAndTreasureRoom(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	characters := createTestCharacters()
	rng := func(n int) int { return 0 }

	session, err := adventure.NewCrawlSession(stage, characters, rng)
	if err != nil {
		t.Fatalf("NewCrawlSession failed: %v", err)
	}

	if session.CurrentFloor != 1 {
		t.Fatalf("initial floor should be 1, got %d", session.CurrentFloor)
	}

	engine := corebattle.Engine{}

	// Advance floors 1 to 9 (Normal monsters)
	for f := 1; f <= 9; f++ {
		res, err := session.AdvanceFloor(stages, monsters, engine)
		if err != nil {
			t.Fatalf("floor %d advance error: %v", f, err)
		}
		if res.IsBoss {
			t.Fatalf("floor %d should not be boss floor", f)
		}
		if !res.Cleared {
			t.Fatalf("floor %d should be cleared", f)
		}
		if session.CurrentFloor != f+1 {
			t.Fatalf("after floor %d, current floor should be %d, got %d", f, f+1, session.CurrentFloor)
		}
	}

	// Floor 10 (Boss Floor)
	bossRes, err := session.AdvanceFloor(stages, monsters, engine)
	if err != nil {
		t.Fatalf("boss floor 10 advance error: %v", err)
	}
	if !bossRes.IsBoss {
		t.Fatalf("floor 10 must be boss floor")
	}
	if !bossRes.Cleared {
		t.Fatalf("boss floor 10 should be cleared")
	}
	if !session.StageCleared {
		t.Fatalf("StageCleared should be true after defeating boss")
	}
	if session.CurrentFloor != adventure.TreasureRoomFloor {
		t.Fatalf("expected next floor to be 11 (Treasure Room), got %d", session.CurrentFloor)
	}

	// Treasure boxes should be spawned (Base 3 alive; leader is Hero without job bonuses, so 3 boxes)
	if len(session.TreasureBoxes) != 3 {
		t.Fatalf("expected 3 treasure boxes, got %d", len(session.TreasureBoxes))
	}

	// Examine treasure on Floor 11 (@しらべる)
	box1, err := session.ExamineTreasure(characters[0].ID)
	if err != nil {
		t.Fatalf("ExamineTreasure char-leader error: %v", err)
	}
	if box1.OpenedBy != characters[0].ID {
		t.Errorf("box1 opened by %s, want %s", box1.OpenedBy, characters[0].ID)
	}

	box2, err := session.ExamineTreasure(characters[1].ID)
	if err != nil {
		t.Fatalf("ExamineTreasure char-merchant error: %v", err)
	}
	if box2.OpenedBy != characters[1].ID {
		t.Errorf("box2 opened by %s, want %s", box2.OpenedBy, characters[1].ID)
	}

	// Floor 11 conclude
	treasureRes, err := session.AdvanceFloor(stages, monsters, engine)
	if err != nil {
		t.Fatalf("floor 11 advance error: %v", err)
	}
	if !treasureRes.IsTreasureRoom {
		t.Fatalf("floor 11 must be treasure room")
	}

	result := session.Result()
	if result.FloorsCleared != 10 {
		t.Errorf("FloorsCleared = %d, want 10", result.FloorsCleared)
	}
	if !result.StageCleared {
		t.Errorf("StageCleared = false, want true")
	}
	if result.Outcome != corebattle.OutcomeWin {
		t.Errorf("Outcome = %v, want win", result.Outcome)
	}
	if result.TotalEXP <= 0 {
		t.Errorf("TotalEXP should be positive, got %d", result.TotalEXP)
	}
}

func TestCrawlSession_PartyWipeoutTerminatesCrawl(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, _ := stages.FindByID("stage-01")

	// Weak character with 1 HP
	weakChar := []corecharacter.Character{
		{
			ID:    "weakling",
			Name:  "Weakling",
			Level: 1,
			Stats: corecharacter.Stats{
				HP:    1,
				MaxHP: 1,
				MP:    0,
				MaxMP: 0,
			},
		},
	}

	session, err := adventure.NewCrawlSession(stage, weakChar, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession failed: %v", err)
	}

	// Force defeat engine stub
	defeatEngine := &defeatBattleResolver{}

	res, err := session.AdvanceFloor(stages, monsters, defeatEngine)
	if err != nil {
		t.Fatalf("AdvanceFloor error: %v", err)
	}
	if res.Cleared {
		t.Fatalf("expected floor 1 not cleared on defeat")
	}
	if !session.Concluded {
		t.Fatalf("session should be concluded on defeat")
	}
	if session.Outcome != corebattle.OutcomeDefeat {
		t.Fatalf("outcome should be defeat, got %v", session.Outcome)
	}
	if session.StageCleared {
		t.Fatalf("stage should NOT be cleared on defeat")
	}

	// Attempting to advance after defeat should error
	_, err = session.AdvanceFloor(stages, monsters, defeatEngine)
	if err != adventure.ErrDungeonCrawlFinished {
		t.Fatalf("expected ErrDungeonCrawlFinished, got %v", err)
	}
}

func TestCrawlSession_CrystalRewardAccumulation(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	characters := createTestCharacters()
	session, err := adventure.NewCrawlSession(stage, characters, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession failed: %v", err)
	}

	resolver := crystalRewardBattleResolver{crystalsPerFloor: 3}
	for f := 1; f <= 5; f++ {
		res, err := session.AdvanceFloor(stages, monsters, resolver)
		if err != nil {
			t.Fatalf("floor %d advance error: %v", f, err)
		}
		if !res.Cleared {
			t.Fatalf("floor %d should be cleared", f)
		}
		if session.TotalCrystals != f*3 {
			t.Errorf("after floor %d, TotalCrystals = %d, want %d", f, session.TotalCrystals, f*3)
		}
	}

	result := session.Result()
	if result.TotalCrystals != 15 {
		t.Errorf("result.TotalCrystals = %d, want 15", result.TotalCrystals)
	}
}

func TestCrawlSession_DefeatPreservesEarnedRewards(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	characters := createTestCharacters()
	session, err := adventure.NewCrawlSession(stage, characters, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSession failed: %v", err)
	}

	resolver := crystalRewardBattleResolver{crystalsPerFloor: 5}
	// Floor 1 win: gets 5 crystals
	_, err = session.AdvanceFloor(stages, monsters, resolver)
	if err != nil {
		t.Fatalf("floor 1 advance error: %v", err)
	}
	if session.TotalCrystals != 5 {
		t.Fatalf("TotalCrystals = %d, want 5", session.TotalCrystals)
	}

	// Floor 2 defeat
	defeatEngine := &defeatBattleResolver{}
	res, err := session.AdvanceFloor(stages, monsters, defeatEngine)
	if err != nil {
		t.Fatalf("floor 2 advance error: %v", err)
	}
	if res.Cleared {
		t.Fatalf("floor 2 should not be cleared")
	}
	result := session.Result()
	if result.TotalEXP != 10 || result.TotalGold != 20 || result.TotalCrystals != 5 {
		t.Fatalf("defeat lost prior earnings: EXP=%d gold=%d crystals=%d", result.TotalEXP, result.TotalGold, result.TotalCrystals)
	}
}

func TestCrawlSession_GamblerAndStageMultipliers(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage17, err := stages.FindByID("stage-17")
	if err != nil {
		t.Fatalf("FindByID(stage-17): %v", err)
	}

	gamblerChar := []corecharacter.Character{
		{
			ID:       "gambler-1",
			Name:     "勝負師",
			JobID:    "81",
			Level:    50,
			JobLevel: 10,
			Stats: corecharacter.Stats{
				HP:      50000,
				MaxHP:   50000,
				Attack:  20000,
				Defense: 20000,
			},
		},
	}

	// Gambler bonus: rng(3) - rng(2). With rng(3)=2, rng(2)=0 -> bonus = +2
	rng := func(n int) int {
		if n == 3 {
			return 2
		}
		return 0
	}

	session, err := adventure.NewCrawlSession(stage17, gamblerChar, rng)
	if err != nil {
		t.Fatalf("NewCrawlSession: %v", err)
	}

	engine := corebattle.Engine{}
	// Advance through all 10 floors
	for f := 1; f <= 10; f++ {
		res, err := session.AdvanceFloor(stages, monsters, engine)
		if err != nil {
			t.Fatalf("floor %d advance error: %v", f, err)
		}
		if !res.Cleared {
			t.Fatalf("floor %d should be cleared", f)
		}
	}

	// Floor 11 is the treasure room
	res, err := session.AdvanceFloor(stages, monsters, engine)
	if err != nil {
		t.Fatalf("floor 11 treasure room error: %v", err)
	}
	if !res.Cleared {
		t.Fatalf("floor 11 should be cleared")
	}

	// Expected boxes:
	// Stage 17 has 3x multiplier: 1 alive member * 3 = 3 base index.
	// Gambler adds +2 bonus -> index 5. Inclusive generation 0..5 generates 6 boxes!
	if len(session.TreasureBoxes) != 6 {
		t.Errorf("len(TreasureBoxes) = %d, want 6 (1*3 + 2 + 1)", len(session.TreasureBoxes))
	}
}

func TestCrawlSession_NowFuncInjectedJSTOrb(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage1, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	characters := createTestCharacters()[:1]
	// Inject a time corresponding to JST Monday 08:00 (UTC Sunday 23:00)
	fixedUTC := time.Date(2026, 8, 23, 23, 0, 0, 0, time.UTC)

	// Configure Service with WithNowFunc
	repo := &mockAdvRepo{}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{characters[0].ID: characters[0]}}
	engine := corebattle.Engine{}

	svc, err := adventure.NewServiceWithCatalogs(
		repo, charRepo, nil, stages, monsters, engine, nil, nil, adventure.RealClock{},
		adventure.WithNowFunc(func() time.Time { return fixedUTC }),
	)
	if err != nil {
		t.Fatalf("NewServiceWithCatalogs error: %v", err)
	}

	// Execute crawl via Service
	res, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{characters[0].ID},
		StageID:      stage1.ID,
		Rng: func(n int) int {
			if n == 4 {
				// Category roll: rng(4)+1 where 3 or 4 = item (default/tool pool).
				// Return 2 -> v = 3 (item pool)
				return 2
			}
			if n == 27 {
				// stage-01 has 26 treasure items + 1 orb = 27 items total.
				// Return index 26 (last index) for the injected JST Monday orb (item-060).
				return 26
			}
			return 0
		},
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl error: %v", err)
	}

	if len(res.TreasureBoxes) == 0 {
		t.Fatalf("expected treasure boxes")
	}

	// JST Monday orb is item-060
	foundOrb := false
	for _, box := range res.TreasureBoxes {
		if box.ItemID == "item-060" {
			foundOrb = true
			break
		}
	}
	if !foundOrb {
		t.Errorf("expected item-060 (Monday JST orb) in treasure boxes, got boxes: %+v", res.TreasureBoxes)
	}
}

type mockAdvRepo struct{}

func (mockAdvRepo) Save(ctx context.Context, value adventure.Adventure) error { return nil }
func (mockAdvRepo) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	return adventure.Adventure{}, nil
}
func (mockAdvRepo) ListByCharacterID(ctx context.Context, characterID string, limit, offset int) ([]adventure.Adventure, int, error) {
	return nil, 0, nil
}
func (mockAdvRepo) ListByCharacterIDByCursor(ctx context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]adventure.Adventure, error) {
	return nil, nil
}
func (mockAdvRepo) GetAggregatedStats(ctx context.Context, characterID string) (adventure.AggregatedStats, error) {
	return adventure.AggregatedStats{}, nil
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if c, ok := m.chars[id]; ok {
		return c, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

type defeatBattleResolver struct{}

func (defeatBattleResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	remHP := make(map[string]int)
	for _, a := range req.Allies {
		remHP[a.ID] = 0 // dead
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeDefeat,
		Turns:       2,
		RemainingHP: remHP,
	}, nil
}

type crystalRewardBattleResolver struct {
	crystalsPerFloor int
}

func (r crystalRewardBattleResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	remHP := make(map[string]int)
	for _, a := range req.Allies {
		remHP[a.ID] = a.HP
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		Turns:       1,
		RemainingHP: remHP,
		TotalReward: corebattle.Reward{
			Experience: 10,
			Currency:   20,
			Crystals:   r.crystalsPerFloor,
		},
	}, nil
}

type multiTurnBattleResolver struct {
	turnsPerFloor int
	failOnCall    int
	calls         int
}

func (r *multiTurnBattleResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	r.calls++
	remHP := make(map[string]int)
	outcome := corebattle.OutcomeWin
	if r.failOnCall > 0 && r.calls == r.failOnCall {
		outcome = corebattle.OutcomeDefeat
		for _, a := range req.Allies {
			remHP[a.ID] = 0
		}
	} else {
		for _, a := range req.Allies {
			remHP[a.ID] = a.HP
		}
	}
	return corebattle.PartyBattleResult{
		Outcome:     outcome,
		Turns:       r.turnsPerFloor,
		RemainingHP: remHP,
		TotalReward: corebattle.Reward{
			Experience: 15,
			Currency:   10,
		},
	}, nil
}

func TestCalculateExitFatigue_NormalAndCalmModes(t *testing.T) {
	cases := []struct {
		round      int
		wantNormal int
		wantCalm   int
	}{
		{round: -1, wantNormal: 1, wantCalm: 0},
		{round: 0, wantNormal: 1, wantCalm: 0},
		{round: 1, wantNormal: 4, wantCalm: 2},
		{round: 2, wantNormal: 7, wantCalm: 3},
		{round: 3, wantNormal: 10, wantCalm: 5},
		{round: 5, wantNormal: 16, wantCalm: 8},
		{round: 8, wantNormal: 25, wantCalm: 12},
		{round: 9, wantNormal: 28, wantCalm: 14},
		{round: 10, wantNormal: 31, wantCalm: 15},
		{round: 11, wantNormal: 34, wantCalm: 17},
	}

	for _, tc := range cases {
		for _, mode := range []string{"", adventure.CrawlModeNormal, adventure.CrawlModeTry, adventure.CrawlModeHard} {
			if got := adventure.CalculateExitFatigue(tc.round, mode); got != tc.wantNormal {
				t.Errorf("CalculateExitFatigue(round=%d, mode=%q) = %d, want %d", tc.round, mode, got, tc.wantNormal)
			}
		}
		if got := adventure.CalculateExitFatigue(tc.round, adventure.CrawlModeCalm); got != tc.wantCalm {
			t.Errorf("CalculateExitFatigue(round=%d, mode=calm) = %d, want %d", tc.round, got, tc.wantCalm)
		}
	}
}

func TestValidateCrawlMode(t *testing.T) {
	for _, valid := range []string{"", "normal", "calm", "try", "hard", "NORMAL", "CALM"} {
		got, err := adventure.ValidateCrawlMode(valid)
		if err != nil {
			t.Errorf("ValidateCrawlMode(%q) unexpected error: %v", valid, err)
		}
		if valid == "" && got != adventure.CrawlModeNormal {
			t.Errorf("ValidateCrawlMode(\"\") = %q, want normal", got)
		}
	}

	for _, invalid := range []string{"invalid", "unknown", "calmm"} {
		if _, err := adventure.ValidateCrawlMode(invalid); err == nil {
			t.Errorf("ValidateCrawlMode(%q) expected error, got nil", invalid)
		}
	}
}

func TestCrawlSession_RoundAndTravelTransitions(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}
	chars := createTestCharacters()[:1]
	rng := func(n int) int { return 0 }

	t.Run("0-round exit before advancing", func(t *testing.T) {
		session, err := adventure.NewCrawlSession(stage, chars, rng)
		if err != nil {
			t.Fatal(err)
		}
		if session.Round != 0 {
			t.Fatalf("initial Round = %d, want 0", session.Round)
		}
		res := session.Result()
		if res.Round != 0 || res.ExitFatigue != 1 {
			t.Fatalf("normal 0-round result: Round=%d ExitFatigue=%d, want 0 and 1", res.Round, res.ExitFatigue)
		}

		_ = session.SetMode(adventure.CrawlModeCalm)
		calmRes := session.Result()
		if calmRes.Round != 0 || calmRes.ExitFatigue != 0 {
			t.Fatalf("calm 0-round result: Round=%d ExitFatigue=%d, want 0 and 0", calmRes.Round, calmRes.ExitFatigue)
		}
	})

	t.Run("Floor 1 defeat sets Round=1 while FloorsCleared=0 and TotalTurns=7", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		resolver := &multiTurnBattleResolver{turnsPerFloor: 7, failOnCall: 1}
		_, err := session.AdvanceFloor(stages, monsters, resolver)
		if err != nil {
			t.Fatal(err)
		}
		res := session.Result()
		if res.Round != 1 || res.FloorsCleared != 0 || res.TotalTurns != 7 || len(res.FloorResults) != 1 {
			t.Fatalf("got Round=%d FloorsCleared=%d TotalTurns=%d len(FloorResults)=%d, want 1, 0, 7, 1",
				res.Round, res.FloorsCleared, res.TotalTurns, len(res.FloorResults))
		}
		if res.ExitFatigue != 4 {
			t.Errorf("Floor 1 defeat normal ExitFatigue = %d, want 4", res.ExitFatigue)
		}
		session.SetMode(adventure.CrawlModeCalm)
		if got := session.Result().ExitFatigue; got != 2 {
			t.Errorf("Floor 1 defeat calm ExitFatigue = %d, want 2", got)
		}
	})

	t.Run("Full 10 floors + Floor 11 treasure room sets Round=11", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		resolver := &multiTurnBattleResolver{turnsPerFloor: 5}
		for f := 1; f <= 11; f++ {
			if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
				t.Fatalf("floor %d advance: %v", f, err)
			}
		}
		res := session.Result()
		if res.Round != 11 || res.FloorsCleared != 10 || res.TotalTurns != 50 {
			t.Fatalf("got Round=%d FloorsCleared=%d TotalTurns=%d, want 11, 10, 50", res.Round, res.FloorsCleared, res.TotalTurns)
		}
		if res.ExitFatigue != 34 {
			t.Errorf("Floor 11 normal ExitFatigue = %d, want 34", res.ExitFatigue)
		}
		session.SetMode(adventure.CrawlModeCalm)
		if got := session.Result().ExitFatigue; got != 17 {
			t.Errorf("Floor 11 calm ExitFatigue = %d, want 17", got)
		}
	})

	t.Run("Travel destination floor 1 (tools 66-71, 104, 184)", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		resolver := &multiTurnBattleResolver{turnsPerFloor: 4}
		for f := 1; f <= 5; f++ {
			if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
				t.Fatal(err)
			}
		}
		session.SetRound(1)
		res := session.Result()
		if res.Round != 1 || res.FloorsCleared != 5 || len(res.FloorResults) != 5 || res.TotalTurns != 20 {
			t.Fatalf("got Round=%d FloorsCleared=%d len=%d turns=%d", res.Round, res.FloorsCleared, len(res.FloorResults), res.TotalTurns)
		}
		if res.ExitFatigue != 4 {
			t.Errorf("destination floor 1 ExitFatigue = %d, want 4", res.ExitFatigue)
		}
	})

	t.Run("Travel destination floor 10 (tool 107)", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		resolver := &multiTurnBattleResolver{turnsPerFloor: 3}
		for f := 1; f <= 2; f++ {
			if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
				t.Fatal(err)
			}
		}
		session.SetRound(10)
		res := session.Result()
		if res.Round != 10 || res.FloorsCleared != 2 || len(res.FloorResults) != 2 {
			t.Fatalf("got Round=%d FloorsCleared=%d len=%d", res.Round, res.FloorsCleared, len(res.FloorResults))
		}
		if res.ExitFatigue != 31 {
			t.Errorf("destination floor 10 ExitFatigue = %d, want 31", res.ExitFatigue)
		}
		session.SetMode(adventure.CrawlModeCalm)
		if got := session.Result().ExitFatigue; got != 15 {
			t.Errorf("destination floor 10 calm ExitFatigue = %d, want 15", got)
		}
	})

	t.Run("Tool 83 floor increment (round++ when < 9)", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		session.SetRound(4)
		session.AdvanceRoundIfBelow9()
		if session.Round != 5 || session.ExitFatigue() != 16 {
			t.Fatalf("from round 4: Round=%d ExitFatigue=%d, want 5 and 16", session.Round, session.ExitFatigue())
		}

		session.SetRound(9)
		session.AdvanceRoundIfBelow9()
		if session.Round != 9 || session.ExitFatigue() != 28 {
			t.Fatalf("from round 9 (not < 9): Round=%d ExitFatigue=%d, want 9 and 28", session.Round, session.ExitFatigue())
		}
	})

	t.Run("Tool 150 warp back to an earlier floor (1..10)", func(t *testing.T) {
		session, _ := adventure.NewCrawlSession(stage, chars, rng)
		resolver := &multiTurnBattleResolver{turnsPerFloor: 5}
		for f := 1; f <= 8; f++ {
			if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
				t.Fatal(err)
			}
		}
		// Warp back to floor 3
		session.SetRound(3)
		res := session.Result()
		if res.Round != 3 || res.FloorsCleared != 8 || len(res.FloorResults) != 8 || res.TotalTurns != 40 {
			t.Fatalf("got Round=%d FloorsCleared=%d len=%d turns=%d", res.Round, res.FloorsCleared, len(res.FloorResults), res.TotalTurns)
		}
		if res.ExitFatigue != 10 {
			t.Errorf("warped back to floor 3 normal ExitFatigue = %d, want 10 (not 25 from FloorsCleared=8)", res.ExitFatigue)
		}
		session.SetMode(adventure.CrawlModeCalm)
		if got := session.Result().ExitFatigue; got != 5 {
			t.Errorf("warped back to floor 3 calm ExitFatigue = %d, want 5", got)
		}
	})
}

func TestCrawlSession_SoloVsParty_SynergyMultipliers(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	rng := func(n int) int { return 0 }
	engine := corebattle.Engine{}

	// 1. Solo Crawl (1 player -> 0% bonus)
	soloChar := []corecharacter.Character{
		{
			ID:    "solo-hero",
			Name:  "SoloHero",
			Level: 99,
			Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999},
		},
	}
	soloSess, err := adventure.NewCrawlSession(stage, soloChar, rng)
	if err != nil {
		t.Fatalf("NewCrawlSession solo: %v", err)
	}
	for f := 1; f <= 10; f++ {
		res, err := soloSess.AdvanceFloor(stages, monsters, engine)
		if err != nil {
			t.Fatalf("solo floor %d: %v", f, err)
		}
		if res.BattleResult.BonusPercent != 0 {
			t.Errorf("solo floor %d bonusPercent = %d, want 0", f, res.BattleResult.BonusPercent)
		}
	}
	soloResult := soloSess.Result()

	// 2. 4-Player Crawl (4 players -> +30% bonus)
	partyChars := []corecharacter.Character{
		{ID: "p1", Name: "P1", Level: 99, Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999}},
		{ID: "p2", Name: "P2", Level: 99, Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999}},
		{ID: "p3", Name: "P3", Level: 99, Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999}},
		{ID: "p4", Name: "P4", Level: 99, Stats: corecharacter.Stats{HP: 99999, MaxHP: 99999, Attack: 99999, Defense: 99999}},
	}
	partySess, err := adventure.NewCrawlSession(stage, partyChars, rng)
	if err != nil {
		t.Fatalf("NewCrawlSession party: %v", err)
	}
	expectedPartyExp := 0
	expectedPartyGold := 0
	for f := 1; f <= 10; f++ {
		res, err := partySess.AdvanceFloor(stages, monsters, engine)
		if err != nil {
			t.Fatalf("party floor %d: %v", f, err)
		}
		if res.BattleResult.BonusPercent != 30 {
			t.Errorf("party floor %d bonusPercent = %d, want 30", f, res.BattleResult.BonusPercent)
		}
		expectedPartyExp += res.BattleResult.TotalReward.Experience
		expectedPartyGold += res.BattleResult.TotalReward.Currency
	}
	partyResult := partySess.Result()

	if partyResult.TotalEXP != expectedPartyExp {
		t.Errorf("party TotalEXP = %d, want %d", partyResult.TotalEXP, expectedPartyExp)
	}
	if partyResult.TotalGold != expectedPartyGold {
		t.Errorf("party TotalGold = %d, want %d", partyResult.TotalGold, expectedPartyGold)
	}

	// Verify that 4-player rewards correctly scale by +30% over solo rewards, and are not doubled
	approxScale := soloResult.TotalEXP * 130 / 100
	if partyResult.TotalEXP < soloResult.TotalEXP || partyResult.TotalEXP > approxScale+20 {
		t.Errorf("party TotalEXP (%d) unexpected relative to solo (%d), approx expected %d",
			partyResult.TotalEXP, soloResult.TotalEXP, approxScale)
	}
}

func TestCrawlSession_LeaderVsMemberBonuses(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}
	engine := corebattle.Engine{}
	fixedRng := func(n int) int { return 0 }

	advanceToFloor11 := func(t *testing.T, sess *adventure.CrawlSession) {
		t.Helper()
		for f := 1; f <= 10; f++ {
			res, err := sess.AdvanceFloor(stages, monsters, engine)
			if err != nil {
				t.Fatalf("floor %d advance error: %v", f, err)
			}
			if !res.Cleared {
				t.Fatalf("floor %d should be cleared", f)
			}
		}
	}

	// 1. Leader is Merchant (job-07) -> +1 box (2 alive + 1 = 3 boxes)
	charsLeaderMerchant := []corecharacter.Character{
		{ID: "leader-merch", Name: "Leader", JobID: "job-07", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-hero", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess1, err := adventure.NewCrawlSession(stage, charsLeaderMerchant, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess1)
	if len(sess1.TreasureBoxes) != 3 {
		t.Errorf("leader merchant: len(TreasureBoxes) = %d, want 3", len(sess1.TreasureBoxes))
	}

	// 2. Leader is Hero (job-01), Member is Merchant (job-07) -> NO merchant bonus (2 boxes)
	charsMemberMerchant := []corecharacter.Character{
		{ID: "leader-hero", Name: "Leader", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-merch", Name: "Member", JobID: "job-07", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess2, err := adventure.NewCrawlSession(stage, charsMemberMerchant, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess2)
	if len(sess2.TreasureBoxes) != 2 {
		t.Errorf("non-leader merchant: len(TreasureBoxes) = %d, want 2 (bonus must not apply to non-leader)", len(sess2.TreasureBoxes))
	}

	// 3. Leader is Treasure Hunter (job-78) -> +1 box with rng=0 (2 alive + 1 = 3 boxes)
	charsLeaderTH := []corecharacter.Character{
		{ID: "leader-th", Name: "Leader", JobID: "job-78", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-hero", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess3, err := adventure.NewCrawlSession(stage, charsLeaderTH, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess3)
	if len(sess3.TreasureBoxes) != 3 {
		t.Errorf("leader treasure hunter: len(TreasureBoxes) = %d, want 3", len(sess3.TreasureBoxes))
	}

	// 4. Leader is Hero, Member is Treasure Hunter -> NO TH bonus (2 boxes)
	charsMemberTH := []corecharacter.Character{
		{ID: "leader-hero", Name: "Leader", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-th", Name: "Member", JobID: "job-78", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess4, err := adventure.NewCrawlSession(stage, charsMemberTH, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess4)
	if len(sess4.TreasureBoxes) != 2 {
		t.Errorf("non-leader treasure hunter: len(TreasureBoxes) = %d, want 2", len(sess4.TreasureBoxes))
	}

	// 5. Leader holds Lucky Pendant (item-191) -> triggers +1 box with rng=0 (2 alive + 1 = 3 boxes)
	p1Pendant := corebattle.MustNewParticipant("leader-pendant", 50000, 10000, 10000)
	p1Pendant.Gear = &corebattle.BattleGearStats{AccessoryID: adventure.ItemLuckyPendant}
	p2Plain := corebattle.MustNewParticipant("member-plain", 50000, 10000, 10000)
	charsPendantLeader := []corecharacter.Character{
		{ID: "leader-pendant", Name: "Leader", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-plain", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess5, err := adventure.NewCrawlSessionWithParticipants(stage, charsPendantLeader, []corebattle.Participant{p1Pendant, p2Plain}, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess5)
	if len(sess5.TreasureBoxes) != 3 {
		t.Errorf("leader lucky pendant: len(TreasureBoxes) = %d, want 3", len(sess5.TreasureBoxes))
	}

	// 6. Member holds Lucky Pendant, but Leader does not -> NO pendant bonus (2 boxes)
	p1Plain := corebattle.MustNewParticipant("leader-plain", 50000, 10000, 10000)
	p2Pendant := corebattle.MustNewParticipant("member-pendant", 50000, 10000, 10000)
	p2Pendant.Gear = &corebattle.BattleGearStats{AccessoryID: adventure.ItemLuckyPendant}
	charsPendantMember := []corecharacter.Character{
		{ID: "leader-plain", Name: "Leader", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-pendant", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess6, err := adventure.NewCrawlSessionWithParticipants(stage, charsPendantMember, []corebattle.Participant{p1Plain, p2Pendant}, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess6)
	if len(sess6.TreasureBoxes) != 2 {
		t.Errorf("non-leader lucky pendant: len(TreasureBoxes) = %d, want 2", len(sess6.TreasureBoxes))
	}

	// 7. Leader has Lucky Pendant in unequipped inventory only (not in Gear.AccessoryID) -> NO pendant bonus (2 boxes)
	p1UnequippedPendant := corebattle.MustNewParticipant("leader-unequipped", 50000, 10000, 10000)
	p1UnequippedPendant.ItemDefinitionIDs = []string{adventure.ItemLuckyPendant}
	charsUnequippedLeader := []corecharacter.Character{
		{ID: "leader-unequipped", Name: "Leader", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-plain", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess7, err := adventure.NewCrawlSessionWithParticipants(stage, charsUnequippedLeader, []corebattle.Participant{p1UnequippedPendant, p2Plain}, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess7)
	if len(sess7.TreasureBoxes) != 2 {
		t.Errorf("unequipped lucky pendant: len(TreasureBoxes) = %d, want 2", len(sess7.TreasureBoxes))
	}

	// 8. 2-player party with Pirate (job-81) leader, delta -1 roll, no pendant:
	// Initial count = 1. Gambler delta -1 -> count becomes 0.
	// Legacy shift || $#partys fallback restores count to 1 -> 2 boxes!
	rngGamblerNeg := func(n int) int {
		if n == 3 {
			return 0
		}
		if n == 2 {
			return 1
		}
		return 1
	}
	charsPirateLeader := []corecharacter.Character{
		{ID: "leader-pirate", Name: "Leader", JobID: "job-81", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "member-plain", Name: "Member", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess8, err := adventure.NewCrawlSession(stage, charsPirateLeader, rngGamblerNeg)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess8)
	if len(sess8.TreasureBoxes) != 2 {
		t.Errorf("pirate leader delta -1 fallback: len(TreasureBoxes) = %d, want 2", len(sess8.TreasureBoxes))
	}
}

func TestCrawlSession_Stage17TwoPlayersInclusiveCount(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage17, err := stages.FindByID("stage-17")
	if err != nil {
		t.Fatalf("FindByID(stage-17): %v", err)
	}
	engine := corebattle.Engine{}
	fixedRng := func(n int) int { return 0 }

	advanceToFloor11 := func(t *testing.T, sess *adventure.CrawlSession) {
		t.Helper()
		for f := 1; f <= 10; f++ {
			res, err := sess.AdvanceFloor(stages, monsters, engine)
			if err != nil {
				t.Fatalf("floor %d advance error: %v", f, err)
			}
			if !res.Cleared {
				t.Fatalf("floor %d should be cleared", f)
			}
		}
	}

	// Case 1: 2 alive players clear stage-17 -> exactly 7 boxes (2*3 + 1)
	chars2Alive := []corecharacter.Character{
		{ID: "p1", Name: "P1", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "p2", Name: "P2", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess1, err := adventure.NewCrawlSession(stage17, chars2Alive, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess1)
	if len(sess1.TreasureBoxes) != 7 {
		t.Errorf("stage 17 with 2 alive players: len(TreasureBoxes) = %d, want 7", len(sess1.TreasureBoxes))
	}

	// Case 2: 2 players on stage-17, 1 alive, 1 dead -> exactly 6 boxes (2*3 - 1 + 1)
	p1Alive := corebattle.MustNewParticipant("p1-alive", 50000, 10000, 10000)
	p2Dead := corebattle.MustNewParticipant("p2-dead", 100, 10000, 10000)
	p2Dead.HP = 0 // explicitly unconscious
	chars1Dead := []corecharacter.Character{
		{ID: "p1-alive", Name: "P1", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "p2-dead", Name: "P2", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess2, err := adventure.NewCrawlSessionWithParticipants(stage17, chars1Dead, []corebattle.Participant{p1Alive, p2Dead}, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess2)
	if len(sess2.TreasureBoxes) != 6 {
		t.Errorf("stage 17 with 1 alive, 1 dead: len(TreasureBoxes) = %d, want 6", len(sess2.TreasureBoxes))
	}

	// Case 3: 2 alive players on stage-17 with leader job-07 (Merchant) -> exactly 8 boxes (2*3 + 1 + 1)
	charsMerchantLeader := []corecharacter.Character{
		{ID: "p1-merch", Name: "P1", JobID: "job-07", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
		{ID: "p2-plain", Name: "P2", JobID: "job-01", Level: 50, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000, Attack: 10000, Defense: 10000}},
	}
	sess3, err := adventure.NewCrawlSession(stage17, charsMerchantLeader, fixedRng)
	if err != nil {
		t.Fatal(err)
	}
	advanceToFloor11(t, sess3)
	if len(sess3.TreasureBoxes) != 8 {
		t.Errorf("stage 17 with 2 alive players and merchant leader: len(TreasureBoxes) = %d, want 8", len(sess3.TreasureBoxes))
	}
}

func TestCrawlSession_ExamineTreasure_AllowanceAndMasterKey(t *testing.T) {
	stages, _ := setupTestCatalogs(t)
	stage1, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	chars := []corecharacter.Character{
		{ID: "char-1", Name: "Hero 1", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
		{ID: "char-2", Name: "Hero 2", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
		{ID: "char-dead", Name: "Dead Hero", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
	}
	p1 := corebattle.MustNewParticipant("char-1", 100, 10, 10)
	p1.ItemDefinitionIDs = []string{adventure.ItemMasterKey} // char-1 has 1 master key
	p2 := corebattle.MustNewParticipant("char-2", 100, 10, 10)
	// char-2 has no keys
	pDead := corebattle.MustNewParticipant("char-dead", 100, 10, 10)
	pDead.HP = 0 // unconscious

	sess, err := adventure.NewCrawlSessionWithParticipants(stage1, chars, []corebattle.Participant{p1, p2, pDead}, func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("NewCrawlSessionWithParticipants: %v", err)
	}

	// Cannot examine before Floor 11
	if _, err := sess.ExamineTreasure("char-1"); !errors.Is(err, adventure.ErrInvalidFloorAction) {
		t.Fatalf("expected ErrInvalidFloorAction on floor %d, got %v", sess.CurrentFloor, err)
	}

	sess.CurrentFloor = adventure.TreasureRoomFloor
	sess.TreasureBoxes = []adventure.TreasureBox{
		{ID: "box-1", Name: "Chest 1", ItemID: "item-001"},
		{ID: "box-2", Name: "Chest 2", ItemID: "item-002"},
		{ID: "box-3", Name: "Chest 3", ItemID: "item-003"},
	}

	// 1. Non-participant rejected (未参加actor拒否)
	if _, err := sess.ExamineTreasure("char-outside"); !errors.Is(err, adventure.ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant for outside character, got %v", err)
	}

	// 2. Dead character rejected (死亡者不可)
	if _, err := sess.ExamineTreasure("char-dead"); !errors.Is(err, adventure.ErrUnconsciousCharacter) {
		t.Fatalf("expected ErrUnconsciousCharacter for dead character, got %v", err)
	}

	// 3. char-1 opens 1st box (base allowance)
	box1, err := sess.ExamineTreasure("char-1")
	if err != nil {
		t.Fatalf("char-1 1st box error: %v", err)
	}
	if box1.OpenedBy != "char-1" {
		t.Errorf("box1 opened by %s, want char-1", box1.OpenedBy)
	}
	if len(sess.ConsumedItems["char-1"]) != 0 {
		t.Errorf("char-1 consumed items on base box = %v, want empty", sess.ConsumedItems["char-1"])
	}

	// 4. char-2 opens 1st box (別actor可)
	box2, err := sess.ExamineTreasure("char-2")
	if err != nil {
		t.Fatalf("char-2 1st box error: %v", err)
	}
	if box2.OpenedBy != "char-2" {
		t.Errorf("box2 opened by %s, want char-2", box2.OpenedBy)
	}

	// 5. char-2 tries to open 2nd box without key -> rejected (同一actor2回拒否)
	if _, err := sess.ExamineTreasure("char-2"); !errors.Is(err, adventure.ErrTreasureLimitReached) {
		t.Fatalf("expected ErrTreasureLimitReached for char-2 2nd box, got %v", err)
	}

	// 6. char-1 opens 2nd box with key -> succeeds (key所持で追加1回/1消費)
	box3, err := sess.ExamineTreasure("char-1")
	if err != nil {
		t.Fatalf("char-1 2nd box with key error: %v", err)
	}
	if box3.OpenedBy != "char-1" {
		t.Errorf("box3 opened by %s, want char-1", box3.OpenedBy)
	}
	consumed := sess.ConsumedItems["char-1"]
	if len(consumed) != 1 || consumed[0].ID != adventure.ItemMasterKey || consumed[0].Quantity != 1 {
		t.Fatalf("char-1 consumed items = %v, want 1 master key", consumed)
	}

	// 7. char-1 tries to open again -> rejected (no unopened boxes remaining)
	if _, err := sess.ExamineTreasure("char-1"); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
		t.Fatalf("expected ErrNoUnopenedTreasure when no boxes left, got %v", err)
	}

	// 8. Add a 4th box; char-1 tries to open -> rejected (ErrTreasureLimitReached because key was spent)
	sess.TreasureBoxes = append(sess.TreasureBoxes, adventure.TreasureBox{ID: "box-4", Name: "Chest 4", ItemID: "item-004"})
	if _, err := sess.ExamineTreasure("char-1"); !errors.Is(err, adventure.ErrTreasureLimitReached) {
		t.Fatalf("expected ErrTreasureLimitReached for char-1 after key exhausted, got %v", err)
	}
}

func TestCrawlSession_ExamineTreasure_StackedTreasurePocket(t *testing.T) {
	stages, _ := setupTestCatalogs(t)
	stage1, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatalf("FindByID(stage-01): %v", err)
	}

	chars := []corecharacter.Character{
		{ID: "char-stack", Name: "StackHero", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
	}
	// Participant with stacked quantity 2 of ItemTreasurePocket
	p := corebattle.MustNewParticipant("char-stack", 100, 10, 10)
	p.ItemDefinitionIDs = []string{adventure.ItemTreasurePocket, adventure.ItemTreasurePocket}

	sess, err := adventure.NewCrawlSessionWithParticipants(stage1, chars, []corebattle.Participant{p}, func(n int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	sess.CurrentFloor = adventure.TreasureRoomFloor
	sess.TreasureBoxes = []adventure.TreasureBox{
		{ID: "box-1", Name: "Chest 1", ItemID: "item-001"},
		{ID: "box-2", Name: "Chest 2", ItemID: "item-002"},
		{ID: "box-3", Name: "Chest 3", ItemID: "item-003"},
		{ID: "box-4", Name: "Chest 4", ItemID: "item-004"},
	}

	// 1st box: base allowance (0 consumed)
	if _, err := sess.ExamineTreasure("char-stack"); err != nil {
		t.Fatalf("1st box: %v", err)
	}
	if len(sess.ConsumedItems["char-stack"]) != 0 {
		t.Fatalf("base box should not consume items: %v", sess.ConsumedItems["char-stack"])
	}

	// 2nd box: consumes 1st key
	if _, err := sess.ExamineTreasure("char-stack"); err != nil {
		t.Fatalf("2nd box: %v", err)
	}
	consumed := sess.ConsumedItems["char-stack"]
	if len(consumed) != 1 || consumed[0].Quantity != 1 {
		t.Fatalf("expected 1 consumed key, got: %v", consumed)
	}

	// 3rd box: consumes 2nd key (stack quantity now exhausted)
	if _, err := sess.ExamineTreasure("char-stack"); err != nil {
		t.Fatalf("3rd box: %v", err)
	}
	consumed = sess.ConsumedItems["char-stack"]
	if len(consumed) != 1 || consumed[0].Quantity != 2 {
		t.Fatalf("expected 2 consumed keys, got: %v", consumed)
	}

	// 4th box: rejected (quantity 2 exhausted)
	if _, err := sess.ExamineTreasure("char-stack"); !errors.Is(err, adventure.ErrTreasureLimitReached) {
		t.Fatalf("expected ErrTreasureLimitReached after 2 keys exhausted, got: %v", err)
	}
}
