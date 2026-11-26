package gvg_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
		gvg.WithPostExitHook(func(ctx context.Context, characterID string) (*gvg.DeliveryOutcome, error) {
			if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
				return nil, err
			}
			res, err := tavernSvc.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) {
				return nil, nil
			}
			return &gvg.DeliveryOutcome{ItemName: res.Item.Name, Paid: true}, err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "ExitIntegration", MaxMembers: 2, TargetWins: 1})
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
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected Valkey DeleteCharacterRoom failure")
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
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

	// c2 leaves -> consumed item-193 must be unequipped and removed from inventory, HP stays 0, Tired = 4.
	if _, err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatalf("c2 LeaveRoom failed: %v", err)
	}
	updatedC2, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedC2.Stats.HP != 0 || updatedC2.Stats.MP != 0 || updatedC2.Tired != 4 {
		t.Fatalf("c2 state after exit = %+v, want HP=0 MP=0 Tired=4", updatedC2)
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
		gvg.WithPostExitHook(func(ctx context.Context, characterID string) (*gvg.DeliveryOutcome, error) {
			if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
				return nil, err
			}
			res, err := tavernSvc.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) {
				return nil, nil
			}
			return &gvg.DeliveryOutcome{ItemName: res.Item.Name, Paid: true}, err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "RejoinIntegration", MaxMembers: 2, TargetWins: 1})
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
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
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
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected Valkey DeleteCharacterRoom failure on 2nd exit")
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
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

type faultIntegrationRoomRepo struct {
	gvg.RoomRepository
	failSave          atomic.Bool
	failDeleteRoom    atomic.Bool
	failDeleteChar    atomic.Bool
	getCharErr        error
	saveErr           error
	deleteRoomErr     error
	deleteCharRoomErr error
}

func (r *faultIntegrationRoomRepo) GetCharacterRoom(ctx context.Context, characterID string) (string, error) {
	if r.getCharErr != nil {
		return "", r.getCharErr
	}
	return r.RoomRepository.GetCharacterRoom(ctx, characterID)
}

