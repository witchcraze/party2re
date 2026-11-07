package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateJobAssets never invokes external ports or draws. Unsupported tails
// reject before the wallet debit, target resolution or any earlier effect.
func (s *LifetimeState) ValidateJobAssets(c *JobAssetContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Assets == nil || c.Assets.RequesterID == "" || c.Assets.Wallet < 0 || s.actor(c.Assets.RequesterID) == nil {
		return ErrUnsupportedEffect
	}
	switch id {
	case "job-78-entry-0":
		return s.validateEnhancedTool(c, rng)
	case "job-50-entry-7":
		return s.validateStoredTool(c, rng)
	case "job-7-entry-0", "job-7-entry-2", "job-8-entry-0", "job-28-entry-3", "job-78-entry-5", "job-81-entry-2":
	default:
		return ErrUnsupportedEffect
	}
	if (id == "job-7-entry-2" || id == "job-81-entry-2") && c.Assets.AddWallet == nil {
		return ErrUnsupportedEffect
	}
	if id == "job-78-entry-5" {
		// Only a player actor writes the durable hand; only an empty hand draws.
		a := s.actor(c.ActorID)
		if a.HeldToolID == nil || (!a.NPC && c.Assets.HeldToolID == nil) || (*a.HeldToolID == "" && c.Assets.HelperToolIDs == nil) {
			return ErrUnsupportedEffect
		}
		if !a.NPC && *a.HeldToolID == "" && *c.Assets.HeldToolID != "" {
			return ErrUnsupportedEffect
		}
	}
	p, child := s.callbackCopy(), *c
	child.ActorNPC = p.actor(c.ActorID).NPC
	child.Arguments = append([]string(nil), c.Arguments...)
	if child.DefeatContext != nil {
		child.DamageContext = p.nativeDamageContext(child.DamageContext, child.DefeatContext, rng)
	}
	power := child.power(p.actor(c.ActorID), id)
	e := dualAgilityEffect(child.shift(), power, CategoryPhysical, false)
	if id == "job-28-entry-3" {
		e.Arguments = []string{"1"}
		if err := p.validateRecovery(&child.EffectContext, e, rng); err != nil {
			return err
		}
		preserve := p.canPreserveAreaPower(&child.DamageContext, e)
		if p.actor(c.ActorID).categorySealed(CategoryPhysical) {
			p.actor(c.ActorID).Status = ""
		}
		child.AreaDamageCalls++
		if err := p.validateJobAssetWave(&child, 0, power, true, rng); err != nil {
			return err
		}
		if preserve {
			return p.validateJobAssetWave(&child, 0, power, false, rng)
		}
		return nil
	}
	states, err := p.projectCallbackDamage(&child.DamageContext, child.DefeatContext, e, power, id == "job-78-entry-5" || id == "job-81-entry-2", rng)
	if err != nil {
		return err
	}
	for _, next := range states {
		if next.TargetID == "" {
			continue
		}
		if id == "job-78-entry-5" {
			if err := next.State.ValidateStatUp(&next.Context.EffectContext, controlEffect(c.ActorID, .25, CategoryUntyped), StatAgility, rng); err != nil {
				return err
			}
		} else if id == "job-81-entry-2" {
			if err := next.State.validatePartyTarget(next.Context.TargetContext, controlEffect(c.ActorID, 1, CategoryPhysical), rng); err != nil {
				return err
			}
		}
	}
	return nil
}

// Area damage must admit later targets against each earlier death/reset outcome.
func (s *LifetimeState) validateJobAssetWave(c *JobAssetContext, index int, power float64, decay bool, rng random.Generator) error {
	if index == len(c.EnemyIDs) || s.actor(c.ActorID).HP <= 0 {
		return nil
	}
	e := dualAgilityEffect(c.EnemyIDs[index], power, CategoryPhysical, true)
	states, err := s.projectCallbackDamage(&c.DamageContext, c.DefeatContext, e, power, index+1 < len(c.EnemyIDs), rng)
	if err != nil {
		return err
	}
	if decay {
		power *= .85
	}
	for _, next := range states {
		child := *c
		child.DamageContext = next.Context
		if err := next.State.validateJobAssetWave(&child, index+1, power, decay, rng); err != nil {
			return err
		}
	}
	return nil
}
