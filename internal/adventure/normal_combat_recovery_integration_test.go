package adventure_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/testutil"
)

// combatParty starts a normal party run served by the production RunCombat
// and places enemies on its encounter floor 1. Enemy responses wait an hour so
// race outcomes depend only on the submitted commands.
func combatParty(t *testing.T, count int, enemies ...corebattle.LifetimeActor) (*normalFixture, adventure.RunCommand) {
	t.Helper()
	f := combatFixture(t, count)
	for i := range f.actors {
		f.actors[i].JobID = "job-01"
		if err := f.chars.Update(t.Context(), f.actors[i]); err != nil {
			t.Fatal(err)
		}
	}
	_, lobby := f.waitingParty(t, adventure.CrawlModeNormal)
	leader := f.actors[0]
	entry, err := f.svc.EnterParty(t.Context(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	cmd := adventure.RunCommand{RunID: entry.Command.RunID, Version: entry.Version}
	editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
		r.Floor, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
		for _, c := range f.actors {
			a := runActorByID(r, c.ID)
			a.Attack, a.HP, a.MaxHP = 400, 500, 500
			r.LastActions[c.ID] = f.clock.Now()
		}
		for _, e := range enemies {
			r.Battle.Actors = append(r.Battle.Actors, e)
			r.Enemies = append(r.Enemies, e.ID)
			r.LastActions[e.ID] = f.clock.Now().Add(time.Hour)
		}
	})
	return f, cmd
}

func rewardFoe(id string) corebattle.LifetimeActor {
	foe := runEnemy(id, 1)
	foe.ExperienceReward, foe.GoldReward = 3, 5
	return foe
}

func actorCommand(f *normalFixture, r *adventure.NormalRun, actor int, id, action, target string) adventure.RunCommand {
	c := f.actors[actor]
	return adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: r.ID, ID: id, Version: r.Version, Action: action, TargetID: target}
}

// raceCommands submits both commands behind one barrier and returns the index
// of the one that committed; the other must lose the version race. Each has a
// dead-man bound, so a lock-order hang surfaces as a deadlock, not a stuck test.
func raceCommands(t *testing.T, f *normalFixture, a, b adventure.RunCommand) int {
	t.Helper()
	run := func(cmd adventure.RunCommand) func() error {
		return func() error {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err := f.svc.Command(ctx, cmd)
			return err
		}
	}
	errA, errB := testutil.RunRace2(run(a), run(b))
	switch {
	case testutil.IsDeadlockError(errA) || testutil.IsDeadlockError(errB):
		t.Fatalf("deadlock: %v / %v", errA, errB)
	case errA == nil && errors.Is(errB, adventure.ErrRunConflict):
		return 0
	case errB == nil && errors.Is(errA, adventure.ErrRunConflict):
		return 1
	}
	t.Fatalf("race outcome: %v / %v", errA, errB)
	return -1
}