func (r *faultIntegrationRoomRepo) SaveRoom(ctx context.Context, room gvg.GvGRoom, members []gvg.GvGMember) error {
	if r.failSave.Load() {
		return r.saveErr
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func (r *faultIntegrationRoomRepo) DeleteRoom(ctx context.Context, id string) error {
	if r.failDeleteRoom.Load() {
		return r.deleteRoomErr
	}
	return r.RoomRepository.DeleteRoom(ctx, id)
}

func (r *faultIntegrationRoomRepo) DeleteCharacterRoom(ctx context.Context, characterID, roomID string) error {
	if r.failDeleteChar.Load() {
		return r.deleteCharRoomErr
	}
	return r.RoomRepository.DeleteCharacterRoom(ctx, characterID, roomID)
}

func TestGvG_LeaveRoom_PartialPublicationRecoveryAndAdmissionGuard_Integration(t *testing.T) {
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
	roomRepo := &faultIntegrationRoomRepo{
		RoomRepository:    baseRooms,
		saveErr:           errors.New("valkey save failure"),
		deleteRoomErr:     errors.New("valkey delete room failure"),
		deleteCharRoomErr: errors.New("valkey delete character room failure"),
	}
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

	g1, c1, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGRec1_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g2, c2, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGRec2_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g3, c3, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGRec3_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range []string{g1.ID, g3.ID} {
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", gid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#33CCFF' WHERE id = ?", g2.ID); err != nil {
		t.Fatal(err)
	}

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{c1.ID: 25, c2.ID: 15},
				RemainingMP: map[string]int{c1.ID: 6, c2.ID: 3},
			},
		},
	}
	svc, err := gvg.NewService(
		roomRepo, gvgRepo, guildRepo, charRepo, engine,
		gvg.WithParticipantBuilder(battleSvc),
		gvg.WithPostBattleSettler(battleSvc),
		gvg.WithTransactionProvider(txProv),
		gvg.WithExitClaimRepository(gvgRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	var roomIDs []string
	t.Cleanup(func() {
		for _, rid := range roomIDs {
			_ = baseRooms.DeleteRoom(ctx, rid)
			for _, cid := range []string{c1.ID, c2.ID, c3.ID} {
				_ = baseRooms.DeleteCharacterRoom(ctx, cid, rid)
			}
		}
	})

	// 1. Lookup failure other than ErrRoomNotFound aborts CreateRoom and JoinRoom without writes.
	lookupErr := errors.New("valkey lookup failure")
	roomRepo.getCharErr = lookupErr
	if _, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "AbortedCreate"}); !errors.Is(err, lookupErr) {
		t.Fatalf("CreateRoom lookup error = %v, want %v", err, lookupErr)
	}
	roomRepo.getCharErr = nil

	room1, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "Room1", MaxMembers: 4, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	roomIDs = append(roomIDs, room1.Room.ID)
	if _, err := svc.JoinRoom(ctx, c2.ID, room1.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	roomRepo.getCharErr = lookupErr
	if _, err := svc.JoinRoom(ctx, c3.ID, room1.Room.ID, ""); !errors.Is(err, lookupErr) {
		t.Fatalf("JoinRoom lookup error = %v, want %v", err, lookupErr)
	}
	roomRepo.getCharErr = nil
	if gotRoom1, err := svc.GetRoom(ctx, room1.Room.ID); err != nil || len(gotRoom1.Members) != 2 {
		t.Fatalf("room1 mutated after aborted JoinRoom: %+v err=%v", gotRoom1, err)
	}
	if _, err := baseRooms.GetCharacterRoom(ctx, c3.ID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("c3 mapped after aborted JoinRoom: %v", err)
	}

	// Recruiting room 2 owned by c3.
	room2, err := svc.CreateRoom(ctx, c3.ID, gvg.CreateRoomRequest{Name: "Room2", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	roomIDs = append(roomIDs, room2.Room.ID)

	if _, err := svc.StartMatch(ctx, c1.ID, room1.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c1.ID, room1.Room.ID, 1); err != nil {
		t.Fatal(err)
	}

	// 2. Leader c1 leaves; SaveRoom fails after SQL side-effects commit.
	roomRepo.failSave.Store(true)
	if _, err := svc.LeaveRoom(ctx, c1.ID, room1.Room.ID); !errors.Is(err, roomRepo.saveErr) {
		t.Fatalf("c1 LeaveRoom expected SaveRoom failure, got %v", err)
	}
	roomRepo.failSave.Store(false)

	// Mapping still points to room1; c1 cannot join room2 or create a new room.
	if mapped, err := baseRooms.GetCharacterRoom(ctx, c1.ID); err != nil || mapped != room1.Room.ID {
		t.Fatalf("c1 mapping after failed SaveRoom = %q err=%v, want %q", mapped, err, room1.Room.ID)
	}
	if _, err := svc.JoinRoom(ctx, c1.ID, room2.Room.ID, ""); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("JoinRoom(room2) during partial leave = %v, want ErrAlreadyInRoom", err)
	}
	if _, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "Room3"}); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("CreateRoom during partial leave = %v, want ErrAlreadyInRoom", err)
	}

	// Retry c1 LeaveRoom, injecting a DeleteCharacterRoom failure after SaveRoom succeeds.
	roomRepo.failDeleteChar.Store(true)
	if _, err := svc.LeaveRoom(ctx, c1.ID, room1.Room.ID); !errors.Is(err, roomRepo.deleteCharRoomErr) {
		t.Fatalf("c1 LeaveRoom expected DeleteCharacterRoom failure, got %v", err)
	}
	roomRepo.failDeleteChar.Store(false)

	afterSave, err := svc.GetRoom(ctx, room1.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterSave.Members) != 1 || afterSave.Members[0].CharacterID != c2.ID || afterSave.Room.LeaderCharacterID != c2.ID {
		t.Fatalf("room1 leader transfer / removal mismatch: %+v", afterSave)
	}

	// Retry c1 LeaveRoom after roster removal -> clears mapping without repeating exit effects.
	if _, err := svc.LeaveRoom(ctx, c1.ID, room1.Room.ID); err != nil {
		t.Fatalf("c1 LeaveRoom mapping retry failed: %v", err)
	}
	if _, err := baseRooms.GetCharacterRoom(ctx, c1.ID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("c1 mapping should be deleted after retry, got %v", err)
	}
	updatedC1, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedC1.Tired != 7 {
		t.Fatalf("c1 Tired = %d, want 7 (round 2 exit applied once)", updatedC1.Tired)
	}

	// c1 joins room2; a stale LeaveRoom or DeleteCharacterRoom for room1 must not delete room2's mapping.
	if _, err := svc.JoinRoom(ctx, c1.ID, room2.Room.ID, ""); err != nil {
		t.Fatalf("c1 JoinRoom(room2) failed: %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, room1.Room.ID); !errors.Is(err, gvg.ErrCharacterNotInRoom) {
		t.Fatalf("stale LeaveRoom(room1) = %v, want ErrCharacterNotInRoom", err)
	}
	if err := baseRooms.DeleteCharacterRoom(ctx, c1.ID, room1.Room.ID); err != nil {
		t.Fatalf("stale DeleteCharacterRoom(room1): %v", err)
	}
	if mapped, err := baseRooms.GetCharacterRoom(ctx, c1.ID); err != nil || mapped != room2.Room.ID {
		t.Fatalf("c1 room2 mapping overwritten by stale room1 cleanup: got=%q err=%v want=%q", mapped, err, room2.Room.ID)
	}

	// 3. Last member c2 leaves room1: fail DeleteRoom first, then fail DeleteCharacterRoom after DeleteRoom.
	roomRepo.failDeleteRoom.Store(true)
	if _, err := svc.LeaveRoom(ctx, c2.ID, room1.Room.ID); !errors.Is(err, roomRepo.deleteRoomErr) {
		t.Fatalf("c2 LeaveRoom expected DeleteRoom failure, got %v", err)
	}
	roomRepo.failDeleteRoom.Store(false)
	if _, err := svc.CreateRoom(ctx, c2.ID, gvg.CreateRoomRequest{Name: "Blocked"}); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("c2 CreateRoom after failed DeleteRoom = %v, want ErrAlreadyInRoom", err)
	}

	roomRepo.failDeleteChar.Store(true)
	if _, err := svc.LeaveRoom(ctx, c2.ID, room1.Room.ID); !errors.Is(err, roomRepo.deleteCharRoomErr) {
		t.Fatalf("c2 LeaveRoom expected DeleteCharacterRoom failure, got %v", err)
	}
	roomRepo.failDeleteChar.Store(false)
	if _, err := svc.GetRoom(ctx, room1.Room.ID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("room1 should be deleted before DeleteCharacterRoom, got %v", err)
	}
	if mapped, err := baseRooms.GetCharacterRoom(ctx, c2.ID); err != nil || mapped != room1.Room.ID {
		t.Fatalf("c2 mapping should still await cleanup: got=%q err=%v", mapped, err)
	}

	// Retry c2 LeaveRoom completes mapping cleanup; c2 can then join room2 and stale room1 leave leaves room2 intact.
	if _, err := svc.LeaveRoom(ctx, c2.ID, room1.Room.ID); err != nil {
		t.Fatalf("c2 LeaveRoom mapping retry failed: %v", err)
	}
	updatedC2, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedC2.Tired != 7 {
		t.Fatalf("c2 Tired = %d, want 7 (round 2 exit applied once)", updatedC2.Tired)
	}
	if _, err := svc.JoinRoom(ctx, c2.ID, room2.Room.ID, ""); err != nil {
		t.Fatalf("c2 JoinRoom(room2) failed: %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c2.ID, room1.Room.ID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("stale LeaveRoom(room1) after disband = %v, want ErrRoomNotFound", err)
	}
	if mapped, err := baseRooms.GetCharacterRoom(ctx, c2.ID); err != nil || mapped != room2.Room.ID {
		t.Fatalf("c2 room2 mapping corrupted: got=%q err=%v want=%q", mapped, err, room2.Room.ID)
	}
}

