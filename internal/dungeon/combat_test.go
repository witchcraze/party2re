package dungeon_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type customParticipantBuilder struct {
	buildFn func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error)
}

func (b *customParticipantBuilder) BuildParticipantWithCurrentHP(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
	if b.buildFn != nil {
		return b.buildFn(ctx, charID, currentHP)
	}
	return corebattle.Participant{ID: charID, HP: currentHP, MaxHP: currentHP}, nil
}

func (b *customParticipantBuilder) BuildParticipantWithState(ctx context.Context, charID string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error) {
	p, err := b.BuildParticipantWithCurrentHP(ctx, charID, hp)
	if err != nil {
		return corebattle.Participant{}, err
	}
	if mp != nil {
		p.MP = *mp
	}
	return p, nil
}

func TestDungeonSolo_FirstStrikeDefeatWinsWithoutWipeout(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	// Player has 10 HP and high Attack/Agility to strike first and deal lethal damage.
	// Monster also has high Attack (50) which WOULD be lethal to player (10 HP) if counterattack executed.
	char := corecharacter.Character{
		ID:    "hero-speedy",
		Name:  "Speedy Hero",
		Level: 10,
		Stats: corecharacter.Stats{
			HP:      10,
			MaxHP:   10,
			Attack:  100,
			Defense: 0,
			Agility: 50,
		},
	}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}

	customDungeon := dungeon.Dungeon{
		ID:               "dungeon-lethal-draw-check",
		MinLevel:         1,
		MaxTurnsPerFloor: 10,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       2,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S0"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "glass-cannon-monster",
						Name:       "Glass Cannon",
						HP:         10,
						Attack:     50,
						Defense:    0,
						Agility:    5,
						ExpReward:  100,
						GoldReward: 50,
					},
				},
			},
		},
	}

	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithRNG(random.NewDeterministic(1)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	_, err = service.StartExpedition(ctx, char.ID, customDungeon.ID)
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	// Move east to (1, 0) to encounter the monster
	res, err := service.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v (wipeout=%v)", res.EventType, res.EventType == dungeon.EventWipeout)
	}
	if res.BattleResult == nil {
		t.Fatalf("expected non-nil BattleResult")
	}
	if res.BattleResult.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected BattleResult.Outcome == OutcomeWin, got %v", res.BattleResult.Outcome)
	}
	if res.BattleResult.WinnerID != char.ID {
		t.Fatalf("expected WinnerID %s, got %s", char.ID, res.BattleResult.WinnerID)
	}
}

func TestDungeonSolo_ParticipantWithSkillsUsesSkills(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	char := corecharacter.Character{
		ID:    "mage",
		Name:  "Mage Hero",
		Level: 10,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			MP:      20,
			MaxMP:   20,
			Attack:  5, // Low physical attack
			Defense: 10,
			Agility: 30,
		},
	}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}

	// High defense monster that physical attack cannot hurt effectively, but magic skill defeats
	customDungeon := dungeon.Dungeon{
		ID:               "dungeon-magic-test",
		MinLevel:         1,
		MaxTurnsPerFloor: 10,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       2,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S0"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "iron-turtle",
						Name:       "Iron Turtle",
						HP:         30,
						Attack:     5,
						Defense:    100,
						Agility:    5,
						ExpReward:  50,
						GoldReward: 20,
					},
				},
			},
		},
	}

	builder := &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			return corebattle.Participant{
				ID:      charID,
				Name:    "Mage Hero",
				HP:      currentHP,
				MaxHP:   100,
				MP:      20,
				MaxMP:   20,
				Attack:  5,
				Defense: 10,
				Agility: 30,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "skill-fireball",
						Name:        "ファイアボール",
						MPCost:      5,
						Power:       50,
						Kind:        "attack",
						TargetScope: "single_enemy",
						Element:     "fire",
					},
				},
			}, nil
		},
	}

	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithParticipantBuilder(builder),
		dungeon.WithRNG(random.NewDeterministic(1)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	_, err = service.StartExpedition(ctx, char.ID, customDungeon.ID)
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	res, err := service.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res.EventType)
	}
	if res.BattleResult == nil {
		t.Fatalf("expected BattleResult")
	}
	if res.BattleResult.Outcome != corebattle.OutcomeWin || res.BattleResult.WinnerID != char.ID {
		t.Fatalf("expected player victory, got outcome=%v winner=%s", res.BattleResult.Outcome, res.BattleResult.WinnerID)
	}

	// Verify that the skill was actually used in logs!
	skillUsed := false
	for _, log := range res.BattleResult.Logs {
		if log.ActionName == "ファイアボール" {
			skillUsed = true
			break
		}
	}
	if !skillUsed {
		t.Fatalf("expected skill 'ファイアボール' to be executed in combat logs, logs: %+v", res.BattleResult.Logs)
	}
}

