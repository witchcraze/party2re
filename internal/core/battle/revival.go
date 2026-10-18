package battle

import "github.com/witchcraze/party2re/internal/core/random"

// RevivalContext shares the fully admitted defeat closure with damage. Defeat
// includes automatic revival, reset and rewards, even for a living sacrifice.
// Missing owners retain whole-entry gates; no fallback damage/reset is invented.
type RevivalContext struct {
	TargetContext
	Defeat func(string) error
	Deaths *RevivalDeathCallbacks
}

// RevivalDeathCallbacks is the status owner's complete area instant-death
// operation. Validate must admit every descendant before the enclosing action.
type RevivalDeathCallbacks struct {
	Validate func(*LifetimeState, TargetContext, CallbackCategory, float64) error
	Execute  func(*LifetimeState, TargetContext, CallbackCategory, float64, random.Generator) error
}

type RevivalActionResult struct {
	Applied, TargetConsumed bool
	Applications            []EffectApplication
}

func revivalTarget(id, targetID string) TargetEffect {
	e := TargetEffect{TargetID: targetID, Category: CategoryMagic, Payload: CallbackPayload{Symbol: "revival"}}
	if id == "tool-6" || id == "job-66-revive" {
		e.Category = CategoryUntyped
	}
	if id == "job-50-entry-6" {
		e.Payload.Symbol = "last"
	}
	return e
}

// ValidateRevival is the inert discovery/execution gate. Ordinary learning,
// equipment ownership and listed costs belong to the enclosing action owner.
// The enclosing ordinary action admits fallen-self only for66.6; these bodies
// retain the source's absence of a living-caster guard for indirect invocation.
func (s *LifetimeState) ValidateRevival(c *RevivalContext, id, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	b, ok := revivalSpec(id)
	if !ok {
		return ErrUnsupportedEffect
	}
	for _, a := range s.Actors {
		if a.MaxHP < 0 || a.MaxHP > 1e12 || a.MaxMP < 0 || a.MaxMP > 1e12 || a.MP < 0 || a.MP > 1e12 {
			return ErrInvalidParticipant
		}
	}
	if b.mode == "helper" {
		h := s.actor(s.FindCallingHelper(c.TargetContext, 66))
		if h == nil || h.HP <= 0 || s.actor(c.ActorID).HP <= 0 {
			return nil
		}
	}
	if b.mode == "single" || b.mode == "last" || b.mode == "mp" || b.mode == "helper" {
		e := revivalTarget(id, targetID)
		if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return err
		}
		if b.mode != "last" {
			return nil
		}
		reachable := false
		for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, nil) {
			reachable = reachable || candidate.TargetID != c.ActorID
		}
		if !reachable {
			return nil
		}
	}
	if b.mode == "sacrifice" || b.mode == "death-sacrifice" || b.mode == "last" {
		if c.Defeat == nil {
			return ErrUnsupportedEffect
		}
	}
	if b.mode == "death-sacrifice" {
		if c.Deaths == nil || c.Deaths.Validate == nil || c.Deaths.Execute == nil {
			return ErrUnsupportedEffect
		}
		return c.Deaths.Validate(s, c.TargetContext, CategoryUntyped, 60)
	}
	return nil
}

