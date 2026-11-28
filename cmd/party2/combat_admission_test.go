package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

func retireDungeon(ctx context.Context, store dungeon.ActiveExpeditionStore, actor string) error {
	run, err := store.GetActiveExpedition(ctx, actor)
	if err != nil || run == nil || run.CharacterID != actor {
		return err
	}
	return store.DeleteActiveExpedition(ctx, actor, run.ID)
}

type noopMonsterTamer struct{}

func (noopMonsterTamer) TameMonster(context.Context, string, string, string) error { return nil }

func TestCombatAdmissionProduction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx := context.Background()
	db := database.OpenTestDB(t)
	client := testutil.OpenValkey(t)
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
	// Map0 moves settle defeat credits under production RNG without wireHooks.
	combat.battle.SetMonsterTamer(noopMonsterTamer{})
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
	lobby, err := combat.party.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Speed: 18, Name: "AuthorizedRoster", StageID: "stage-00"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Run exits may already have removed members; only the lobby must go.
		for _, id := range []string{member.ID, leader.ID} {
			if _, err := combat.party.LeaveParty(ctx, lobby.Party.ID, id); err != nil && !errors.Is(err, party.ErrCharacterNotInParty) && !errors.Is(err, party.ErrNotFound) {
				t.Error(err)
			}
		}
	})
	if _, err := combat.party.JoinParty(ctx, lobby.Party.ID, member.ID, ""); err != nil {
		t.Fatal(err)
	}
	ds, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
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
			t.Cleanup(func() {
				if err := soc.timer.ReleaseLock(ctx, timer.CategorySleep, member.ID); err != nil {
					t.Error(err)
				}
				if which == "dungeon" {
					for _, char := range chars {
						if err := retireDungeon(ctx, ds, char.ID); err != nil {
							t.Error(err)
						}
					}
				} else {
					for _, actor := range chars {
						if err := retireChallenge(ctx, combat.challenge, actor.ID); err != nil {
							t.Error(err)
						}
					}
				}
			})
			start := func(actor string, ids []string) error {
				if which == "dungeon" {
					_, err := combat.dungeon.StartPartyExpedition(ctx, actor, ids, dungeon.NativeMap0ID, lobby.Party.ID)
					return err
				}
				_, err := combat.challenge.StartPartySession(ctx, actor, ids, "1", "Consented", "#FFFFFF")
				return err
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
				// Any living member moves the shared run; Escape stays with the leader.
				if _, err := combat.dungeon.Move(ctx, member.ID, dungeon.DirectionEast); err != nil {
					t.Fatalf("member move rejected: %v", err)
				}
				if _, err := combat.dungeon.Escape(ctx, member.ID); !errors.Is(err, dungeon.ErrForbidden) {
					t.Fatalf("member escaped leader: %v", err)
				}
				if _, err := combat.challenge.StartSession(ctx, member.ID, "1"); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
					t.Fatalf("busy member accepted: %v", err)
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
				if _, err := combat.dungeon.StartExpedition(ctx, member.ID, dungeon.NativeMap0ID); !errors.Is(err, challenge.ErrActiveSessionExists) {
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
	// Earlier moves may have committed defeat credits, so the baseline is taken here.
	baseline := make([]corecharacter.Character, len(chars))
	for i, c := range chars {
		var err error
		if baseline[i], err = core.charRepo.FindByID(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	first, second := testutil.RunRace2(
		func() error {
			_, err := combat.dungeon.StartPartyExpedition(ctx, leader.ID, nil, dungeon.NativeMap0ID, lobby.Party.ID)
			return err
		},
		func() error { _, err := combat.challenge.StartSession(ctx, member.ID, "1"); return err },
	)
	if (first == nil) == (second == nil) || testutil.IsDeadlockError(first) || testutil.IsDeadlockError(second) {
		t.Fatalf("starts did not exclude each other: %v / %v", first, second)
	}
	for _, before := range baseline {
		after, err := core.charRepo.FindByID(ctx, before.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.Money != before.Money || after.Experience != before.Experience || after.Stats.HP != before.Stats.HP {
			t.Fatalf("admission changed assets: %+v", after)
		}
	}
	t.Run("normal run owns live state", func(t *testing.T) {
		for _, c := range chars {
			if err := retireDungeon(ctx, ds, c.ID); err != nil {
				t.Fatal(err)
			}
			if err := retireChallenge(ctx, combat.challenge, c.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := combat.party.LeaveParty(ctx, lobby.Party.ID, member.ID); err != nil {
			t.Fatal(err)
		}
		entry, err := combat.normal.EnterSolo(ctx, member.PlayerID, member.ID, "normal-owned", "normal-owned-run", adventure.StarterAdventure, "normal", 3)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := soc.home.Sleep(ctx, member.ID, ""); !errors.Is(err, party.ErrMemberBusy) {
			t.Fatal("sleep bypassed enrollment", err)
		}
		if _, err := combat.dungeon.StartExpedition(ctx, member.ID, dungeon.NativeMap0ID); !errors.Is(err, party.ErrMemberBusy) {
			t.Fatal("Dungeon bypassed enrollment", err)
		}
		if _, err := combat.challenge.StartSession(ctx, member.ID, "1"); !errors.Is(err, party.ErrMemberBusy) {
			t.Fatal("Challenge bypassed enrollment", err)
		}
		called := false
		if _, err := core.economy.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: member.ID}, func(*economy.TxContext) error { called = true; return nil }); !errors.Is(err, runhost.ErrBusy) || called {
			t.Fatal("actor mutation bypassed enrollment", err)
		}
		if err := core.txProvider.RunInTx(ctx, func(ctx context.Context) error {
			_, err := normalWriter(core).FindByIDForUpdate(ctx, member.ID)
			return err
		}); !errors.Is(err, runhost.ErrBusy) {
			t.Fatal("live-state writer bypassed enrollment", err)
		}
		if err := normalWriter(core).Delete(ctx, member.ID); !errors.Is(err, runhost.ErrBusy) {
			t.Fatal("deletion orphaned an enrolled actor", err)
		}
		// Incoming peer credits remain valid and must survive later native exit.
		start, err := core.charRepo.FindByID(ctx, member.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := core.txProvider.RunInTx(ctx, func(ctx context.Context) error {
			c, err := core.charRepo.FindByIDForUpdate(ctx, member.ID)
			if err != nil {
				return err
			}
			if err := c.AddMoney(7); err != nil {
				return err
			}
			return core.charRepo.Update(ctx, c)
		}); err != nil {
			t.Fatal(err)
		}
		// This fixture constructs combat services without the later Tavern wiring.
		combat.normal.SetExitEffects(func(context.Context, string) error { return nil })
		if _, err := combat.normal.Command(ctx, runhost.Command{PlayerID: member.PlayerID, ActorID: member.ID, RunID: entry.Command.RunID, ID: "exit", Version: 1, Action: "exit"}); err != nil {
			t.Fatal(err)
		}
		if after, err := core.charRepo.FindByID(ctx, member.ID); err != nil || after.Money != start.Money+7 {
			t.Fatal("exit overwrote a peer credit", after, err)
		}
	})
}

// stage26Fixture wires production services on MariaDB and Valkey with one
// stage-26-eligible character per name, cleaned up with its once lock.
func stage26Fixture(t *testing.T, names ...string) (*coreServices, *socServices, *cmbtServices, []corecharacter.Character) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx := context.Background()
	db := database.OpenTestDB(t)
	client := testutil.OpenValkey(t)
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
	// The fixture constructs combat services without the later Tavern wiring.
	combat.normal.SetExitEffects(func(context.Context, string) error { return nil })
	chars := make([]corecharacter.Character, len(names))
	for i, name := range names {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, name+id.New()[:8], 1000)
		if err != nil {
			t.Fatal(err)
		}
		c.Level, c.Stats.HP, c.Stats.MaxHP = 99, 500, 500
		if err := core.charRepo.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		chars[i] = c
		t.Cleanup(func() {
			if err := soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, c.ID); err != nil {
				t.Error(err)
			}
			if err := core.charRepo.Delete(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := core.playerRepo.Delete(ctx, c.PlayerID); err != nil {
				t.Error(err)
			}
		})
	}
	return core, soc, combat, chars
}

func exitNormalRun(t *testing.T, combat *cmbtServices, c corecharacter.Character, entry runhost.Receipt) {
	t.Helper()
	if _, err := combat.normal.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: entry.Command.RunID, ID: "exit-" + id.New()[:8], Version: entry.Version, Action: "exit"}); err != nil {
		t.Fatal(err)
	}
}

// Stage 26 once per wake across Party creation and solo entry on production
// Valkey (quest.cgi:594–599 $m{once}=1; cleared only on waking at Home).
func TestCombatAdmissionProduction_Stage26OncePerWake(t *testing.T) {
	ctx := context.Background()
	_, soc, combat, chars := stage26Fixture(t, "S26L_", "S26M_")
	leader, member := chars[0], chars[1]
	stage26 := party.CreatePartyRequest{Speed: 18, Name: "Stage26Party", StageID: "stage-26", MaxMembers: 2}
	lobby, err := combat.party.CreateParty(ctx, leader.ID, stage26)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := combat.party.JoinParty(ctx, lobby.Party.ID, member.ID, ""); err != nil {
		t.Fatal(err)
	}
	for c, want := range map[string]bool{leader.ID: true, member.ID: false} {
		if locked, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, c); err != nil || locked != want {
			t.Fatalf("once lock for %s = %v (err=%v), want %v", c, locked, err, want)
		}
	}
	for _, c := range []string{member.ID, leader.ID} {
		if _, err := combat.party.LeaveParty(ctx, lobby.Party.ID, c); err != nil {
			t.Fatal(err)
		}
	}
	// Leaving does not restore the quota for either entry route.
	if _, err := combat.party.CreateParty(ctx, leader.ID, stage26); !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("leader re-created stage-26 party: %v", err)
	}
	if _, err := combat.normal.EnterSolo(ctx, leader.PlayerID, leader.ID, "s26-blocked", "solo-quest", "stage-26", "normal", 3); !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("leader entered stage-26 solo: %v", err)
	}
	// Waking at Home releases the quota.
	if err := soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, leader.ID); err != nil {
		t.Fatal(err)
	}
	entry, err := combat.normal.EnterSolo(ctx, leader.PlayerID, leader.ID, "s26-after-wake", "solo-quest", "stage-26", "normal", 3)
	if err != nil {
		t.Fatalf("leader rejected after wake: %v", err)
	}
	exitNormalRun(t, combat, leader, entry)
}

