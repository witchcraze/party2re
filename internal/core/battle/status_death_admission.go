package battle

import "github.com/witchcraze/party2re/internal/core/random"

// Each possible recipient keeps its own continuation. Combining mutually
// exclusive hits would invent deaths; dropping survivors loses later lethal hits.
func (s *LifetimeState) validateAreaDeath(c *StatusContext, e TargetEffect, chance float64, bypass bool, rng random.Generator) error {
	p := s.defeatCopy()
	converted, _, err := p.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	if p.actor(c.ActorID).categorySealed(converted.Category) {
		p.actor(c.ActorID).Status = ""
	}
	states := []LifetimeState{p}
	for index, id := range c.EnemyIDs {
		e.TargetID = id
		var next []LifetimeState
		for _, state := range states {
			if err := state.ValidateStatus(c, e, chance, bypass, true, rng); err != nil {
				return err
			}
			next = appendDefeatState(next, state)
			converted, _, err := state.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
			if err != nil {
				return err
			}
			for _, candidate := range state.damageCandidates(&DamageContext{EffectContext: c.EffectContext}, converted) {
				t := state.actor(candidate.TargetID)
				// Symbolic magic absorption still restores30..49 MP before death.
				recoveries := []int{0}
				if candidate.Category == CategoryMagic && t.Temporary == TemporaryAbsorbMagic && state.actor(c.ActorID).Temporary != TemporaryPhantom {
					recoveries = nil
					for i := 0; i < 20; i++ {
						r, err := state.ResolveTemporary(c.TargetContext, candidate, resetBoundary((float64(i)+.5)/20))
						if err != nil {
							return err
						}
						recoveries = append(recoveries, r.MPRecovery)
					}
				}
				for _, recovery := range recoveries {
					p := state.defeatCopy()
					t := p.actor(candidate.TargetID)
					if recovery != 0 {
						t.MP = min(t.MaxMP, t.MP+recovery)
					}
					next = appendDefeatState(next, p)
					if t.HP <= 0 || candidate.Payload.Symbol == "" || statusImmune(t, bypass, true, p.Fields.Current()) {
						continue
					}
					child := *c.Defeat
					child.TargetContext = c.TargetContext
					continuations, err := p.projectDefeatContinuation(&child, t.ID, rng, map[string]bool{}, index+1 < len(c.EnemyIDs))
					if err != nil {
						return err
					}
					for _, continuation := range continuations {
						next = appendDefeatState(next, continuation)
					}
				}
			}
		}
		states = next
	}
	return nil
}