// ValidateRevivalAction adds ordinary submission's ban and fallen-self gates.
// Random/custom/delegated bodies use ValidateRevival, like the source callback.
func (s *LifetimeState) ValidateRevivalAction(c *RevivalContext, id, targetID string, rng random.Generator) error {
	if err := s.ValidateRevival(c, id, targetID, rng); err != nil {
		return err
	}
	if s.SkillBans[id] > 0 {
		return ErrUnsupportedEffect
	}
	if s.actor(c.ActorID).HP <= 0 {
		h := s.actor(s.FindCallingHelper(c.TargetContext, 66))
		if id != "job-66-revive" || h == nil || h.HP <= 0 {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

// ApplyRevival preserves callback-specific target, seal, chance and rounding
// rules. It never runs an ordinary heal, resets a direct recipient or clamps
// MP-based HP. Evidence: _skill.cgi and _data.cgi spans in docs/design/battle.md.
func (s *LifetimeState) ApplyRevival(c *RevivalContext, id, targetID string, rng random.Generator) (RevivalActionResult, error) {
	r := RevivalActionResult{}
	if err := s.ValidateRevival(c, id, targetID, rng); err != nil {
		return r, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	b, _ := revivalSpec(id)
	if b.mode == "helper" {
		return s.applyHelperRevival(c, targetID, rng)
	}
	if b.mode == "party" {
		if id == "job-32-entry-4" {
			allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryMagic, rng)
			if err != nil || !allowed {
				return r, err
			}
		}
		for _, target := range c.PartyIDs {
			t := s.actor(target)
			if t.HP > 0 || (id == "extension-261" && hasAbility(t.Abilities, "cursed")) {
				continue
			}
			if b.chance < 1 && rng.Float64() >= b.chance {
				continue
			}
			before := t.HP
			t.HP = int(float64(t.MaxHP) * b.fraction)
			if id == "extension-261" {
				t.Abilities = append(t.Abilities, "cursed")
			}
			r.Applications = append(r.Applications, arithmeticApplication(target, before, t.HP, 0))
		}
	} else if b.mode == "sacrifice" || b.mode == "death-sacrifice" {
		if b.mode == "death-sacrifice" {
			if err := c.Deaths.Execute(s, c.TargetContext, CategoryUntyped, 60, rng); err != nil {
				return r, err
			}
		} else {
			for _, target := range c.PartyIDs {
				if target == c.ActorID {
					continue
				}
				t := s.actor(target)
				before := t.HP
				t.HP = t.MaxHP
				r.Applications = append(r.Applications, arithmeticApplication(target, before, t.HP, 0))
			}
		}
		return s.finishSacrifice(c, id, r)
	} else {
		r.TargetConsumed = true
		target, err := s.resolveRevivalTarget(c, id, targetID, rng)
		if err != nil || !target.Resolved {
			return r, err
		}
		t := s.actor(target.TargetID)
		if b.mode == "last" {
			if t.ID == c.ActorID {
				return r, nil
			}
			before := t.HP
			t.HP, t.MP = t.MaxHP, t.MaxMP
			r.Applications = append(r.Applications, arithmeticApplication(t.ID, before, t.HP, 0))
			return s.finishSacrifice(c, id, r)
		}
		if t.HP > 0 || (b.mode == "mp" && t.MP < 1) {
			return r, nil
		}
		if b.chance < 1 && rng.Float64() >= b.chance {
			return r, nil
		}
		before := t.HP
		t.HP = int(float64(t.MaxHP) * b.fraction)
		if b.mode == "mp" {
			t.HP, t.MP = t.MP, t.MP/2
		}
		if id == "book-196" && s.Fields.Current() != "library" {
			s.actor(c.ActorID).Status = StatusMagicSeal
		}
		r.Applications = append(r.Applications, arithmeticApplication(t.ID, before, t.HP, 0))
	}
	r.Applied = len(r.Applications) > 0
	return r, nil
}

func (s *LifetimeState) finishSacrifice(c *RevivalContext, id string, r RevivalActionResult) (RevivalActionResult, error) {
	// Do not clear HP first: defeat can prevent the sacrifice by reviving actor.
	if err := c.Defeat(c.ActorID); err != nil {
		return r, err
	}
	s.actor(c.ActorID).MP = 1
	if id == "gem-66" {
		s.actor(c.ActorID).MP = 0
	}
	r.Applied = true
	return r, nil
}

func (s *LifetimeState) applyHelperRevival(c *RevivalContext, targetID string, rng random.Generator) (RevivalActionResult, error) {
	r := RevivalActionResult{}
	helperID := s.FindCallingHelper(c.TargetContext, 66)
	h, a := s.actor(helperID), s.actor(c.ActorID)
	if h == nil || h.HP <= 0 {
		return r, nil
	}
	t := a
	before := t.HP
	if a.HP <= 0 {
		t.HP = t.MaxHP / 2
	} else {
		r.TargetConsumed = true
		target, err := s.resolveRevivalTarget(c, "job-66-revive", targetID, rng)
		if err != nil || !target.Resolved {
			return r, err
		}
		t = s.actor(target.TargetID)
		if t.HP > 0 || a.MP <= 40 {
			return r, nil
		}
		before, t.HP = t.HP, 1
		a.MP -= 40
	}
	t.Status = StatusDofuu
	h.HP /= 2
	if s.SkillBans == nil {
		s.SkillBans = make(map[string]int)
	}
	s.SkillBans["job-66-revive"] = 3
	r.Applied = true
	r.Applications = append(r.Applications, arithmeticApplication(t.ID, before, t.HP, 0))
	return r, nil
}

func (s *LifetimeState) resolveRevivalTarget(c *RevivalContext, id, targetID string, rng random.Generator) (TargetResolution, error) {
	r, err := s.ResolvePartyTarget(c.TargetContext, revivalTarget(id, targetID), rng)
	if err == nil && r.MPRecoveryTarget != "" {
		t := s.actor(r.MPRecoveryTarget)
		t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
	}
	return r, err
}
