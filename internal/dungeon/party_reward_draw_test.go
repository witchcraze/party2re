package dungeon_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type customPartyBattleEngine struct {
	resolvePartyFn func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

func (e *customPartyBattleEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{}, nil
}

func (e *customPartyBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	if e.resolvePartyFn != nil {
		return e.resolvePartyFn(req)
	}
	return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies"}, nil
}

func TestPartyCombat_FallenMemberDoesNotReceiveSubsequentDefeatRewards(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	cLead := createTestChar("lead", 10, 100, 50, 30)
	cFallen := createTestChar("fallen", 10, 10, 10, 5) // Low HP, dies in fight 1
	cSurv := createTestChar("survivor", 10, 100, 50, 30)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		cLead.ID:   cLead,
		cFallen.ID: cFallen,
		cSurv.ID:   cSurv,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-reward-parity",
		MinLevel:         1,
		MaxTurnsPerFloor: 30,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       4,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S000"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "mon-1",
						Name:       "Monster 1",
						HP:         20,
						Attack:     10,
						ExpReward:  50,
						GoldReward: 30,
					},
					{
						ID:         "mon-2",
						Name:       "Monster 2",
						HP:         20,
						Attack:     10,
						ExpReward:  100,
						GoldReward: 60,
					},
				},
			},
		},
	}

	fightCount := 0
	engine := &customPartyBattleEngine{
		resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			fightCount++
			if fightCount == 1 {
				// Fight 1: fallen dies (0 HP), lead and survivor survive
				return corebattle.PartyBattleResult{
					Outcome:    corebattle.OutcomeWin,
					WinnerSide: "allies",
					Turns:      2,
					RemainingHP: map[string]int{
						"lead":     80,
						"fallen":   0, // Died in fight 1
						"survivor": 90,
						"mon-1":    0,
					},
					RemainingMP: map[string]int{
						"lead":     25,
						"fallen":   0,
						"survivor": 20,
					},
				}, nil
			}
			// Fight 2: lead and survivor fight and win
			return corebattle.PartyBattleResult{
				Outcome:    corebattle.OutcomeWin,
				WinnerSide: "allies",
				Turns:      2,
				RemainingHP: map[string]int{
					"lead":     70,
					"survivor": 80,
					"mon-2":    0,
				},
				RemainingMP: map[string]int{
					"lead":     20,
					"survivor": 15,
				},
			}, nil
		},
	}

	builder := &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			char := charRepo.chars[charID]
			return corebattle.Participant{
				ID:      char.ID,
				Name:    char.Name,
				HP:      currentHP,
				MaxHP:   char.Stats.HP,
				Attack:  char.Stats.Attack,
				Defense: char.Stats.Defense,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		engine,
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.StartPartyExpedition(ctx, "lead", []string{"lead", "fallen", "survivor"}, "dg-reward-parity", "p1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// 1. Move to (1, 0) -> Fight 1
	res1, err := svc.Move(ctx, "lead", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move 1 failed: %v", err)
	}
	if res1.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res1.EventType)
	}

	// In fight 1, fallen died (0 HP) so per original Party2 defeat logic, it gets 0.
	// lead and survivor survived so they got 50 EXP, 30 Gold.
	active1, err := svc.GetActiveExpedition(ctx, "lead")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range active1.Members {
		switch m.CharacterID {
		case "fallen":
			if m.CurrentHP != 0 {
				t.Errorf("expected fallen HP 0, got %d", m.CurrentHP)
			}
			if m.AccumulatedExp != 0 || m.AccumulatedGold != 0 {
				t.Errorf("fallen died in combat, expected 0 EXP/Gold, got exp=%d gold=%d", m.AccumulatedExp, m.AccumulatedGold)
			}
		case "lead", "survivor":
			if m.AccumulatedExp != 100 || m.AccumulatedGold != 60 {
				t.Errorf("member %s expected 100 EXP 60 Gold, got exp=%d gold=%d", m.CharacterID, m.AccumulatedExp, m.AccumulatedGold)
			}
		}
	}

	// 2. Move to (2, 0) -> Fight 2 (Monster 2: 100 EXP, 60 Gold)
	res2, err := svc.Move(ctx, "lead", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move 2 failed: %v", err)
	}
	if res2.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res2.EventType)
	}

	active2, err := svc.GetActiveExpedition(ctx, "lead")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range active2.Members {
		switch m.CharacterID {
		case "fallen":
			// Critical assertion: fallen member must NOT receive rewards from Fight 2!
			if m.AccumulatedExp != 0 || m.AccumulatedGold != 0 {
				t.Errorf("fallen member erroneously received Fight 2 rewards! got exp=%d gold=%d", m.AccumulatedExp, m.AccumulatedGold)
			}
		case "lead", "survivor":
			// Living members got 50 + 100 = 150 EXP and 30 + 60 = 90 Gold!
			if m.AccumulatedExp != 150 || m.AccumulatedGold != 90 {
				t.Errorf("surviving member %s expected 150 EXP 90 Gold, got exp=%d gold=%d", m.CharacterID, m.AccumulatedExp, m.AccumulatedGold)
			}
		}
	}

	// 3. Escape and verify FinalizeExpedition persistence in mock repo
	_, err = svc.Escape(ctx, "lead")
	if err != nil {
		t.Fatalf("Escape failed: %v", err)
	}

	savedFallen := repo.savedChars["fallen"]
	savedSurv := repo.savedChars["survivor"]

	if savedFallen.Money != 0 {
		t.Errorf("expected fallen character to have 0 money reward, got %d", savedFallen.Money)
	}
	if savedSurv.Money != 90 {
		t.Errorf("expected survivor character to have 90 money reward, got %d", savedSurv.Money)
	}
}

