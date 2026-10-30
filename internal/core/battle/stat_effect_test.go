package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestStatUpCapsBindingAndNoEffect(t *testing.T) {
	for _, tc := range []struct {
		name, target, item                   string
		stat                                 BattleStat
		current, want                        int
		bound, returned, applied, additional bool
	}{
		{"normal cap", "ally", "", StatAttack, 240, 250, false, true, true, false},
		{"self164", "self", "item-164", StatAttack, 240, 300, false, true, true, false},
		{"self183AG", "self", "item-183", StatAgility, 240, 300, false, true, true, false},
		{"self183AT", "self", "item-183", StatAttack, 240, 250, false, true, true, false},
		{"other164", "ally", "item-164", StatAttack, 240, 250, false, true, true, false},
		{"already cap", "ally", "", StatAttack, 250, 250, false, true, false, false},
		{"bound", "ally", "", StatAttack, 150, 150, true, true, false, false},
		{"cap additional", "ally", "", StatAttack, 250, 250, false, false, false, true},
		{"cap bound additional", "ally", "", StatAttack, 250, 250, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			a, tgt := s.actor("self"), s.actor(tc.target)
			a.ItemDefinitionIDs = []string{tc.item}
			a.Tension = 3
			*tgt.currentStat(tc.stat) = tc.current
			if tc.bound {
				tgt.Status = StatusSabaku
			}
			c.AdditionalEffect = tc.additional
			r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: tc.target, Payload: CallbackPayload{Number: 1}}, tc.stat, &targetRNG{t: t})
			if err != nil || r.Returned != tc.returned || r.Applied != tc.applied || *tgt.currentStat(tc.stat) != tc.want {
				t.Fatal(r, err, s)
			}
			wantTension := 3.0
			if tc.applied {
				wantTension = 1
			}
			if a.Tension != wantTension {
				t.Fatal(a)
			}
		})
	}
}

func TestStatDownImmunitiesFloorsAndCounts(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		stat                   BattleStat
		def, hp, current, want int
		item                   string
		returned               bool
	}{
		{"baseDF999", StatDefense, 999, 100, 150, 112, "", true},
		{"baseDF1000", StatDefense, 1000, 100, 150, 150, "", false},
		{"hp999AT", StatAttack, 100, 999, 150, 113, "", true},
		{"hp1000AT", StatAttack, 100, 1000, 150, 150, "", false},
		{"hp1000DF", StatDefense, 100, 1000, 150, 113, "", true},
		{"hp999HP", StatHP, 100, 999, 999, 750, "", true},
		{"hp1000HP", StatHP, 100, 1000, 1000, 1000, "", false},
		{"stone159", StatAgility, 100, 100, 150, 150, "item-159", false},
		{"hit stone197", StatHit, 100, 100, 95, 95, "item-197", false},
		{"hit50", StatHit, 100, 100, 50, 50, "", false},
		{"hit51", StatHit, 100, 100, 51, 50, "", true},
		{"floor20", StatAttack, 100, 100, 21, 20, "", true},
		{"already20", StatAttack, 100, 100, 20, 20, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			a := s.actor("enemy")
			a.BaseStats.Defense, a.HP = tc.def, tc.hp
			if tc.stat == StatDefense && tc.def == 999 {
				a.Defense = 1000
				tc.current = 1000
				tc.want = 750
			}
			*a.currentStat(tc.stat) = tc.current
			a.ItemDefinitionIDs = []string{tc.item}
			r, err := s.ApplyStatDown(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: .25}}, tc.stat, &targetRNG{t: t})
			if err != nil || r.Returned != tc.returned || *a.currentStat(tc.stat) != tc.want || c.StatDownCalls != 1 {
				t.Fatal(r, err, s, c)
			}
		})
	}
}

