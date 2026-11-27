package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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
		if _, err := combat.adv.StartStage(ctx, member.ID, adventure.StarterAdventure); !errors.Is(err, party.ErrMemberBusy) {
			t.Fatal("batch bypassed enrollment", err)
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

func TestCombatAdmissionProduction_Stage26OncePerWake(t *testing.T) {
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

	char1, err := database.CreateTestCharacterWithFunds(ctx, db, "S26L_"+id.New()[:8], 1000)
	if err != nil {
		t.Fatal(err)
	}
	char1.Level = 99
	char1.Stats.HP = 9999
	char1.Stats.MaxHP = 9999
	char1.Stats.Attack = 9999
	char1.Stats.Defense = 9999
	if err := core.charRepo.Update(ctx, char1); err != nil {
		t.Fatal(err)
	}

	char2, err := database.CreateTestCharacterWithFunds(ctx, db, "S26M_"+id.New()[:8], 1000)
	if err != nil {
		t.Fatal(err)
	}
	char2.Level = 99
	char2.Stats.HP = 9999
	char2.Stats.MaxHP = 9999
	char2.Stats.Attack = 9999
	char2.Stats.Defense = 9999
	if err := core.charRepo.Update(ctx, char2); err != nil {
		t.Fatal(err)
	}

	// Clean up any lingering dungeon_once lock
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, id := range []string{char1.ID, char2.ID} {
			if err := soc.timer.ReleaseLock(cleanupCtx, timer.CategoryDungeonOnce, id); err != nil {
				t.Errorf("cleanup ReleaseLock dungeon_once %s: %v", id, err)
			}
		}
	})

	// 1. Leader creates party on stage-26
	partyDetail, err := combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Speed:      18,
		Name:       "Stage26Party",
		StageID:    "stage-26",
		MaxMembers: 2,
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	// Leader must be locked in production Valkey immediately after CreateParty (lib/quest.cgi:596-600 $m{once}=1)
	lockedImmediate, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, char1.ID)
	if err != nil || !lockedImmediate {
		t.Fatalf("expected leader to have dungeon_once lock immediately after CreateParty: %v, err=%v", lockedImmediate, err)
	}

	// Member joins
	if _, err := combat.party.JoinParty(ctx, partyDetail.Party.ID, char2.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
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

	// 2. Both leave and the leader attempts to re-create stage-26 party -> rejected with ErrDailyOnceAlreadyUsed
	for _, id := range []string{char2.ID, char1.ID} {
		if _, err := combat.party.LeaveParty(ctx, partyDetail.Party.ID, id); err != nil {
			t.Fatalf("LeaveParty failed: %v", err)
		}
	}
	_, err = combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Speed:   18,
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
		Speed:   18,
		Name:    "MemberStage26",
		StageID: "stage-26",
	})
	if err != nil {
		t.Fatalf("member should be allowed to create stage-26 party: %v", err)
	}
	if _, err := combat.party.LeaveParty(ctx, memberParty.Party.ID, char2.ID); err != nil {
		t.Fatalf("LeaveParty failed: %v", err)
	}

	// 3. Releasing lock (wake reset) allows leader entry again
	if err := soc.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, char1.ID); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}
	p2, err := combat.party.CreateParty(ctx, char1.ID, party.CreatePartyRequest{
		Speed:   18,
		Name:    "AfterWakeParty",
		StageID: "stage-26",
	})
	if err != nil {
		t.Fatalf("leader should be allowed to create stage-26 party after wake: %v", err)
	}
	if _, err := combat.party.LeaveParty(ctx, p2.Party.ID, char1.ID); err != nil {
		t.Fatalf("LeaveParty failed: %v", err)
	}
}

