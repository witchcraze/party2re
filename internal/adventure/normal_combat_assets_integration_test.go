package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
)

// assetRunCombat serves Command with the production RunCombat and its asset
// session (economy runner, helper requests and the requester depot).
func assetRunCombat(t *testing.T, count int) (*normalFixture, *storedDepots) {
	t.Helper()
	f := newNormalFixture(t, count)
	f.cfg.RNG = &midRNG{}
	depots := withAssetSession(t, f)
	installRunCombat(t, f)
	return f, depots
}

// withAssetSession configures the production asset session, as cmd/party2 does.
func withAssetSession(t *testing.T, f *normalFixture) *storedDepots {
	t.Helper()
	runner, err := economy.NewService(f.chars, f.invs, economy.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}
	depots := &storedDepots{DepotRepository: f.depots}
	f.cfg.JobAssetRunner, f.cfg.JobAssetDepots = runner, depots
	f.cfg.JobAssetHelpers = func(context.Context) ([]int, error) { return []int{}, nil }
	return depots
}

// assertUnchanged checks that a rejected command left run, hands, depot and
// receipt untouched.
func assertUnchanged(t *testing.T, f *normalFixture, depots *storedDepots, cmd adventure.RunCommand, run *adventure.NormalRun, held, stored []item.Instance) {
	t.Helper()
	if got := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(got, run) {
		t.Fatalf("%s changed the run", cmd.ID)
	}
	if got := heldItems(t, f, 0); !reflect.DeepEqual(got, held) {
		t.Fatalf("%s changed the hand: %+v", cmd.ID, got)
	}
	if got := depotItems(t, depots, cmd.ActorID); (len(got) != 0 || len(stored) != 0) && !reflect.DeepEqual(got, stored) {
		t.Fatalf("%s changed the depot: %+v", cmd.ID, got)
	}
	if got, err := f.svc.GetRunReceipt(t.Context(), cmd.PlayerID, cmd.ActorID, cmd.RunID, cmd.ID); err != nil || got != nil {
		t.Fatalf("%s stored receipt: %+v %v", cmd.ID, got, err)
	}
}

func TestRunCombatHeldToolConsumesOnceAndRejectsAtomically(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-001")
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		runActorByID(r, f.actors[0].ID).HP = 100
		r.LastActions[f.actors[0].ID] = f.clock.Now().Add(time.Hour)
	}, runEnemy("foe", 900))
	held := heldItems(t, f, 0)
	before := readStockRoom(t, f, cmd.RunID)
	early := combatCommand(cmd, "herb-early", "tool", "", "")
	if _, err := f.svc.Command(t.Context(), early); !errors.Is(err, adventure.ErrRunCooldown) {
		t.Fatalf("early tool: %v", err)
	}
	assertUnchanged(t, f, depots, early, before, held, nil)

	f.clock.add(2 * time.Hour)
	herb := combatCommand(cmd, "herb", "tool", "", "")
	receipt, err := f.svc.Command(t.Context(), herb)
	if err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	a := runActorByID(r, cmd.ActorID)
	if a.HP <= 100 || a.HeldToolID == nil || *a.HeldToolID != "" || len(heldItems(t, f, 0)) != 0 {
		t.Fatalf("herb not applied and consumed: hp=%d held=%v items=%+v", a.HP, a.HeldToolID, heldItems(t, f, 0))
	}
	f.restart(t)
	if again, err := f.svc.Command(t.Context(), herb); err != nil || !reflect.DeepEqual(again, receipt) {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if !reflect.DeepEqual(readStockRoom(t, f, cmd.RunID), r) {
		t.Fatal("replay re-executed the tool")
	}
}

