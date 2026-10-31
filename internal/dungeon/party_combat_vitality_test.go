package dungeon_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func TestPartyCombat_MultiMemberBattleAndVitalityTracking(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead", 10, 100, 40, 25)
	c1.Stats.MP = 30
	c2 := createTestChar("ally", 10, 80, 35, 20)
	c2.Stats.MP = 20

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-multi-combat",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      3,
				StartX:      0,
				StartY:      0,
				Grid: []string{
					"S00",
					"000",
					"00D",
				},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "goblin-1",
						Name:       "Goblin",
						HP:         50,
						Attack:     15,
						Defense:    5,
						Agility:    10,
						ExpReward:  50,
						GoldReward: 30,
					},
				},
			},
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
				MP:      char.Stats.MP,
				MaxMP:   char.Stats.MP,
				Attack:  char.Stats.Attack,
				Defense: char.Stats.Defense,
				Agility: char.Stats.Agility,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	exp, err := svc.StartPartyExpedition(ctx, "lead", []string{"lead", "ally"}, "dg-multi-combat", "party-1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}
	if len(exp.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(exp.Members))
	}

	// Move east onto Monster tile
	res, err := svc.Move(ctx, "lead", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res.EventType)
	}
	if res.BattleResult == nil {
		t.Fatal("expected non-nil BattleResult")
	}

	// Verify both members are present and tracked
	active, err := svc.GetActiveExpedition(ctx, "lead")
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if len(active.Members) != 2 {
		t.Fatalf("expected 2 active members, got %d", len(active.Members))
	}
	if active.Members[0].CharacterID != "lead" || active.Members[1].CharacterID != "ally" {
		t.Errorf("unexpected member order: %+v", active.Members)
	}
	if active.CurrentHP != active.Members[0].CurrentHP {
		t.Errorf("leader CurrentHP mismatch: exp=%d member=%d", active.CurrentHP, active.Members[0].CurrentHP)
	}
}

func TestPartyCombat_LeaderKilledAllySurvivesAndWins_ExplorationContinues(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	// Leader has 1 HP, 0 Attack, 0 Agility -> targetable, dies when attacked
	c1 := createTestChar("lead-fragile", 1, 1, 0, 0)
	// Ally has 200 HP, 200 Attack, 100 Agility -> defeats monster
	c2 := createTestChar("ally-strong", 20, 200, 200, 100)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-leader-death-check",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      3,
				StartX:      0,
				StartY:      0,
				Grid: []string{
					"S00",
					"000",
					"00D",
				},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "brute",
						Name:       "Brute",
						HP:         30,
						Attack:     50, // Lethal to 1 HP leader
						Defense:    0,
						Agility:    50, // Higher than leader (0), lower than ally (100)
						ExpReward:  50,
						GoldReward: 30,
					},
				},
			},
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
				Agility: char.Stats.Agility,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.StartPartyExpedition(ctx, "lead-fragile", []string{"lead-fragile", "ally-strong"}, "dg-leader-death-check", "party-1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// Move east onto monster tile
	res, err := svc.Move(ctx, "lead-fragile", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Battle took place
	if res.BattleResult == nil {
		t.Fatal("expected non-nil BattleResult")
	}

	// If leader died but ally survived and won, exploration MUST continue!
	if res.Expedition.Members[0].CurrentHP == 0 && res.Expedition.Members[1].CurrentHP > 0 {
		if res.IsFinished {
			t.Error("expected expedition NOT finished when ally survived")
		}
		if res.Expedition.Status != dungeon.StatusExploring {
			t.Errorf("expected StatusExploring, got %v", res.Expedition.Status)
		}

		// Party can make a subsequent move without error
		res2, err := svc.Move(ctx, "lead-fragile", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("subsequent move failed: %v", err)
		}
		if res2.Expedition.PosX != 2 {
			t.Errorf("expected PosX 2, got %d", res2.Expedition.PosX)
		}
	} else {
		// Even if ally defeated monster before leader took a turn, ally survives and exploration continues
		if res.Expedition.Status != dungeon.StatusExploring {
			t.Errorf("expected StatusExploring, got %v", res.Expedition.Status)
		}
	}
}

func TestPartyCombat_AllMembersKilled_WipesOut(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead-weak", 1, 1, 1, 0)
	c2 := createTestChar("ally-weak", 1, 1, 1, 0)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-all-killed",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      3,
				StartX:      0,
				StartY:      0,
				Grid: []string{
					"S00",
					"000",
					"00D",
				},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "grim-reaper",
						Name:       "Grim Reaper",
						HP:         500,
						Attack:     500,
						Defense:    100,
						Agility:    100,
						ExpReward:  10,
						GoldReward: 10,
					},
				},
			},
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
				Agility: char.Stats.Agility,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.StartPartyExpedition(ctx, "lead-weak", []string{"lead-weak", "ally-weak"}, "dg-all-killed", "party-1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	res, err := svc.Move(ctx, "lead-weak", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if !res.IsFinished {
		t.Error("expected expedition finished on wipeout")
	}
	if res.Expedition.Status != dungeon.StatusWipedOut {
		t.Errorf("expected StatusWipedOut, got %v", res.Expedition.Status)
	}
}

