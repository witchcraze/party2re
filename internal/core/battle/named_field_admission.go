package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNamedField checks every finite branch before any creation, argument
// consumption, resource writes or caller RNG. External Apply hooks never run.
func (s *LifetimeState) ValidateNamedField(c *NamedFieldContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	if id == "job-56-entry-3" {
		return nil
	}
	if id == "job-55-entry-7" {
		return p.validateRainbow(&child, rng)
	}
	name, base, _ := namedFieldSpec(id)
	if name == "" {
		return ErrUnsupportedEffect
	}
	if name == "grave" {
		h := p.actor(p.FindCallingHelper(c.TargetContext, 64))
		if h == nil || h.HP <= 0 {
			return nil
		}
	}
	if !p.Fields.Create(name, base) {
		return nil
	}
	switch name {
	case "darkness":
		p.createDarkness(&child)
		selected, target, err := child.darknessContinuation()
		if err != nil {
			return err
		}
		if selected != "" {
			child.GuaranteedHit = true
			frame := CallbackContext{TargetContext: child.TargetContext, Arguments: []string{target}, Damage: &child.DamageContext}
			return p.ValidateInvocations(&frame, child.Catalog, []CallbackInvocation{{Kind: "entry", EntryID: selected}}, rng)
		}
	case "cosmic":
		return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, float64(p.actor(c.ActorID).MaxMP)*.075, CategoryMagic), rng)
	case "antlion", "grave":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), name, CategoryUntyped), false)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return ErrUnsupportedEffect
		}
		for _, target := range targets {
			if target == "" {
				return ErrUnsupportedEffect
			}
			if name == "grave" {
				nested := child.DamageContext
				nested.TargetContext = possessionContext(child.TargetContext, target)
				nested.ActorNPC = p.actor(target).NPC
				nested = p.nativeDamageContext(nested, child.Death, rng)
				if err := p.validateGraveStrikes(&nested, child.Death, rng); err != nil {
					return err
				}
			}
		}
	case "burn":
		if child.Death == nil {
			return ErrUnsupportedEffect
		}
		states := []LifetimeState{p}
		for _, a := range p.Actors {
			var next []LifetimeState
			for _, state := range states {
				variants, err := state.projectDamageReset(child.Death, child.TargetContext, a.ID, false, rng)
				if err != nil {
					return err
				}
				for _, variant := range variants {
					variant.actor(a.ID).Tension = 1
					next = appendDefeatState(next, variant)
				}
			}
			states = next
		}
		for _, state := range states {
			for _, id := range child.PartyIDs {
				a := state.actor(id)
				if id != child.ActorID && a.HP > 0 {
					a.HP = max(1, a.HP-300)
				}
			}
			// Area admission already owns ordered defeat/reset projections.
			if err := state.validateNamedBurnArea(&child, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) validateNamedBurnArea(c *NamedFieldContext, rng random.Generator) error {
	// Native projection accepts category-specific effects below; breath stays
	// breath even if a target redirects it back into the producing actor.
	context := c.DamageContext
	power := 300.0
	decay := !s.canPreserveAreaPower(&context, dualAgilityEffect("", power, CategoryBreath, true))
	context.AreaDamageCalls++
	states := []damageContinuation{{State: s.callbackCopy(), Context: context}}
	for _, target := range c.EnemyIDs {
		var next []damageContinuation
		for _, state := range states {
			if state.State.actor(c.ActorID).HP <= 0 {
				continue
			}
			e := dualAgilityEffect(target, power, CategoryBreath, true)
			variants, err := state.State.projectCallbackDamage(&state.Context, c.Death, e, power, true, rng)
			if err != nil {
				return err
			}
			for _, v := range variants {
				next = appendDamageContinuation(next, v.State, v.Context, "")
			}
		}
		states = next
		if decay {
			power *= .85
		}
	}
	for _, state := range states {
		if _, err := state.State.gemTensionCeiling(&state.Context.EffectContext, state.State.actor(c.ActorID), 1); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) validateGraveStrikes(c *DamageContext, defeat *DefeatContext, rng random.Generator) error {
	if err := s.validateNormalStrikes(c, "", 2, rng); err != nil {
		return err
	}
	states := []damageContinuation{{State: s.callbackCopy(), Context: *c}}
	// Include critical/direct and normal power; item147 may add a third/fourth
	// strike. Eligibility is re-read from every earlier continuation.
	hits := 2
	if hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-147") && !c.ActorNPC {
		hits = 4
	}
	for hit := range hits {
		var next []damageContinuation
		for _, state := range states {
			for _, direct := range []bool{false, true} {
				power := float64(state.State.actor(c.ActorID).Attack)
				if direct {
					power *= .75
				}
				e := dualAgilityEffect("", power, CategoryPhysical, direct)
				e.Arguments = []string{"0", "1"}
				if direct {
					e.Arguments[0] = "1"
				}
				variants, err := state.State.projectCallbackDamage(&state.Context, defeat, e, power, hit+1 < hits, rng)
				if err != nil {
					return err
				}
				for _, v := range variants {
					next = appendDamageContinuation(next, v.State, v.Context, "")
				}
			}
		}
		states = next
	}
	return nil
}