func TestStatIncidentalTailsAndAdmission(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("ally").ItemDefinitionIDs = []string{"item-205"}
	before := reloadLifetime(t, s)
	_, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatDefense, &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
	level := 99
	c.RequesterLevel = &level
	r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatDefense, &targetRNG{t: t})
	if err != nil || !r.Applied || s.actor("ally").Defense != 175 || s.actor("ally").Attack != 175 || s.actor("ally").Tension != 1.7 || s.actor("ally").Temporary != TemporaryDouble {
		t.Fatal(r, err, s)
	}
	// Item201 fires with post-cap AG, untyped/direct, BEFORE the205 test.
	s, c = recoveryFixture()
	a := s.actor("ally")
	a.Agility = 240
	a.ItemDefinitionIDs = []string{"item-201", "item-205"}
	before = reloadLifetime(t, s)
	_, err = s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 1}}, StatAgility, &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
	c.RequesterLevel = &level
	called := false
	c.Damage = func(ctx TargetContext, hit TargetEffect, rng random.Generator) error {
		called = true
		if hit.TargetID != "" || hit.Category != CategoryUntyped || hit.Payload.Number != 125 || !reflect.DeepEqual(hit.Arguments, []string{"1"}) || ctx.ActorID != "self" || a.Agility != 250 || a.Attack != 150 || a.Temporary != "" {
			t.Fatal(hit, a)
		}
		// The damage owner may reset/remove equipment;205 reads it afterwards.
		a.ItemDefinitionIDs = nil
		return nil
	}
	r, err = s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 1}}, StatAgility, &targetRNG{t: t})
	if err != nil || !called || r.Amount != 100 || r.Delta != 10 || a.Attack != 150 || a.Temporary != "" {
		t.Fatal(r, err, s)
	}
}

func TestStatModifiersAndTargetReflection(t *testing.T) {
	for _, tc := range []struct {
		name, item string
		cat        CallbackCategory
		tmp        TemporaryEffect
		field      string
		want       int
		draws      []float64
	}{
		{"double", "", CategoryMagic, TemporaryDouble, "", 200, nil},
		{"reflection", "", CategoryMagic, TemporaryReflectMagic, "", 175, nil},
		{"song148", "item-148", CategorySong, "", "", 180, nil},
		{"stone159", "item-159", CategoryMagic, "", "", 180, nil},
		{"dance190", "item-190", CategoryDance, "", "", 187, nil},
		{"darkness186", "item-186", CategoryMagic, "", "darkness", 150, []float64{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			s.actor("self").ItemDefinitionIDs = []string{tc.item}
			s.actor("ally").Temporary = tc.tmp
			s.Fields.Default = tc.field
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Category: tc.cat, Payload: CallbackPayload{Number: .25}}, StatAttack, rng)
			target := "ally"
			if tc.tmp == TemporaryReflectMagic {
				target = "self"
			}
			if err != nil || s.actor(target).Attack != tc.want || r.TargetID != target {
				t.Fatal(r, err, s)
			}
			rng.done()
		})
	}
	// Down amount uses current stat, but cannot cross20%base or hit50.
	s, c := recoveryFixture()
	r, err := s.ApplyStatDown(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: 2}}, StatAgility, &targetRNG{t: t})
	if err != nil || r.Amount != 300 || r.Delta != -130 || s.actor("enemy").Agility != 20 {
		t.Fatal(r, err, s)
	}
	// Unknown/native flat kinds are not certified here.
	before := reloadLifetime(t, s)
	_, err = s.ApplyStatDown(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: .25}}, BattleStat("unknown"), &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
}

func TestStat205TensionHooksAndRequestedIncrement(t *testing.T) {
	for _, tc := range []struct {
		level int
		draws []float64
		want  float64
	}{
		{24, nil, 5}, {25, []float64{.99}, 5}, {69, []float64{.99}, 8},
	} {
		s, c := recoveryFixture()
		c.RequesterLevel = &tc.level
		a := s.actor("ally")
		a.ItemDefinitionIDs = []string{"item-205"}
		a.Attack = 990
		a.Defense = 249
		a.Tension = 5
		rng := &targetRNG{t: t, draws: tc.draws}
		r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatDefense, rng)
		if err != nil || r.Amount != 25 || r.Delta != 1 || a.Attack != 999 || a.Tension != tc.want || a.Temporary != TemporaryDouble {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
	// Recursion and defense hook draws occur before205 overwrites the tmp.
	s, c := recoveryFixture()
	level := 99
	c.RequesterLevel = &level
	a := s.actor("self")
	a.ItemDefinitionIDs = []string{"item-145", "item-161", "item-205"}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0}}
	r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "self", Payload: CallbackPayload{Number: .25}}, StatDefense, rng)
	if err != nil || !r.Applied || a.Tension != 3 || a.Temporary != TemporaryDouble {
		t.Fatal(r, err, s)
	}
	rng.done()
}
