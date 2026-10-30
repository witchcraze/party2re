package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func retireDungeon(ctx context.Context, store dungeon.ActiveExpeditionStore, actor string) error {
	run, err := store.GetActiveExpedition(ctx, actor)
	if err != nil || run == nil || run.CharacterID != actor {
		return err
	}
	return store.DeleteActiveExpedition(ctx, actor, run.ID)
}

func TestCombatAdmissionProduction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	combat, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	chars := make([]corecharacter.Character, 2)
	for i := range chars {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "Roster_"+id.New()[:8], 1000)
		if err != nil {
			t.Fatal(err)
		}
		char.Level = 15
		char.Stats.HP = 200
		char.Stats.MaxHP = 200
		if err := core.charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		chars[i] = char
		t.Cleanup(func() {
			if err := core.charRepo.Delete(ctx, char.ID); err != nil {
				t.Error(err)
			}
			if err := core.playerRepo.Delete(ctx, char.PlayerID); err != nil {
				t.Error(err)
			}
		})
	}
	leader, member := chars[0], chars[1]
	if leader.PlayerID == member.PlayerID {
		t.Fatal("fixture must exercise cross-account consent")
	}
	lobby, err := combat.party.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Name: "Authorized roster", StageID: "stage-00"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := combat.party.DisbandParty(ctx, lobby.Party.ID, leader.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := combat.party.JoinParty(ctx, lobby.Party.ID, member.ID, ""); err != nil {
		t.Fatal(err)
	}
	ds, _ := dungeon.NewValkeyExpeditionRepository(client)
	t.Cleanup(func() {
		for _, char := range chars {
			if err := retireDungeon(ctx, ds, char.ID); err != nil {
				t.Error(err)
			}
			if err := retireChallenge(ctx, combat.challenge, char.ID); err != nil {
				t.Error(err)
			}
		}
	})
	for _, which := range []string{"dungeon", "challenge"} {
		t.Run(which, func(t *testing.T) {
			start := func(actor string, ids []string) error {
				if which == "dungeon" {
					_, err := combat.dungeon.StartPartyExpedition(ctx, actor, ids, "dungeon-01", lobby.Party.ID)
					return err
				}
				_, err := combat.challenge.StartPartySession(ctx, actor, ids, "1", "Consented", "#FFFFFF")
				return err
			}
			if _, err := combat.party.SetReady(ctx, lobby.Party.ID, member.ID, false); err != nil {
				t.Fatal(err)
			}
			if err := start(leader.ID, nil); !errors.Is(err, party.ErrPartyNotReady) {
				t.Fatalf("unready accepted: %v", err)
			}
			if _, err := combat.party.SetReady(ctx, lobby.Party.ID, member.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := start(member.ID, nil); !errors.Is(err, party.ErrNotPartyLeader) {
				t.Fatalf("nonleader accepted: %v", err)
			}
			for _, claimed := range [][]string{{leader.ID, leader.ID}, {leader.ID, id.New()}} {
				if err := start(leader.ID, claimed); !errors.Is(err, party.ErrCharacterNotInParty) {
					t.Fatalf("bad roster accepted: %v", err)
				}
			}
			if err := soc.timer.SetLock(ctx, timer.CategorySleep, member.ID, time.Minute); err != nil {
				t.Fatal(err)
			}
			if err := start(leader.ID, nil); !errors.Is(err, party.ErrMemberBusy) {
				t.Fatalf("sleeping member accepted: %v", err)
			}
			if err := soc.timer.ReleaseLock(ctx, timer.CategorySleep, member.ID); err != nil {
				t.Fatal(err)
			}
			if err := start(leader.ID, nil); err != nil {
				t.Fatalf("consented cross-account party rejected: %v", err)
			}
			if which == "dungeon" {
				run, err := combat.dungeon.GetActiveExpedition(ctx, member.ID)
				if err != nil || run == nil || run.CharacterID != leader.ID || len(run.Members) != 2 {
					t.Fatalf("member discovery: %+v %v", run, err)
				}
				if _, err := combat.dungeon.Move(ctx, member.ID, dungeon.DirectionEast); !errors.Is(err, dungeon.ErrForbidden) {
					t.Fatalf("member moved leader: %v", err)
				}
				if _, err := combat.dungeon.Escape(ctx, member.ID); !errors.Is(err, dungeon.ErrForbidden) {
					t.Fatalf("member escaped leader: %v", err)
				}
				if _, err := combat.challenge.StartSession(ctx, member.ID, "1"); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
					t.Fatalf("busy member accepted: %v", err)
				}
				if _, err := combat.party.StartPartyAdventure(ctx, lobby.Party.ID, leader.ID); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
					t.Fatalf("ordinary party start bypassed enrollment: %v", err)
				}
				if err := ds.DeleteActiveExpedition(ctx, leader.ID, run.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				run, err := combat.challenge.GetActiveSession(ctx, member.ID)
				if err != nil || run == nil || run.CharacterID != leader.ID || len(run.Members) != 2 || run.PartyID != lobby.Party.ID {
					t.Fatalf("member discovery: %+v %v", run, err)
				}
				if _, _, err := combat.challenge.AdvanceRound(ctx, member.ID, run.ID, run.Revision); !errors.Is(err, challenge.ErrForbidden) {
					t.Fatalf("member advanced leader: %v", err)
				}
				if _, err := combat.challenge.GetSession(ctx, id.New(), run.ID); !errors.Is(err, challenge.ErrForbidden) {
					t.Fatalf("private run exposed: %v", err)
				}
				if _, err := combat.dungeon.StartExpedition(ctx, member.ID, "dungeon-01"); !errors.Is(err, challenge.ErrActiveSessionExists) {
					t.Fatalf("busy member accepted: %v", err)
				}
				for _, actor := range chars {
					if err := retireChallenge(ctx, combat.challenge, actor.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
	// A solo start and the other owner's party start share one character lock.
	first, second := testutil.RunRace2(
		func() error {
			_, err := combat.dungeon.StartPartyExpedition(ctx, leader.ID, nil, "dungeon-01", lobby.Party.ID)
			return err
		},
		func() error { _, err := combat.challenge.StartSession(ctx, member.ID, "1"); return err },
	)
	if (first == nil) == (second == nil) || testutil.IsDeadlockError(first) || testutil.IsDeadlockError(second) {
		t.Fatalf("starts did not exclude each other: %v / %v", first, second)
	}
	for _, before := range chars {
		after, err := core.charRepo.FindByID(ctx, before.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.Money != before.Money || after.Experience != before.Experience || after.Stats.HP != before.Stats.HP {
			t.Fatalf("admission changed assets: %+v", after)
		}
	}
}

func TestCombatAdmissionProduction_Stage26OncePerWake(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	combat, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}

	char1, err := database.CreateTestCharacterWithFunds(ctx, db, "S26L_"+id.New()[:8], 1000)
	if err != nil {
		t.Fatal(err)
	}
	char1.Level = 99
	char1.Stats.HP = 9999
	char1.Stats.MaxHP = 9999
	char1.Stats.Attack = 9999
	char1.Stats.Defense = 9999
	_ = core.charRepo.Update(ctx, char1)

	char2, err := database.CreateTestCharacterWithFunds(ctx, db, "S26M_"+id.New()[:8], 1000)
	if err != nil {
		t.Fatal(err)
	}
	char2.Level = 99
	char2.Stats.HP = 9999
	char2.Stats.MaxHP = 9999
	char2.Stats.Attack = 9999
	char2.Stats.Defense = 9999
	_ = core.charRepo.Update(ctx, char2)

	// Clean up any lingering dungeon_once lock
	t.Cleanup(func() {
		_ = soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, char1.ID)
		_ = soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, char2.ID)
	})

	// 1. Leader creates party on stage-26
	partyDetail, err := combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Name:       "Stage26Party",
		StageID:    "stage-26",
		MaxMembers: 2,
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	// Member joins and readies
	if _, err := combat.party.JoinParty(ctx, partyDetail.Party.ID, char2.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	if _, err := combat.party.SetReady(ctx, partyDetail.Party.ID, char2.ID, true); err != nil {
		t.Fatalf("SetReady failed: %v", err)
	}

	// Leader starts adventure
	res, err := combat.party.StartPartyAdventure(ctx, partyDetail.Party.ID, char1.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure on stage-26 failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected win outcome, got: %s", res.Outcome)
	}

	// Leader must be locked in production Valkey
	locked, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, char1.ID)
	if err != nil || !locked {
		t.Fatalf("expected leader to have dungeon_once lock in production Valkey: %v, err=%v", locked, err)
	}

	// Member must NOT be locked
	memberLocked, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, char2.ID)
	if err != nil || memberLocked {
		t.Fatalf("expected member to NOT have dungeon_once lock: %v, err=%v", memberLocked, err)
	}

	// 2. Leader disbands party and attempts to re-create stage-26 party -> rejected with ErrDailyOnceAlreadyUsed
	if err := combat.party.DisbandParty(ctx, partyDetail.Party.ID, char1.ID); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}
	_, err = combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Name:    "RetryStage26",
		StageID: "stage-26",
	})
	if !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("expected ErrDailyOnceAlreadyUsed on re-create by leader, got: %v", err)
	}

	// Solo crawl by leader on stage-26 must also be rejected
	_, err = combat.adv.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char1.ID},
		StageID:      "stage-26",
	})
	if !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("expected ErrDailyOnceAlreadyUsed on solo crawl by leader, got: %v", err)
	}

	// Member CAN create a party on stage-26
	memberParty, err := combat.party.CreateParty(ctx, char2.ID, party.CreatePartyRequest{
		Name:    "MemberStage26",
		StageID: "stage-26",
	})
	if err != nil {
		t.Fatalf("member should be allowed to create stage-26 party: %v", err)
	}
	_ = combat.party.DisbandParty(ctx, memberParty.Party.ID, char2.ID)

	// 3. Releasing lock (wake reset) allows leader entry again
	if err := soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, char1.ID); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}
	p2, err := combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Name:    "AfterWakeParty",
		StageID: "stage-26",
	})
	if err != nil {
		t.Fatalf("leader should be allowed to create stage-26 party after wake: %v", err)
	}
	_ = combat.party.DisbandParty(ctx, p2.Party.ID, char1.ID)
}
