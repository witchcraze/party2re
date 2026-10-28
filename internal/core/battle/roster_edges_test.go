package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDismissalKeepsRequestOrderUntilSave(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 61)
	party := append([]string(nil), c.PartyIDs...)
	if r, err := s.ApplyRoster(&c, "job-61-dismiss", "", &targetRNG{t: t}); err != nil || !r.Applied {
		t.Fatal(r, err)
	}
	if s.actor(id).HP != 0 || s.actor(id).TeamID != "npc" || !reflect.DeepEqual(c.PartyIDs, party) {
		t.Fatal(s.Actors, c)
	}
	// A later callback in this same request can still revive the dismissed
	// member through the original party array. Reload then classifies its color.
	s.actor(id).HP = 1
	if err := s.validateTargetContext(c.TargetContext, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	s.PruneRoster(&c.TargetContext)
	if !reflect.DeepEqual(c.PartyIDs, []string{"player"}) || !reflect.DeepEqual(c.EnemyIDs, []string{"enemy", id}) || s.actor(id).RosterDetached {
		t.Fatal(c)
	}
	if err := s.validateTargetContext(c.TargetContext, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeCapsUseSummonerBaselinesWithoutChangingMaxima(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 59)
	a := s.actor("player")
	a.OverLevel = true
	a.HP = 1400
	a.MaxHP = 1500
	a.MP = 1100
	a.MaxMP = 1200
	a.Attack = 1600
	a.Defense = 1700
	a.Agility = 1500
	a.BaseStats = BattleBaseStats{500, 800, 400}
	h := s.actor(id)
	h.HP = 500
	h.MP = 300
	h.Attack = 1000
	h.Defense = 1000
	h.Agility = 1000
	if _, err := s.ApplyRoster(&c, "job-59-merge", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if [5]int{a.HP, a.MP, a.Attack, a.Defense, a.Agility} != ([5]int{1500, 1200, 1500, 1998, 1200}) || a.MaxHP != 1500 || a.MaxMP != 1200 || h.HP != 0 || h.Attack != 999 {
		t.Fatal(a, h)
	}
}

func TestDuplicationRollsCurrentStatsRetainsBaseAndPreSpawnCount(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 63)
	a := s.actor(id)
	a.HP = 20
	a.MP = 30
	a.Attack = 300
	a.BaseStats.Attack = 80
	a.Temporary = TemporaryDefense
	s.actor("player").HP = 1
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0, 0, 0}}
	r, err := s.ApplyRoster(&c, "job-63-duplicate", "", rng)
	if err != nil {
		t.Fatal(err)
	}
	rng.done()
	copy := s.actor(r.HelperID)
	if copy == nil || copy.Attack != 270 || copy.BaseStats.Attack != 80 || copy.MP != 30 || copy.HP != 65 || copy.Icon != s.actor(id).Icon || copy.Temporary != TemporaryDefense || s.actor("player").HP != 91 || c.PartyHealCalls != 1 {
		t.Fatal(r, s.Actors, c)
	}
	// A fallen present copy blocks recreation. Both present records still count.
	copy.HP = 0
	count := len(s.Actors)
	if _, err := s.ApplyRoster(&c, "job-63-duplicate", "", random.NewDeterministic(1)); err != nil || len(s.Actors) != count {
		t.Fatal(err, len(s.Actors))
	}
	// Remove the original; the living copy can recreate its counterpart.
	copy.HP = 1
	s.actor(id).HP = 0
	s.PruneRoster(&c.TargetContext)
	if r, err := s.ApplyRoster(&c, "job-63-duplicate", "", random.NewDeterministic(1)); err != nil || s.actor(r.HelperID).RosterIdentity != callingIdentity(63) {
		t.Fatal(r, err)
	}
}

