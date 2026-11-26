package party_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/tavern"
	"github.com/witchcraze/party2re/internal/testutil"
)

type testTavernExitAdapter struct {
	svc *tavern.Service
}

func (a testTavernExitAdapter) ResetFullness(ctx context.Context, characterID string) error {
	return a.svc.ResetFullness(ctx, characterID)
}

func (a testTavernExitAdapter) SettleDelivery(ctx context.Context, characterID string) (*party.DeliveryOutcome, error) {
	res, err := a.svc.ClaimDelivery(ctx, characterID)
	if errors.Is(err, tavern.ErrNoActiveDelivery) {
		return nil, nil
	}
	if unpaid, ok := errors.AsType[tavern.UnaffordableDeliveryError](err); ok {
		return &party.DeliveryOutcome{ItemName: unpaid.ItemName, Paid: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &party.DeliveryOutcome{ItemName: res.Item.Name, Paid: true}, nil
}

type failingUpdateCharRepo struct {
	party.CharacterRepository
	err error
}

func (r failingUpdateCharRepo) Update(context.Context, corecharacter.Character) error {
	return r.err
}

type failingRemovePartyRepo struct {
	party.Repository
	err error
}

func (r failingRemovePartyRepo) RemoveMember(context.Context, string, string) error {
	return r.err
}

func (r failingRemovePartyRepo) DeleteParty(context.Context, string) error {
	return r.err
}

type leaveTestStageProvider struct{}

func (leaveTestStageProvider) FindByID(stageID string) (adventure.Stage, error) {
	return adventure.Stage{ID: stageID, Name: "はじまりの森"}, nil
}

func (leaveTestStageProvider) CanAccessStage(corecharacter.Character, string) error { return nil }

type leaveTestMonsterProvider struct{}

func (leaveTestMonsterProvider) FindByID(string) (adventure.Monster, error) {
	return adventure.Monster{}, adventure.ErrMonsterNotFound
}

func TestLeavePartySQLAndValkeyIntegration(t *testing.T) {
	db := database.OpenTestDB(t)
	client := testutil.OpenValkey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	adapter := battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips),
		battle.WithJobHistoryRepository(jobs),
		battle.WithTransactionProvider(tx),
	)

	ns := id.New()
	lobbyPrefix := "party2:test:leave:" + ns + ":lobby:"
	charPrefix := "party2:test:leave:" + ns + ":character:"
	indexKey := "party2:test:leave:" + ns + ":lobbies"
	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(lobbyPrefix),
		party.WithCharacterKeyPrefix(charPrefix),
		party.WithLobbiesIndexKey(indexKey),
	)

	svc, err := party.NewService(repo, chars, invs, leaveTestStageProvider{}, leaveTestMonsterProvider{}, nil,
		party.WithParticipantBuilder(adapter),
		party.WithTransactionProvider(tx),
	)
	if err != nil {
		t.Fatal(err)
	}

	newChar := func(name string, tired, hp, maxHP, mp, maxMP int) corecharacter.Character {
		t.Helper()
		c, err := database.CreateTestCharacterWithFunds(ctx, db, name, 100)
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, c)
		c.Tired = tired
		c.Stats.HP, c.Stats.MaxHP = hp, maxHP
		c.Stats.MP, c.Stats.MaxMP = mp, maxMP
		if err := chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := invs.Save(ctx, coreinventory.Inventory{CharacterID: c.ID}); err != nil {
			t.Fatal(err)
		}
		return c
	}

	cleanupKeys := func(partyID string, charIDs ...string) {
		if partyID != "" {
			_ = client.Do(ctx, client.B().Del().Key(lobbyPrefix+partyID).Build()).Error()
			_ = client.Do(ctx, client.B().Zrem().Key(indexKey).Member(partyID).Build()).Error()
		}
		for _, cid := range charIDs {
			_ = client.Do(ctx, client.B().Del().Key(charPrefix+cid).Build()).Error()
		}
	}

	t.Run("normal mode applies +1 fatigue and battle-row HP/MP with rollback on failure", func(t *testing.T) {
		leader := newChar("LeaveLeader", 10, 80, 100, 12, 30)
		member := newChar("LeaveMember", 5, 70, 90, 10, 25)

		created, err := svc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{
			Name: "NormalLeaveSQL", StageID: adventure.StarterAdventure, Speed: 18,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupKeys(created.Party.ID, leader.ID, member.ID) })

		if _, err := svc.JoinParty(ctx, created.Party.ID, member.ID, ""); err != nil {
			t.Fatal(err)
		}

		// Character write failure rolls back SQL and keeps Valkey membership.
		writeErr := errors.New("injected character update failure")
		failCharSvc, err := party.NewService(repo, failingUpdateCharRepo{CharacterRepository: chars, err: writeErr}, invs, leaveTestStageProvider{}, leaveTestMonsterProvider{}, nil,
			party.WithTransactionProvider(tx),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := failCharSvc.LeaveParty(ctx, created.Party.ID, member.ID); !errors.Is(err, writeErr) {
			t.Fatalf("expected writeErr, got %v", err)
		}
		if _, err := repo.GetMember(ctx, created.Party.ID, member.ID); err != nil {
			t.Fatalf("member missing from Valkey after failed SQL update: %v", err)
		}

		// Valkey removal failure rolls back the SQL character write.
		valkeyErr := errors.New("injected valkey remove failure")
		failRepoSvc, err := party.NewService(failingRemovePartyRepo{Repository: repo, err: valkeyErr}, chars, invs, leaveTestStageProvider{}, leaveTestMonsterProvider{}, nil,
			party.WithTransactionProvider(tx),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := failRepoSvc.LeaveParty(ctx, created.Party.ID, member.ID); !errors.Is(err, valkeyErr) {
			t.Fatalf("expected valkeyErr, got %v", err)
		}
		unchanged, err := chars.FindByID(ctx, member.ID)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged.Tired != 5 || unchanged.Stats.HP != 70 || unchanged.Stats.MP != 10 {
			t.Fatalf("SQL state mutated after Valkey failure: %+v", unchanged)
		}

		// Lower live MaxHP/MaxMP after join so LeaveParty clamps the captured battle-row HP/MP.
		liveLeader, err := chars.FindByID(ctx, leader.ID)
		if err != nil {
			t.Fatal(err)
		}
		liveLeader.Stats.MaxHP, liveLeader.Stats.MaxMP = 60, 8
		liveLeader.Stats.HP, liveLeader.Stats.MP = 60, 8
		if err := chars.Update(ctx, liveLeader); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.LeaveParty(ctx, created.Party.ID, member.ID); err != nil {
			t.Fatalf("LeaveParty(member) failed: %v", err)
		}
		gotMember, err := chars.FindByID(ctx, member.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotMember.Tired != 6 || gotMember.Stats.HP != 70 || gotMember.Stats.MP != 10 {
			t.Fatalf("member after leave = tired:%d hp:%d mp:%d, want 6/70/10", gotMember.Tired, gotMember.Stats.HP, gotMember.Stats.MP)
		}
		if _, _, err := svc.GetActiveParty(ctx, member.ID); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("member still active in Valkey: %v", err)
		}

		if _, err := svc.LeaveParty(ctx, created.Party.ID, leader.ID); err != nil {
			t.Fatalf("LeaveParty(leader) failed: %v", err)
		}
		gotLeader, err := chars.FindByID(ctx, leader.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotLeader.Tired != 11 || gotLeader.Stats.HP != 60 || gotLeader.Stats.MP != 8 {
			t.Fatalf("leader after leave = tired:%d hp:%d mp:%d, want 11/60/8", gotLeader.Tired, gotLeader.Stats.HP, gotLeader.Stats.MP)
		}
		if _, err := svc.GetParty(ctx, created.Party.ID); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("expected lobby deleted after last leave, got %v", err)
		}
	})

	t.Run("calm mode halves round-0 fatigue to 0", func(t *testing.T) {
		leader := newChar("CalmLeader", 10, 50, 100, 10, 30)

		created, err := svc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{
			Name: "CalmLeaveSQL", StageID: adventure.StarterAdventure, Speed: 18,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupKeys(created.Party.ID, leader.ID) })

		p := created.Party
		p.Mode = adventure.CrawlModeCalm
		if err := repo.UpdateParty(ctx, p); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.LeaveParty(ctx, created.Party.ID, leader.ID); err != nil {
			t.Fatalf("LeaveParty(calm) failed: %v", err)
		}
		gotLeader, err := chars.FindByID(ctx, leader.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotLeader.Tired != 10 || gotLeader.Stats.HP != 50 || gotLeader.Stats.MP != 10 {
			t.Fatalf("calm leader after leave = tired:%d hp:%d mp:%d, want 10/50/10", gotLeader.Tired, gotLeader.Stats.HP, gotLeader.Stats.MP)
		}
	})

	t.Run("resets tavern fullness and settles standing delivery atomically", func(t *testing.T) {
		catalog, err := tavern.LoadDefaultCatalog()
		if err != nil {
			t.Fatal(err)
		}
		tavernRepo, err := database.NewTavernRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		tavernSvc, err := tavern.NewService(catalog, tavernRepo, chars, tx)
		if err != nil {
			t.Fatal(err)
		}
		tavernExit := testTavernExitAdapter{svc: tavernSvc}
		leaveSvc, err := party.NewService(repo, chars, invs, leaveTestStageProvider{}, leaveTestMonsterProvider{}, nil,
			party.WithParticipantBuilder(adapter),
			party.WithTransactionProvider(tx),
			party.WithTavernExitSettler(tavernExit),
		)
		if err != nil {
			t.Fatal(err)
		}

		// 1. Affordable delivery + fullness reset + Valkey rollback check.
		paidChar := newChar("LeavePaid", 0, 95, 100, 10, 30)
		// OrderMeal costs 20G and sets is_full=true; give 200G -> 180G after meal.
		paidChar.Money = 200
		if err := chars.Update(ctx, paidChar); err != nil {
			t.Fatal(err)
		}
		if _, err := tavernSvc.OrderMeal(ctx, paidChar.ID, "tavern_water"); err != nil {
			t.Fatal(err)
		}
		// Reserve tavern_tomato_juice (price 100G, hp_heal 10, mp_heal 80).
		if _, err := tavernSvc.ReserveDelivery(ctx, paidChar.ID, "tavern_tomato_juice"); err != nil {
			t.Fatal(err)
		}
		// Drop HP/MP after eating so delivery healing & clamping are observable.
		paidChar, err = chars.FindByID(ctx, paidChar.ID)
		if err != nil {
			t.Fatal(err)
		}
		paidChar.Stats.HP, paidChar.Stats.MP = 95, 10
		if err := chars.Update(ctx, paidChar); err != nil {
			t.Fatal(err)
		}

		createdPaid, err := leaveSvc.CreateParty(ctx, paidChar.ID, party.CreatePartyRequest{
			Name: "TavernPaid", StageID: adventure.StarterAdventure, Speed: 18,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupKeys(createdPaid.Party.ID, paidChar.ID) })

		// Valkey removal failure rolls back ResetFullness and ClaimDelivery too.
		valkeyErr := errors.New("injected valkey failure")
		failLeaveSvc, err := party.NewService(failingRemovePartyRepo{Repository: repo, err: valkeyErr}, chars, invs, leaveTestStageProvider{}, leaveTestMonsterProvider{}, nil,
			party.WithTransactionProvider(tx),
			party.WithTavernExitSettler(tavernExit),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := failLeaveSvc.LeaveParty(ctx, createdPaid.Party.ID, paidChar.ID); !errors.Is(err, valkeyErr) {
			t.Fatalf("expected valkeyErr, got %v", err)
		}
		if st, err := tavernSvc.GetStatus(ctx, paidChar.ID); err != nil || !st.IsFull {
			t.Fatalf("fullness cleared despite rollback: %+v err=%v", st, err)
		}
		if got, err := chars.FindByID(ctx, paidChar.ID); err != nil || got.Money != 180 || got.Stats.HP != 95 || got.Stats.MP != 10 {
			t.Fatalf("delivery charged/healed despite rollback: %+v err=%v", got, err)
		}

		res, err := leaveSvc.LeaveParty(ctx, createdPaid.Party.ID, paidChar.ID)
		if err != nil {
			t.Fatalf("LeaveParty(paid) failed: %v", err)
		}
		if res.Delivery == nil || !res.Delivery.Paid || res.Delivery.ItemName != "完熟トマトジュース" {
			t.Fatalf("unexpected delivery outcome: %+v", res.Delivery)
		}
		gotPaid, err := chars.FindByID(ctx, paidChar.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotPaid.Money != 80 || gotPaid.Stats.HP != 100 || gotPaid.Stats.MP != 30 || gotPaid.Tired != 1 {
			t.Fatalf("paid leave char = money:%d hp:%d mp:%d tired:%d, want 80/100/30/1", gotPaid.Money, gotPaid.Stats.HP, gotPaid.Stats.MP, gotPaid.Tired)
		}
		stPaid, err := tavernSvc.GetStatus(ctx, paidChar.ID)
		if err != nil || stPaid.IsFull || stPaid.DeliveryReservation == nil {
			t.Fatalf("expected is_full=false and standing delivery preserved, got %+v err=%v", stPaid, err)
		}

		// 2. Unaffordable delivery: fullness still resets, order stays active, no charge/heal.
		unpaidChar := newChar("LeaveUnpaid", 0, 50, 100, 10, 30)
		unpaidChar.Money = 20
		if err := chars.Update(ctx, unpaidChar); err != nil {
			t.Fatal(err)
		}
		if _, err := tavernSvc.OrderMeal(ctx, unpaidChar.ID, "tavern_water"); err != nil {
			t.Fatal(err)
		}
		if _, err := tavernSvc.ReserveDelivery(ctx, unpaidChar.ID, "tavern_tomato_juice"); err != nil {
			t.Fatal(err)
		}
		unpaidChar, err = chars.FindByID(ctx, unpaidChar.ID)
		if err != nil {
			t.Fatal(err)
		}
		unpaidChar.Stats.HP = 50
		if err := chars.Update(ctx, unpaidChar); err != nil {
			t.Fatal(err)
		}

		createdUnpaid, err := leaveSvc.CreateParty(ctx, unpaidChar.ID, party.CreatePartyRequest{
			Name: "TavernUnpaid", StageID: adventure.StarterAdventure, Speed: 18,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupKeys(createdUnpaid.Party.ID, unpaidChar.ID) })

		unpaidRes, err := leaveSvc.LeaveParty(ctx, createdUnpaid.Party.ID, unpaidChar.ID)
		if err != nil {
			t.Fatalf("LeaveParty(unpaid) failed: %v", err)
		}
		if unpaidRes.Delivery == nil || unpaidRes.Delivery.Paid || unpaidRes.Delivery.ItemName != "完熟トマトジュース" {
			t.Fatalf("unexpected unpaid delivery outcome: %+v", unpaidRes.Delivery)
		}
		gotUnpaid, err := chars.FindByID(ctx, unpaidChar.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotUnpaid.Money != 0 || gotUnpaid.Stats.HP != 50 || gotUnpaid.Stats.MP != 10 || gotUnpaid.Tired != 1 {
			t.Fatalf("unpaid leave char = money:%d hp:%d mp:%d tired:%d, want 0/50/10/1", gotUnpaid.Money, gotUnpaid.Stats.HP, gotUnpaid.Stats.MP, gotUnpaid.Tired)
		}
		stUnpaid, err := tavernSvc.GetStatus(ctx, unpaidChar.ID)
		if err != nil || stUnpaid.IsFull || stUnpaid.DeliveryReservation == nil {
			t.Fatalf("expected is_full=false and unpaid delivery preserved, got %+v err=%v", stUnpaid, err)
		}
	})
}