func TestPartyCombat_OutcomeDraw_DoesNotWipeOut_PreventsMovement(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead-stalemate", 10, 100, 10, 10)
	c2 := createTestChar("ally-stalemate", 10, 100, 10, 10)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-draw-test",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S00"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "golem",
						Name:       "Iron Golem",
						HP:         500,
						Attack:     5,
						ExpReward:  100,
						GoldReward: 50,
					},
				},
			},
		},
	}

	engine := &customPartyBattleEngine{
		resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			// Stalemate: 100 turns expired, both sides still have HP
			return corebattle.PartyBattleResult{
				Outcome:    corebattle.OutcomeDraw,
				WinnerSide: "none",
				Turns:      100,
				RemainingHP: map[string]int{
					"lead-stalemate": 85,
					"ally-stalemate": 90,
					"golem":          420,
				},
				RemainingMP: map[string]int{
					"lead-stalemate": 10,
					"ally-stalemate": 15,
				},
			}, nil
		},
	}

	builder := &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			char := charRepo.chars[charID]
			return corebattle.Participant{
				ID:      char.ID,
				Name:    char.Name,
				HP:      currentHP,
				MaxHP:   char.Stats.HP,
				Attack:  char.Stats.Attack,
				Defense: char.Stats.Defense,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		engine,
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	exp, err := svc.StartPartyExpedition(ctx, "lead-stalemate", []string{"lead-stalemate", "ally-stalemate"}, "dg-draw-test", "p1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}
	initialTurns := exp.TurnsRemaining

	// Attempt move east onto golem tile (1, 0)
	res, err := svc.Move(ctx, "lead-stalemate", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// 1. Battle result was OutcomeDraw
	if res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("expected BattleResult.Outcome == OutcomeDraw, got %+v", res.BattleResult)
	}

	// 2. Party MUST NOT be wiped out!
	if res.IsFinished {
		t.Error("expected expedition NOT finished on draw with surviving members")
	}
	if res.Expedition.Status != dungeon.StatusExploring {
		t.Errorf("expected StatusExploring, got %v", res.Expedition.Status)
	}

	// 3. Player MUST NOT advance past or into the enemy tile: position remains at StartX (0, 0)!
	if res.Expedition.PosX != 0 || res.Expedition.PosY != 0 {
		t.Errorf("expected player position to remain at (0, 0), got (%d, %d)", res.Expedition.PosX, res.Expedition.PosY)
	}

	// 4. Turns budget is consumed (-1)
	if res.Expedition.TurnsRemaining != initialTurns-1 {
		t.Errorf("expected TurnsRemaining %d, got %d", initialTurns-1, res.Expedition.TurnsRemaining)
	}

	// 5. HP/MP deltas from the 100-turn battle are applied to members
	if res.Expedition.Members[0].CurrentHP != 85 || res.Expedition.Members[1].CurrentHP != 90 {
		t.Errorf("expected surviving HPs 85 and 90, got %d and %d",
			res.Expedition.Members[0].CurrentHP, res.Expedition.Members[1].CurrentHP)
	}
	if res.Expedition.Members[0].CurrentMP != 10 || res.Expedition.Members[1].CurrentMP != 15 {
		t.Errorf("expected surviving MPs 10 and 15, got %d and %d",
			res.Expedition.Members[0].CurrentMP, res.Expedition.Members[1].CurrentMP)
	}
}

func TestPartyCombat_OutcomeDraw_MutualWipeout_WipesOut(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead-mutual", 10, 100, 10, 10)
	c2 := createTestChar("ally-mutual", 10, 100, 10, 10)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-draw-mutual-death",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S00"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:     "bomb",
						Name:   "Self-Destruct Bomb",
						HP:     50,
						Attack: 100,
					},
				},
			},
		},
	}

	engine := &customPartyBattleEngine{
		resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			// Mutual death: both sides died simultaneously
			return corebattle.PartyBattleResult{
				Outcome:    corebattle.OutcomeDraw,
				WinnerSide: "none",
				Turns:      1,
				RemainingHP: map[string]int{
					"lead-mutual": 0,
					"ally-mutual": 0,
					"bomb":        0,
				},
			}, nil
		},
	}

	builder := &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			char := charRepo.chars[charID]
			return corebattle.Participant{
				ID:      char.ID,
				Name:    char.Name,
				HP:      currentHP,
				MaxHP:   char.Stats.HP,
				Attack:  char.Stats.Attack,
				Defense: char.Stats.Defense,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		engine,
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.StartPartyExpedition(ctx, "lead-mutual", []string{"lead-mutual", "ally-mutual"}, "dg-draw-mutual-death", "p1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	res, err := svc.Move(ctx, "lead-mutual", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Mutual wipeout: all party members died, must wipe out!
	if !res.IsFinished {
		t.Error("expected expedition finished on mutual wipeout")
	}
	if res.Expedition.Status != dungeon.StatusWipedOut {
		t.Errorf("expected StatusWipedOut, got %v", res.Expedition.Status)
	}
}
