package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestControlClimaxCasterDeathAdmission(t *testing.T) {
	for _, row := range []struct {
		name            string
		hp              int
		revived, reject bool
	}{
		{"certain-death", 1, false, false},
		{"damage-boundary-certain-death", 90, false, false},
		{"survives", 1000, false, true},
		{"damage-boundary-can-survive", 91, false, true},
		{"revival-can-survive", 1, true, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := controlFixture()
			a := &s.Actors[0]
			a.HP, a.Attack, a.Defense, a.Agility, a.BaseStats.Agility = row.hp, 50, 60, 100, 100
			a.Status = corebattle.StatusConfusion
			a.ItemDefinitionIDs = []string{"item-201"}
			if row.revived {
				a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-193")
			}
			defeats := 0
			c.Defeat = func(string) error { defeats++; return nil }
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
			original := frame
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			ops, err := d.InvocationAction(&s, &frame, []string{"job-70-entry-4"}, noArithmeticDraws{t})
			if row.reject {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-70-entry-4"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatalf("unreachable AG hook was required: %v", err)
			}
			after, marshalErr := json.Marshal(s)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if string(before) != string(after) || !reflect.DeepEqual(frame, original) || defeats != 0 || c.DamageCalls != 0 {
				t.Fatal("admission changed inputs", s, c, frame)
			}
			if row.reject {
				return
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0}}); err != nil {
				t.Fatal(err)
			}
			if s.Actors[0].HP != 0 || s.Actors[0].Agility != 100 || c.StatUpCalls != 0 || c.DamageCalls != 1 || defeats != 1 {
				t.Fatal(s, c, defeats)
			}
		})
	}
}

