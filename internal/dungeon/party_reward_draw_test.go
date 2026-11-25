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
		ID:         "dg-reward-parity",
		RoundLimit: 31,
		Variants: []dungeon.Floor{
			{
				Width:  4,
				Height: 1,
				StartX: 0,
				StartY: 0,
				Grid:   []string{"S000"},
				Pools: []dungeon.EncounterPool{{Monsters: []dungeon.DungeonMonster{
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
				}}},
			},
		},
	}

	fightCount := 0
	engine := &customPartyBattleEngine{
		resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			fightCount++
			if fightCount == 1 {
				// Fight 1: lead's kill reaches everyone; fallen dies afterwards.
				return corebattle.PartyBattleResult{
					Outcome:        corebattle.OutcomeWin,
					WinnerSide:     "allies",
					Turns:          2,
					DefeatEarnings: []corebattle.DefeatEarnings{{ActorID: "lead", TargetID: "mon-1", RecipientIDs: []string{"lead", "fallen", "survivor"}}},
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
			// Fight 2: survivor's kill reaches only the living lead and survivor.
			return corebattle.PartyBattleResult{
				Outcome:        corebattle.OutcomeWin,
				WinnerSide:     "allies",
				Turns:          2,
				DefeatEarnings: []corebattle.DefeatEarnings{{ActorID: "survivor", TargetID: "mon-1", RecipientIDs: []string{"lead", "survivor"}}},
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
		dungeon.WithRNG(zeroRNG{}), // passage spawn of one enemy
		dungeon.WithParticipantBuilder(builder),
		dungeon.WithActiveExpeditionStore(dungeon.NewMemoryExpeditionRepository()),
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

	// Fight 1: fallen was alive at the defeat, so its share stays in stock
	// although it fell by the end. lead is killer and requester: credited at once.
	stock := func(want map[string][2]int) {
		t.Helper()
		active, err := svc.GetActiveExpedition(ctx, "lead")
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range active.Members {
			if got := [2]int{m.AccumulatedExp, m.AccumulatedGold}; got != want[m.CharacterID] {
				t.Errorf("%s stock = %v, want %v", m.CharacterID, got, want[m.CharacterID])
			}
		}
		if len(active.Credits) != 0 {
			t.Errorf("credits left pending: %+v", active.Credits)
		}
	}
	stock(map[string][2]int{"lead": {0, 0}, "fallen": {50, 30}, "survivor": {50, 30}})
	if repo.savedChars["lead"].Money != 30 {
		t.Errorf("killer/requester lead not credited at once: %+v", repo.savedChars["lead"])
	}

	// 2. Move to (2, 0) -> Fight 2: the fallen member is not a recipient.
	res2, err := svc.Move(ctx, "lead", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move 2 failed: %v", err)
	}
	if res2.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res2.EventType)
	}
	stock(map[string][2]int{"lead": {0, 0}, "fallen": {50, 30}, "survivor": {50, 30}})

	// 3. Escape applies the remaining stock once.
	if _, err = svc.Escape(ctx, "lead"); err != nil {
		t.Fatalf("Escape failed: %v", err)
	}
	for id, want := range map[string]int{"lead": 60, "fallen": 30, "survivor": 60} {
		if got := repo.savedChars[id].Money; got != want {
			t.Errorf("%s money = %d, want %d", id, got, want)
		}
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
		ID:         "dg-draw-test",
		RoundLimit: 21,
		Variants: []dungeon.Floor{
			{
				Width:  3,
				Height: 1,
				StartX: 0,
				StartY: 0,
				Grid:   []string{"S00"},
				Pools: []dungeon.EncounterPool{{Monsters: []dungeon.DungeonMonster{
					{
						ID:         "golem",
						Name:       "Iron Golem",
						HP:         500,
						Attack:     5,
						ExpReward:  100,
						GoldReward: 50,
					},
				}}},
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
		dungeon.WithRNG(&minimalRandomSource{}), // passage spawn
		dungeon.WithParticipantBuilder(builder),
		dungeon.WithActiveExpeditionStore(dungeon.NewMemoryExpeditionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	exp, err := svc.StartPartyExpedition(ctx, "lead-stalemate", []string{"lead-stalemate", "ally-stalemate"}, "dg-draw-test", "p1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}
	initialRound := exp.Round

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

	// 3. The party already stands on the encounter tile (vs_dungeon.cgi _susumu).
	if res.Expedition.PosX != 1 || res.Expedition.PosY != 0 {
		t.Errorf("expected player position (1, 0), got (%d, %d)", res.Expedition.PosX, res.Expedition.PosY)
	}

	// 4. The move spends one round
	if res.Expedition.Round != initialRound+1 {
		t.Errorf("expected Round %d, got %d", initialRound+1, res.Expedition.Round)
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
		ID:         "dg-draw-mutual-death",
		RoundLimit: 21,
		Variants: []dungeon.Floor{
			{
				Width:  3,
				Height: 1,
				StartX: 0,
				StartY: 0,
				Grid:   []string{"S00"},
				Pools: []dungeon.EncounterPool{{Monsters: []dungeon.DungeonMonster{
					{
						ID:     "bomb",
						Name:   "Self-Destruct Bomb",
						HP:     50,
						Attack: 100,
					},
				}}},
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
		dungeon.WithRNG(&minimalRandomSource{}), // passage spawn
		dungeon.WithParticipantBuilder(builder),
		dungeon.WithActiveExpeditionStore(dungeon.NewMemoryExpeditionRepository()),
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

	// Mutual death: the fallen party stays in the run with only Escape left.
	if res.IsFinished || res.Expedition.Status != dungeon.StatusExploring {
		t.Fatalf("mutual death ended the run: %+v", res)
	}
	if c := res.Expedition.Controls("lead-mutual"); c.CanMove || c.CanFight || !c.CanEscape {
		t.Fatalf("mutual death controls %+v", c)
	}
}
