package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestRevivalGemIndependentPartyChanceAndHalfFullOracles(t *testing.T) {
	s, c := revivalFixture()
	s.actor("ally").HP = 0
	branches, err := s.ProjectRevivalGemBranches(&c, "gem-65", "")
	if err != nil || len(branches) != 4 {
		t.Fatal(len(branches), err)
	}
	combinations := map[[2]int]bool{}
	for _, branch := range branches {
		combinations[[2]int{branch.State.actor("fallen").HP, branch.State.actor("ally").HP}] = true
	}
	for _, hp := range [][2]int{{0, 0}, {0, 50}, {50, 0}, {50, 50}} {
		if !combinations[hp] {
			t.Fatal("lost independent revival", hp)
		}
	}
	for _, draws := range [][]float64{{.49999, .5}, {.5, .49999}} {
		p := s.callbackCopy()
		child := c
		rng := &targetRNG{t: t, draws: draws}
		if _, err = p.ApplyRevival(&child, "gem-65", "", rng); err != nil {
			t.Fatal(err)
		}
		rng.done()
		if p.actor("fallen").HP != map[bool]int{false: 0, true: 50}[draws[0] < .5] || p.actor("ally").HP != map[bool]int{false: 0, true: 50}[draws[1] < .5] {
			t.Fatal(p)
		}
	}
	for _, id := range []string{"gem-63", "gem-64"} {
		states, err := s.ProjectRevivalGemBranches(&c, id, "fallen")
		want := 50
		if id == "gem-64" {
			want = 101
		}
		if err != nil || len(states) != 1 || states[0].State.actor("fallen").HP != want {
			t.Fatal(id, states, err)
		}
	}
}

func TestRevivalGemSealAbsorptionAndNestedIsolation(t *testing.T) {
	s, c := revivalFixture()
	s.actor("self").Status = StatusMagicSeal
	s.actor("fallen").MP, s.actor("fallen").Temporary = 0, TemporaryAbsorbMagic
	level, weapon, sp := 70, 12, 30
	s.actor("fallen").Gear.WeaponAttack = &weapon
	s.actor("fallen").JobHistory = map[string]*int{"job-01": &sp}
	c.NativeDefeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{RequesterLevel: &level, Seals: &WeaponSealTable{DragonAttack: 7}}}
	states, err := s.ProjectRevivalGemBranches(&c, "gem-63", "fallen")
	if err != nil || len(states) != 21 {
		t.Fatal(len(states), err)
	}
	mp := map[int]bool{}
	for _, state := range states {
		if state.State.actor("self").Status == StatusMagicSeal {
			if state.State.actor("fallen").HP != 0 || state.State.actor("fallen").MP != 0 {
				t.Fatal(state)
			}
		} else {
			if state.State.actor("fallen").HP != 50 {
				t.Fatal(state)
			}
			mp[state.State.actor("fallen").MP] = true
		}
	}
	for value := 30; value < 50; value++ {
		if !mp[value] {
			t.Fatal("missing absorption", value)
		}
	}
	first := &states[0]
	*first.State.actor("fallen").Gear.WeaponAttack = 99
	*first.State.actor("fallen").JobHistory["job-01"] = 99
	first.Context.PartyIDs[0] = "changed"
	*first.Context.NativeDefeat.RequesterLevel = 99
	first.Context.NativeDefeat.Seals.DragonAttack = 99
	if weapon != 12 || sp != 30 || level != 70 || c.PartyIDs[0] != "self" || c.NativeDefeat.Seals.DragonAttack != 7 || *states[1].State.actor("fallen").Gear.WeaponAttack != 12 || states[1].Context.PartyIDs[0] != "self" || *states[1].Context.NativeDefeat.RequesterLevel != 70 {
		t.Fatal("projection aliased caller or sibling")
	}
}

func TestStatusGemNoEffectAndConversionKeepCompletePredecessor(t *testing.T) {
	for _, mode := range []string{"immunity", "null", "zero", "conversion", "magic sword", "empty"} {
		t.Run(mode, func(t *testing.T) {
			s, c := statusFixture()
			id := "gem_instant_death_1"
			draws := []float64{}
			switch mode {
			case "immunity":
				s.actor("enemy").HP = 1000
				s.actor("enemy").Gear = nil
			case "null":
				s.actor("enemy").Temporary = TemporaryNullMagic
			case "zero":
				id = "gem_inflict_poison"
				s.actor("self").ItemDefinitionIDs = []string{"item-239"}
				c.DamageCalls = 1
				draws = []float64{0}
			case "conversion":
				id = "gem_inflict_seal_atk"
				s.actor("self").Abilities = []string{"seal_kotowari"}
			case "magic sword":
				s.actor("self").Extended = ExtendedMagicSword
				c.Command = "attack"
			case "empty":
				s.Actors = s.Actors[:3]
				c.EnemyIDs = nil
				c.Item195Activated = true
				draws = []float64{0}
			}
			before := s.callbackCopy()
			states, err := s.ProjectStatusGemBranches(&c, id, "enemy")
			if err != nil || len(states) == 0 {
				t.Fatal(states, err)
			}
			if _, err = s.ApplyStatusEntry(&c, nil, id, "enemy", &targetRNG{t: t, draws: draws}); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, state := range states {
				if reflect.DeepEqual(state.State, s) {
					found = true
				}
			}
			if !found {
				t.Fatal("missing no-effect continuation", s, c, states)
			}
			if mode == "conversion" && s.actor("self").MP != 0 {
				t.Fatal("lost seal12 debit", s)
			}
			if mode == "magic sword" && s.actor("enemy").HP != before.actor("enemy").HP {
				t.Fatal("symbolic conversion must zero payload")
			}
		})
	}
}

