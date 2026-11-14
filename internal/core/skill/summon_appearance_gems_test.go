package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func summonAppearanceFixture() (corebattle.LifetimeState, corebattle.RosterContext, corebattle.TransformationContext, BattleDefinitions) {
	s, c, _ := rosterRegistryFixture()
	weapon, sp := 12, 17
	c.EnemyPool = []corebattle.LifetimeActor{{Participant: corebattle.Participant{
		ID: "species", Name: "creature", HP: 101, MP: 51, Attack: 41, Defense: 31, Agility: 21, CMP: 11,
		MaxHP: 777, MaxMP: 888, MaxCMP: 999, BaseStats: corebattle.BattleBaseStats{Attack: 222},
		Gear:      &corebattle.BattleGearStats{WeaponAttack: &weapon},
		Abilities: []string{"source"}, ItemDefinitionIDs: []string{"item-1"},
		CustomSkills: []corebattle.ActionCustomSkill{{ID: "custom", CMPCost: 47, Gems: []corebattle.GemEffect{{Kind: "unresolved"}}}},
		Status:       corebattle.StatusPoison, ExperienceReward: 11, GoldReward: 7,
	}, Jobs: corebattle.BattleJobs{JobID: "job-2", SP: 13, OldJobID: "job-3", OldSP: 9}, JobHistory: map[string]*int{"job-7": &sp},
		Extended: corebattle.ExtendedFlame, ExtendedTurns: 4, CrystalReward: 3}}
	c.AppearancePositions = []int{0}
	c.Hard = true
	tc := corebattle.TransformationContext{EffectContext: c.EffectContext}
	tc.MonsterIcons[107] = "assets/images/placeholders/battle/forms.svg#job-39-m"
	var d BattleDefinitions
	return s, c, tc, d
}

func TestSummonAppearanceGemBindingsAndGates(t *testing.T) {
	s, c, tc, d := summonAppearanceFixture()
	d.RegisterSummonAppearanceGems(&c, &tc)
	want := []SummonAppearanceGemEntry{{"gem-82", 82, 18}, {"gem-100-appearance", 100, 0}}
	if !reflect.DeepEqual(SummonAppearanceGemEntries(), want) || len(d.Invocations.Entries) != 2 {
		t.Fatal(SummonAppearanceGemEntries(), d.Invocations)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	for _, e := range want {
		def := d.Invocations.Entries[e.ID]
		if def.Validate == nil || def.Execute == nil || def.ProjectBranches == nil {
			t.Fatal(e, "missing binding")
		}
		if _, err := d.InvocationAction(&s, &f, []string{e.ID}, noArithmeticDraws{t}); err != nil {
			t.Fatal(e, err)
		}
		unknown := f
		unknown.AfterEffects = true
		if err := def.Validate(&s, &unknown); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(e, "unknown predecessors must remain gated", err)
		}
		var empty BattleDefinitions
		empty.RegisterSummonAppearanceGems(nil, nil)
		if _, err := empty.InvocationAction(&s, &f, []string{e.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(e, err)
		}
	}
	for _, id := range []string{"gem-101", "gem-102", "gem-104"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	if c.DamageCalls != 0 {
		t.Fatal("discovery changed counters")
	}
}

func TestSummonGemBeforeRegisteredTargetHelper(t *testing.T) {
	s, c, tc, d := summonAppearanceFixture()
	c.EnemyPool[0].Tension = 5
	d.RegisterSummonAppearanceGems(&c, &tc)
	d.RegisterRecoveryStatGems(&c.EffectContext)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
	ops, err := d.InvocationAction(&s, &f, []string{"gem-82", "gem_calm_orb"}, noArithmeticDraws{t})
	if err != nil || len(s.Actors) != 2 || c.DamageCalls != 0 {
		t.Fatal("discovery", err, s, c)
	}
	// Suffix I, pool0, enemy fallback chooses the newly added record.
	rng := &damageGemDraws{t: t, draws: []float64{0, 0, .99}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != 3 {
		t.Fatal(err, rng)
	}
	a := s.Actors[2]
	if a.RosterIdentity != "enemy-species-I" || a.Tension != 1 || a.ID != "summoned-1" || s.RosterSequence != 1 || !reflect.DeepEqual(f.EnemyIDs, []string{"enemy", a.ID}) || !reflect.DeepEqual(f.Arguments, []string{"self", "tail"}) {
		t.Fatal(a, f)
	}
	// ff97fc5:_skill.cgi:9885–9906; hard truncation precedes party-size truncation.
	if [6]int{a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP} != [6]int{143, 72, 57, 43, 29, 15} || [3]int{a.MaxHP, a.MaxMP, a.MaxCMP} != [3]int{777, 888, 999} || a.BaseStats.Attack != 222 || a.BaseStats.Defense != 43 || a.Jobs.SP != 19 || a.Jobs.OldSP != 13 || a.OriginalJobs != a.Jobs || a.CustomSkills[0].CMPCost != 47 || a.Status != corebattle.StatusPoison || a.ExtendedTurns != 4 || [3]int{a.ExperienceReward, a.GoldReward, a.CrystalReward} != [3]int{16, 10, 4} || a.Hit != 95 {
		t.Fatal(a)
	}
}

func TestSummonGemRejectsNewUnsupportedTargetBeforePrefix(t *testing.T) {
	s, c, tc, d := summonAppearanceFixture()
	// An NPC adds to its own party. The new appearance opens a calling helper
	// whose requester-level prerequisite was unreachable in the original roster.
	c.ActorID = "enemy"
	c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	for i := 1; i < 4; i++ {
		a := c.EnemyPool[0]
		a.ID = "species-" + strconv.Itoa(i)
		c.EnemyPool = append(c.EnemyPool, a)
	}
	c.EnemyPool[3].Icon = "assets/images/placeholders/battle/forms.svg#job-69-m"
	c.AppearancePositions = []int{0, 0, 3, 2, 1}
	c.RequesterLevel = nil
	d.RegisterSummonAppearanceGems(&c, &tc)
	d.RegisterCallingJobs(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-69-small"}, noArithmeticDraws{t}); err != nil {
		t.Fatal("negative fixture rejects before roster mutation", err)
	}
	before, _ := json.Marshal(s)
	for _, discovery := range []bool{true, false} {
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem-82"}, {Kind: "entry", EntryID: "job-69-small"}}
		var err error
		if discovery {
			_, err = d.InvocationAction(&s, &f, []string{"gem-82", "job-69-small"}, noArithmeticDraws{t})
		} else {
			err = s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t})
		}
		after, _ := json.Marshal(s)
		if !errors.Is(err, corebattle.ErrUnsupportedEffect) || string(before) != string(after) || len(c.EnemyIDs) != 1 || c.DamageCalls != 0 || len(f.EnemyIDs) != 1 {
			t.Fatal(discovery, err, s, c, f)
		}
	}
}

func TestAppearanceGemAllPositionsChangeOnlyIcon(t *testing.T) {
	s, c, tc, d := summonAppearanceFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	for i := range tc.MonsterIcons {
		tc.MonsterIcons[i] = "appearance-" + strconv.Itoa(i)
	}
	tc.MonsterIcons[53] = ""
	d.RegisterSummonAppearanceGems(&c, &tc)
	branches, err := d.Invocations.Entries["gem-100-appearance"].ProjectBranches(&s, &f)
	if err != nil || len(branches) != 108 {
		t.Fatal(err, len(branches))
	}
	for position := range 108 {
		want := tc.MonsterIcons[position]
		if want == "" {
			want = "assets/images/placeholders/battle/monster-default.svg"
		}
		if branches[position].State.Actors[0].Icon != want {
			t.Fatal(position, branches[position])
		}
		// An independent icon-only oracle protects every other state/context fact.
		expected := s
		expected.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
		expected.Actors[0].Icon = want
		if !reflect.DeepEqual(branches[position].State, expected) || !reflect.DeepEqual(branches[position].Context, f) {
			t.Fatal(position, "appearance changed other facts")
		}
		rng := &damageGemDraws{t: t, draws: []float64{(float64(position) + .5) / 108}}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem-100-appearance"}}, rng); err != nil || rng.used != 1 || s.Actors[0].Icon != want {
			t.Fatal(position, err, rng, s)
		}
	}
}

