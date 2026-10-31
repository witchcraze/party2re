package dungeon_test

import (
	"context"
	"os"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func customChestDungeon() dungeon.Dungeon {
	return dungeon.Dungeon{
		ID:               "dg-chest-revisit",
		Tier:             1,
		MinLevel:         1,
		MaxTurnsPerFloor: 30,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1, Width: 3, Height: 2, StartX: 0, StartY: 0,
				Grid: []string{"S0T", "000"},
			},
		},
	}
}

func makeTestBuilder(charRepo *mockCharRepo) *customParticipantBuilder {
	return &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			c := charRepo.chars[charID]
			return corebattle.Participant{
				ID: c.ID, Name: c.Name, HP: currentHP, MaxHP: c.Stats.HP, Attack: c.Stats.Attack, Defense: c.Stats.Defense,
			}, nil
		},
	}
}

func TestPartyChest_RevisitChestDoesNotDuplicateMemberGold_Memory(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	cLead := createTestChar("lead-chest", 10, 100, 50, 30)
	cAlly := createTestChar("ally-chest", 10, 100, 50, 30)
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cLead.ID: cLead, cAlly.ID: cAlly}}

	svc, err := dungeon.NewService(
		repo, charRepo, &customPartyBattleEngine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customChestDungeon()}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(makeTestBuilder(charRepo)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if _, err := svc.StartPartyExpedition(ctx, cLead.ID, []string{cLead.ID, cAlly.ID}, "dg-chest-revisit", "p1"); err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// 1. Move East to (1, 0)
	if _, err := svc.Move(ctx, cLead.ID, dungeon.DirectionEast); err != nil {
		t.Fatalf("Move (1,0) failed: %v", err)
	}

	// 2. Move East to (2, 0) - Chest first visit
	res, err := svc.Move(ctx, cLead.ID, dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventTreasure || res.Expedition.AccumulatedGold != 100 {
		t.Fatalf("First chest visit failed: %+v, err=%v", res, err)
	}
	for _, m := range res.Expedition.Members {
		if m.AccumulatedGold != 100 {
			t.Fatalf("expected member %s gold=100, got %d", m.CharacterID, m.AccumulatedGold)
		}
	}

	// 3. Move West back to (1, 0)
	if _, err := svc.Move(ctx, cLead.ID, dungeon.DirectionWest); err != nil {
		t.Fatalf("Move (1,0) back failed: %v", err)
	}

	// 4. Move East to (2, 0) - Revisit chest: must NOT duplicate rewards!
	res, err = svc.Move(ctx, cLead.ID, dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventMove || res.Expedition.AccumulatedGold != 100 {
		t.Fatalf("Revisit chest failed: %+v, err=%v", res, err)
	}
	for _, m := range res.Expedition.Members {
		if m.AccumulatedGold != 100 {
			t.Fatalf("member %s gold duplicated on revisit: got %d, expected 100", m.CharacterID, m.AccumulatedGold)
		}
	}

	// 5. Escape and verify history
	if _, err := svc.Escape(ctx, cLead.ID); err != nil {
		t.Fatalf("Escape failed: %v", err)
	}
	if len(repo.histories[cLead.ID]) != 1 || repo.histories[cLead.ID][0].GoldReward != 100 {
		t.Fatalf("expected history GoldReward=100, got %+v", repo.histories[cLead.ID])
	}
}

func TestPartyChest_RevisitChestDoesNotDuplicateMemberGold_Valkey(t *testing.T) {
	client := openTestValkey(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	valkeyStore, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatalf("NewValkeyExpeditionRepository failed: %v", err)
	}
	repo := newMockDungeonRepo()

	leadID := "lead-" + id.New()
	allyID := "ally-" + id.New()
	t.Cleanup(func() {
		_ = valkeyStore.DeleteActiveExpedition(ctx, leadID, "")
		_ = valkeyStore.DeleteActiveExpedition(ctx, allyID, "")
	})

	cLead := createTestChar(leadID, 10, 100, 50, 30)
	cAlly := createTestChar(allyID, 10, 100, 50, 30)
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cLead.ID: cLead, cAlly.ID: cAlly}}

	svc, err := dungeon.NewService(
		repo, charRepo, &customPartyBattleEngine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customChestDungeon()}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(makeTestBuilder(charRepo)),
		dungeon.WithActiveExpeditionStore(valkeyStore),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if _, err := svc.StartPartyExpedition(ctx, cLead.ID, []string{cLead.ID, cAlly.ID}, "dg-chest-revisit", "p1"); err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// 1. East to (1, 0)
	if _, err := svc.Move(ctx, cLead.ID, dungeon.DirectionEast); err != nil {
		t.Fatalf("Move (1,0) failed: %v", err)
	}
	// 2. East to (2, 0) - Chest visit
	res, err := svc.Move(ctx, cLead.ID, dungeon.DirectionEast)
	if err != nil || res.Expedition.AccumulatedGold != 100 {
		t.Fatalf("Chest visit failed: %+v, err=%v", res, err)
	}
	for _, m := range res.Expedition.Members {
		if m.AccumulatedGold != 100 {
			t.Fatalf("member %s gold != 100: got %d", m.CharacterID, m.AccumulatedGold)
		}
	}
	// 3. West back to (1, 0)
	if _, err := svc.Move(ctx, cLead.ID, dungeon.DirectionWest); err != nil {
		t.Fatalf("Move back failed: %v", err)
	}
	// 4. East to (2, 0) - Revisit in Lua script
	res, err = svc.Move(ctx, cLead.ID, dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventMove || res.Expedition.AccumulatedGold != 100 {
		t.Fatalf("Revisit failed: %+v, err=%v", res, err)
	}
	for _, m := range res.Expedition.Members {
		if m.AccumulatedGold != 100 {
			t.Fatalf("Lua script duplicated member %s gold: got %d, expected 100", m.CharacterID, m.AccumulatedGold)
		}
	}
}

func TestPartyChest_RevisitChest_RealMariaDBFinalization(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := database.CreateTestCharacter(ctx, db, "Chest Leader")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacter(ctx, db, "Chest Follower")
	if err != nil {
		t.Fatal(err)
	}

	c1.Money, c2.Money = 500, 300
	if err := charRepo.Update(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if err := charRepo.Update(ctx, c2); err != nil {
		t.Fatal(err)
	}

	builder := &customParticipantBuilder{
		buildFn: func(ctx context.Context, charID string, currentHP int) (corebattle.Participant, error) {
			c, err := charRepo.FindByID(ctx, charID)
			if err != nil {
				return corebattle.Participant{}, err
			}
			return corebattle.Participant{
				ID: c.ID, Name: c.Name, HP: currentHP, MaxHP: c.Stats.HP, Attack: c.Stats.Attack, Defense: c.Stats.Defense,
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		dungeonRepo, charRepo, &customPartyBattleEngine{},
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customChestDungeon()}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(builder),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if _, err := svc.StartPartyExpedition(ctx, c1.ID, []string{c1.ID, c2.ID}, "dg-chest-revisit", "p1"); err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}

	// Move East to (1, 0) -> East to (2, 0) Chest -> West back -> East revisit
	if _, err := svc.Move(ctx, c1.ID, dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Move(ctx, c1.ID, dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Move(ctx, c1.ID, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Move(ctx, c1.ID, dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}

	// Escape and settle to real MariaDB
	if _, err := svc.Escape(ctx, c1.ID); err != nil {
		t.Fatalf("Escape failed: %v", err)
	}

	// Verify durable MariaDB wallets: exactly 100 gold added to each member (not 200!)
	afterLead, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterAlly, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}

	if afterLead.Money != 500+100 {
		t.Fatalf("expected leader money 600, got %d (duplicate chest gold awarded!)", afterLead.Money)
	}
	if afterAlly.Money != 300+100 {
		t.Fatalf("expected ally money 400, got %d (duplicate chest gold awarded!)", afterAlly.Money)
	}
}

// TestPartyCombat_OutcomeDraw_BypassingEnemyToNonCombatTile_CurrentLimitation
// reproduces the parity gap where an unresolved 100-turn draw allows the party to bypass
// an undefeated monster by moving to an adjacent non-combat tile (treasure chest 'T').
//
// In original Party2 (lib/vs_dungeon.cgi:77):
//
//	if (@enemys > 0 && $event ne '宝') {
//	    $mes .= "※敵を全て倒すまで、先に進むことはできません";
//	    return;
//	}
//
// Under the current synchronous 1-step auto-battle engine without persistent in-combat state,
// a draw keeps the party at (0, 0). The party is then able to move South onto (0, 1) (chest 'T'),
// bypassing the undefeated monster at (1, 0). This test documents this current engine limitation
// and verifies that state and resources remain consistent, without asserting that the bypass is authentic parity.
func TestPartyCombat_OutcomeDraw_BypassingEnemyToNonCombatTile_CurrentLimitation(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()

	cLead := createTestChar("lead-draw-sub", 10, 100, 50, 30)
	cAlly := createTestChar("ally-draw-sub", 10, 100, 50, 30)
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{cLead.ID: cLead, cAlly.ID: cAlly}}

	// Map:
	// (0, 0) Start 'S', (1, 0) Path '0' (Tough monster: Draw)
	// (0, 1) Chest 'T' (Non-combat tile), (1, 1) Path '0'
	customDungeon := dungeon.Dungeon{
		ID: "dg-draw-branch", MinLevel: 1, MaxTurnsPerFloor: 25,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1, Width: 2, Height: 2, StartX: 0, StartY: 0,
				Grid: []string{"S0", "T0"},
				Monsters: []dungeon.DungeonMonster{
					{ID: "mon-stalemate", Name: "Stone Wall Golem", HP: 500, Attack: 5, ExpReward: 100},
				},
			},
		},
	}

	engine := &customPartyBattleEngine{
		resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			// Fight 1 (moving East onto (1,0)): Draw, party takes 10 damage
			return corebattle.PartyBattleResult{
				Outcome: corebattle.OutcomeDraw, Turns: 100,
				RemainingHP: map[string]int{"lead-draw-sub": 90, "ally-draw-sub": 90},
				RemainingMP: map[string]int{"lead-draw-sub": 10, "ally-draw-sub": 10},
			}, nil
		},
	}

	svc, err := dungeon.NewService(
		repo, charRepo, engine,
		dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
		dungeon.WithAdmission(approvedRoster{}),
		dungeon.WithParticipantBuilder(makeTestBuilder(charRepo)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	exp, err := svc.StartPartyExpedition(ctx, cLead.ID, []string{cLead.ID, cAlly.ID}, "dg-draw-branch", "p1")
	if err != nil {
		t.Fatalf("StartPartyExpedition failed: %v", err)
	}
	initialTurns := exp.TurnsRemaining

	// 1. Move East towards (1, 0) -> Draws against tough enemy
	res, err := svc.Move(ctx, cLead.ID, dungeon.DirectionEast)
	if err != nil || res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("Draw fight failed: %+v, err=%v", res, err)
	}
	// Party does not enter (1, 0); remains at (0, 0)
	if res.Expedition.PosX != 0 || res.Expedition.PosY != 0 || res.Expedition.TurnsRemaining != initialTurns-1 {
		t.Fatalf("expected pos (0,0) and turns-1, got (%d,%d) turns=%d",
			res.Expedition.PosX, res.Expedition.PosY, res.Expedition.TurnsRemaining)
	}

	// 2. Move in another direction to a non-combat tile: South to (0, 1) Chest 'T'
	// Reproduction: under current engine, this succeeds and reaches the chest, demonstrating
	// the bypass of the undefeated enemy at (1, 0).
	res2, err := svc.Move(ctx, cLead.ID, dungeon.DirectionSouth)
	if err != nil {
		t.Fatalf("Move South failed: %v", err)
	}
	if res2.Expedition.PosX != 0 || res2.Expedition.PosY != 1 {
		t.Fatalf("expected pos (0,1), got (%d,%d)", res2.Expedition.PosX, res2.Expedition.PosY)
	}
	if res2.EventType != dungeon.EventTreasure {
		t.Fatalf("expected EventTreasure, got %v", res2.EventType)
	}
	if res2.Expedition.TurnsRemaining != initialTurns-2 {
		t.Fatalf("expected turns-2 (%d), got %d", initialTurns-2, res2.Expedition.TurnsRemaining)
	}
}