func TestRunCombatAssetSessionPersistsTailGrowth(t *testing.T) {
	f, _ := assetRunCombat(t, 1)
	actor := f.actors[0]
	threshold, err := progression.ExperienceForNextLevel(actor.Level)
	if err != nil {
		t.Fatal(err)
	}
	actor.Experience = threshold
	cmd := combatRoom(t, f, actor, nil, runEnemy("foe", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "grow", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	got, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || got.Level != actor.Level+1 || got.SP <= actor.SP {
		t.Fatalf("tail growth was not persisted: level=%d SP=%d err=%v", got.Level, got.SP, err)
	}
}

// Job86.3 (_skill.cgi:6842–6856) withdraws a book, writes the default ban of 2
// (10831–10839) and sets `$npc_turn_skip`, so the eligible tail that would tick
// the ban and the NPC response are both skipped (_battle.cgi:938,1137–1150).
func TestRunCombatBookReleaseBansTwiceAndSuppressesTail(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-169")
	actor := f.actors[0].ID
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, actor)
		a.Jobs, a.MP, a.MaxMP = corebattle.BattleJobs{JobID: "job-86", SP: 100}, 200, 200
	}, runEnemy("foe", 900))
	rows := seedDepot(t, depots, actor, "item-170")
	foeTime := readStockRoom(t, f, cmd.RunID).LastActions["foe"]
	release := combatCommand(cmd, "release", "job-86-entry-3", "item-170", "")
	if _, err := f.svc.Command(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	r := readStockRoom(t, f, cmd.RunID)
	if r.Battle.SkillBans["job-86-entry-3"] != 2 || !r.LastActions["foe"].Equal(foeTime) {
		t.Fatalf("ban=%d foe=%v want 2 and no response", r.Battle.SkillBans["job-86-entry-3"], r.LastActions["foe"])
	}
	if got := heldItems(t, f, 0); len(got) != 1 || got[0] != rows[0] || *runActorByID(r, actor).HeldToolID != "item-170" {
		t.Fatalf("withdrawn book not held: %+v", got)
	}
	if got := depotItems(t, depots, actor); len(got) != 1 || got[0].DefinitionID != "item-169" {
		t.Fatalf("previous book not stored: %+v", got)
	}

	f.clock.add(time.Hour)
	cmd.Version = r.Version
	banned := combatCommand(cmd, "release-again", "job-86-entry-3", "item-169", "")
	if _, err := f.svc.Command(t.Context(), banned); !errors.Is(err, adventure.ErrRunUnavailable) {
		t.Fatalf("banned release: %v", err)
	}
	assertUnchanged(t, f, depots, banned, r, heldItems(t, f, 0), depotItems(t, depots, actor))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "foe", "")); err != nil {
		t.Fatal(err)
	}
	if got := readStockRoom(t, f, cmd.RunID).Battle.SkillBans["job-86-entry-3"]; got != 1 {
		t.Fatalf("eligible tail ban = %d, want 1", got)
	}
}

// A missing book is the source's selection message: nothing but the receipt
// commits, and the actor's interval is not spent (_skill.cgi:10489–10535).
func TestRunCombatBookSelectionErrorCommitsNothing(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-169")
	actor := f.actors[0].ID
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, actor)
		a.Jobs, a.MP, a.MaxMP = corebattle.BattleJobs{JobID: "job-86", SP: 100}, 200, 200
	}, runEnemy("foe", 900))
	rows := seedDepot(t, depots, actor, "item-001")
	before := readStockRoom(t, f, cmd.RunID)
	held := heldItems(t, f, 0)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "missing", "job-86-entry-0", "item-170", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	a, was := runActorByID(r, actor), runActorByID(before, actor)
	if a.MP != was.MP || !r.LastActions[actor].Equal(before.LastActions[actor]) || !reflect.DeepEqual(r.Battle, before.Battle) {
		t.Fatalf("selection error changed combat: mp=%d last=%v", a.MP, r.LastActions[actor])
	}
	if !reflect.DeepEqual(heldItems(t, f, 0), held) || !reflect.DeepEqual(depotItems(t, depots, actor), rows) {
		t.Fatal("selection error moved assets")
	}
}

// Job50 consumes a stored category-1 tool through its catalog child; a travel
// tool whose validation fails keeps the row and commits only the history.
func TestRunCombatStoredToolConsumesAndTravelSelectionKeepsHistory(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	rng := f.cfg.RNG.(*midRNG)
	actor := f.actors[0].ID
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, actor)
		a.Jobs, a.MP, a.HP = corebattle.BattleJobs{JobID: "job-50", SP: 200}, 50, 100
		a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-158")
	}, runEnemy("foe", 900))
	rows := seedDepot(t, depots, actor, "item-001", "item-071", "item-085")
	before := readStockRoom(t, f, cmd.RunID)
	color := runActorByID(before, actor).TeamID
	draws := rng.calls.Load()
	unbound := combatCommand(cmd, "unbound-tool", "job-50-entry-7", "item-085", "")
	if _, err := f.svc.Command(t.Context(), unbound); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatalf("unregistered stored tool: %v", err)
	}
	assertUnchanged(t, f, depots, unbound, before, nil, rows)
	if rng.calls.Load() != draws+1 { // The weapon seal table draws once when the request loads.
		t.Fatal("unregistered stored tool consumed an effect draw")
	}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "stored-herb", "job-50-entry-7", "item-001", actor)); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	if got := depotItems(t, depots, actor); !reflect.DeepEqual(got, rows[1:]) || runActorByID(r, actor).HP <= 100 || !r.Battle.StoredToolHistory[color]["item-001"] {
		t.Fatalf("stored herb: depot=%+v hp=%d history=%v", got, runActorByID(r, actor).HP, r.Battle.StoredToolHistory)
	}

	f.clock.add(time.Hour)
	cmd.Version = r.Version
	// Item071 requires the matching gear; its validation error is a selection message.
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "stored-travel", "job-50-entry-7", "item-071", "")); err != nil {
		t.Fatal(err)
	}
	after := readStockRoom(t, f, cmd.RunID)
	if after.Floor != r.Floor || after.Stage.ID != r.Stage.ID || !after.Battle.StoredToolHistory[color]["item-071"] || runActorByID(after, actor).MP != runActorByID(r, actor).MP {
		t.Fatalf("selection error: floor=%d stage=%s history=%v", after.Floor, after.Stage.ID, after.Battle.StoredToolHistory)
	}
	if got := depotItems(t, depots, actor); !reflect.DeepEqual(got, rows[1:]) {
		t.Fatalf("selection error moved the row: %+v", got)
	}
}