func TestStatusGemReflectedConfusionAndFallenCureFallback(t *testing.T) {
	s, c := statusFixture()
	s.actor("enemy").Temporary = TemporaryReflectMagic
	s.actor("dead_enemy").HP = 100
	states, err := s.ProjectStatusGemBranches(&c, "gem_inflict_confusion", "")
	if err != nil {
		t.Fatal(err)
	}
	selfAndAlly := false
	for _, state := range states {
		if state.State.actor("self").Status == StatusConfusion && state.State.actor("ally").Status == StatusConfusion {
			selfAndAlly = true
		}
	}
	if !selfAndAlly {
		t.Fatal("later helper ignored reflected confusion", states)
	}
	// A fallen cure target falls back to the actor, including62's first helper.
	s.actor("self").Status = StatusSleep
	states, err = s.ProjectStatusGemBranches(&c, "gem_cure_all_abnormal", "fallen")
	if err != nil || len(states) != 1 || states[0].State.actor("self").Status != "" {
		t.Fatal(states, err)
	}
}

func TestStatusGemDeathProjectsCoverResetRewardsAndCursor(t *testing.T) {
	s, c := statusFixture()
	s.actor("enemy").Temporary = TemporaryCover
	s.actor("dead_enemy").Temporary = TemporaryCovering
	s.actor("dead_enemy").HP = 100
	s.actor("dead_enemy").Gear.AccessoryID = "item-159"
	applyCalls := 0
	c.Defeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return nil },
		Apply:    func(*LifetimeState, TargetContext, string) error { applyCalls++; return nil },
		Project: func(s *LifetimeState, _ TargetContext, _ string) ([]LifetimeState, error) {
			p := s.callbackCopy()
			p.Fields.Temporary = "sanctuary"
			return []LifetimeState{p}, nil
		},
	}}
	states, err := s.ProjectStatusGemBranches(&c, "gem_instant_death_sky", "")
	if err != nil || applyCalls != 0 {
		t.Fatal(err, applyCalls)
	}
	seenReset, seenLater := false, false
	for _, state := range states {
		if state.Context.Defeat.Cursor == (TargetCursor{Source: "item159", ActorID: "dead_enemy"}) {
			seenReset = true
			if state.State.actor("enemy").HP == 100 && state.State.actor("dead_enemy").HP == 0 && state.State.Fields.Current() == "sanctuary" {
				seenLater = true
			}
		}
	}
	if !seenReset || !seenLater {
		t.Fatal("lost ordered cover/reward/context transition", states)
	}
}

func TestSacrificeGemAdmitsMissingContextOnlyWhenUnreachable(t *testing.T) {
	s, c := revivalFixture()
	s.actor("self").Abilities = []string{"pharaoh"}
	s.actor("self").MP = 81
	c.NativeDefeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return ErrUnsupportedEffect },
		Apply:    func(*LifetimeState, TargetContext, string) error { t.Fatal("projection called Apply"); return nil },
	}}
	states, err := s.ProjectRevivalGemBranches(&c, "gem-66", "")
	if err != nil || len(states) != 1 || states[0].State.actor("self").HP != 101 || states[0].State.actor("self").MP != 0 || s.actor("fallen").HP != 0 {
		t.Fatal(states, err)
	}
	s.actor("self").Gear.AccessoryID = "item-110"
	before := s.callbackCopy()
	if _, err = s.ApplyRevival(&c, "gem-66", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal("unsupported reset ran after healing", err, s)
	}
}

func TestStatusGemAbsorptionPrecedesRevivalAndPhantomRedirect(t *testing.T) {
	s, c := statusFixture()
	s.actor("enemy").MP, s.actor("enemy").MaxMP = 0, 100
	s.actor("enemy").Temporary = TemporaryAbsorbMagic
	s.actor("enemy").Abilities = []string{"pharaoh"}
	c.Defeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}}
	states, err := s.ProjectStatusGemBranches(&c, "gem_instant_death_1", "enemy")
	if err != nil {
		t.Fatal(err)
	}
	failed, revived := map[int]bool{}, map[int]bool{}
	for _, state := range states {
		a := state.State.actor("enemy")
		if a.HP == 100 {
			failed[a.MP] = true
		} else if a.HP == 101 {
			revived[a.MP] = true
		} else {
			t.Fatal(a)
		}
	}
	for v := 30; v < 50; v++ {
		if !failed[v] || !revived[v-v/2] {
			t.Fatal("incomplete absorption/revival alternatives", v)
		}
	}
	s.Fields.Temporary = "phantom"
	c.PhantomTargetID, c.PhantomEnemyIDs = "enemy", []string{"dead_enemy"}
	s.actor("dead_enemy").HP = 100
	c.Defeat.Rewards = &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return nil },
		Apply: func(*LifetimeState, TargetContext, string) error {
			t.Fatal("projection executed external Apply")
			return nil
		},
	}
	states, err = s.ProjectStatusGemBranches(&c, "gem_instant_death_1", "enemy")
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.State.actor("enemy").MP < 30 || state.State.actor("enemy").MP > 49 || state.State.actor("enemy").HP != 100 {
			t.Fatal("redirect lost original absorber", state)
		}
	}
}
