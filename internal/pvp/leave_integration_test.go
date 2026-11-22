package pvp_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/tavern"
)

type fixedFloatRNG struct{ val float64 }

func (r fixedFloatRNG) Intn(int) (int, error) { return 0, nil }
func (r fixedFloatRNG) Float64() float64      { return r.val }

type scriptedIntegrationEngine struct {
	fn func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

func (e scriptedIntegrationEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return e.fn(req)
}

func TestPvPLeaveExitSideEffectsIntegration(t *testing.T) {
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

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	econSvc, err := economy.NewService(chars, invs, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	tavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernSvc, err := tavern.NewService(tavernCatalog, tavernRepo, chars, txProvider)
	if err != nil {
		t.Fatal(err)
	}
	battleSvc := battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips),
		battle.WithDepotRepository(depots),
		battle.WithTransactionProvider(txProvider),
		battle.WithTransactionRunner(econSvc),
		battle.WithRandomSource(fixedFloatRNG{val: 0.5}),
	)

	store, err := database.NewPvPTransitionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	baseRooms, err := pvp.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	rooms := &transitionRooms{RoomRepository: baseRooms}

	c1, err := database.CreateTestCharacterWithFunds(ctx, db, "P"+id.New()[:7], 1000)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacterWithFunds(ctx, db, "M"+id.New()[:7], 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, c := range []corecharacter.Character{c1, c2} {
			if rid, err := store.FindCharacterClaim(cleanupCtx, c.ID); err == nil && rid != "" {
				_ = baseRooms.DeleteRoom(cleanupCtx, rid)
				_ = baseRooms.DeleteCharacterRoom(cleanupCtx, c.ID, rid)
			}
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", c.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", c.PlayerID)
		}
	})

	// c1 equips item-105 (幸せのくつ), c2 equips item-106 (金の鶏) and consumes a held tool.
	shoes, err := coreitem.NewInstance("item-105", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := invs.Save(ctx, coreinventory.Inventory{CharacterID: c1.ID, Items: []coreitem.Instance{shoes}}); err != nil {
		t.Fatal(err)
	}
	if err := equips.Save(ctx, coreequipment.Equipment{CharacterID: c1.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: shoes.ID}}); err != nil {
		t.Fatal(err)
	}

	rooster, err := coreitem.NewInstance("item-106", 1)
	if err != nil {
		t.Fatal(err)
	}
	herb, err := coreitem.NewInstance("item-101", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := invs.Save(ctx, coreinventory.Inventory{CharacterID: c2.ID, Items: []coreitem.Instance{rooster, herb}}); err != nil {
		t.Fatal(err)
	}
	if err := equips.Save(ctx, coreequipment.Equipment{CharacterID: c2.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: rooster.ID}}); err != nil {
		t.Fatal(err)
	}

	// c1 orders meal + reserves delivery (tavern_water: 20G, +40 HP).
	if _, err := tavernSvc.OrderMeal(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}
	if _, err := tavernSvc.ReserveDelivery(ctx, c1.ID, "tavern_water"); err != nil {
		t.Fatal(err)
	}

	postExitHook := func(hookCtx context.Context, characterID string) (string, error) {
		if err := tavernSvc.ResetFullness(hookCtx, characterID); err != nil {
			return "", err
		}
		res, err := tavernSvc.ClaimDelivery(hookCtx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) {
			return "", nil
		}
		if _, ok := errors.AsType[tavern.UnaffordableDeliveryError](err); ok {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return res.Item.Name, nil
	}

	engine := scriptedIntegrationEngine{fn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		return corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{c1.ID: 0, c2.ID: 35},
			RemainingMP: map[string]int{c1.ID: 3, c2.ID: 4},
			ConsumedItems: map[string][]corebattle.ConsumedItem{
				c2.ID: {{ID: "item-101", InstanceID: herb.ID, Quantity: 1}},
			},
		}, nil
	}}

	svc, err := pvp.NewService(
		rooms, chars, engine,
		pvp.WithParticipantBuilder(battleSvc),
		pvp.WithPostBattleSettler(battleSvc),
		pvp.WithPostExitHook(postExitHook),
		pvp.WithTransactionProvider(txProvider),
		pvp.WithTransitionRepository(store),
		pvp.WithPrizeRemitter(pvpPrizeStub{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	room, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "ExitTail", Bet: 100, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM pvp_room_transitions WHERE room_id = ?", room.Room.ID)
	})
	if _, err := svc.JoinRoom(ctx, c2.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c1.ID, room.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c2.ID, room.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c1.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c1.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}

	// Inject Valkey failure on c1's LeaveRoom, then retry: side-effects must apply exactly once.
	c1Before, _ := chars.FindByID(ctx, c1.ID)
	rooms.fail = "delete-member"
	rooms.after = false
	rooms.armed.Store(true)
	if err := svc.LeaveRoom(ctx, c1.ID, room.Room.ID); !errors.Is(err, errPublication) {
		t.Fatalf("expected publication failure, got %v", err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, room.Room.ID); err != nil {
		t.Fatalf("retry LeaveRoom(c1): %v", err)
	}

	gotC1, err := chars.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	// c1 had 0 HP in final round -> 1 HP survival floor + 40 HP delivery = 41 HP (clamped to MaxHP),
	// Tired = 4 (1*3+1), EXP = +20 (item-105), Money = -20 (tavern_water).
	wantHP := min(gotC1.Stats.MaxHP, 1+40)
	if gotC1.Stats.HP != wantHP || gotC1.Stats.MP != 3 || gotC1.Tired != 4 || gotC1.Experience != c1Before.Experience+20 || gotC1.Money != c1Before.Money-20 {
		t.Fatalf("c1 after exit: HP=%d(want %d) MP=%d Tired=%d EXP=%d(before %d) Money=%d(before %d)",
			gotC1.Stats.HP, wantHP, gotC1.Stats.MP, gotC1.Tired, gotC1.Experience, c1Before.Experience, gotC1.Money, c1Before.Money)
	}
	tStatus, err := tavernSvc.GetStatus(ctx, c1.ID)
	if err != nil || tStatus.IsFull || tStatus.DeliveryReservation == nil {
		t.Fatalf("unexpected tavern status after c1 exit: %+v err=%v", tStatus, err)
	}

	// c2 leaves: item-106 gold bonus int(1 * 0.5 * 77) = 38, herb consumed, Tired = 4, HP = 35, MP = 4.
	c2Before, _ := chars.FindByID(ctx, c2.ID)
	if err := svc.LeaveRoom(ctx, c2.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom(c2): %v", err)
	}
	gotC2, err := chars.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantC2HP := min(c2Before.Stats.MaxHP, 35)
	if gotC2.Stats.HP != wantC2HP || gotC2.Stats.MP != 4 || gotC2.Tired != 4 || gotC2.Money != c2Before.Money+38 {
		t.Fatalf("c2 after exit: HP=%d(want %d) MP=%d Tired=%d Money=%d(before %d)", gotC2.Stats.HP, wantC2HP, gotC2.Stats.MP, gotC2.Tired, gotC2.Money, c2Before.Money)
	}
	gotInv2, err := invs.FindByCharacterID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := gotInv2.Find(herb.ID); found {
		t.Fatalf("expected consumed herb %s cleared from inventory", herb.ID)
	}
}

func TestPvPKickIntegration(t *testing.T) {
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

	chars, _ := database.NewCharacterRepository(db)
	store, _ := database.NewPvPTransitionRepository(db)
	baseRooms, _ := pvp.NewValkeyRoomRepository(client)
	rooms := &transitionRooms{RoomRepository: baseRooms}

	now := time.Unix(1_700_000_000, 0).UTC()
	waits := &recordingActionWait{err: errors.New("injected wait failure")}
	svc, err := pvp.NewService(
		rooms, chars, mockBattleEngine{},
		pvp.WithTransitionRepository(store),
		pvp.WithActionWait(waits),
		pvp.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := database.CreateTestCharacterWithFunds(ctx, db, "K"+id.New()[:7], 500)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacterWithFunds(ctx, db, "T"+id.New()[:7], 500)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, c := range []corecharacter.Character{c1, c2} {
			if rid, err := store.FindCharacterClaim(cleanupCtx, c.ID); err == nil && rid != "" {
				_ = baseRooms.DeleteRoom(cleanupCtx, rid)
				_ = baseRooms.DeleteCharacterRoom(cleanupCtx, c.ID, rid)
			}
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", c.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", c.PlayerID)
		}
	})

	room, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "KickTest", Bet: 100})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM pvp_room_transitions WHERE room_id = ?", room.Room.ID)
	})
	if _, err := svc.JoinRoom(ctx, c2.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	if err := svc.KickMember(ctx, room.Room.ID, c1.ID, c2.ID); err == nil {
		t.Fatal("expected KickMember error when wait write fails")
	}
	if claim, err := svc.GetCharacterClaim(ctx, c2.ID); err != nil || claim != room.Room.ID {
		t.Fatalf("claim must remain reserved until wait succeeds: %q %v", claim, err)
	}

	waits.err = nil
	if err := svc.RecoverCharacterDeparture(ctx, c2.ID); err != nil {
		t.Fatalf("RecoverCharacterDeparture: %v", err)
	}
	if !waits.waits[c2.ID].Equal(now.Add(pvp.KickActionWait)) {
		t.Fatalf("wait = %v, want %v", waits.waits[c2.ID], now.Add(pvp.KickActionWait))
	}
	if claim, err := svc.GetCharacterClaim(ctx, c2.ID); err != nil || claim != "" {
		t.Fatalf("expected claim released after recovery, got %q err=%v", claim, err)
	}
	gotC2, _ := chars.FindByID(ctx, c2.ID)
	if gotC2.Money != 400 || gotC2.Tired != 0 {
		t.Fatalf("kicked c2 money=%d tired=%d, want 400/0", gotC2.Money, gotC2.Tired)
	}
	detail, err := svc.GetRoom(ctx, room.Room.ID)
	if err != nil || detail.Room.PrizePool != 200 || len(detail.Members) != 1 {
		t.Fatalf("unexpected room after kick: %+v err=%v", detail, err)
	}
}

type pvpPrizeStub struct{}

func (pvpPrizeStub) HoldPrize(context.Context, string, string, int) error { return nil }
