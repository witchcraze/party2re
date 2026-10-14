package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestImmediateRecoilBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		power                 float64
		hp                    int
		field                 string
		item                  bool
		status                string
		draws                 []float64
		wantHP, wantAT, calls int
		wantStatus            string
	}{
		{"zero", 0, 100, "", false, "", nil, 99, 150, 1, ""},
		{"large", 1000, 100, "", false, "", nil, 1, 150, 1, ""},
		{"fallen", 1000, 0, "", true, StatusPoison, nil, 0, 150, 0, StatusPoison},
		{"darkness", 1000, 100, "darkness", false, "", nil, 100, 150, 1, ""},
		{"cure", 1000, 100, "darkness", true, StatusPoison, []float64{.499}, 300, 220, 1, ""},
		{"keep", 10.9, 100, "", true, StatusPoison, []float64{.5}, 111, 150, 1, StatusPoison},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			a := s.actor("self")
			a.HP, a.Status, s.Fields.Default = tc.hp, tc.status, tc.field
			if tc.item {
				a.ItemDefinitionIDs = []string{"item-160"}
			}
			end := EndProcessing{}
			rng := &targetRNG{t: t, draws: tc.draws}
			_, err := s.ApplyRecoil(&c, &end, tc.power, rng)
			if err != nil || a.HP != tc.wantHP || a.Attack != tc.wantAT || a.Status != tc.wantStatus || end.ImmediateRiskCalls != tc.calls {
				t.Fatal(err, a, end)
			}
			rng.done()
		})
	}
}

func damageFixture() (LifetimeState, DamageContext) {
	s, c := recoveryFixture()
	for i := range s.Actors {
		s.Actors[i].Gear = &BattleGearStats{}
	}
	return s, DamageContext{EffectContext: c}
}

func TestDamageDirectNormalAndResolvedDefense(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, normal := range []bool{false, true} {
			s, c := damageFixture()
			c.GuaranteedHit = true
			c.Defeat = func(string) error { return nil }
			s.actor("enemy").HP = 1000
			s.actor("enemy").Defense = 100
			s.actor("enemy").Gear.ArmorDefense = 20
			s.actor("enemy").ItemDefinitionIDs = []string{"item-200"}
			e := TargetEffect{TargetID: "enemy", Category: CategoryPhysical, Payload: CallbackPayload{Number: 200}}
			if direct {
				e.Arguments = []string{"1"}
			} else {
				e.Arguments = []string{"0"}
			}
			if normal {
				e.Arguments = append(e.Arguments, "1")
			}
			rng := &targetRNG{t: t, draws: []float64{0}}
			r, err := s.ApplyDamage(&c, e, rng)
			want := 57
			if !direct && !normal {
				want = 75
			}
			if direct {
				want = 180
			}
			if err != nil || r.Amount != want || !r.Returned || c.DamageCalls != 1 || s.actor("enemy").HP != 1000-want {
				t.Fatal(r, err, c)
			}
			rng.done()
		}
	}
}

func TestDamageAdmissionBeforeDraws(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").Status = StatusMagicSeal
	before := reloadLifetime(t, s)
	rng := &targetRNG{t: t}
	e := TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 50}, Arguments: []string{"1"}}
	err := s.ValidateDamage(&c, e, rng)
	_, runErr := s.ApplyDamage(&c, e, rng)
	if !errors.Is(err, ErrUnsupportedEffect) || !errors.Is(runErr, err) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
		t.Fatal(err, runErr, s)
	}
	rng.done()
}
