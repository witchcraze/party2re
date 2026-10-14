package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ApplyRecoil is immediate _risk, not deferred __risk. Darkness suppresses the
// debit after counting; item160 still heals/cures there. Zero power rounds to1.
func (s *LifetimeState) ApplyRecoil(c *EffectContext, end *EndProcessing, power float64, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if c == nil || end == nil {
		return r, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return r, err
	}
	if power < 0 || math.IsNaN(power) || math.IsInf(power, 0) || power >= 1e15 {
		return r, ErrUnsupportedEffect
	}
	a := s.actor(c.ActorID)
	if a.HP <= 0 {
		return r, nil
	}
	end.ImmediateRiskCalls++
	r.Amount = int(power + 1)
	before := a.HP
	if hasItem(a.ItemDefinitionIDs, "item-160") {
		r.Amount = min(r.Amount, int(float64(a.MaxHP)*.2))
		a.HP = min(a.MaxHP, a.HP+r.Amount)
		if a.Status != "" && rng.Float64() < .5 {
			a.Status = ""
			a.Attack = min(999, a.Attack+int(float64(a.BaseStats.Attack)*.7))
		}
	} else if s.Fields.Current() != "darkness" {
		a.HP = max(1, a.HP-r.Amount)
	}
	r.TargetResolution = TargetResolution{TargetEffect: TargetEffect{TargetID: a.ID}, Resolved: true}
	r.Returned, r.Delta, r.Applied = true, a.HP-before, a.HP != before
	return r, nil
}