type minimalRandomSource struct {
	intnCalls    int
	float64Calls int
}

func (m *minimalRandomSource) Intn(n int) int {
	m.intnCalls++
	return 0
}

func (m *minimalRandomSource) Float64() float64 {
	m.float64Calls++
	return 0.5
}

func TestDungeonSolo_CustomRNGWithoutIntN(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	char := corecharacter.Character{
		ID:    "hero-rng",
		Name:  "RNG Hero",
		Level: 10,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			Attack:  50,
			Defense: 10,
			Agility: 20,
		},
	}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}

	customDungeon := dungeon.Dungeon{
		ID:               "dungeon-rng-test",
		MinLevel:         1,
		MaxTurnsPerFloor: 10,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       2,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S0"},
				Monsters: []dungeon.DungeonMonster{
					{
						ID:         "target-monster",
						Name:       "Target",
						HP:         20,
						Attack:     10,
						Defense:    5,
						Agility:    5,
						ExpReward:  10,
						GoldReward: 10,
					},
				},
			},
		},
	}

	customRNG := &minimalRandomSource{}

	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithRNG(customRNG),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	_, err = service.StartExpedition(ctx, char.ID, customDungeon.ID)
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	res, err := service.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res.EventType)
	}

	// Verify that the custom RNG's Intn or Float64 was indeed called during combat resolution!
	if customRNG.float64Calls == 0 && customRNG.intnCalls == 0 {
		t.Fatalf("expected custom RandomSource to be used by battle engine, but calls: Intn=%d, Float64=%d",
			customRNG.intnCalls, customRNG.float64Calls)
	}
}

func TestDungeonSolo_ConsecutiveCombatsPreserveHP(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	char := corecharacter.Character{
		ID:    "hero-runner",
		Name:  "Runner Hero",
		Level: 10,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			Attack:  30,
			Defense: 0,
			Agility: 10, // Slower than monsters so monsters get an attack in
		},
	}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}

	// A 3-tile path: Start at (0,0), monster at (1,0), monster at (2,0)
	customDungeon := dungeon.Dungeon{
		ID:               "dungeon-consecutive",
		MinLevel:         1,
		MaxTurnsPerFloor: 10,
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
						ID:         "monster-1",
						Name:       "Wolf",
						HP:         20,
						Attack:     30, // Deals ~15 damage
						Defense:    0,
						Agility:    30, // Acts first
						ExpReward:  10,
						GoldReward: 10,
					},
					{
						ID:         "monster-2",
						Name:       "Bear",
						HP:         20,
						Attack:     40, // Deals ~20 damage
						Defense:    0,
						Agility:    30, // Acts first
						ExpReward:  20,
						GoldReward: 20,
					},
				},
			},
		},
	}

	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithRNG(random.NewDeterministic(1)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	exp, err := service.StartExpedition(ctx, char.ID, customDungeon.ID)
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}
	if exp.CurrentHP != 100 {
		t.Fatalf("expected initial HP 100, got %d", exp.CurrentHP)
	}

	// Step 1: Move East to (1,0) - encounters monster-2 (1%2 = 1)
	res1, err := service.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move 1 failed: %v", err)
	}
	if res1.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle on step 1, got %v", res1.EventType)
	}
	hpAfterBattle1 := res1.Expedition.CurrentHP
	if hpAfterBattle1 >= 100 || hpAfterBattle1 <= 0 {
		t.Fatalf("expected reduced HP after battle 1, got %d", hpAfterBattle1)
	}

	// Step 2: Move East to (2,0) - encounters monster-1 (2%2 = 0)
	res2, err := service.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("Move 2 failed: %v", err)
	}
	if res2.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle on step 2, got %v", res2.EventType)
	}
	hpAfterBattle2 := res2.Expedition.CurrentHP
	if hpAfterBattle2 >= hpAfterBattle1 || hpAfterBattle2 <= 0 {
		t.Fatalf("expected HP to further decrease after battle 2 (started with %d), got %d",
			hpAfterBattle1, hpAfterBattle2)
	}
}