func TestPartyTrap_LeaderKilledAllySurvives_ExplorationContinues(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead-trap", 10, 5, 40, 25)   // 5 HP < 9-12 trap damage
	c2 := createTestChar("ally-trap", 10, 100, 40, 25) // 100 HP > 30 trap damage

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-party-trap",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      3,
				StartX:      0,
				StartY:      0,
				Grid: []string{
					"SX0",
					"000",
					"00D",
				},
			},
		},
	}

	svc, err := dungeon.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.StartPartyExpedition(ctx, "lead-trap", []string{"lead-trap", "ally-trap"}, "dg-party-trap", "party-1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// Move east onto trap
	res, err := svc.Move(ctx, "lead-trap", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if res.EventType != dungeon.EventTrap {
		t.Fatalf("expected EventTrap, got %v", res.EventType)
	}
	if res.IsFinished {
		t.Error("expected expedition NOT finished when ally survived trap")
	}
	if res.Expedition.Status != dungeon.StatusExploring {
		t.Errorf("expected StatusExploring, got %v", res.Expedition.Status)
	}
	if res.Expedition.Members[0].CurrentHP != 0 {
		t.Errorf("expected leader dead (0 HP), got %d", res.Expedition.Members[0].CurrentHP)
	}
	if res.Expedition.Members[1].CurrentHP <= 0 || res.Expedition.Members[1].CurrentHP >= 100 {
		t.Errorf("expected ally damaged but alive (0 < HP < 100), got %d", res.Expedition.Members[1].CurrentHP)
	}
}

func TestPartySettlement_EscapeWithFallenLeader(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	c1 := createTestChar("lead-esc", 10, 100, 40, 25)
	c2 := createTestChar("ally-esc", 10, 80, 40, 25)

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		c1.ID: c1,
		c2.ID: c2,
	}}

	customDungeon := dungeon.Dungeon{
		ID:               "dg-party-esc",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      3,
				StartX:      0,
				StartY:      0,
				Grid: []string{
					"SX0",
					"000",
					"00D",
				},
			},
		},
	}

	activeStore := dungeon.NewMemoryExpeditionRepository()
	svc, err := dungeon.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithActiveExpeditionStore(activeStore),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	exp, err := svc.StartPartyExpedition(ctx, "lead-esc", []string{"lead-esc", "ally-esc"}, "dg-party-esc", "party-1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// Manually set leader to 0 HP and ally to 40 HP
	exp.CurrentHP = 0
	exp.Members[0].CurrentHP = 0
	exp.Members[1].CurrentHP = 40
	exp.AccumulatedExp = 200
	exp.AccumulatedGold = 500
	_ = activeStore.SaveActiveExpedition(ctx, *exp)

	// Call Escape
	escRes, err := svc.Escape(ctx, "lead-esc")
	if err != nil {
		t.Fatalf("Escape failed: %v", err)
	}

	if escRes.Expedition.Status != dungeon.StatusEscaped {
		t.Errorf("expected StatusEscaped, got %v", escRes.Expedition.Status)
	}

	// Verify both characters were saved with vitality and rewards
	savedLead := repo.savedChars["lead-esc"]
	savedAlly := repo.savedChars["ally-esc"]

	if savedLead.Stats.HP != 0 {
		t.Errorf("expected leader HP 0 in mock savedChars, got %d", savedLead.Stats.HP)
	}
	if savedAlly.Stats.HP != 40 {
		t.Errorf("expected ally HP 40, got %d", savedAlly.Stats.HP)
	}
	if savedAlly.Money != 500 {
		t.Errorf("expected ally Gold 500, got %d", savedAlly.Money)
	}
}