func members(r *adventure.NormalRun) []string {
	var ids []string
	for _, a := range r.Battle.Actors {
		if !slices.Contains(r.Enemies, a.ID) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func ledgerGold(r *adventure.NormalRun) int {
	total := 0
	for _, p := range r.Recipients {
		total += p.Applied.Gold + p.Pending.Gold
	}
	return total
}

// assertSettledLedger requires every character's balance to move exactly by
// its applied ledger: nothing granted twice and nothing applied but unpaid.
func assertSettledLedger(t *testing.T, f *normalFixture, r *adventure.NormalRun, before []corecharacter.Character) {
	t.Helper()
	for _, b := range before {
		got, err := f.chars.FindByID(t.Context(), b.ID)
		applied := r.Recipients[b.ID].Applied
		if err != nil || got.Money-b.Money != applied.Gold || got.Experience-b.Experience != applied.EXP {
			t.Fatalf("%s: money %d->%d exp %d->%d applied=%+v err=%v", b.ID, b.Money, got.Money, b.Experience, got.Experience, applied, err)
		}
	}
}

func countReceipts(t *testing.T, f *normalFixture, runID, prefix string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM normal_adventure_receipts WHERE run_id=? AND command_id LIKE ?`, runID, prefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Two members striking the same version: one operation commits per round and
// every round credits one defeat's earnings to the unchanged roster.
func TestRunCombatPairedMembersSerializeAndConserveEarnings(t *testing.T) {
	rounds := testutil.GetStressConfig().OpsPerWorker
	var foes []corebattle.LifetimeActor
	for i := range rounds {
		foes = append(foes, rewardFoe(fmt.Sprintf("foe-%02d", i)))
	}
	f, cmd := combatParty(t, 2, foes...)
	before := slices.Clone(f.actors)
	initial := readStockRoom(t, f, cmd.RunID)
	perKill, total := 0, ledgerGold(initial)
	for i, foe := range foes {
		f.clock.add(3 * time.Second)
		r := readStockRoom(t, f, cmd.RunID)
		raceCommands(t, f,
			actorCommand(f, r, 0, fmt.Sprintf("race-%d-a", i), "attack", foe.ID),
			actorCommand(f, r, 1, fmt.Sprintf("race-%d-b", i), "attack", foe.ID))
		after := readStockRoom(t, f, cmd.RunID)
		if after.Version != r.Version+1 || runActorByID(after, foe.ID) != nil ||
			!reflect.DeepEqual(after.Roster, initial.Roster) || !reflect.DeepEqual(members(after), members(initial)) {
			t.Fatalf("round %d: version %d->%d roster=%v members=%v", i, r.Version, after.Version, after.Roster, members(after))
		}
		delta := ledgerGold(after) - total
		if i == 0 {
			perKill = delta
		}
		if delta <= 0 || delta != perKill {
			t.Fatalf("round %d credited %d gold, first round %d", i, delta, perKill)
		}
		total += delta
	}
	assertSettledLedger(t, f, readStockRoom(t, f, cmd.RunID), before)
	if n := countReceipts(t, f, cmd.RunID, "race-"); n != rounds {
		t.Fatalf("receipts=%d rounds=%d", n, rounds)
	}
}

// A strike racing another member's exit: whichever commits first, the retried
// loser applies once and the survivors keep their order and leader.
func TestRunCombatStrikeRacingExitKeepsSurvivorsAndEarnings(t *testing.T) {
	f, cmd := combatParty(t, 3, rewardFoe("foe"), runEnemy("guard", 900))
	before := slices.Clone(f.actors)
	f.clock.add(3 * time.Second)
	r := readStockRoom(t, f, cmd.RunID)
	strike := actorCommand(f, r, 0, "strike", "attack", "foe")
	exit := actorCommand(f, r, 1, "exit", "exit", "")
	loser := strike
	if raceCommands(t, f, strike, exit) == 0 {
		loser = exit
	}
	loser.ID, loser.Version = "retry-"+loser.ID, r.Version+1
	if _, err := f.svc.Command(t.Context(), loser); err != nil {
		t.Fatal(err)
	}
	after := readStockRoom(t, f, cmd.RunID)
	survivors := []string{f.actors[0].ID, f.actors[2].ID}
	departed := after.Recipients[f.actors[1].ID]
	if after.Version != r.Version+2 || !reflect.DeepEqual(after.Roster, survivors) || !reflect.DeepEqual(members(after), survivors) ||
		after.LeaderID != f.actors[0].ID || runActorByID(after, "foe") != nil || !reflect.DeepEqual(after.Enemies, []string{"guard"}) ||
		departed.Disposition != "exit" || departed.Pending != (adventure.RunEarnings{}) || ledgerGold(after) <= 0 {
		t.Fatalf("roster=%v members=%v leader=%s enemies=%v departed=%+v", after.Roster, members(after), after.LeaderID, after.Enemies, departed)
	}
	assertSettledLedger(t, f, after, before)
	if active, err := f.svc.GetActorRun(t.Context(), f.actors[1].PlayerID, f.actors[1].ID); err != nil || active != nil {
		t.Fatalf("departed enrollment: %+v %v", active, err)
	}
}

// A lost commit acknowledgement returns the owned receipt without rerunning
// the operation; a restarted service replays it without drawing, consuming
// or granting again.
func TestRunCombatLostCommitReturnsReceiptOnce(t *testing.T) {
	f, _ := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-001")
	foe := runEnemy("foe", 5)
	foe.ExperienceReward, foe.GoldReward = 3, 7
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		runActorByID(r, f.actors[0].ID).HP = 100
		r.LastActions["foe"] = f.clock.Now().Add(time.Hour)
	}, foe, runEnemy("other", 900))
	for _, step := range []struct{ id, action, target string }{{"herb", "tool", ""}, {"strike", "attack", "foe"}} {
		r := readStockRoom(t, f, cmd.RunID)
		sent := combatCommand(cmd, step.id, step.action, step.target, "")
		sent.Version = r.Version
		f.store.loseCommit.Store(true)
		receipt, err := f.svc.Command(t.Context(), sent)
		if err != nil || f.store.loseCommit.Load() || receipt.Version != r.Version+1 {
			t.Fatalf("%s: lost commit not resolved: %+v %v", step.id, receipt, err)
		}
		committed := readStockRoom(t, f, cmd.RunID)
		balance, err := f.chars.FindByID(t.Context(), cmd.ActorID)
		if err != nil {
			t.Fatal(err)
		}
		f.restart(t)
		draws := f.cfg.RNG.(*midRNG).calls.Load()
		if again, err := f.svc.Command(t.Context(), sent); err != nil || !reflect.DeepEqual(again, receipt) || f.cfg.RNG.(*midRNG).calls.Load() != draws {
			t.Fatalf("%s: replay %+v %v", step.id, again, err)
		}
		if got := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(got, committed) {
			t.Fatalf("%s: replay re-executed the operation", step.id)
		}
		if got, err := f.chars.FindByID(t.Context(), cmd.ActorID); err != nil || got.Money != balance.Money || got.Experience != balance.Experience {
			t.Fatalf("%s: replay granted twice: %+v %v", step.id, got, err)
		}
		f.clock.add(3 * time.Second)
	}
	got, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || got.Money != f.actors[0].Money+7 || got.Experience != f.actors[0].Experience+3 || len(heldItems(t, f, 0)) != 0 {
		t.Fatalf("money=%d exp=%d held=%+v err=%v", got.Money, got.Experience, heldItems(t, f, 0), err)
	}
}

// Cleanup committed before its timer projection failed keeps the real-combat
// stock of the remaining member durable; the retried cleanup pays nothing twice.
func TestRunCombatCleanupFailureAfterCommitKeepsEarnedStock(t *testing.T) {
	f, cmd := combatParty(t, 2, rewardFoe("foe"), runEnemy("guard", 900))
	before := slices.Clone(f.actors)
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(t.Context(), actorCommand(f, readStockRoom(t, f, cmd.RunID), 0, "strike", "attack", "foe")); err != nil {
		t.Fatal(err)
	}
	earned := readStockRoom(t, f, cmd.RunID).Recipients[f.actors[1].ID].Pending
	if earned.Gold <= 0 {
		t.Fatalf("member stock not recorded: %+v", earned)
	}
	f.clock.add(1800*time.Second + time.Nanosecond)
	cleanup := actorCommand(f, readStockRoom(t, f, cmd.RunID), 1, "cleanup", "cleanup", "")
	f.penalty.fail.Store(true)
	result, err := f.svc.Command(t.Context(), cleanup)
	if err == nil || result.Disposition != "timeout" || result.Recipient.Pending != earned {
		t.Fatalf("cleanup: %+v %v", result, err)
	}
	f.penalty.fail.Store(false)
	f.restart(t)
	again, err := f.svc.Command(t.Context(), cleanup)
	if err != nil || !reflect.DeepEqual(again, result) {
		t.Fatalf("retry: %+v %v", again, err)
	}
	facts, err := f.svc.GetActorPenalty(t.Context(), f.actors[1].PlayerID, f.actors[1].ID)
	if err != nil || facts.Recipient.Pending != earned || facts.Recipient.TimerPending {
		t.Fatalf("retained stock: %+v %v", facts, err)
	}
	if got, err := f.chars.FindByID(t.Context(), f.actors[1].ID); err != nil || got.Money != before[1].Money || got.Experience != before[1].Experience {
		t.Fatalf("recovery paid retained stock: %+v %v", got, err)
	}
}

// A defeat reached by the NPC response after the requester's own kill keeps
// that kill's settled earnings.
func TestRunCombatResponseDefeatKeepsEarlierEarnings(t *testing.T) {
	f := combatFixture(t, 1)
	foe := runEnemy("foe", 5)
	foe.ExperienceReward, foe.GoldReward = 3, 7
	killer := runEnemy("killer", 900)
	killer.Attack = 999
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		runActorByID(r, f.actors[0].ID).HP = 1
	}, foe, killer)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "foe", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	if a := runActorByID(r, cmd.ActorID); a == nil || a.HP != 0 || runActorByID(r, "foe") != nil {
		t.Fatalf("response did not defeat the requester after the kill: %+v", a)
	}
	got, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || got.Money != f.actors[0].Money+7 || got.Experience != f.actors[0].Experience+3 {
		t.Fatalf("defeat erased earned rewards: money=%d exp=%d err=%v", got.Money, got.Experience, err)
	}
}