func TestCombatAdmissionProduction_Stage26ConcurrentSoloAndParty(t *testing.T) {
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

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "S26R_"+id.New()[:8], 1000)
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 99
	char.Stats.HP = 9999
	char.Stats.MaxHP = 9999
	char.Stats.Attack = 9999
	char.Stats.Defense = 9999
	if err := core.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if err := soc.timer.ReleaseLock(cleanupCtx, timer.CategoryDungeonOnce, char.ID); err != nil {
			t.Errorf("cleanup ReleaseLock dungeon_once %s: %v", char.ID, err)
		}
		if err := core.charRepo.Delete(cleanupCtx, char.ID); err != nil {
			t.Errorf("cleanup delete character %s: %v", char.ID, err)
		}
		if err := core.playerRepo.Delete(cleanupCtx, char.PlayerID); err != nil {
			t.Errorf("cleanup delete player %s: %v", char.PlayerID, err)
		}
	})

	const attempts = 6
	var successCount int32
	var rejectedCount int32
	var otherCount int32
	var createdPartyIDs []string
	var partyMu sync.Mutex
	var errMu sync.Mutex
	var capturedErrors []error
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	for i := 0; i < attempts/2; i++ {
		wg.Add(1)
		pName := "RaceP_" + id.New()[:6]
		go func() {
			defer wg.Done()
			<-startBarrier
			pDetail, err := combat.party.CreateParty(ctx, char.ID, party.CreatePartyRequest{
				Speed:   18,
				Name:    pName,
				StageID: "stage-26",
			})
			if err == nil {
				atomic.AddInt32(&successCount, 1)
				partyMu.Lock()
				createdPartyIDs = append(createdPartyIDs, pDetail.Party.ID)
				partyMu.Unlock()
			} else if errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) || errors.Is(err, party.ErrAlreadyInParty) {
				// Valid admission rejections:
				// - ErrDailyOnceAlreadyUsed: Valkey daily-once lock already acquired by winner
				// - ErrAlreadyInParty: Winning CreateParty committed first, so leader is already assigned to active party
				atomic.AddInt32(&rejectedCount, 1)
			} else {
				isDeadlock := database.IsDeadlockError(err)
				t.Logf("unexpected party create error (deadlock/timeout=%v): %v", isDeadlock, err)
				errMu.Lock()
				capturedErrors = append(capturedErrors, fmt.Errorf("create party (deadlock/timeout=%v): %w", isDeadlock, err))
				errMu.Unlock()
				atomic.AddInt32(&otherCount, 1)
			}
		}()
	}

	for i := 0; i < attempts/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			_, err := combat.adv.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
				CharacterIDs: []string{char.ID},
				StageID:      "stage-26",
			})
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) || errors.Is(err, party.ErrAlreadyInParty) {
				// Valid admission rejection for daily-once stage crawl.
				atomic.AddInt32(&rejectedCount, 1)
			} else {
				isDeadlock := database.IsDeadlockError(err)
				t.Logf("unexpected execute crawl error (deadlock/timeout=%v): %v", isDeadlock, err)
				errMu.Lock()
				capturedErrors = append(capturedErrors, fmt.Errorf("execute crawl (deadlock/timeout=%v): %w", isDeadlock, err))
				errMu.Unlock()
				atomic.AddInt32(&otherCount, 1)
			}
		}()
	}

	close(startBarrier)
	wg.Wait()

	partyMu.Lock()
	for _, pid := range createdPartyIDs {
		if _, err := combat.party.LeaveParty(ctx, pid, char.ID); err != nil {
			t.Errorf("cleanup LeaveParty %s: %v", pid, err)
		}
	}
	partyMu.Unlock()

	if successCount != 1 {
		errMu.Lock()
		errs := append([]error(nil), capturedErrors...)
		errMu.Unlock()
		t.Fatalf("expected exactly 1 success across solo and party, got %d (rejected=%d, other=%d, capturedErrors=%v)",
			successCount, rejectedCount, otherCount, errs)
	}
	if rejectedCount != attempts-1 {
		errMu.Lock()
		errs := append([]error(nil), capturedErrors...)
		errMu.Unlock()
		t.Fatalf("expected %d rejections, got %d (other=%d, capturedErrors=%v)",
			attempts-1, rejectedCount, otherCount, errs)
	}
	if otherCount != 0 {
		errMu.Lock()
		errs := append([]error(nil), capturedErrors...)
		errMu.Unlock()
		t.Fatalf("expected 0 unexpected errors, got %d (capturedErrors=%v)", otherCount, errs)
	}

	locked, err := soc.timer.IsLocked(ctx, timer.CategoryDungeonOnce, char.ID)
	if err != nil || !locked {
		t.Fatalf("expected character to be locked in Valkey, got %v", locked)
	}
}
