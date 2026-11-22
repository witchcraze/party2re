package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
)

type tameWiringRandom struct{}

func (tameWiringRandom) Intn(int) (int, error) { return 0, nil }

func TestMonsterTamerProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	// A deterministic battle service leaves Monster injection to wireHooks.
	f.cmbt.battle = battle.NewService(
		battle.WithCharacterRepository(f.core.charRepo),
		battle.WithInventoryRepository(f.core.invRepo),
		battle.WithEquipmentRepository(f.econ.equipRepo),
		battle.WithTransactionProvider(f.core.txProvider),
		battle.WithRandomSource(tameWiringRandom{}))
	if err := wireHooks(f.core, f.cmbt, f.econ, f.misc, f.soc); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.cmbt.battle.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs:    []string{f.char.ID},
		BattleResult:    corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, RemainingHP: map[string]int{f.char.ID: f.char.Stats.HP}},
		DefeatedEnemies: []corebattle.Participant{{ID: "monster-002-f1-1", Name: "Slime", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5}},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := f.misc.monsterRepo.ListByCharacterIDAndLocation(ctx, f.char.ID, monster.LocationBox)
	if err != nil || len(stored) != 1 || stored[0].MonsterID != "monster-002" {
		t.Fatalf("production tamer connection = %+v, %v", stored, err)
	}
}

func TestBossVictoryBanquetProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	tracker := &victoryCrowdTracker{}
	plaza, err := eventplaza.NewService(
		&struct{ eventplaza.Repository }{}, &struct{ eventplaza.CharacterRepository }{},
		&struct{ eventplaza.InventoryRepository }{}, eventplaza.WithPresenceTracker(tracker))
	if err != nil {
		t.Fatal(err)
	}
	f.misc.eventplaza = plaza
	if err := wireHooks(f.core, f.cmbt, f.econ, f.misc, f.soc); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f.char.Level, f.char.Stats.HP, f.char.Stats.MaxHP = 99, 500000, 500000
	f.char.Stats.Attack, f.char.Stats.Defense, f.char.Stats.Agility = 999999, 99999, 9999
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	result, err := f.cmbt.boss.ChallengeBoss(ctx, f.char.ID, "king1")
	if err != nil || result.Outcome != corebattle.OutcomeWin {
		t.Fatalf("boss victory = %+v, %v", result, err)
	}
	if tracker.calls != 1 {
		t.Fatalf("production banquet calls = %d", tracker.calls)
	}
}

func TestGvGExitTavernProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	ctx := context.Background()

	// Create 3 guild leaders for affordable, unaffordable, and no-delivery exit paths.
	g1, c1, err := database.CreateTestGuildWithLeader(ctx, f.db, fmt.Sprintf("GvGHook1_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g2, c2, err := database.CreateTestGuildWithLeader(ctx, f.db, fmt.Sprintf("GvGHook2_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g3, c3, err := database.CreateTestGuildWithLeader(ctx, f.db, fmt.Sprintf("GvGHook3_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, gid := range []string{g1.ID, g2.ID, g3.ID} {
			if _, err := f.db.ExecContext(cleanupCtx, "DELETE FROM guilds WHERE id = ?", gid); err != nil {
				t.Error(err)
			}
		}
		for _, ch := range []struct{ id, playerID string }{
			{c1.ID, c1.PlayerID},
			{c2.ID, c2.PlayerID},
			{c3.ID, c3.PlayerID},
		} {
			if err := f.core.charRepo.Delete(cleanupCtx, ch.id); err != nil {
				t.Error(err)
			}
			if _, err := f.db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", ch.playerID); err != nil {
				t.Error(err)
			}
		}
	})
	for _, gid := range []string{g1.ID, g2.ID, g3.ID} {
		if _, err := f.db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", gid); err != nil {
			t.Fatal(err)
		}
	}

	// c1: affordable delivery (tavern_water: 20G).
	if _, err := f.misc.tavern.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.tavern.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	c1Before, err := f.core.charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}

	// c2: unaffordable delivery (tavern_curry: 400G, wallet 10G).
	if _, err := f.misc.tavern.OrderMeal(ctx, c2.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.tavern.ReserveDelivery(ctx, c2.ID, "tavern_curry"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, "UPDATE characters SET money = 10 WHERE id = ?", c2.ID); err != nil {
		t.Fatal(err)
	}

	// c3: fullness without delivery reservation.
	if _, err := f.misc.tavern.OrderMeal(ctx, c3.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name         string
		charID       string
		wantMoney    int
		wantEaten    int
		wantDelivery bool
	}{
		{"affordable", c1.ID, c1Before.Money - 20, 2, true},
		{"unaffordable", c2.ID, 10, 1, true},
		{"no order", c3.ID, 500 - 20, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			room, err := f.cmbt.gvg.CreateRoom(ctx, tc.charID, gvg.CreateRoomRequest{Name: "Hook " + tc.name, MaxMembers: 2, TargetWins: 1})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.cmbt.gvg.LeaveRoom(ctx, tc.charID, room.Room.ID); err != nil {
				t.Fatalf("LeaveRoom(%s): %v", tc.name, err)
			}
			gotChar, err := f.core.charRepo.FindByID(ctx, tc.charID)
			if err != nil {
				t.Fatal(err)
			}
			if gotChar.Money != tc.wantMoney {
				t.Fatalf("Money = %d, want %d", gotChar.Money, tc.wantMoney)
			}
			tStatus, err := f.misc.tavern.GetStatus(ctx, tc.charID)
			if err != nil {
				t.Fatal(err)
			}
			if tStatus.IsFull || (tStatus.DeliveryReservation != nil) != tc.wantDelivery {
				t.Fatalf("tavern status = %+v, want IsFull=false hasDelivery=%v", tStatus, tc.wantDelivery)
			}
			var mealsEaten int
			if err := f.db.QueryRowContext(ctx, `SELECT total_meals_eaten FROM tavern_character_status WHERE character_id = ?`, tc.charID).Scan(&mealsEaten); err != nil {
				t.Fatal(err)
			}
			if mealsEaten != tc.wantEaten {
				t.Fatalf("total_meals_eaten = %d, want %d", mealsEaten, tc.wantEaten)
			}
			var deliveryDone bool
			if err := f.db.QueryRowContext(ctx, `SELECT delivery_done FROM gvg_exit_claims WHERE room_id = ? AND character_id = ? AND join_id = ?`, room.Room.ID, tc.charID, room.Members[0].JoinID).Scan(&deliveryDone); err != nil {
				t.Fatal(err)
			}
			if !deliveryDone {
				t.Fatal("expected delivery_done = true")
			}
		})
	}
}
