package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestNativeDefeatResetFinalStrikeAndAsymmetricInheritance(t *testing.T) {
	for _, dying := range []string{"self", "enemy"} {
		s, c := statusFixture()
		if dying == "self" {
			s.Actors = s.Actors[:4]
			c.EnemyIDs = []string{"enemy"}
		}
		dc := nativeDefeatContext(c.TargetContext)
		a := s.actor(dying)
		a.Attack = 900
		a.Gear.AccessoryID = "item-212"
		a.ItemDefinitionIDs = []string{"item-212"}
		inheritor := "fallen"
		if dying == "enemy" {
			inheritor = "dead_enemy"
		}
		h := s.actor(inheritor)
		h.ItemDefinitionIDs = []string{"item-212"}
		h.Gear.AccessoryID = "item-212"
		h.Attack, h.Defense, h.Agility = 950, 950, 950
		target := "enemy"
		if dying == "enemy" {
			target = "self"
		}
		s.actor(target).Defense = 0
		rewards := 0
		dc.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { rewards++; return nil }
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		r, err := s.ApplyDefeat(dc, dying, rng)
		want := 999
		if dying == "enemy" {
			want = 1050
		}
		if err != nil || !r.Defeated || r.FinalStrike == nil || r.FinalStrike.Amount != 67 || s.actor(target).HP != 33 || a.Attack != 100 || a.HP != 0 || h.HP != 0 || h.Attack != want || h.Defense != want || h.Agility != want {
			t.Fatal(s, r, err)
		}
		rng.done()
		if rewards != map[bool]int{true: 0, false: 1}[dying == "self"] {
			t.Fatal(rewards)
		}
	}
}

func TestNativeDefeatRevivalOrderConsumptionAndSecondCurseDeath(t *testing.T) {
	for _, tc := range []struct {
		name, accessory string
		tmp             TemporaryEffect
		abilities       []string
		mp              int
		tension         float64
		hp, transfer    int
		draws           []float64
	}{
		{"temporary", "item-193", TemporaryRevival, []string{"pharaoh"}, 80, 8, 10, 0, []float64{0}},
		{"pharaoh", "item-193", "", []string{"pharaoh"}, 80, 8, 101, 0, nil},
		{"shield", "item-161", "", nil, 0, 8, 50, 50, nil},
		{"amulet", "item-193", "", nil, 0, 1, 50, 50, nil},
		{"curse", "item-260", "", nil, 0, 1, 30, 50, nil},
		{"cursed again", "item-260", "", []string{"cursed"}, 0, 1, 0, 50, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusFixture()
			dc := nativeDefeatContext(c.TargetContext)
			a := s.actor("self")
			a.Attack = 100
			a.Gear.AccessoryID = tc.accessory
			a.ItemDefinitionIDs = []string{tc.accessory}
			a.Temporary, a.Abilities, a.MP, a.Tension = tc.tmp, tc.abilities, tc.mp, tc.tension
			raw := 0
			h := s.actor("fallen")
			h.Gear = &BattleGearStats{WeaponID: "weapon-37", ArmorID: "armor-38", WeaponAttack: &raw}
			h.Attack = 0
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyDefeat(dc, a.ID, rng)
			if err != nil || a.HP != tc.hp || h.Attack != tc.transfer || r.Prevented != (tc.hp > 0) {
				t.Fatal(s, r, err)
			}
			rng.done()
			if tc.name == "amulet" {
				if len(a.ConsumedItems) != 1 || a.Gear.AccessoryID != "" || a.Status != StatusDofuu {
					t.Fatal(a)
				}
			}
			if tc.name == "curse" {
				if a.Attack != 400 || a.Defense != 400 || a.Agility != 400 || !hasAbility(a.Abilities, "cursed") {
					t.Fatal(a)
				}
				s = reloadLifetime(t, s)
				if r, err := s.ApplyDefeat(dc, "self", &targetRNG{t: t}); err != nil || r.Prevented || s.actor("self").HP != 0 {
					t.Fatal(s, r, err)
				}
			}
		})
	}
}

func TestDefeatRewardAndFortressReachabilityAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, dying, snapshot   string
		npc, anti, revive, fail bool
		want                    []string
	}{
		{"snapshot fortress current other", "enemy", "fortress", false, false, false, false, []string{"reward", "fortress"}},
		{"new fortress no snapshot", "enemy", "", false, false, false, false, []string{"reward"}},
		{"anti suppresses", "enemy", "fortress", false, true, false, false, []string{"reward"}},
		{"NPC guard", "enemy", "fortress", true, false, false, false, nil},
		{"self guard", "self", "fortress", false, false, false, false, nil},
		{"revival guard", "enemy", "fortress", false, false, true, false, nil},
		{"reward failure", "enemy", "fortress", false, false, false, true, []string{"reward"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusFixture()
			dc := nativeDefeatContext(c.TargetContext)
			dc.FieldAtLoad = tc.snapshot
			s.Fields.Default = "other"
			if tc.anti {
				s.Fields.AntiTurns = 1
			}
			s.actor("self").NPC = tc.npc
			if tc.revive {
				s.actor(tc.dying).ItemDefinitionIDs = []string{"item-161"}
				s.actor(tc.dying).Tension = 8
			}
			var order []string
			failure := errors.New("reward failed")
			dc.Rewards.Apply = func(*LifetimeState, TargetContext, string) error {
				order = append(order, "reward")
				if tc.fail {
					return failure
				}
				return nil
			}
			dc.Fortress = &DefeatRewards{Validate: func(*LifetimeState, TargetContext, string) error { return nil }, Apply: func(*LifetimeState, TargetContext, string) error { order = append(order, "fortress"); return nil }}
			_, err := s.ApplyDefeat(dc, tc.dying, &targetRNG{t: t})
			if !reflect.DeepEqual(order, tc.want) || (tc.fail && !errors.Is(err, failure)) || (!tc.fail && err != nil) {
				t.Fatal(order, err)
			}
		})
	}
}

func TestDefeatRejectsMissingResetAndRecursiveFinalStrikeBeforeEffects(t *testing.T) {
	for _, kind := range []string{"raw weapon", "fallen110", "recursive212", "fortress context", "tool category"} {
		s, c := statusFixture()
		dc := nativeDefeatContext(c.TargetContext)
		switch kind {
		case "raw weapon":
			s.actor("enemy").Gear.WeaponID = "weapon-1"
		case "fallen110":
			s.actor("enemy").Gear.AccessoryID = "item-110"
		case "recursive212":
			s.actor("self").Status = StatusConfusion
			for _, id := range []string{"self", "enemy"} {
				s.actor(id).ItemDefinitionIDs = []string{"item-212"}
				s.actor(id).Gear.AccessoryID = "item-212"
			}
		case "fortress context":
			dc.FieldAtLoad = "fortress"
		case "tool category":
			dc.ToolCategory.Source = "physical"
			s.actor("enemy").ItemDefinitionIDs = []string{"item-212"}
			s.actor("enemy").Gear.AccessoryID = "item-212"
		}
		before := reloadLifetime(t, s)
		if _, err := s.ApplyDefeat(dc, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
			t.Fatal(kind, s, err)
		}
	}
}

func TestDeathMarkAndFinalStrikeToolCategory(t *testing.T) {
	s, c := statusFixture()
	dc := nativeDefeatContext(c.TargetContext)
	s.actor("enemy").Temporary = TemporaryEffect("death_mark")
	if _, err := s.ApplyDefeat(dc, "enemy", &targetRNG{t: t}); err != nil || s.actor("enemy").Temporary != TemporaryEffect("death_mark") {
		t.Fatal(s, err)
	}
	s, c = statusFixture()
	s.Actors = s.Actors[:4]
	c.EnemyIDs = []string{"enemy"}
	dc = nativeDefeatContext(c.TargetContext)
	s.actor("self").ItemDefinitionIDs = []string{"item-212"}
	s.actor("self").Gear.AccessoryID = "item-212"
	s.actor("enemy").ItemDefinitionIDs = []string{"item-162"}
	s.actor("enemy").Defense = 100
	dc.ToolCategory = ToolCategoryContext{Source: "job77", SavedAttack: 500}
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	r, err := s.ApplyDefeat(dc, "self", rng)
	if err != nil || r.FinalStrike == nil || r.FinalStrike.Amount != 27 {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestFinalStrikeMissingHelperResultRejectsBeforeReset(t *testing.T) {
	for _, empty := range []bool{false, true} {
		s, c := statusFixture()
		if empty {
			s.Actors = s.Actors[:3]
			c.EnemyIDs = nil
		}
		dc := nativeDefeatContext(c.TargetContext)
		s.actor("self").ItemDefinitionIDs = []string{"item-212"}
		s.actor("self").Gear.AccessoryID = "item-212"
		before := reloadLifetime(t, s)
		if _, err := s.ApplyDefeat(dc, "self", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
			t.Fatal(empty, s, err)
		}
	}
}
