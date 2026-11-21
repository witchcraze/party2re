package gvg_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/tavern"
)

type testDetRNG struct{ gen random.Generator }

func (r testDetRNG) Intn(max int) (int, error) { return r.gen.Intn(max), nil }
func (r testDetRNG) Float64() float64          { return r.gen.Float64() }

func TestGvG_LeaveRoom_SideEffectsAndRetryIdempotency_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db := database.OpenTestDB(t)
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{os.Getenv("PARTY2_VALKEY_ADDR")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	baseRooms, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	roomRepo := &failingDeleteCharRoomRepo{RoomRepository: baseRooms, err: errors.New("valkey delete failure")}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProv := database.NewTransactionProvider(db)
	econSvc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
	if err != nil {
		t.Fatal(err)
	}
	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionRunner(econSvc),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(testDetRNG{gen: random.NewDeterministic(1)}),
	)
	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernSvc, err := tavern.NewService(tavernCatalog, tavernRepo, charRepo, txProv)
	if err != nil {
		t.Fatal(err)
	}

	g1, c1, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGExit1_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g2, c2, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGExit2_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", g1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#33CCFF' WHERE id = ?", g2.ID); err != nil {
		t.Fatal(err)
	}

	// c1 equips item-105 (幸せのくつ) and has fullness + delivery reservation.
	shoes, err := coreitem.NewInstance("item-105", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, c1.ID, []coreitem.Instance{shoes}); err != nil {
		t.Fatal(err)
	}
	if err := equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: c1.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: shoes.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	c1AfterOrder, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}

	// c2 equips item-193 (ドクロのお守り) which gets consumed in round 1.
	amulet, err := coreitem.NewInstance("item-193", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, c2.ID, []coreitem.Instance{amulet}); err != nil {
		t.Fatal(err)
	}
	if err := equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: c2.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: amulet.ID}}); err != nil {
		t.Fatal(err)
	}

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:       corebattle.OutcomeWin,
				RemainingHP:   map[string]int{c1.ID: 20, c2.ID: 0},
				RemainingMP:   map[string]int{c1.ID: 4, c2.ID: 0},
				ConsumedItems: map[string][]corebattle.ConsumedItem{c2.ID: {{ID: "item-193", Quantity: 1}}},
			},
		},
	}

	svc, err := gvg.NewService(
		roomRepo, gvgRepo, guildRepo, charRepo, engine,
		gvg.WithParticipantBuilder(battleSvc),
		gvg.WithPostBattleSettler(battleSvc),
		gvg.WithTransactionProvider(txProv),
		gvg.WithExitClaimRepository(gvgRepo),
		gvg.WithPostExitHook(func(ctx context.Context, characterID string) (string, error) {
			if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
				return "", err
			}
			res, err := tavernSvc.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) {
				return "", nil
			}
			return res.Item.Name, err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "Exit Integration", MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = baseRooms.DeleteRoom(ctx, detail.Room.ID)
	})
	if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 1); err != nil {
		t.Fatal(err)
	}

	// Simulate transient Valkey failure on c1 LeaveRoom, then retry -> must not duplicate fatigue, EXP, or delivery charge.
	roomRepo.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected Valkey DeleteCharacterRoom failure")
	}
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("c1 LeaveRoom retry failed: %v", err)
	}

	updatedC1, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	// HP: 20 + 40 (amore water delivery) = 60 (clamped to MaxHP), Tired: 4 (1*3+1), EXP: +20 (item-105), Money: -20 (amore water).
	wantHP := min(updatedC1.Stats.MaxHP, 20+40)
	if updatedC1.Stats.HP != wantHP || updatedC1.Stats.MP != 4 || updatedC1.Tired != 4 || updatedC1.Experience != c1AfterOrder.Experience+20 || updatedC1.Money != c1AfterOrder.Money-20 {
		t.Fatalf("c1 state after exit = %+v (before money=%d exp=%d, want HP=%d MP=4 Tired=4)", updatedC1, c1AfterOrder.Money, c1AfterOrder.Experience, wantHP)
	}
	tStatus, err := tavernSvc.GetStatus(ctx, c1.ID)
	if err != nil || tStatus.IsFull {
		t.Fatalf("c1 fullness should be reset: %+v err=%v", tStatus, err)
	}

	// c2 leaves -> consumed item-193 must be unequipped and removed from inventory, HP floored to 1, Tired = 4.
	if err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatalf("c2 LeaveRoom failed: %v", err)
	}
	updatedC2, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedC2.Stats.HP != 1 || updatedC2.Stats.MP != 0 || updatedC2.Tired != 4 {
		t.Fatalf("c2 state after exit = %+v, want HP=1 MP=0 Tired=4", updatedC2)
	}
	c2Inv, err := invRepo.FindByCharacterID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(c2Inv.Items) != 0 {
		t.Fatalf("expected consumed item-193 cleared from inventory, got %+v", c2Inv.Items)
	}
	c2Equip, err := equipRepo.FindByCharacterID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, equipped := c2Equip.Equipped(coreitem.SlotAccessory); equipped {
		t.Fatalf("expected consumed accessory unequipped, got %+v", c2Equip.Slots)
	}
}

