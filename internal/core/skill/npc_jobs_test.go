package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type npcJobRNG struct {
	t     *testing.T
	draws []float64
}

func (r *npcJobRNG) Float64() float64 {
	r.t.Helper()
	if len(r.draws) == 0 {
		r.t.Fatal("unexpected random draw")
	}
	v := r.draws[0]
	r.draws = r.draws[1:]
	return v
}

func (r *npcJobRNG) IntN(int) int   { r.t.Fatal("unexpected integer draw"); return 0 }
func (r *npcJobRNG) Intn(n int) int { return r.IntN(n) }

func npcJobFixture(npc bool) (corebattle.LifetimeState, corebattle.RosterContext, BattleDefinitions) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "player", TeamID: "party", HP: 100, MP: 100}},
		{Participant: corebattle.Participant{ID: "ally", TeamID: "party", HP: 100}},
		{Participant: corebattle.Participant{ID: "npc", TeamID: "enemy", HP: 100, MP: 100}, NPC: true},
		{Participant: corebattle.Participant{ID: "other-npc", TeamID: "enemy", HP: 100}, NPC: true},
	}}
	c := corebattle.RosterContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{
		ActorID: "player", PartyIDs: []string{"player", "ally"}, EnemyIDs: []string{"npc", "other-npc"},
	}}}, EnemyTeamID: "enemy", EnemyPool: []corebattle.LifetimeActor{{
		Participant: corebattle.Participant{ID: "species", Name: "Stage creature", HP: 100, MP: 60, MaxHP: 900, Gear: &corebattle.BattleGearStats{},
			CustomSkills: []corebattle.ActionCustomSkill{{ID: "deferred", CMPCost: 20, Gems: []corebattle.GemEffect{{Kind: "unsupported-until-selected"}}}}},
		Jobs: corebattle.BattleJobs{JobID: "job-95", SP: 99, OldJobID: "job-99"},
	}}, AppearancePositions: []int{0, 0, 999}}
	if npc {
		c.ActorID, c.ActorNPC = "npc", true
		c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	}
	var d BattleDefinitions
	// Callers rebuild closures against their operation context after reload.
	return s, c, d
}

func applyNPCJob(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RosterContext, d BattleDefinitions, id string, rng random.Generator) {
	t.Helper()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"unused-target", "unused-tail"}}
	ops, err := d.InvocationAction(s, &frame, []string{id}, rng)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frame.Arguments, []string{"unused-target", "unused-tail"}) {
		t.Fatal(frame.Arguments)
	}
}

