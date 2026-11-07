package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type npcSelectionRNG struct {
	value float64
	calls int
}

func (r *npcSelectionRNG) Float64() float64 { r.calls++; return r.value }
func (r *npcSelectionRNG) Intn(n int) int   { return int(r.Float64() * float64(n)) }
func (r *npcSelectionRNG) IntN(n int) int   { return r.Intn(n) }

func npcSelectionFixture() (corebattle.LifetimeState, *corebattle.BaseGuardContext, corebattle.CallbackContext, BattleDefinitions) {
	s, c, d := baseGuardFixture()
	s.Actors[0].NPC = true
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}
	c.ActorNPC = true
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}
	return s, c, f, d
}

func TestNPCDefaultIndependentSPAndDuplicateBinding(t *testing.T) {
	for _, row := range []struct {
		sp, oldSP, count int
		id               string
	}{{0, 0, 1, "npc-normal-attack"}, {10, 0, 4, "job-0-entry-2"}, {0, 10, 4, "job-0-entry-2"}, {10, 9, 6, "job-0-entry-1"}} {
		s, _, f, d := npcSelectionFixture()
		s.Actors[0].Jobs.SP = row.sp
		s.Actors[0].Jobs.OldSP = row.oldSP
		rng := &npcSelectionRNG{}
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng)
		if err != nil || len(choices) != row.count || rng.calls != 0 {
			t.Fatalf("%+v: %v %+v draws%d", row, err, choices, rng.calls)
		}
		for _, choice := range choices {
			if choice.ID != row.id {
				t.Fatalf("last binding/weight: %+v", choices)
			}
		}
	}
	// A new request receives a fresh mode seed, regardless of the old actor/job.
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Jobs.SP = 30
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{}); err != nil {
		t.Fatal(err)
	}
	s.Actors[0].Jobs.SP = 0
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 1 || choices[0].ID != "npc-normal-attack" {
		t.Fatal(choices, err)
	}
	s.Actors[0].Jobs.OldSP = 10
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(result.NextSeed) != 5 {
		t.Fatal(result, err)
	}
	repeat, err := d.NPCChoices(&s, &f, result.NextSeed, &npcSelectionRNG{})
	if err != nil || len(repeat) != 3 {
		t.Fatal("same-request seed was reset", repeat, err)
	}
}

func TestNPCPreviousSPAndExtensionAdmission(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 0, OldJobID: "job-02", OldSP: 5}
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 2 || choices[1].ID != "job-2-entry-1" {
		t.Fatal(choices, err)
	}
	s.Actors[0].ItemDefinitionIDs = []string{"item-158"}
	choices, err = d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 4 {
		t.Fatal(choices, err)
	}
	// MP is listed NPC MP, independent of player item249's modifier.
	s.Actors[0].MP = 2
	s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
	choices, err = d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 1 {
		t.Fatal(choices, err)
	}
}

func TestNPCActualActionLateCostAndReload(t *testing.T) {
	for _, reload := range []bool{false, true} {
		s, c, f, d := npcSelectionFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 25, OldJobID: "starter"}
		if reload {
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var saved corebattle.LifetimeState
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			s = saved
			// Rebuild native closures against this operation's actual NPC context.
			d = BattleDefinitions{}
			d.RegisterBaseGuard(c)
		}
		before := s.Actors[0].MP
		choice, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
		if err != nil || choice.ID != "job-1-entry-3" || s.Actors[0].Attack != 200 || s.Actors[0].MP != before-8 {
			t.Fatal(choice, s.Actors[0], err)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var saved corebattle.LifetimeState
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, saved) {
			t.Fatal("post-response reload changed result")
		}
	}
}