// One character racing solo entry against Party creation for stage 26 gets
// exactly one admission; the others are rejected without unexpected errors.
func TestCombatAdmissionProduction_Stage26ConcurrentSoloAndParty(t *testing.T) {
	ctx := context.Background()
	_, soc, combat, chars := stage26Fixture(t, "S26R_")
	c := chars[0]
	var mu sync.Mutex
	var lobbies []string
	var entries []runhost.Receipt
	var fns []func() error
	for i := 0; i < 3; i++ {
		key, name := fmt.Sprintf("s26-race-%d", i), fmt.Sprintf("Race%d", i)
		fns = append(fns,
			func() error {
				entry, err := combat.normal.EnterSolo(ctx, c.PlayerID, c.ID, key, "solo-quest", "stage-26", "normal", 3)
				if err == nil {
					mu.Lock()
					entries = append(entries, entry)
					mu.Unlock()
				}
				return err
			},
			func() error {
				lobby, err := combat.party.CreateParty(ctx, c.ID, party.CreatePartyRequest{Speed: 18, Name: name, StageID: "stage-26"})
				if err == nil {
					mu.Lock()
					lobbies = append(lobbies, lobby.Party.ID)
					mu.Unlock()
				}
				return err
			})
	}
	errs := testutil.RunRace(fns...)
	for _, p := range lobbies {
		if _, err := combat.party.LeaveParty(ctx, p, c.ID); err != nil {
			t.Error(err)
		}
	}
	for _, e := range entries {
		exitNormalRun(t, combat, c, e)
	}
	admitted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, adventure.ErrDailyOnceAlreadyUsed), errors.Is(err, party.ErrAlreadyInParty),
			errors.Is(err, party.ErrMemberBusy), errors.Is(err, runhost.ErrBusy):
		default:
			t.Errorf("unexpected admission error (deadlock=%v): %v", testutil.IsDeadlockError(err), err)
		}
	}
	if admitted != 1 || len(lobbies)+len(entries) != 1 {
		t.Fatalf("admitted %d (lobbies=%v entries=%d), want exactly one: %v", admitted, lobbies, len(entries), errs)
	}
	if locked, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, c.ID); err != nil || !locked {
		t.Fatalf("winner's once lock missing: %v %v", locked, err)
	}
}