type faultExitClaimRepo struct {
	gvg.ExitClaimRepository
	failMarkDone atomic.Bool
	markDoneErr  error
}

func (r *faultExitClaimRepo) MarkExitDeliveryDone(ctx context.Context, roomID, characterID, joinID string) error {
	if r.failMarkDone.CompareAndSwap(true, false) {
		return r.markDoneErr
	}
	return r.ExitClaimRepository.MarkExitDeliveryDone(ctx, roomID, characterID, joinID)
}

func TestGvG_LeaveRoom_AtomicDeliveryClaimAndRetry_Integration(t *testing.T) {
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
	roomRepo := &faultIntegrationRoomRepo{
		RoomRepository:    baseRooms,
		deleteCharRoomErr: errors.New("valkey delete character room failure"),
	}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	claimRepo := &faultExitClaimRepo{
		ExitClaimRepository: gvgRepo,
		markDoneErr:         errors.New("mark exit delivery done failure"),
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

	g1, c1, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGAtomic1_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g2, c2, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGAtomic2_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	g3, c3, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGAtomic3_%s", id.New()[:8]), 500)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, gid := range []string{g1.ID, g2.ID, g3.ID} {
			if _, err := db.ExecContext(cleanupCtx, "DELETE FROM guilds WHERE id = ?", gid); err != nil {
				t.Error(err)
			}
		}
		for _, ch := range []struct{ id, playerID string }{
			{c1.ID, c1.PlayerID},
			{c2.ID, c2.PlayerID},
			{c3.ID, c3.PlayerID},
		} {
			if err := charRepo.Delete(cleanupCtx, ch.id); err != nil {
				t.Error(err)
			}
			if _, err := db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", ch.playerID); err != nil {
				t.Error(err)
			}
		}
	})
	for _, gid := range []string{g1.ID, g3.ID} {
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", gid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#33CCFF' WHERE id = ?", g2.ID); err != nil {
		t.Fatal(err)
	}
	for _, cid := range []string{c1.ID, c2.ID, c3.ID} {
		if _, err := db.ExecContext(ctx, "UPDATE characters SET max_hp = 100, hp = 100 WHERE id = ?", cid); err != nil {
			t.Fatal(err)
		}
	}

	// c1 has standing delivery (20G, +40 HP) and fullness, with enough gold for multiple charges.
	if _, err := tavernSvc.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	c1Before, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	c1StatusBefore, err := tavernRepo.GetCharacterStatus(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}

	// c2 has an unaffordable standing delivery (400G meal, wallet 10G) and fullness.
	if _, err := tavernSvc.OrderMeal(ctx, c2.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c2.ID, "tavern_curry"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 10 WHERE id = ?", c2.ID); err != nil {
		t.Fatal(err)
	}
	c2StatusBefore, err := tavernRepo.GetCharacterStatus(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// c3 has fullness and NO delivery reservation.
	if _, err := tavernSvc.OrderMeal(ctx, c3.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{c1.ID: 20, c2.ID: 0, c3.ID: 0},
				RemainingMP: map[string]int{c1.ID: 4, c2.ID: 5, c3.ID: 6},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{c1.ID: 15, c2.ID: 0},
				RemainingMP: map[string]int{c1.ID: 3, c2.ID: 5},
			},
		},
	}

	svc, err := gvg.NewService(
		roomRepo, gvgRepo, guildRepo, charRepo, engine,
		gvg.WithParticipantBuilder(battleSvc),
		gvg.WithPostBattleSettler(battleSvc),
		gvg.WithTransactionProvider(txProv),
		gvg.WithExitClaimRepository(claimRepo),
		gvg.WithPostExitHook(func(ctx context.Context, characterID string) (*gvg.DeliveryOutcome, error) {
			if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
				return nil, err
			}
			res, err := tavernSvc.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) {
				return nil, nil
			}
			if unpaid, ok := errors.AsType[tavern.UnaffordableDeliveryError](err); ok {
				return &gvg.DeliveryOutcome{ItemName: unpaid.ItemName}, nil
			}
			return &gvg.DeliveryOutcome{ItemName: res.Item.Name, Paid: true}, err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "AtomicDelivery", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if err := baseRooms.DeleteRoom(cleanupCtx, detail.Room.ID); err != nil {
			t.Error(err)
		}
		for _, cid := range []string{c1.ID, c2.ID, c3.ID} {
			if err := baseRooms.DeleteCharacterRoom(cleanupCtx, cid, detail.Room.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := db.ExecContext(cleanupCtx, "DELETE FROM gvg_match_transitions WHERE room_id = ?", detail.Room.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(cleanupCtx, "DELETE FROM gvg_exit_claims WHERE room_id = ?", detail.Room.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, c3.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 1); err != nil {
		t.Fatal(err)
	}

	// 1. Fail MarkExitDeliveryDone for c1 -> whole SQL exit + delivery must roll back.
	claimRepo.failMarkDone.Store(true)
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); !errors.Is(err, claimRepo.markDoneErr) {
		t.Fatalf("c1 LeaveRoom expected MarkExitDeliveryDone error, got %v", err)
	}
	c1AfterFail, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c1AfterFail.Money != c1Before.Money || c1AfterFail.Stats.HP != c1Before.Stats.HP || c1AfterFail.Tired != c1Before.Tired {
		t.Fatalf("c1 character mutated on MarkExitDeliveryDone failure: got=%+v before=%+v", c1AfterFail, c1Before)
	}
	c1StatusAfterFail, err := tavernRepo.GetCharacterStatus(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !c1StatusAfterFail.IsFull || c1StatusAfterFail.TotalMealsEaten != c1StatusBefore.TotalMealsEaten || c1StatusAfterFail.TotalGoldSpent != c1StatusBefore.TotalGoldSpent {
		t.Fatalf("c1 tavern status mutated on MarkExitDeliveryDone failure: got=%+v before=%+v", c1StatusAfterFail, c1StatusBefore)
	}
	var claimCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gvg_exit_claims WHERE room_id = ? AND character_id = ?`, detail.Room.ID, c1.ID).Scan(&claimCount); err != nil {
		t.Fatal(err)
	}
	if claimCount != 0 {
		t.Fatalf("expected 0 exit claims after rollback, got %d", claimCount)
	}

	// Retry c1 with Valkey DeleteCharacterRoom failure, then final retry -> single charge, heal, and meal increment.
	roomRepo.failDeleteChar.Store(true)
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); !errors.Is(err, roomRepo.deleteCharRoomErr) {
		t.Fatalf("c1 LeaveRoom expected Valkey failure, got %v", err)
	}
	roomRepo.failDeleteChar.Store(false)
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("c1 LeaveRoom retry failed: %v", err)
	}
	c1AfterRetry, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantC1HP := 20 + 40
	if c1AfterRetry.Money != c1Before.Money-20 || c1AfterRetry.Stats.HP != wantC1HP || c1AfterRetry.Stats.MP != 4 || c1AfterRetry.Tired != 4 {
		t.Fatalf("c1 state after retry = %+v (want Money=%d HP=%d MP=4 Tired=4)", c1AfterRetry, c1Before.Money-20, wantC1HP)
	}
	c1StatusAfterRetry, err := tavernRepo.GetCharacterStatus(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c1StatusAfterRetry.IsFull || c1StatusAfterRetry.TotalMealsEaten != c1StatusBefore.TotalMealsEaten+1 || c1StatusAfterRetry.TotalGoldSpent != c1StatusBefore.TotalGoldSpent+20 {
		t.Fatalf("c1 tavern status after retry = %+v (before=%+v)", c1StatusAfterRetry, c1StatusBefore)
	}
	var c1DeliveryDone bool
	if err := db.QueryRowContext(ctx, `SELECT delivery_done FROM gvg_exit_claims WHERE room_id = ? AND character_id = ? AND join_id = ?`, detail.Room.ID, c1.ID, detail.Members[0].JoinID).Scan(&c1DeliveryDone); err != nil {
		t.Fatal(err)
	}
	if !c1DeliveryDone {
		t.Fatal("expected c1 delivery_done = true")
	}

	// 2. Unaffordable delivery for c2 + Valkey failure -> marks delivery_done=true, so funding before retry does NOT charge.
	roomRepo.failDeleteChar.Store(true)
	if _, err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); !errors.Is(err, roomRepo.deleteCharRoomErr) {
		t.Fatalf("c2 LeaveRoom expected Valkey failure, got %v", err)
	}
	roomRepo.failDeleteChar.Store(false)
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 1000 WHERE id = ?", c2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatalf("c2 LeaveRoom retry failed: %v", err)
	}
	c2After, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c2After.Money != 1000 || c2After.Stats.HP != 0 || c2After.Stats.MP != 5 || c2After.Tired != 4 {
		t.Fatalf("c2 state after unaffordable retry = %+v", c2After)
	}
	c2StatusAfter, err := tavernRepo.GetCharacterStatus(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c2StatusAfter.IsFull || c2StatusAfter.TotalMealsEaten != c2StatusBefore.TotalMealsEaten || c2StatusAfter.TotalGoldSpent != c2StatusBefore.TotalGoldSpent {
		t.Fatalf("c2 tavern status changed on unaffordable order: %+v", c2StatusAfter)
	}
	if _, err := tavernSvc.GetDelivery(ctx, c2.ID); err != nil {
		t.Fatalf("expected c2 standing delivery preserved, got %v", err)
	}

	// 3. No active delivery for c3 + Valkey failure -> marks delivery_done=true and resets fullness.
	roomRepo.failDeleteChar.Store(true)
	if _, err := svc.LeaveRoom(ctx, c3.ID, detail.Room.ID); !errors.Is(err, roomRepo.deleteCharRoomErr) {
		t.Fatalf("c3 LeaveRoom expected Valkey failure, got %v", err)
	}
	roomRepo.failDeleteChar.Store(false)
	if _, err := svc.LeaveRoom(ctx, c3.ID, detail.Room.ID); err != nil {
		t.Fatalf("c3 LeaveRoom retry failed: %v", err)
	}
	c3StatusAfter, err := tavernRepo.GetCharacterStatus(ctx, c3.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c3StatusAfter.IsFull {
		t.Fatalf("expected c3 fullness reset: %+v", c3StatusAfter)
	}

	// 4. Unconscious 0 HP survivor cannot re-enter until recovered; once healed, rejoin and departure settles standing order again.
	if _, err := svc.CreateRoom(ctx, c2.ID, gvg.CreateRoomRequest{Name: "AtomicRejoin", MaxMembers: 2, TargetWins: 1}); !errors.Is(err, gvg.ErrCharacterUnconscious) {
		t.Fatalf("expected ErrCharacterUnconscious for 0 HP survivor c2, got %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET hp = 100 WHERE id = ?", c2.ID); err != nil {
		t.Fatal(err)
	}
	room2, err := svc.CreateRoom(ctx, c2.ID, gvg.CreateRoomRequest{Name: "AtomicRejoin", MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if err := baseRooms.DeleteRoom(cleanupCtx, room2.Room.ID); err != nil {
			t.Error(err)
		}
		for _, cid := range []string{c1.ID, c2.ID} {
			if err := baseRooms.DeleteCharacterRoom(cleanupCtx, cid, room2.Room.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := db.ExecContext(cleanupCtx, "DELETE FROM gvg_match_transitions WHERE room_id = ?", room2.Room.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(cleanupCtx, "DELETE FROM gvg_exit_claims WHERE room_id = ?", room2.Room.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := svc.JoinRoom(ctx, c1.ID, room2.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c2.ID, room2.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c2.ID, room2.Room.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, room2.Room.ID); err != nil {
		t.Fatalf("c1 second room LeaveRoom failed: %v", err)
	}
	c1AfterSecond, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c1AfterSecond.Money != c1Before.Money-40 {
		t.Fatalf("expected standing order to settle once per room departure: Money=%d want=%d", c1AfterSecond.Money, c1Before.Money-40)
	}
}

func TestGvG_LeaveRoom_RecoverCommittedRoundBeforeDeparture_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}

	type scenario struct {
		name              string
		targetWins        int
		departLeader      bool
		retryFailedSave   bool
		round1RemainingHP map[string]int
		round1RemainingMP map[string]int
		round1Consumed    map[string][]corebattle.ConsumedItem
		wantDepartedHP    int
		wantDepartedMP    int
		wantDepartedTired int
		wantDepartedEXP   int64
	}

	type runOutcome struct {
		room             gvg.RoomDetail
		departedChar     corecharacter.Character
		departedInvCount int
		departedEquipped bool
		departedClaimed  bool
		departedDone     bool
		departedMapped   bool
		remainingMapped  string
		replayResult     gvg.RoundResolution
		winnerStanding   gvg.GvGStanding
		loserStanding    gvg.GvGStanding
		winnerGuildGP    int64
		loserGuildGP     int64
		tavernMeals      int
		tavernSpent      int64
		tavernIsFull     bool
	}

	runScenario := func(t *testing.T, sc scenario, injectPublishFailure bool) runOutcome {
		t.Helper()
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
		roomRepo := &faultIntegrationRoomRepo{
			RoomRepository: baseRooms,
			saveErr:        errors.New("valkey save failure"),
		}
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

		g1, c1, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGCR1_%s", id.New()[:8]), 500)
		if err != nil {
			t.Fatal(err)
		}
		g2, c2, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("GvGCR2_%s", id.New()[:8]), 500)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx := context.Background()
			for _, gid := range []string{g1.ID, g2.ID} {
				_, _ = db.ExecContext(cleanupCtx, "DELETE FROM guilds WHERE id = ?", gid)
			}
			for _, ch := range []struct{ id, playerID string }{{c1.ID, c1.PlayerID}, {c2.ID, c2.PlayerID}} {
				_ = charRepo.Delete(cleanupCtx, ch.id)
				_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", ch.playerID)
			}
		})
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#FF3333' WHERE id = ?", g1.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET color = '#33CCFF' WHERE id = ?", g2.ID); err != nil {
			t.Fatal(err)
		}
		// Start both members below MaxHP and with distinct MP so round boundary changes are observable.
		if _, err := db.ExecContext(ctx, "UPDATE characters SET max_hp = 100, hp = 80, max_mp = 40, mp = 30, experience = 0 WHERE id IN (?, ?)", c1.ID, c2.ID); err != nil {
			t.Fatal(err)
		}

		departID, stayID := c2.ID, c1.ID
		if sc.departLeader {
			departID, stayID = c1.ID, c2.ID
		}

		// Departing member equips item-193 (consumed in round 1) and reserves a tavern meal/delivery.
		amulet, err := coreitem.NewInstance("item-193", 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestInventoryWithItems(ctx, db, departID, []coreitem.Instance{amulet}); err != nil {
			t.Fatal(err)
		}
		if err := equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: departID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: amulet.ID}}); err != nil {
			t.Fatal(err)
		}
		if _, err := tavernSvc.OrderMeal(ctx, departID, "tavern_water"); err != nil {
			t.Fatal(err)
		}
		if _, err := tavernSvc.ReserveDelivery(ctx, departID, "tavern_water"); err != nil {
			t.Fatal(err)
		}

		remHP := map[string]int{c1.ID: sc.round1RemainingHP["c1"], c2.ID: sc.round1RemainingHP["c2"]}
		remMP := map[string]int{c1.ID: sc.round1RemainingMP["c1"], c2.ID: sc.round1RemainingMP["c2"]}
		consumed := make(map[string][]corebattle.ConsumedItem)
		if items, ok := sc.round1Consumed["c1"]; ok {
			consumed[c1.ID] = items
		}
		if items, ok := sc.round1Consumed["c2"]; ok {
			consumed[c2.ID] = items
		}
		engine := &scriptedMPBattleEngine{
			results: []corebattle.PartyBattleResult{
				{
					Outcome:       corebattle.OutcomeWin,
					RemainingHP:   remHP,
					RemainingMP:   remMP,
					ConsumedItems: consumed,
				},
			},
		}

		svc, err := gvg.NewService(
			roomRepo, gvgRepo, guildRepo, charRepo, engine,
			gvg.WithParticipantBuilder(battleSvc),
			gvg.WithPostBattleSettler(battleSvc),
			gvg.WithTransactionProvider(txProv),
			gvg.WithExitClaimRepository(gvgRepo),
			gvg.WithPostExitHook(func(ctx context.Context, characterID string) (*gvg.DeliveryOutcome, error) {
				if err := tavernSvc.ResetFullness(ctx, characterID); err != nil {
					return nil, err
				}
				res, err := tavernSvc.ClaimDelivery(ctx, characterID)
				if errors.Is(err, tavern.ErrNoActiveDelivery) {
					return nil, nil
				}
				return &gvg.DeliveryOutcome{ItemName: res.Item.Name, Paid: true}, err
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{
			Name:       "RoundRecovery_" + sc.name,
			MaxMembers: 2,
			TargetWins: sc.targetWins,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_ = baseRooms.DeleteRoom(cleanupCtx, detail.Room.ID)
			_ = baseRooms.DeleteCharacterRoom(cleanupCtx, c1.ID, detail.Room.ID)
			_ = baseRooms.DeleteCharacterRoom(cleanupCtx, c2.ID, detail.Room.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM gvg_match_transitions WHERE room_id = ?", detail.Room.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM gvg_exit_claims WHERE room_id = ?", detail.Room.ID)
		})
		if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID); err != nil {
			t.Fatal(err)
		}

		if injectPublishFailure {
			roomRepo.failSave.Store(true)
			if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 1); !errors.Is(err, roomRepo.saveErr) {
				t.Fatalf("expected SaveRoom failure on AdvanceRound, got %v", err)
			}
			if sc.retryFailedSave {
				beforeDepart, err := charRepo.FindByID(ctx, departID)
				if err != nil {
					t.Fatal(err)
				}
				beforeTavern, err := tavernRepo.GetCharacterStatus(ctx, departID)
				if err != nil {
					t.Fatal(err)
				}
				// Round publication fails again -> LeaveRoom must abort before exit claim, copy-back, fatigue or delivery.
				if _, err := svc.LeaveRoom(ctx, departID, detail.Room.ID); !errors.Is(err, roomRepo.saveErr) {
					t.Fatalf("LeaveRoom with failing SaveRoom = %v, want %v", err, roomRepo.saveErr)
				}
				var claimCount int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gvg_exit_claims WHERE room_id = ? AND character_id = ?", detail.Room.ID, departID).Scan(&claimCount); err != nil {
					t.Fatal(err)
				}
				if claimCount != 0 {
					t.Fatalf("expected 0 exit claims on failed publication recovery, got %d", claimCount)
				}
				afterAbort, err := charRepo.FindByID(ctx, departID)
				if err != nil {
					t.Fatal(err)
				}
				if afterAbort.Stats.HP != beforeDepart.Stats.HP || afterAbort.Stats.MP != beforeDepart.Stats.MP || afterAbort.Tired != beforeDepart.Tired || afterAbort.Money != beforeDepart.Money {
					t.Fatalf("character mutated on failed publication recovery: before=%+v after=%+v", beforeDepart, afterAbort)
				}
				afterTavern, err := tavernRepo.GetCharacterStatus(ctx, departID)
				if err != nil {
					t.Fatal(err)
				}
				if afterTavern.IsFull != beforeTavern.IsFull || afterTavern.TotalMealsEaten != beforeTavern.TotalMealsEaten || afterTavern.TotalGoldSpent != beforeTavern.TotalGoldSpent {
					t.Fatalf("tavern status mutated on failed publication recovery: before=%+v after=%+v", beforeTavern, afterTavern)
				}
				invAfterAbort, err := invRepo.FindByCharacterID(ctx, departID)
				if err != nil {
					t.Fatal(err)
				}
				if len(invAfterAbort.Items) != 1 {
					t.Fatalf("inventory mutated on failed publication recovery: %+v", invAfterAbort.Items)
				}
			}
			roomRepo.failSave.Store(false)
		} else {
			if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 1); err != nil {
				t.Fatalf("healthy AdvanceRound failed: %v", err)
			}
		}

		if _, err := svc.LeaveRoom(ctx, departID, detail.Room.ID); err != nil {
			t.Fatalf("LeaveRoom(%s) failed: %v", departID, err)
		}

		if sc.departLeader {
			// Old departed leader cannot replay AdvanceRound.
			if _, err := svc.AdvanceRound(ctx, departID, detail.Room.ID, 1); !errors.Is(err, gvg.ErrNotRoomLeader) {
				t.Fatalf("departed leader replay = %v, want ErrNotRoomLeader", err)
			}
		}
		replayRes, err := svc.AdvanceRound(ctx, stayID, detail.Room.ID, 1)
		if err != nil {
			t.Fatalf("staying leader AdvanceRound(1) replay failed: %v", err)
		}
		if engine.calls.Load() != 1 {
			t.Fatalf("battle engine calls = %d, want 1", engine.calls.Load())
		}

		roomAfter, err := svc.GetRoom(ctx, detail.Room.ID)
		if err != nil {
			t.Fatalf("GetRoom after departure+replay failed: %v", err)
		}
		departedChar, err := charRepo.FindByID(ctx, departID)
		if err != nil {
			t.Fatal(err)
		}
		departedInv, err := invRepo.FindByCharacterID(ctx, departID)
		if err != nil {
			t.Fatal(err)
		}
		departedEquip, err := equipRepo.FindByCharacterID(ctx, departID)
		if err != nil {
			t.Fatal(err)
		}
		_, equippedAcc := departedEquip.Equipped(coreitem.SlotAccessory)
		var deliveryDone bool
		claimErr := db.QueryRowContext(ctx, "SELECT delivery_done FROM gvg_exit_claims WHERE room_id = ? AND character_id = ?", detail.Room.ID, departID).Scan(&deliveryDone)
		if claimErr != nil && !errors.Is(claimErr, sql.ErrNoRows) {
			t.Fatal(claimErr)
		}
		_, mapErr := baseRooms.GetCharacterRoom(ctx, departID)
		stayMapped, err := baseRooms.GetCharacterRoom(ctx, stayID)
		if err != nil {
			t.Fatalf("staying member mapping missing: %v", err)
		}
		stayMappedLabel := "other"
		if stayMapped == detail.Room.ID {
			stayMappedLabel = "current_room"
		}
		st1, err := gvgRepo.GetOrCreateStanding(ctx, g1.ID)
		if err != nil {
			t.Fatal(err)
		}
		st2, err := gvgRepo.GetOrCreateStanding(ctx, g2.ID)
		if err != nil {
			t.Fatal(err)
		}
		g1After, _, err := guildRepo.GetGuild(ctx, g1.ID)
		if err != nil {
			t.Fatal(err)
		}
		g2After, _, err := guildRepo.GetGuild(ctx, g2.ID)
		if err != nil {
			t.Fatal(err)
		}
		tavernAfter, err := tavernRepo.GetCharacterStatus(ctx, departID)
		if err != nil {
			t.Fatal(err)
		}

		return runOutcome{
			room:             roomAfter,
			departedChar:     departedChar,
			departedInvCount: len(departedInv.Items),
			departedEquipped: equippedAcc,
			departedClaimed:  claimErr == nil,
			departedDone:     deliveryDone,
			departedMapped:   !errors.Is(mapErr, gvg.ErrRoomNotFound),
			remainingMapped:  stayMappedLabel,
			replayResult:     replayRes,
			winnerStanding:   st1,
			loserStanding:    st2,
			winnerGuildGP:    g1After.Points,
			loserGuildGP:     g2After.Points,
			tavernMeals:      tavernAfter.TotalMealsEaten,
			tavernSpent:      tavernAfter.TotalGoldSpent,
			tavernIsFull:     tavernAfter.IsFull,
		}
	}

	cases := []scenario{
		{
			name:              "non_final_nonleader_departure",
			targetWins:        2,
			departLeader:      false,
			retryFailedSave:   true,
			round1RemainingHP: map[string]int{"c1": 55, "c2": 35},
			round1RemainingMP: map[string]int{"c1": 12, "c2": 7},
			round1Consumed:    map[string][]corebattle.ConsumedItem{"c2": {{ID: "item-193", Quantity: 1}}},
			// Delivery (+40 HP) applied to round 2 restored HP (100), clamped to MaxHP 100.
			wantDepartedHP:    100,
			wantDepartedMP:    7,
			wantDepartedTired: 7,
		},
		{
			name:              "non_final_leader_departure",
			targetWins:        2,
			departLeader:      true,
			retryFailedSave:   false,
			round1RemainingHP: map[string]int{"c1": 45, "c2": 60},
			round1RemainingMP: map[string]int{"c1": 9, "c2": 14},
			round1Consumed:    map[string][]corebattle.ConsumedItem{"c1": {{ID: "item-193", Quantity: 1}}},
			wantDepartedHP:    100,
			wantDepartedMP:    9,
			wantDepartedTired: 7,
		},
		{
			name:              "final_round_nonleader_departure",
			targetWins:        1,
			departLeader:      false,
			retryFailedSave:   true,
			round1RemainingHP: map[string]int{"c1": 0, "c2": 22},
			round1RemainingMP: map[string]int{"c1": 11, "c2": 5},
			round1Consumed:    map[string][]corebattle.ConsumedItem{"c2": {{ID: "item-193", Quantity: 1}}},
			// Final round preserves RemainingHP (22) + delivery (+40 HP) = 62, MP = 5, Tired = 4.
			wantDepartedHP:    62,
			wantDepartedMP:    5,
			wantDepartedTired: 4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			control := runScenario(t, tc, false)
			recovered := runScenario(t, tc, true)

			if len(recovered.room.Members) != 1 || len(control.room.Members) != 1 {
				t.Fatalf("member count mismatch: recovered=%d control=%d", len(recovered.room.Members), len(control.room.Members))
			}
			if recovered.room.Room.Round != control.room.Room.Round ||
				recovered.room.Room.Status != control.room.Room.Status ||
				recovered.room.Room.LeaderCharacterID == "" ||
				(recovered.room.Room.LeaderCharacterID == recovered.departedChar.ID) ||
				!recovered.room.Members[0].IsLeader ||
				recovered.room.Members[0].HP != control.room.Members[0].HP ||
				!reflect.DeepEqual(recovered.room.Members[0].MP, control.room.Members[0].MP) {
				t.Fatalf("persisted room mismatch:\nrecovered=%+v\ncontrol=%+v", recovered.room, control.room)
			}
			if recovered.departedMapped || recovered.remainingMapped != "current_room" {
				t.Fatalf("reverse mapping mismatch: departedMapped=%v remainingMapped=%q", recovered.departedMapped, recovered.remainingMapped)
			}
			if recovered.departedChar.Stats.HP != tc.wantDepartedHP ||
				recovered.departedChar.Stats.MP != tc.wantDepartedMP ||
				recovered.departedChar.Tired != tc.wantDepartedTired ||
				recovered.departedChar.Money != control.departedChar.Money ||
				recovered.departedInvCount != 0 ||
				recovered.departedEquipped ||
				!recovered.departedClaimed ||
				!recovered.departedDone ||
				recovered.tavernIsFull ||
				recovered.tavernMeals != control.tavernMeals ||
				recovered.tavernSpent != control.tavernSpent {
				t.Fatalf("departed character/claim/tavern mismatch:\nrecoveredChar=%+v inv=%d equipped=%v claimed=%v done=%v meals=%d spent=%d\ncontrolChar=%+v",
					recovered.departedChar, recovered.departedInvCount, recovered.departedEquipped, recovered.departedClaimed, recovered.departedDone, recovered.tavernMeals, recovered.tavernSpent, control.departedChar)
			}
			if recovered.replayResult.Round != control.replayResult.Round ||
				recovered.replayResult.Outcome != control.replayResult.Outcome ||
				recovered.replayResult.MatchCompleted != control.replayResult.MatchCompleted ||
				recovered.replayResult.MatchCompleted != (tc.targetWins == 1) ||
				recovered.replayResult.PrizeGP != control.replayResult.PrizeGP {
				t.Fatalf("replay result mismatch:\nrecovered=%+v\ncontrol=%+v", recovered.replayResult, control.replayResult)
			}
			if recovered.winnerGuildGP != control.winnerGuildGP ||
				recovered.loserGuildGP != control.loserGuildGP ||
				recovered.winnerStanding.VictoryPoints != control.winnerStanding.VictoryPoints ||
				recovered.winnerStanding.Wins != control.winnerStanding.Wins ||
				recovered.winnerStanding.BronzeMedals != control.winnerStanding.BronzeMedals ||
				recovered.loserStanding.VictoryPoints != control.loserStanding.VictoryPoints ||
				recovered.loserStanding.Losses != control.loserStanding.Losses {
				t.Fatalf("guild/standing mismatch: recovered=(%d,%d,%+v,%+v) control=(%d,%d,%+v,%+v)",
					recovered.winnerGuildGP, recovered.loserGuildGP, recovered.winnerStanding, recovered.loserStanding,
					control.winnerGuildGP, control.loserGuildGP, control.winnerStanding, control.loserStanding)
			}
		})
	}
}