func TestNPCJobPositionsRegistrationAndWeightedInvocation(t *testing.T) {
	want := []corebattle.CallbackEntry{
		{ID: "npc-job-95-position-0", Name: "stage-summon", MPCost: 50},
		{ID: "npc-job-95-position-1", Name: "stage-summon", MPCost: 50},
	}
	if !reflect.DeepEqual(corebattle.NPCJobEntries(95), want) {
		t.Fatal(corebattle.NPCJobEntries(95))
	}
	if got := corebattle.NPCJobEntries(99); !reflect.DeepEqual(got, []corebattle.CallbackEntry{{ID: "npc-job-99-position-0", Name: "escape"}}) {
		t.Fatal(got)
	}
	if got := corebattle.NPCJobEntries(96); len(got) != 0 {
		t.Fatal(got)
	}
	for _, draw := range []float64{0, .999} {
		s, c, d := npcJobFixture(true)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &frame, []string{want[0].ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		d.RegisterNPCJobs(&c)
		if !reflect.DeepEqual(d.Invocations.JobTables[95].Variants, [][]corebattle.CallbackEntry{want}) {
			t.Fatal(d.Invocations.JobTables[95])
		}
		// The NPC consumer retains both names as weights, resolving each through
		// the last admitted same-name binding, rather than deduplicating positions.
		bindings := map[string]corebattle.CallbackEntry{}
		for _, entry := range want {
			bindings[entry.Name] = entry
		}
		selected := bindings[want[int(draw*float64(len(want)))].Name]
		if selected.ID != want[1].ID {
			t.Fatal(selected)
		}
		applyNPCJob(t, &s, &c, d, selected.ID, &npcJobRNG{t: t, draws: []float64{0, 0}})
		if len(s.Actors) != 5 || s.Actors[2].MP != 100 {
			t.Fatal(s)
		}
		// Only the operation/receipt owner debits the selected cost, once afterward.
		s.Actors[2].MP -= selected.MPCost
		if s.Actors[2].MP != 50 {
			t.Fatal(s.Actors[2].MP)
		}
	}
}

func TestNPCSummonPositionsRolesAndNoEffect(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, entry := range corebattle.NPCJobEntries(95) {
			s, c, d := npcJobFixture(npc)
			d.RegisterNPCJobs(&c)
			before, _ := json.Marshal(c.EnemyPool)
			applyNPCJob(t, &s, &c, d, entry.ID, &npcJobRNG{t: t, draws: []float64{0, 0}})
			a := s.Actors[4]
			if !a.NPC || a.TeamID != "enemy" || a.RosterIdentity != "enemy-species-I" || a.MaxHP != 900 || a.Jobs.JobID != "job-95" || a.CustomSkills[0].Gems[0].Kind != "unsupported-until-selected" {
				t.Fatal(a)
			}
			if npc && !reflect.DeepEqual(c.PartyIDs, []string{"npc", "other-npc", a.ID}) || !npc && !reflect.DeepEqual(c.EnemyIDs, []string{"npc", "other-npc", a.ID}) {
				t.Fatal(c.TargetContext)
			}
			after, _ := json.Marshal(c.EnemyPool)
			if string(before) != string(after) {
				t.Fatal("stage payload changed")
			}
			applyNPCJob(t, &s, &c, d, entry.ID, &npcJobRNG{t: t, draws: []float64{0, 0}})
			if s.Actors[5].RosterIdentity != "enemy-species-Z" {
				t.Fatal(s.Actors[5])
			}
			applyNPCJob(t, &s, &c, d, entry.ID, &npcJobRNG{t: t, draws: []float64{0, 0}})
			if len(s.Actors) != 6 || s.RosterSequence != 2 {
				t.Fatal("collision should be accepted no-effect", s)
			}
			actorIndex := 0
			if npc {
				actorIndex = 2
			}
			s.Actors[actorIndex].MP -= entry.MPCost
			if s.Actors[actorIndex].MP != 50 {
				t.Fatal("no-effect summon cost", s.Actors[actorIndex])
			}
			for len(s.Actors) < 10 {
				a := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: string(rune('a' + len(s.Actors))), TeamID: "enemy", HP: 1}, NPC: true}
				s.Actors = append(s.Actors, a)
				if npc {
					c.PartyIDs = append(c.PartyIDs, a.ID)
				} else {
					c.EnemyIDs = append(c.EnemyIDs, a.ID)
				}
			}
			c.EnemyPool, c.EnemyTeamID = nil, ""
			applyNPCJob(t, &s, &c, d, entry.ID, noArithmeticDraws{t})
			if len(s.Actors) != 10 || s.RosterSequence != 2 {
				t.Fatal(s)
			}
		}
	}
}

func TestNPCJobsRejectWholeEntryBeforeMutation(t *testing.T) {
	for _, id := range []string{"npc-job-95-position-0", "npc-job-95-position-1", "npc-job-99-position-0"} {
		s, c, d := npcJobFixture(true)
		d.RegisterNPCJobs(&c)
		entry := d.Invocations.Entries[id]
		entry.Children = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "unsupported-later"}}
		d.Invocations.Entries[id] = entry
		before, _ := json.Marshal(s)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) || c.DamageCalls != 0 {
			t.Fatal("rejection mutated state/cost/counters")
		}
		d.RegisterNPCJobs(&c)
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}, {Kind: "entry", EntryID: "unsupported-later"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		after, _ = json.Marshal(s)
		if string(before) != string(after) {
			t.Fatal("unsupported later slot executed earlier entry")
		}
	}
	s, c, d := npcJobFixture(true)
	c.EnemyPool = nil
	d.RegisterNPCJobs(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &frame, []string{"npc-job-95-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	c.EnemyPool = []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "invalid"}}}
	if _, err := d.InvocationAction(&s, &frame, []string{"npc-job-95-position-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrInvalidParticipant) {
		t.Fatal(err)
	}
}

func TestNPCJobDelegationAndDeferredSpawnedSelection(t *testing.T) {
	for _, draw := range []float64{0, .999} {
		s, c, d := npcJobFixture(true)
		s.Actors[2].Jobs = corebattle.BattleJobs{JobID: "job-95"}
		s.Actors[2].MP = 0
		c.EnemyPool[0].Jobs.JobID = "job-99"
		d.RegisterNPCJobs(&c)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, &npcJobRNG{t: t, draws: []float64{draw, 0, 0}}); err != nil {
			t.Fatal(err)
		}
		if len(s.Actors) != 5 || s.Actors[4].HP != 100 || s.Actors[2].MP != 0 {
			t.Fatal("spawn executed prematurely or delegated listed cost charged", s)
		}
		// A later operation selects the new NPC's callback against its own frame.
		c.ActorID = s.Actors[4].ID
		frame.TargetContext = c.TargetContext
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, &npcJobRNG{t: t, draws: []float64{.5}}); err != nil {
			t.Fatal(err)
		}
		if s.Actors[4].HP != 0 || s.Actors[2].HP != 100 || len(s.Actors) != 5 {
			t.Fatal(s)
		}
	}
}