func TestGvG_LeaveRoom_RejoinAfterPreStartExit_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db := database.OpenTestDB(t)
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{os.Getenv("PARTY2_VALKEY_ADDR")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	baseRooms, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	roomRepo := &failingDeleteCharRoomRepo{RoomRepository: baseRooms, err: errors.New("valkey delete failure")}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProv := database.NewTransactionProvider(db)
	econSvc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
	if err != nil {
		t.Fatal(err)
	}
	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionRunner(econSvc),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(testDetRNG{gen: random.NewDeterministic(1)}),
	)
	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernSvc, err := tavern.NewService(tavernCatalog, tavernRepo, charRepo, txProv)
	if err != nil {
		t.Fatal(err)
	}

	g1, c1, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGRejoin1_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g2, c2, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGRejoin2_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", g1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#33CCFF' WHERE id = ?", g2.ID); err != nil {
		t.Fatal(err)
	}

	shoes, err := coreitem.NewInstance("item-105", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, c1.ID, []coreitem.Instance{shoes}); err != nil {
		t.Fatal(err)
	}
	if err := equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: c1.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: shoes.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{c1.ID: 20, c2.ID: 0},
				RemainingMP: map[string]int{c1.ID: 4, c2.ID: 0},
			},
		},
	}
	svc, err := gvg.NewService(
		roomRepo, gvgRepo, guildRepo, charRepo, engine,
		gvg.WithParticipantBuilder(battleSvc),
		gvg.WithPostBattleSettler(battleSvc),
		gvg.WithTransactionProvider(txProv),
		gvg.WithExitClaimRepository(gvgRepo),
		gvg.WithPostExitHook(func(ctx context.Context, characterID string) (string, error) {
			if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
				return "", err
			}
			res, err := tavernSvc.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) {
				return "", nil
			}
			return res.Item.Name, err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "Rejoin Integration", MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = baseRooms.DeleteRoom(ctx, detail.Room.ID)
	})
	if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	// 1st exit pre-start (c1).
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("c1 pre-start LeaveRoom failed: %v", err)
	}
	afterFirstExit, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFirstExit.Tired != 1 {
		t.Fatalf("after 1st pre-start exit Tired = %d, want 1", afterFirstExit.Tired)
	}

	// Reserve a new meal/delivery, rejoin same room, fight round 1, then leave with transient Valkey failure + retry.
	if _, err := tavernSvc.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	c1BeforeRejoinExit, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.JoinRoom(ctx, c1.ID, detail.Room.ID, ""); err != nil {
		t.Fatalf("c1 rejoin failed: %v", err)
	}
	if _, err := svc.StartMatch(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c2.ID, detail.Room.ID, 1); err != nil {
		t.Fatal(err)
	}

	roomRepo.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected Valkey DeleteCharacterRoom failure on 2nd exit")
	}
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("c1 2nd LeaveRoom retry failed: %v", err)
	}

	afterSecondExit, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	// HP: 20 + 40 (delivery), MP: 4, Tired: 1 (first exit) + 4 (round 1 exit), EXP: +20 (item-105), Money: -20 (2nd delivery, not doubled).
	wantHP := min(afterSecondExit.Stats.MaxHP, 20+40)
	if afterSecondExit.Stats.HP != wantHP || afterSecondExit.Stats.MP != 4 || afterSecondExit.Tired != 5 ||
		afterSecondExit.Experience != c1BeforeRejoinExit.Experience+20 || afterSecondExit.Money != c1BeforeRejoinExit.Money-20 {
		t.Fatalf("c1 state after rejoin exit = %+v (before money=%d exp=%d, want HP=%d MP=4 Tired=5)", afterSecondExit, c1BeforeRejoinExit.Money, c1BeforeRejoinExit.Experience, wantHP)
	}
	tStatus, err := tavernSvc.GetStatus(ctx, c1.ID)
	if err != nil || tStatus.IsFull {
		t.Fatalf("c1 fullness should be reset after rejoin exit: %+v err=%v", tStatus, err)
	}
}