func TestSmallHelpersDoNotRespawnFallenPresentRecords(t *testing.T) {
	s, c := rosterFixture()
	addRosterHelper(t, &s, &c, 69)
	rng := &targetRNG{t: t, draws: make([]float64, 15)}
	if _, err := s.ApplyRoster(&c, "job-69-small", "", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if len(c.PartyIDs) != 5 || len(s.Actors) != 6 {
		t.Fatal(c)
	}
	for _, key := range []string{"small-0", "small-1", "small-2"} {
		if s.rosterIdentity(key).HP != 8 {
			t.Fatal(s.rosterIdentity(key))
		}
	}
	s.rosterIdentity("small-1").HP = 0
	if _, err := s.ApplyRoster(&c, "job-69-small", "", &targetRNG{t: t}); err != nil || len(s.Actors) != 6 {
		t.Fatal(err)
	}
}

func TestGrowthPreservesStatusTensionAndIdentityAcrossReload(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 87)
	a := s.actor(id)
	a.Status = StatusPoison
	a.Tension = 3
	a.Extended = ExtendedIncense
	a.ExtendedTurns = 5
	c.RequesterItemID = "item-165"
	rng := &targetRNG{t: t, draws: []float64{.1999, 0, 0, 0, 0, 0}}
	if _, err := s.ApplyRoster(&c, "job-87-defend", "", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if a.ID != id || a.HP != 324 || a.MaxHP != 324 || a.SummonForm != 1 || a.Status != StatusPoison || a.Tension != 3 || a.Temporary != "" || a.ExtendedTurns != 5 {
		t.Fatal(a)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.ApplyRoster(&c, "job-87-defend", "", &targetRNG{t: t, draws: []float64{.2}}); err != nil || loaded.actor(id).SummonForm != 1 || loaded.actor(id).Temporary != TemporaryDefense {
		t.Fatal(err, loaded.actor(id))
	}
	if _, err := loaded.ApplyRoster(&c, "job-87-defend", "", &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0}}); err != nil || loaded.actor(id).SummonForm != 2 {
		t.Fatal(err, loaded.actor(id))
	}
	if _, err := loaded.ApplyRoster(&c, "job-87-defend", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
}

func TestToolIdentitiesJobPayloadAndNoEffectPrerequisites(t *testing.T) {
	s, c := rosterFixture()
	for _, id := range []string{"tool-72", "tool-216", "extension-244"} {
		draws := make([]float64, 5)
		if id == "extension-244" {
			draws = append([]float64{.99, .99}, draws...)
		}
		if _, err := s.ApplyRoster(&c, id, "", &targetRNG{t: t, draws: draws}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Actors) != 5 || s.rosterIdentity("snow").SummonForm != 2 {
		t.Fatal(s.Actors)
	}
	c.RequesterLevel = nil
	if _, err := s.ApplyRoster(&c, "extension-244", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	p := HelperPayload{Identity: "payload", Name: "援護者", Stats: [5]float64{10, 20, 30, 40, 50}, Jobs: BattleJobs{JobID: "job-2", SP: 42, OldJobID: "job-3", OldSP: 17}}
	if _, err := s.AddHelper(&c, p, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	level := 40
	c.RequesterLevel = &level
	r, err := s.AddHelper(&c, p, &targetRNG{t: t, draws: make([]float64, 5)})
	if err != nil || s.actor(r.HelperID).Jobs != p.Jobs {
		t.Fatal(r, err)
	}
}

func TestRosterAdmissionKeepsStateCountersAndRNGUnchanged(t *testing.T) {
	for _, id := range []string{"job-61-special", "job-62-special", "job-63-special", "job-64-special", "job-66-special", "job-67-special", "job-71-special", "job-80-special", "job-87-special", "job-87-venom", "job-64-grave", "tool-110"} {
		s, c := rosterFixture()
		if job, _, ok := rosterCommand(id); ok {
			addRosterHelper(t, &s, &c, job)
		}
		if id == "job-87-venom" {
			s.actor("enemy").MaxHP = 999
		}
		// Every supported body still rejects a missing reached damage/status
		// prerequisite; unsupported identities remain rejected independently.
		s.actor("enemy").HP, s.actor("enemy").MaxHP = 999, 999
		s.actor("enemy").Gear = nil
		before, _ := json.Marshal(s)
		context := c
		if _, err := s.ApplyRoster(&c, id, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		after, _ := json.Marshal(s)
		if string(after) != string(before) || !reflect.DeepEqual(c, context) {
			t.Fatal(id, "changed rejected operation")
		}
	}
}