func TestNPCProxyAndGeneratedBindingsUseCurrentRole(t *testing.T) {
	for _, job := range []string{"job-73", "job-82"} {
		s, c, f, d := npcSelectionFixture()
		if job == "job-73" {
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 20
		} else {
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 407
			d.RegisterPlayerCallbacks(PlayerCallbackContexts{BaseGuard: c}, 1)
		}
		sp := 5
		if job == "job-82" {
			sp = 10
		}
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: "starter"}
		s.Actors[0].Icon = "assets/images/placeholders/character/character-default.svg"
		s.Actors[0].ItemDefinitionIDs = []string{"item-158"}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
		if err != nil || len(choices) != 2 {
			t.Fatalf("%s %+v %v", job, choices, err)
		}
		binding := choices[1]
		if job == "job-73" && binding.ID != "job-1-entry-1" || job == "job-82" && binding.Invocation.GeneratedChildID != "job-1-entry-1" {
			t.Fatal(job, binding)
		}
		mp := s.Actors[0].MP
		if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999}); err != nil {
			t.Fatal(job, err)
		}
		if s.Actors[0].MP != mp-binding.MPCost || !s.Actors[0].NPC {
			t.Fatal(job, s.Actors[0])
		}
	}
}

func TestNPCCustomOrderedRepeatedSlotsAndUnsupportedClosure(t *testing.T) {
	s, c, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 200
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "custom", Gems: [3]string{"gem_guard_1", "gem_guard_1", "gem_guard_1"}, GemCosts: [3]int{20, 20, 20}, CustomCost: 72}
	choice, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
	if err != nil || !choice.Custom || len(choice.Invocations) != 3 || s.Actors[0].CMP != 128 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(choice, s.Actors[0], err)
	}
	// A primitive scalar payload cannot admit an unknown explicit descendant.
	s.Actors[0].NPCLoadout.Gems[2] = "gem-101"
	d.Gems = map[string]corebattle.GemEffect{"gem-101": {Kind: corebattle.ActionKindHeal, Power: 100}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	rng := &npcSelectionRNG{}
	if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || rng.calls != 0 {
		t.Fatal("unsupported descendant drew/charged/executed")
	}
	// Empty production registry cannot certify a job table using a primitive.
	d = BattleDefinitions{}
	f.Damage = &c.DamageContext
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNPCCustomOriginalOrder(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 200
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "ordered", Gems: [3]string{"gem_guard_1", "gem_big_guard_sky", "gem_guard_1"}, GemCosts: [3]int{20, 37, 20}, CustomCost: 92}
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
	if err != nil || !result.Custom || s.Actors[0].Temporary != corebattle.TemporaryDefense || s.Actors[0].CMP != 108 {
		t.Fatal(result, s.Actors[0], err)
	}
}

func TestNPCUnsupportedProducerCannotBypassSelection(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Abilities = []string{"specter"}
	rng := &npcSelectionRNG{}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || rng.calls != 0 {
		t.Fatal("unsupported producer mutated/drew")
	}
}

func TestNPCCustomNameOverridesPriorBindingsWithoutLosingWeight(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].Jobs.SP = 8
	s.Actors[0].CMP = 100
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "normal-attack", Gems: [3]string{"gem_guard_1"}, GemCosts: [3]int{20}, CustomCost: 20}
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 3 {
		t.Fatal(choices, err)
	}
	for _, choice := range choices {
		if !choice.Custom {
			t.Fatal("last custom binding lost", choices)
		}
	}
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || !result.Custom || s.Actors[0].CMP != 80 || s.Actors[0].Temporary != corebattle.TemporaryDefense || f.Damage.DamageCalls != 0 {
		t.Fatal(result, s.Actors[0], err)
	}
}

// EX-only NPC jobs have no registered tables yet (#1914–#1916); their EX bosses
// must reject the whole response before draws or mutation.
func TestNPCEXOnlyJobsRemainWholeActionGates(t *testing.T) {
	for _, job := range []string{"job-405", "job-406", "job-407"} {
		s, _, f, d := npcSelectionFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: job, SP: 1000, OldJobID: "starter"}
		before, _ := json.Marshal(s)
		rng := &npcSelectionRNG{}
		if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
		if after, _ := json.Marshal(s); string(after) != string(before) || rng.calls != 0 {
			t.Fatal(job, "gate mutated or drew")
		}
	}
}
