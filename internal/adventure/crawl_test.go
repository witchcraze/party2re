package adventure_test

import (
	"context"
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

	// Treasure boxes should be spawned (Base 3 alive + Merchant +1 + Treasure Hunter +1 = 5)
	if len(session.TreasureBoxes) != 5 {
		t.Fatalf("expected 5 treasure boxes, got %d", len(session.TreasureBoxes))
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

func TestCrawlSession_DefeatZeroesCrystals(t *testing.T) {
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
	if session.TotalCrystals != 0 {
		t.Errorf("after defeat, TotalCrystals = %d, want 0", session.TotalCrystals)
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
	// Stage 17 has 3x multiplier: 1 alive member * 3 = 3 base boxes.
	// Gambler adds +2 bonus. Total = 5 boxes!
	if len(session.TreasureBoxes) != 5 {
		t.Errorf("len(TreasureBoxes) = %d, want 5 (1*3 + 2)", len(session.TreasureBoxes))
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