func TestNPCEscapeOnlyWritesHPAndPrunesAtSave(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, c, d := npcJobFixture(npc)
		index := 0
		if npc {
			index = 2
		}
		a := &s.Actors[index]
		a.Status, a.Temporary, a.Extended, a.ExtendedTurns, a.Tension = corebattle.StatusPoison, corebattle.TemporaryRevival, corebattle.ExtendedFlame, 4, 3
		a.ExperienceReward, a.GoldReward, a.CrystalReward = 100, 200, 3
		want := *a
		want.HP = 0
		beforeContext := c.TargetContext
		c.Defeat = func(string) error { t.Fatal("escape invoked defeat"); return nil }
		d.RegisterNPCJobs(&c)
		applyNPCJob(t, &s, &c, d, "npc-job-99-position-0", noArithmeticDraws{t})
		if !reflect.DeepEqual(s.Actors[index], want) || !reflect.DeepEqual(c.TargetContext, beforeContext) || s.DroppedCrystals != 0 || c.DamageCalls != 0 {
			t.Fatal(s, c)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var loaded corebattle.LifetimeState
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, loaded) {
			t.Fatal("escape facts changed on reload")
		}
		loaded.PruneRoster(&c.TargetContext)
		if npc && (len(loaded.Actors) != 3 || !reflect.DeepEqual(c.PartyIDs, []string{"other-npc"}) || !reflect.DeepEqual(c.EnemyIDs, []string{"player", "ally"})) {
			t.Fatal(loaded, c)
		}
		if !npc && (len(loaded.Actors) != 4 || loaded.Actors[0].HP != 0) {
			t.Fatal("fallen real actor must remain", loaded)
		}
	}
}

func TestNPCSummonFrozenReloadContinuation(t *testing.T) {
	s, c, d := npcJobFixture(true)
	d.RegisterNPCJobs(&c)
	applyNPCJob(t, &s, &c, d, "npc-job-95-position-0", &npcJobRNG{t: t, draws: []float64{0, 0}})
	type snapshotData struct {
		State     corebattle.LifetimeState
		Targets   corebattle.TargetContext
		Pool      []corebattle.LifetimeActor
		Positions []int
	}
	snapshot := snapshotData{s, c.TargetContext, c.EnemyPool, c.AppearancePositions}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var loaded snapshotData
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, c2, d2 := npcJobFixture(true)
	c2.TargetContext, c2.EnemyPool, c2.AppearancePositions = loaded.Targets, loaded.Pool, loaded.Positions
	d2.RegisterNPCJobs(&c2)
	applyNPCJob(t, &s, &c, d, "npc-job-95-position-1", &npcJobRNG{t: t, draws: []float64{0, .999}})
	applyNPCJob(t, &loaded.State, &c2, d2, "npc-job-95-position-1", &npcJobRNG{t: t, draws: []float64{0, .999}})
	if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(c.TargetContext, c2.TargetContext) || !reflect.DeepEqual(c.EnemyPool, c2.EnemyPool) {
		t.Fatal("reload changed continuation")
	}
}
