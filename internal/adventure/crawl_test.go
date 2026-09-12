package adventure_test

import (
	"testing"

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
