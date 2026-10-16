package skill

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type sealRolls struct{ draws int }

func (r *sealRolls) Float64() float64 { r.draws++; return 0 }
func (r *sealRolls) Intn(int) int     { panic("unexpected integer draw") }
func (r *sealRolls) IntN(n int) int   { return r.Intn(n) }

func TestRegisteredWeaponSealCategoryDirectPowerAndEnemyOrder(t *testing.T) {
	for i, entry := range corebattle.WeaponSealEntries() {
		for _, tc := range []struct {
			name                     string
			temporary                corebattle.TemporaryEffect
			selfHit, reduced, immune bool
		}{
			{"direct", "", false, false, false},
			{"reduce breath", corebattle.TemporaryReduceBreath, false, i == 0, false},
			{"reduce magic", corebattle.TemporaryReduceMagic, false, i != 0, false},
			{"reflect breath", corebattle.TemporaryReflectBreath, i == 0, false, false},
			{"reflect magic", corebattle.TemporaryReflectMagic, i != 0, false, false},
			{"null magic", corebattle.TemporaryNullMagic, false, false, i != 0},
		} {
			t.Run(entry.ID+"/"+tc.name, func(t *testing.T) {
				s, c := weaponSealFixture(i + 7)
				// A high current defense must not reduce these direct callbacks.
				s.Actors[0].Defense, s.Actors[1].Defense, s.Actors[2].Defense = 500, 500, 500
				s.Actors[1].Temporary = tc.temporary
				var d BattleDefinitions
				d.RegisterWeaponSeals(&c)
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"last"}}
				rng := &sealRolls{}
				ops, err := d.InvocationAction(&s, &frame, []string{entry.ID}, rng)
				if err != nil || rng.draws != 0 {
					t.Fatal(err, rng)
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
					t.Fatal(err)
				}
				power := 180.0
				if i == 1 {
					power = 160
				}
				first := int(power * .9)
				if tc.reduced {
					first = int(power * .25 * .9)
				}
				if tc.immune {
					first = 0
				}
				wantSelf, wantEnemy := 1000, 1000-first
				if tc.selfHit {
					wantSelf, wantEnemy = 1000-first, 1000
				}
				wantLast := 1000 - int(power*.85*.9)
				calls, draws := 2, 2
				if tc.immune {
					calls, draws = 1, 1
				}
				if s.Actors[0].HP != wantSelf || s.Actors[1].HP != wantEnemy || s.Actors[2].HP != wantLast || c.AreaDamageCalls != 1 || c.DamageCalls != calls || rng.draws != draws || s.Actors[0].MP != 100 {
					t.Fatal(s, c, rng, wantSelf, wantEnemy, wantLast)
				}
			})
		}
	}
}

func TestWeaponSealNoEffectAndUnsupportedTails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*corebattle.LifetimeState, *corebattle.DamageContext)
		reject bool
	}{
		{"missing defeat", func(_ *corebattle.LifetimeState, c *corebattle.DamageContext) { c.Defeat = nil }, true},
		{"unsupported field", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Fields.Default = "cosmic" }, true},
		{"missing reset", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
			s.Actors[0].Extended = corebattle.ExtendedPurification
		}, true},
		{"blocked before tails", func(s *corebattle.LifetimeState, c *corebattle.DamageContext) {
			c.Defeat = nil
			s.Fields.Default = "cosmic"
			s.Actors[1].Temporary, s.Actors[2].Temporary = corebattle.TemporaryOneHitDefense, corebattle.TemporaryOneHitDefense
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := weaponSealFixture(7)
			tc.setup(&s, &c)
			var d BattleDefinitions
			d.RegisterWeaponSeals(&c)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "seal-7-breath"}}
			err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t})
			if tc.reject {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
					t.Fatal(err, c)
				}
			} else if err != nil || c.DamageCalls != 2 || c.AreaDamageCalls != 1 || s.Actors[1].Temporary != "" || s.Actors[2].Temporary != "" {
				t.Fatal(err, s, c)
			}
			if s.Actors[0].MP != 100 || s.Actors[1].HP != 1000 || s.Actors[2].HP != 1000 {
				t.Fatal(s)
			}
		})
	}
}

func TestWeaponSealReflectionDeathStopsLaterEnemiesAndKeepsOuterDebit(t *testing.T) {
	s, c := weaponSealFixture(8)
	s.Actors[0].HP, s.Actors[0].MP = 1, 27
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	defeated := ""
	c.Defeat = func(id string) error {
		if s.Actors[0].MP != 27 {
			t.Fatal("charged before effect", s.Actors[0].MP)
		}
		defeated = id
		return nil
	}
	var d BattleDefinitions
	d.RegisterWeaponSeals(&c)
	entry, err := d.WeaponSealAction(&s, &c, "seal-8-magic", noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}, &sealRolls{}); err != nil {
		t.Fatal(err)
	}
	s.Actors[0].MP = max(0, s.Actors[0].MP-entry.MPCost)
	if defeated != "self" || s.Actors[0].HP != 0 || s.Actors[0].MP != 0 || s.Actors[2].HP != 1000 || c.DamageCalls != 1 || c.AreaDamageCalls != 1 {
		t.Fatal(s, c, defeated)
	}
}

func TestRegisteredWeaponSealAbsorptionBeforeDamageAndDarknessTail(t *testing.T) {
	for i, entry := range corebattle.WeaponSealEntries() {
		t.Run(entry.ID, func(t *testing.T) {
			s, c := weaponSealFixture(i + 7)
			s.Fields.Default = "darkness"
			s.Actors[1].HP = 1
			s.Actors[1].MP, s.Actors[1].MaxMP = 5, 30
			s.Actors[1].Temporary = corebattle.TemporaryAbsorbMagic
			c.Defeat = func(id string) error {
				wantMP := 30
				if i == 0 {
					wantMP = 5
				}
				if id != "enemy" || s.Actors[1].MP != wantMP || s.Actors[0].MP != 100 {
					t.Fatal("target recovery must precede defeat and outer debit", id, s.Actors)
				}
				return nil
			}
			var d BattleDefinitions
			d.RegisterWeaponSeals(&c)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
			if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}, &sealRolls{}); err != nil {
				t.Fatal(err)
			}
			power := 180.0
			if i == 1 {
				power = 160
			}
			if c.End.DarknessDebit == nil || *c.End.DarknessDebit != int(power*.9)/2 || c.DamageCalls != 2 {
				t.Fatal(c)
			}
			c.End.Flush(&s.Actors[0])
			if s.Actors[0].HP != 1000-int(power*.9)/2 {
				t.Fatal(s.Actors[0])
			}
		})
	}
}
