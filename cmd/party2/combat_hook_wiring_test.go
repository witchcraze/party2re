package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
)

type tameWiringRandom struct{}

func (tameWiringRandom) Intn(int) (int, error) { return 0, nil }

func TestMonsterTamerProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, testutil.OpenValkey(t))
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
	client := testutil.OpenValkey(t)
	f := newHookWiringFixture(t, client)
	tracker := &victoryCrowdTracker{}
	plaza, err := eventplaza.NewService(
		tracker, &struct{ eventplaza.CharacterRepository }{}, &struct{ eventplaza.InventoryRepository }{})
	if err != nil {
		t.Fatal(err)
	}
	f.misc.eventplaza = plaza
	if err := wireHooks(f.core, f.cmbt, f.econ, f.misc, f.soc); err != nil {
		t.Fatal(err)
	}
	// Joining checks pending scheduled actions; recurring schedules stay unregistered.
	f.soc.sched = scheduling.NewService(f.soc.schedRepo)
	ctx := context.Background()
	f.char.Stats.HP, f.char.Stats.MaxHP = 500, 500
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	// A private slot keeps this room apart from the shared stage rooms; its enemies are already defeated.
	runID, err := f.cmbt.boss.Open(ctx, "test:"+id.New(), runhost.Admission{StageID: "king1", Name: "test", Speed: 12}, func(r *boss.Run) { r.Round = 1 })
	if err != nil {
		t.Fatal(err)
	}
	joined, err := f.cmbt.boss.Join(ctx, f.char.PlayerID, f.char.ID, runID, id.New())
	if err != nil {
		t.Fatal(err)
	}
	cmd := runhost.Command{PlayerID: f.char.PlayerID, ActorID: f.char.ID, RunID: runID, ID: id.New(), Version: joined.Version, Action: "seal"}
	if _, err := f.cmbt.boss.Command(ctx, cmd); err != nil {
		t.Fatalf("seal: %v", err)
	}
	// A replay returns the receipt without celebrating again.
	if _, err := f.cmbt.boss.Command(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if tracker.calls != 1 {
		t.Fatalf("production banquet calls = %d", tracker.calls)
	}
}

func TestGvGExitTavernProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, testutil.OpenValkey(t))
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
			room, err := f.cmbt.gvg.CreateRoom(ctx, tc.charID, gvg.CreateRoomRequest{Name: "Hook-" + tc.charID, MaxMembers: 2, TargetWins: 1})
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

func TestPvPExitTavernProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, testutil.OpenValkey(t))
	ctx := context.Background()

	f.char.Money = 500
	f.char.Stats.HP, f.char.Stats.MaxHP = 20, 100
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	shoes, err := coreitem.NewInstance("item-105", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.core.invRepo.Save(ctx, coreinventory.Inventory{CharacterID: f.char.ID, Items: []coreitem.Instance{shoes}}); err != nil {
		t.Fatal(err)
	}
	if err := f.econ.equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: f.char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: shoes.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.tavern.OrderMeal(ctx, f.char.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.tavern.ReserveDelivery(ctx, f.char.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}

	room, err := f.cmbt.pvp.CreateRoom(ctx, f.char.ID, pvp.CreateRoomRequest{Name: "PvPHook", Bet: 50, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = f.db.ExecContext(cleanupCtx, "DELETE FROM pvp_character_claims WHERE room_id = ?", room.Room.ID)
		_, _ = f.db.ExecContext(cleanupCtx, "DELETE FROM pvp_room_transitions WHERE room_id = ?", room.Room.ID)
	})
	if err := f.cmbt.pvp.LeaveRoom(ctx, f.char.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	gotChar, err := f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	// OrderMeal spent 20G and healed +40 HP (480G, 60 HP); LeaveRoom refunds bet (50G),
	// claims delivery (-20G -> 460G, +40 HP -> 100 HP), and adds pre-start fatigue +1.
	if gotChar.Money != 460 || gotChar.Stats.HP != 100 || gotChar.Tired != 1 {
		t.Fatalf("unexpected char after PvP exit: Money=%d HP=%d Tired=%d", gotChar.Money, gotChar.Stats.HP, gotChar.Tired)
	}
	tStatus, err := f.misc.tavern.GetStatus(ctx, f.char.ID)
	if err != nil || tStatus.IsFull || tStatus.DeliveryReservation == nil {
		t.Fatalf("unexpected tavern status: %+v err=%v", tStatus, err)
	}
}
