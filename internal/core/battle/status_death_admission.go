package battle

import "github.com/witchcraze/party2re/internal/core/random"

// Keep helper recovery attached to its original recipient across one-pass
// temporary and field redirects; the redirected target does not absorb again.
func (s *LifetimeState) statusTargetCandidates(c *DamageContext, e TargetEffect) ([]TargetResolution, error) {
	var candidates []TargetResolution
	for _, id := range s.damageInitialTargetIDs(c, e) {
		effects := []TargetEffect{e}
		effects[0].TargetID = id
		recoveries := []TargetResolution{{}}
		if s.actor(c.ActorID).Temporary != TemporaryPhantom {
			effects = s.temporaryTargetCandidates(c.TargetContext, e, []string{id}, nil)
			if s.actor(id).Temporary == TemporaryAbsorbMagic && e.Category == CategoryMagic {
				recoveries = nil
				for i := 0; i < 20; i++ {
					incoming := e
					incoming.TargetID = id
					r, err := s.ResolveTemporary(c.TargetContext, incoming, resetBoundary((float64(i)+.5)/20))
					if err != nil {
						return nil, err
					}
					recoveries = append(recoveries, TargetResolution{MPRecoveryTarget: r.MPRecoveryTarget, MPRecovery: r.MPRecovery})
				}
			}
		}
		for _, effect := range s.phantomTargetCandidates(c.TargetContext, effects) {
			for _, recovery := range recoveries {
				recovery.TargetEffect = effect
				candidates = append(candidates, recovery)
			}
		}
	}
	return candidates, nil
}

// Each possible recipient keeps its own continuation. Combining mutually
// exclusive hits would invent deaths; dropping survivors loses later lethal hits.
// followed also projects the last reward/field tail before an enclosing effect.
func (s *LifetimeState) projectAreaDeath(c *StatusContext, e TargetEffect, chance float64, bypass, followed bool, rng random.Generator) ([]LifetimeState, error) {
	p := s.defeatCopy()
	converted, _, err := p.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return nil, err
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
				return nil, err
			}
			converted, _, err := state.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
			if err != nil {
				return nil, err
			}
			candidates, err := state.statusTargetCandidates(&DamageContext{EffectContext: c.EffectContext}, converted)
			if err != nil {
				return nil, err
			}
			// A failed death roll still retains the resolved helper's recovery.
			// Only an unresolved helper can continue without those resource facts.
			if len(candidates) == 0 || state.actor(c.ActorID).categorySealed(converted.Category) {
				next = appendDefeatState(next, state)
			}
			for _, candidate := range candidates {
				p := state.defeatCopy()
				t := p.actor(candidate.TargetID)
				applyStatusResources(&p, candidate)
				next = appendDefeatState(next, p)
				if t.HP <= 0 || candidate.Payload.Symbol == "" || statusImmune(t, bypass, true, p.Fields.Current()) {
					continue
				}
				child := *c.Defeat
				child.TargetContext = c.TargetContext
				continuations, err := p.projectDefeatContinuation(&child, t.ID, rng, map[string]bool{}, followed || index+1 < len(c.EnemyIDs))
				if err != nil {
					return nil, err
				}
				for _, continuation := range continuations {
					next = appendDefeatState(next, continuation)
				}
			}
		}
		states = next
	}
	return states, nil
}
