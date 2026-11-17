package skill

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type zeroDraws struct{}

func (zeroDraws) Float64() float64 { return 0 }
func (zeroDraws) Intn(int) int     { return 0 }
func (zeroDraws) IntN(int) int     { return 0 }

// _battle.cgi:761–789: a custom entry (listed MP 0) is charged CMP once by its
// bound callback; an ordinary entry is charged its listed MP and no CMP.
func TestPlayerOperationChargesCustomCMPAndOrdinaryMP(t *testing.T) {
	for _, custom := range []bool{false, true} {
		s, c, d := playerCustomFixture([3]string{"gem_dance_combo", "", ""}, 30)
		a := &s.Actors[0]
		a.CMPCharge, a.CMPNeedCharge = 0, 9
		cmp, mp := a.CMP, a.MP
		level := 50
		action := corebattle.PlayerAction{EntryID: "job-0-entry-0", Cost: 5, Validate: func(*corebattle.LifetimeState) error { return nil },
			Execute: func(*corebattle.LifetimeState, random.Generator) (corebattle.PlayerActionFacts, error) {
				return corebattle.PlayerActionFacts{}, nil
			}}
		if custom {
			action = corebattle.PlayerAction{EntryID: "custom", Validate: func(*corebattle.LifetimeState) error { return nil },
				Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
					f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
					return corebattle.PlayerActionFacts{}, d.ApplyPlayerCustomAction(s, &f, []string{"attack"}, rng)
				}}
		}
		op := &corebattle.PlayerOperation{First: action, Tail: corebattle.PlayerTailContext{
			DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext, RequesterLevel: &level}},
			DefeatContext: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext, RequesterLevel: &level}},
		}}
		result, err := s.ApplyPlayerOperation(op, zeroDraws{})
		wantCMP, wantMP := cmp, mp-5
		if custom {
			wantCMP, wantMP = cmp-30, mp
		}
		if err != nil || !result.Accepted || s.Actors[0].CMP != wantCMP || s.Actors[0].MP != wantMP {
			t.Fatal(custom, result, err, s.Actors[0].CMP, s.Actors[0].MP)
		}
	}
}
