package battle

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestTensionGemProjectionSourceVectors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id         int
		start      float64
		level      int
		items      []string
		npc, bound bool
		want       []string
	}{
		{"two calls", 67, 1, 0, nil, false, false, []string{"3/2/"}},
		{"recursive calls", 67, 1, 99, []string{"item-145"}, false, false, []string{"3/2/", "5/3/", "8/4/"}},
		{"NPC skips recursion", 67, 1, 99, []string{"item-145"}, true, false, []string{"3/2/"}},
		{"binding skips tails", 67, 5, -1, []string{"item-145", "item-161"}, false, true, []string{"5/2/"}},
		{"low requester", 74, 5, 24, nil, false, false, []string{"5/1/"}},
		{"first high-stage roll", 74, 5, 25, nil, false, false, []string{"5/1/", "8/1/"}},
		{"last uncertain requester", 74, 5, 68, nil, false, false, []string{"5/1/", "8/1/"}},
		{"guaranteed requester", 74, 5, 69, nil, false, false, []string{"8/1/"}},
		{"low defense", 74, 1, 0, []string{"item-161"}, false, false, []string{"1.7/1/", "1.7/1/defense"}},
		{"high defense", 74, 3, 0, []string{"item-161"}, false, false, []string{"5/1/", "5/1/defense", "5/1/strong_defense"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			a := s.actor(c.ActorID)
			a.Tension, a.ItemDefinitionIDs = tc.start, tc.items
			c.ActorNPC = tc.npc
			if tc.bound {
				a.Status = StatusSabaku
			}
			if tc.level >= 0 {
				c.RequesterLevel = &tc.level
			}
			branches, err := s.ProjectRecoveryStatGemBranches([]int{tc.id}, &c, c.ActorID)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool)
			for _, branch := range branches {
				a := branch.State.actor(c.ActorID)
				got[fmt.Sprintf("%g/%d/%s", a.Tension, branch.Context.TensionCalls, a.Temporary)] = true
			}
			want := make(map[string]bool)
			for _, value := range tc.want {
				want[value] = true
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("source tension/call/temporary outcomes", got, want)
			}
		})
	}
}

func TestTensionGemProjectionContainsConcreteExecutions(t *testing.T) {
	// Actual source pools include fallen participants. Seeds exercise outcomes;
	// the explicit vectors above independently specify the staged numeric oracle.
	for _, tc := range []struct {
		id        int
		target    string
		status    string
		temporary TemporaryEffect
		items     []string
		activated bool
	}{
		{54, "ally", "", "", nil, false},
		{69, "ally", "", "", nil, false},
		{67, "enemy", "", "", []string{"item-145", "item-161"}, false},
		{67, "enemy", StatusSabaku, "", []string{"item-145", "item-161"}, false},
		{74, "ally", "", TemporaryCover, nil, false},
		{74, "fallen", "", TemporaryDefense, nil, true},
		{74, "missing", StatusConfusion, TemporaryPhantom, nil, false},
		{68, "ally", "", TemporaryReflectAttack, nil, false},
		{68, "enemy", StatusPhysicalSeal, TemporaryReflectAttack, nil, true},
		{68, "enemy", "", TemporaryDeflect, nil, true},
		{68, "enemy", StatusConfusion, TemporaryNullAttack, nil, false},
		{68, "enemy", "", TemporaryPhantom, nil, false},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			s, c := recoveryFixture()
			level := 40
			c.RequesterLevel, c.Item195Activated, c.Custom = &level, tc.activated, true
			c.TensionCalls = 7
			s.actor(c.ActorID).Status, s.actor(c.ActorID).ItemDefinitionIDs = tc.status, tc.items
			for i := range s.Actors {
				s.Actors[i].Tension = 3
			}
			s.actor("ally").Temporary, s.actor("enemy").Temporary = tc.temporary, tc.temporary
			s.actor(c.ActorID).Temporary = TemporaryCovering
			before := reloadLifetime(t, s)
			c.Damage = func(TargetContext, TargetEffect, random.Generator) error {
				t.Fatal("projection executed external Apply hook")
				return nil
			}
			branches, err := s.ProjectRecoveryStatGemBranches([]int{tc.id}, &c, tc.target)
			if err != nil || !reflect.DeepEqual(s, before) || c.TensionCalls != 7 || level != 40 {
				t.Fatal("projection changed caller", err, s, c)
			}
			for seed := uint64(1); seed <= 24; seed++ {
				actual, context := s.callbackCopy(), c
				if _, err := actual.ApplyRecoveryStatGem(tc.id, &context, tc.target, random.NewDeterministic(seed)); err != nil {
					t.Fatal(seed, err)
				}
				found := false
				for _, branch := range branches {
					found = found || reflect.DeepEqual(actual, branch.State) && context.TensionCalls == branch.Context.TensionCalls
				}
				if !found {
					t.Fatal("missing execution outcome", seed, actual, context.TensionCalls, branches)
				}
			}
		})
	}
}

func TestTensionGemProjectionPreservesNestedReferents(t *testing.T) {
	s, c := recoveryFixture()
	level, weapon, sp := 99, 50, 30
	c.RequesterLevel = &level
	a := s.actor(c.ActorID)
	a.ItemDefinitionIDs = []string{"item-145", "item-161"}
	a.Gear = &BattleGearStats{WeaponAttack: &weapon}
	a.JobHistory = map[string]*int{"job-1": &sp}
	a.CustomSkills = []ActionCustomSkill{{Gems: []GemEffect{{Power: 7}}}}
	before := s.callbackCopy()
	branches, err := s.ProjectRecoveryStatGemBranches([]int{67}, &c, "ally")
	if err != nil || len(branches) < 2 {
		t.Fatal(err, branches)
	}
	first := &branches[0]
	*first.Context.RequesterLevel = 0
	first.Context.PartyIDs[0] = "changed"
	first.State.actor(c.ActorID).CustomSkills[0].Gems[0].Power = 0
	*first.State.actor(c.ActorID).Gear.WeaponAttack = 0
	*first.State.actor(c.ActorID).JobHistory["job-1"] = 0
	if level != 99 || weapon != 50 || sp != 30 || c.PartyIDs[0] != c.ActorID || !reflect.DeepEqual(s, before) || *branches[1].Context.RequesterLevel != 99 || branches[1].State.actor(c.ActorID).CustomSkills[0].Gems[0].Power != 7 {
		t.Fatal("caller/sibling references leaked", s, c, branches[1])
	}
	// Unknown predecessors never expose partial accuracy/tension writes.
	unknown, err := s.ProjectRecoveryStatGemBranches([]int{54, 19}, &c, "ally")
	if err != nil || len(unknown) != 0 || !reflect.DeepEqual(s, before) {
		t.Fatal(err, unknown, s)
	}
	if _, err := s.ProjectRecoveryStatGemBranches([]int{67, 67}, nil, "ally"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}