func TestControlClimaxLivingAlternativesRemainGated(t *testing.T) {
	for _, name := range []string{"other-target", "confused-target-alternatives", "pre-hit-reset", "pharaoh", "retained-revival-tension", "random-damage-can-survive"} {
		t.Run(name, func(t *testing.T) {
			s, c, d := controlFixture()
			a := &s.Actors[0]
			a.HP, a.Attack, a.Defense, a.Agility, a.BaseStats.Agility = 1, 50, 60, 100, 100
			a.Status = corebattle.StatusConfusion
			a.ItemDefinitionIDs = []string{"item-201"}
			args := []string{"self"}
			switch name {
			case "other-target":
				a.Status = ""
				args[0] = "enemy"
			case "confused-target-alternatives":
				args[0] = "enemy"
			case "pre-hit-reset":
				a.Extended = corebattle.ExtendedPurification
				c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
			case "pharaoh":
				a.Abilities = append(a.Abilities, "pharaoh")
			case "retained-revival-tension":
				a.Tension = 8
				a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-161", "item-149")
			case "random-damage-can-survive":
				a.HP = 50
				a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-190")
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			if _, err := d.InvocationAction(&s, &f, []string{"job-70-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
		})
	}
	// The selected self target loses its revival temporary before the hit.
	s, c, d := controlFixture()
	s.Actors[0].HP, s.Actors[0].Status, s.Actors[0].Temporary = 1, corebattle.StatusConfusion, corebattle.TemporaryRevival
	s.Actors[0].Agility, s.Actors[0].BaseStats.Agility = 200, 200
	s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
	executeControl(t, &s, c, d, "job-70-entry-4", []string{"self"}, 0)
	if s.Actors[0].HP != 0 || c.StatUpCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestControlClimaxDeathStillRequiresPrimaryDamage(t *testing.T) {
	for _, missing := range []string{"defeat", "gear"} {
		t.Run(missing, func(t *testing.T) {
			s, c, d := controlFixture()
			s.Actors[0].HP, s.Actors[0].Status = 1, corebattle.StatusConfusion
			s.Actors[0].Agility, s.Actors[0].BaseStats.Agility = 200, 200
			s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
			if missing == "defeat" {
				c.Defeat = nil
			} else {
				s.Actors[0].Gear = nil
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
			if _, err := d.InvocationAction(&s, &f, []string{"job-70-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if s.Actors[0].HP != 1 || c.DamageCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestControlStatusBoundaries(t *testing.T) {
	for _, row := range []struct {
		id       string
		statuses []string
	}{
		{"job-53-entry-2", []string{corebattle.StatusDeadlyPoison, corebattle.StatusParalyze, corebattle.StatusSleep}},
		{"job-58-entry-2", []string{corebattle.StatusMagicSeal, corebattle.StatusPhysicalSeal, corebattle.StatusDofuu}},
	} {
		for i, status := range row.statuses {
			s, c, d := controlFixture()
			executeControl(t, &s, c, d, row.id, nil, float64(i)/3, 0)
			if s.Actors[1].Status != status {
				t.Fatal(row, i, s)
			}
		}
	}
	for _, row := range []struct {
		id     string
		chance float64
		area   bool
	}{
		{"job-53-entry-0", .8, false}, {"job-53-entry-1", .8, false}, {"job-53-entry-2", .6, true},
		{"job-53-entry-4", .8, false}, {"job-58-entry-2", .7, true},
	} {
		for _, immune := range []bool{false, true} {
			s, c, d := controlFixture()
			draws := []float64{row.chance}
			if immune {
				s.Actors[1].HP = 1000
				s.Actors[1].Gear = nil
				draws = nil
			}
			if row.area {
				draws = append([]float64{.9}, draws...)
			}
			executeControl(t, &s, c, d, row.id, []string{"enemy"}, draws...)
			if s.Actors[1].Status != "" || c.StatusCalls != 1 {
				t.Fatal(row, immune, s, c)
			}
		}
	}
}

func TestControlChanceAndNoEffectBoundaries(t *testing.T) {
	for _, row := range []struct {
		id       string
		boundary float64
	}{
		{"job-58-entry-0", .2}, {"job-58-entry-5", 1.0 / 3},
	} {
		s, c, d := controlFixture()
		executeControl(t, &s, c, d, row.id, []string{"enemy"}, row.boundary)
		if s.Actors[1].HP != 900 || c.StatDownCalls != 0 {
			t.Fatal(row, s, c)
		}
	}
	// A fallen enemy returns before the single-target chance draw.
	s, c, d := controlFixture()
	s.Actors[1].HP = 0
	c.Defeat = nil
	executeControl(t, &s, c, d, "job-58-entry-0", []string{"enemy"})
	executeControl(t, &s, c, d, "job-53-entry-5", []string{"enemy"})
	// A fallen ally falls back to self; the web still overwrites its temporary.
	executeControl(t, &s, c, d, "job-53-entry-3", []string{"fallen"})
	if s.Actors[0].Temporary != corebattle.TemporaryReflectAttack {
		t.Fatal(s)
	}
	// Healing at max HP still returns and records its helper invocation.
	s.Actors[0].HP = 1000
	executeControl(t, &s, c, d, "job-70-entry-0", nil, 0)
	if s.Actors[0].HP != 1000 || c.HealCalls != 1 {
		t.Fatal(s, c)
	}
	// The area helper's actor-death guard permits a no-effect without damage ports.
	s.Actors[0].HP = 0
	executeControl(t, &s, c, d, "job-70-entry-2", nil)
	if c.AreaDamageCalls != 1 || c.DamageCalls != 0 {
		t.Fatal(c)
	}
}

func TestControlBuffAndTensionGuards(t *testing.T) {
	for _, id := range []string{"job-58-entry-1", "job-70-entry-1"} {
		s, c, d := controlFixture()
		s.Actors[0].Temporary = corebattle.TemporaryNullAttack
		s.Actors[0].ItemDefinitionIDs = []string{"item-201", "item-205"}
		// Null physical payloads do not reach item201/205 or require their ports.
		executeControl(t, &s, c, d, id, nil)
		if s.Actors[0].Attack != 100 || s.Actors[0].Agility != 40 || s.Actors[0].Defense != 50 {
			t.Fatal(id, s)
		}
		if id == "job-58-entry-1" && s.Actors[0].Temporary != corebattle.TemporaryAbsorbMagic {
			t.Fatal(s)
		}
	}
	s, c, d := controlFixture()
	s.Actors[0].Status = corebattle.StatusPhysicalSeal
	executeControl(t, &s, c, d, "job-58-entry-1", nil)
	if s.Actors[0].Attack != 200 || s.Actors[0].Status != corebattle.StatusConfusion {
		t.Fatal(s)
	}
	s, c, d = controlFixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	executeControl(t, &s, c, d, "job-58-entry-4", nil)
	if s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[0].Tension != 1 || c.TensionCalls != 2 {
		t.Fatal(s, c)
	}
	s, c, d = controlFixture()
	s.Actors[0].Tension = 3
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"job-58-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestControlArrowConsumptionAndAlias(t *testing.T) {
	for _, row := range []struct {
		status         string
		current, prior corebattle.TemporaryEffect
		draws          []float64
		hp, selfHP     int
		want           corebattle.TemporaryEffect
	}{
		{corebattle.StatusSleep, corebattle.TemporaryDefense, corebattle.TemporaryDouble, []float64{0, 0}, 837, 500, corebattle.TemporaryDefense},
		{"", corebattle.TemporaryDouble, corebattle.TemporaryDefense, []float64{0}, 774, 500, ""},
		{"", "", corebattle.TemporaryReflectMagic, []float64{0}, 900, 437, ""},
		{"", corebattle.TemporaryNullMagic, corebattle.TemporaryDouble, nil, 900, 500, ""},
	} {
		s, c, d := controlFixture()
		s.Actors[0].Status, s.Actors[0].Temporary = row.status, row.current
		c.PriorTemporary = map[string]corebattle.TemporaryEffect{"self": row.prior}
		executeControl(t, &s, c, d, "job-58-entry-6", []string{"enemy", "tail"}, row.draws...)
		if s.Actors[0].HP != row.selfHP || s.Actors[1].HP != row.hp || s.Actors[0].Temporary != row.want || s.Actors[0].Status != "" || c.Arguments[0] != "tail" {
			t.Fatal(row, s, c)
		}
		if row.status != "" && s.Actors[1].Status != corebattle.StatusMagicSeal {
			t.Fatal(s)
		}
	}
	// Removing confusion first makes the explicit enemy deterministic, avoiding
	// the unrelated ally's missing gear. Validation must use the consumed state.
	s, c, d := controlFixture()
	s.Actors[0].Status = corebattle.StatusConfusion
	s.Actors[2].HP, s.Actors[2].Gear = 100, nil
	executeControl(t, &s, c, d, "job-58-entry-6", []string{"enemy"}, 0, 0)
	// Newly activated confusion can select self. Its transferred magic seal
	// then rejects the damage helper, after the original state was consumed.
	s, c, d = controlFixture()
	s.Actors[0].Status = corebattle.StatusSleep
	c.Item195Activated = true
	executeControl(t, &s, c, d, "job-58-entry-6", []string{"self"}, 0, 0, .5)
	if s.Actors[0].Status != corebattle.StatusMagicSeal || s.Actors[0].HP != 500 || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestControlClimaxBranches(t *testing.T) {
	for _, row := range []struct {
		name                                 string
		at, df, ag                           int
		hp                                   int
		tmp                                  corebattle.TemporaryEffect
		draws                                []float64
		enemyHP, selfHP, selfAG, area, heals int
		status                               string
	}{
		{"equal", 100, 100, 100, 900, corebattle.TemporaryDouble, []float64{0, 0}, 762, 500, 100, 1, 0, ""},
		{"defense-tie", 50, 100, 100, 900, corebattle.TemporaryDefense, []float64{0, 0, 0}, 810, 1000, 100, 0, 1, ""},
		{"agility-stun", 50, 60, 100, 900, corebattle.TemporaryDefense, []float64{0, 0}, 810, 500, 130, 0, 0, corebattle.StatusDofuu},
		{"agility-no-stun", 50, 60, 100, 900, corebattle.TemporaryDefense, []float64{0, .5}, 810, 500, 130, 0, 0, ""},
		{"target-death", 50, 100, 80, 1, corebattle.TemporaryReflectMagic, []float64{0, 0, 0}, 0, 1000, 80, 0, 1, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := controlFixture()
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = row.at, row.df, row.ag
			s.Actors[0].BaseStats.Agility = row.ag
			s.Actors[1].HP, s.Actors[1].Temporary = row.hp, row.tmp
			executeControl(t, &s, c, d, "job-70-entry-4", []string{"enemy"}, row.draws...)
			if s.Actors[1].HP != row.enemyHP || s.Actors[0].HP != row.selfHP || s.Actors[0].Agility != row.selfAG || c.AreaDamageCalls != row.area || c.PartyHealCalls != row.heals || s.Actors[1].Status != row.status {
				t.Fatal(s, c)
			}
			want := row.tmp
			if row.enemyHP == 0 {
				want = ""
			}
			if s.Actors[1].Temporary != want {
				t.Fatal(s)
			}
		})
	}
	// Zero converted magic has no positive-damage tail, even with unavailable AG hooks.
	s, c, d := controlFixture()
	s.Actors[0].Agility = 200
	s.Actors[0].ItemDefinitionIDs = []string{"item-166", "item-201"}
	c.Defeat = nil
	executeControl(t, &s, c, d, "job-70-entry-4", []string{"enemy"})
	if s.Actors[0].Agility != 200 || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
	// Aliased caster death skips both temporary restoration and every tail.
	s, c, d = controlFixture()
	s.Actors[0].HP, s.Actors[0].Status, s.Actors[0].Temporary = 1, corebattle.StatusConfusion, corebattle.TemporaryDouble
	executeControl(t, &s, c, d, "job-70-entry-4", []string{"self"}, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Temporary != "" || c.DamageCalls != 1 || c.AreaDamageCalls != 0 {
		t.Fatal(s, c)
	}
}