// Equipment-only inventories are not held-tool evidence (#1830): job78's
// enhanced tool stays gated before costs or draws.
func TestRunCombatUnknownHandGatesEnhancedTool(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "weapon-01")
	actor := f.actors[0].ID
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, actor)
		a.Jobs, a.MP = corebattle.BattleJobs{JobID: "job-78", SP: 200}, 50
	}, runEnemy("foe", 900))
	before := readStockRoom(t, f, cmd.RunID)
	rng := f.cfg.RNG.(*midRNG)
	// Baseline: the per-request seal table draws before any parsing.
	calls := rng.calls.Load()
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "nobody", "attack", "nobody", "")); !errors.Is(err, adventure.ErrRunConflict) {
		t.Fatal(err)
	}
	baseline := rng.calls.Load() - calls
	enhanced := combatCommand(cmd, "enhanced", "job-78-entry-0", "", "")
	calls = rng.calls.Load()
	if _, err := f.svc.Command(t.Context(), enhanced); err == nil {
		t.Fatal("unknown hand admitted")
	}
	assertUnchanged(t, f, depots, enhanced, before, heldItems(t, f, 0), nil)
	if rng.calls.Load()-calls != baseline {
		t.Fatal("gated entry drew")
	}
}

// A held coordinate tool assigns the floor, keeps the encounter, consumes the
// hand and lets an old enemy respond (_battle.cgi:761–1178).
func TestRunCombatHeldTravelKeepsEncounterAndResponds(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-083")
	cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "assign", "tool", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	if r.Floor != 2 || !reflect.DeepEqual(r.Enemies, []string{"foe"}) || !r.LastActions["foe"].Equal(f.clock.Now()) || len(heldItems(t, f, 0)) != 0 {
		t.Fatalf("travel: floor=%d enemies=%v foe=%v items=%+v", r.Floor, r.Enemies, r.LastActions["foe"], heldItems(t, f, 0))
	}
	if len(depotItems(t, depots, cmd.ActorID)) != 0 {
		t.Fatal("held travel touched the depot")
	}
}

// Job86.5 uses the held book on two scoped participants, then returns it (_skill.cgi:6868–6897).
func TestRunCombatBookDoubleUseTargetsBothParticipants(t *testing.T) {
	f, depots := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-169")
	actor := f.actors[0].ID
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, actor)
		a.Jobs, a.MP, a.MaxMP, a.HP = corebattle.BattleJobs{JobID: "job-86", SP: 200}, 200, 200, 100
		helper := runEnemy("helper", 500)
		helper.TeamID, helper.HP = a.TeamID, 200
		r.Battle.Actors = append(r.Battle.Actors, helper)
		r.LastActions["foe"] = f.clock.Now().Add(time.Hour)
	}, runEnemy("foe", 900))
	// At .5, each heal adds floor(30 × 1.05). The pair costs floor(6 × 1.5) once.
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "double", "job-86-entry-5", actor, "helper")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	a, helper := runActorByID(r, actor), runActorByID(r, "helper")
	if a.HP != 131 || helper == nil || helper.HP != 231 || a.MP != 191 {
		t.Fatalf("double use: hp=%d mp=%d helper=%+v", a.HP, a.MP, helper)
	}
	// Then the book returns to the depot (send_item).
	if got := depotItems(t, depots, actor); len(heldItems(t, f, 0)) != 0 || len(got) != 1 || got[0].DefinitionID != "item-169" {
		t.Fatalf("book not returned: held=%+v depot=%+v", heldItems(t, f, 0), got)
	}
}

// Roster tools reuse the calling body under their tool identity.
func TestRunCombatHeldRosterToolSummonsHelper(t *testing.T) {
	f, _ := assetRunCombat(t, 1)
	grantHeldTo(t, f, 0, "item-072")
	cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "doll", "tool", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	helper := false
	for _, a := range r.Battle.Actors {
		helper = helper || a.RosterIdentity == "dummy" && a.TeamID == runActorByID(r, cmd.ActorID).TeamID
	}
	if !helper || len(heldItems(t, f, 0)) != 0 {
		t.Fatalf("tool72: helper=%v items=%+v", helper, heldItems(t, f, 0))
	}
}
