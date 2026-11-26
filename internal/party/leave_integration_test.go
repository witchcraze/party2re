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
	"github.com/witchcraze/party2re/internal/testutil"
)

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
		if err := failCharSvc.LeaveParty(ctx, created.Party.ID, member.ID); !errors.Is(err, writeErr) {
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
		if err := failRepoSvc.LeaveParty(ctx, created.Party.ID, member.ID); !errors.Is(err, valkeyErr) {
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

		if err := svc.LeaveParty(ctx, created.Party.ID, member.ID); err != nil {
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

		if err := svc.LeaveParty(ctx, created.Party.ID, leader.ID); err != nil {
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

		if err := svc.LeaveParty(ctx, created.Party.ID, leader.ID); err != nil {
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
}
