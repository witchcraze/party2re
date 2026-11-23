package adventure_test

import (
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
)

// stage edits the saved run between commands: the room state a test starts
// from. It is not a command, so it grants nothing and renews no activity.
func (f *normalFixture) stage(t *testing.T, runID string, edit func(*adventure.NormalRun)) {
	t.Helper()
	cmd := runhost.Command{RunID: runID, Version: readStockRoom(t, f, runID).Version}
	editStockRoom(t, f, &cmd, edit)
}

// rewardEnemy is an NPC any attack fells under the fixture RNG: bound (no
// dodge), 1 HP, and an uploaded icon (no capture roll). It is worth 3 EXP,
// 5 gold and a crystal before party bonuses.
func rewardEnemy(id string) corebattle.LifetimeActor {
	e := runEnemy(id, 1)
	e.Status, e.Icon = corebattle.StatusKinju, "upload/test"
	e.ExperienceReward, e.GoldReward, e.CrystalReward = 3, 5, 1
	return e
}

// earnCommand adds a reward enemy beside any encounter and returns actor's
// attack on it, ready to submit after the action interval. Combat needs a
// floor, so a room still at round 0 moves to floor 1.
func (f *normalFixture) earnCommand(t *testing.T, runID string, actor int) runhost.Command {
	t.Helper()
	e := rewardEnemy("reward-" + id.New()[:8])
	f.stage(t, runID, func(r *adventure.NormalRun) {
		r.Round = max(r.Round, 1)
		r.Battle.Actors = append(r.Battle.Actors, e)
		r.Enemies = append(r.Enemies, e.ID)
	})
	f.clock.add(3 * time.Second)
	c := f.actors[actor]
	return runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: readStockRoom(t, f, runID).Version, Action: "attack", TargetID: e.ID}
}

// earn fells one reward enemy with a real operation; a room at round 0
// returns there afterwards.
func (f *normalFixture) earn(t *testing.T, runID string, actor int) runhost.Receipt {
	t.Helper()
	entry := readStockRoom(t, f, runID).Round == 0
	result, err := f.svc.Command(t.Context(), f.earnCommand(t, runID, actor))
	if err != nil {
		t.Fatal(err)
	}
	if entry {
		f.stage(t, runID, func(r *adventure.NormalRun) { r.Round = 0 })
	}
	return result
}

// kill fells the current encounter with a real operation: its members are
// replaced by one reward enemy on the same encounter floor.
func (f *normalFixture) kill(t *testing.T, runID string, actor int) runhost.Receipt {
	t.Helper()
	f.stage(t, runID, func(r *adventure.NormalRun) { r.DiscardEncounter() })
	return f.earn(t, runID, actor)
}

// travel10 moves the room to floor 10 with its encounter gone, as a
// coordinate tool leaves it.
func travel10(r *adventure.NormalRun) {
	r.Round = 10
	r.DiscardEncounter()
}

// fall leaves the actor unconscious in the room.
func fall(actor string) func(*adventure.NormalRun) {
	return func(r *adventure.NormalRun) { runActorByID(r, actor).HP = 0 }
}

// withActor applies edit to actor's room combatant.
func withActor(actor string, edit func(*corebattle.LifetimeActor)) func(*adventure.NormalRun) {
	return func(r *adventure.NormalRun) { edit(runActorByID(r, actor)) }
}

// ex11 moves the room to stageID's cleared ordinary treasure floor with EX
// unlocked, every chest opened and the actor at 1 HP and 0 MP.
func ex11(t *testing.T, actor, stageID string) func(*adventure.NormalRun) {
	t.Helper()
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID(stageID)
	if err != nil {
		t.Fatal(err)
	}
	return func(r *adventure.NormalRun) {
		r.DiscardEncounter()
		r.Stage, r.Round, r.EXUnlocked = stage.OrdinarySeason(), adventure.TreasureRoomFloor, true
		a := runActorByID(r, actor)
		a.HP, a.MP = 1, 0
	}
}

// idleCommand adds a sturdy enemy that cannot answer and returns actor's
// attack on it: an accepted operation that grants nothing. Like
// earnCommand, it moves a room at round 0 to floor 1.
func (f *normalFixture) idleCommand(t *testing.T, runID string, actor int) runhost.Command {
	t.Helper()
	e := runEnemy("idle-"+id.New()[:8], 900)
	f.stage(t, runID, func(r *adventure.NormalRun) {
		r.Round = max(r.Round, 1)
		r.Battle.Actors = append(r.Battle.Actors, e)
		r.Enemies = append(r.Enemies, e.ID)
		r.LastActions[e.ID] = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	f.clock.add(3 * time.Second)
	c := f.actors[actor]
	return runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: readStockRoom(t, f, runID).Version, Action: "attack", TargetID: e.ID}
}