func TestAppearanceGemBeforeRegisteredCallingHelper(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		s, c, tc, d := summonAppearanceFixture()
		commander := s.Actors[0]
		commander.ID = "commander"
		s.Actors = append(s.Actors, commander)
		c.PartyIDs = []string{"commander", "self"}
		tc.TargetContext = c.TargetContext
		id := "job-59-defend"
		if unsupported {
			id, c.RequesterLevel = "job-69-small", nil
			tc.MonsterIcons[107] = "assets/images/placeholders/battle/forms.svg#job-69-m"
		}
		d.RegisterSummonAppearanceGems(&c, &tc)
		d.RegisterCallingJobs(&c)
		helper := d.Invocations.Entries[id]
		// Synthetic operation-owner frame switch, with the real registered helper
		// supplying all lookup, validation and execution behavior under test.
		bridge := func(f *corebattle.CallbackContext) corebattle.CallbackContext {
			child := *f
			child.ActorID = "commander"
			return child
		}
		d.Invocations.Entries["helper-bridge"] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				child := bridge(f)
				return helper.Validate(s, &child)
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				child := bridge(f)
				return helper.Execute(s, &child, rng)
			},
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
		if _, err := d.InvocationAction(&s, &f, []string{"helper-bridge"}, noArithmeticDraws{t}); err != nil {
			t.Fatal("untransformed helper must be accepted no-effect", err)
		}
		ids := []string{"gem-100-appearance", "helper-bridge"}
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if unsupported {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal("appearance reopened missing requester descendant", err)
			}
			ops = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: ids[0]}, {Kind: "entry", EntryID: ids[1]}}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Icon != "" || s.Actors[0].Temporary != "" || c.DamageCalls != 0 {
				t.Fatal("rejected after a prefix", err, s, c)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rng := &damageGemDraws{t: t, draws: []float64{.999}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != 1 || s.Actors[0].Temporary != corebattle.TemporaryDefense || s.Actors[2].Temporary != "" {
			t.Fatal("changed appearance was not used by helper lookup", err, s)
		}
	}
}
