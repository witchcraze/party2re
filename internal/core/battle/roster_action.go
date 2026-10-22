package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateRoster is shared by discovery and execution. It certifies only this
// producer; missing status/death/MP/field closures remain named whole-entry
// gates. Ordinary costs belong to the caller, delegated listed costs are waived.
func (s *LifetimeState) ValidateRoster(c *RosterContext, id, targetID string, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if id == "job-66-revive" {
		return s.ValidateRevival(&RevivalContext{TargetContext: c.TargetContext}, id, targetID, rng)
	}
	if id == "tool-78" || id == "gem-82" || npcRosterCommand(id) == "summon" {
		return s.validateEnemySummon(c, id)
	}
	if npcRosterCommand(id) == "escape" {
		return nil
	}
	if p, ok := toolHelper(id); ok {
		if s.rosterIdentity(p.Identity) != nil {
			return nil
		}
		return validateSummonRequester(c)
	}
	job, command, ok := rosterCommand(id)
	if !ok {
		return ErrUnsupportedEffect
	} // Includes recursive item110.
	if command == "summon" {
		if s.rosterIdentity(callingIdentity(job)) != nil {
			return nil
		}
		if err := validateSummonRequester(c); err != nil {
			return err
		}
		a := s.actor(c.ActorID)
		for _, v := range []int{a.MaxHP, a.MaxMP, a.BaseStats.Attack, a.BaseStats.Defense, a.BaseStats.Agility} {
			if v < 0 || v > 1e10 {
				return ErrInvalidParticipant
			}
		}
		return nil
	}
	if command == "duplicate" {
		one, two := s.rosterIdentity(callingIdentity(63)), s.rosterIdentity("calling-63-copy")
		if one != nil && one.HP > 0 && two == nil || two != nil && two.HP > 0 && one == nil {
			if err := validateSummonRequester(c); err != nil {
				return err
			}
		}
		return s.validateRosterHealing(c, 100, CategoryUntyped, rng)
	}
	helperID := s.FindCallingHelper(c.TargetContext, job)
	if command == "dismiss" {
		helperID = ""
		if a := s.rosterIdentity(callingIdentity(job)); a != nil {
			helperID = a.ID
		}
	}
	h := s.actor(helperID)
	// Distribution has its dance-state check even when no helper can act.
	if command == "distribution" {
		return s.ValidateDistributedAttack(&c.EffectContext, helperID, c.Cursor, rng)
	}
	if h == nil || h.HP <= 0 {
		return nil
	}
	if h.RosterIdentity == callingIdentity(87) && (h.SummonForm < 0 || h.SummonForm > 2) {
		return ErrInvalidParticipant
	}
	if job == 87 && h.RosterIdentity == callingIdentity(87) && h.SummonForm < 2 {
		if err := validateSummonRequester(c); err != nil {
			return err
		}
	}
	switch command {
	case "dismiss":
		if c.EnemyTeamID == "" || c.EnemyTeamID == s.actor(c.ActorID).TeamID {
			return ErrUnsupportedEffect
		}
	case "defend", "merge", "revive-party":
		return nil
	case "attack":
		child := c.DamageContext
		child.ActorID, child.ActorNPC = helperID, h.NPC
		return s.ValidateNormalAttack(&child, targetID, rng)
	case "small":
		return validateSummonRequester(c)
	case "party-heal":
		return s.validateRosterHealing(c, 999, CategoryMagic, rng)
	case "rain":
		if err := s.validateAreaDamage(&c.DamageContext, rosterEffect("", 170, CategoryMagic, true), rng); err != nil {
			return err
		}
		return s.validateRosterHealing(c, 150, CategoryMagic, rng)
	case "barrage":
		return s.validateRepeatedDamage(&c.DamageContext, rosterEffect("", float64(h.Agility)*2, CategoryPhysical, false), 4, rng)
	case "fallen-power":
		return s.validateAreaDamage(&c.DamageContext, rosterEffect("", float64(s.fallenPower(c)), CategoryUntyped, true), rng)
	case "ride":
		a := s.actor(c.ActorID)
		if s.actor(targetID) != nil {
			return s.ValidateDamage(&c.DamageContext, rosterEffect(targetID, float64(a.Agility+h.Agility)*.6, CategoryPhysical, true), rng)
		}
		return s.validateDamageSequence(&c.DamageContext, []TargetEffect{rosterEffect("", float64(a.Agility)*.7, CategoryPhysical, true), rosterEffect("", float64(h.Agility)*.7, CategoryPhysical, true)}, rng)
	case "share-mp":
		// The symbolic target pass can spend MP, redirect and reject independently
		// of the following damage. Admit the maximum downstream power first.
		converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, TargetEffect{Category: CategoryMagic, Payload: CallbackPayload{Symbol: "party-power"}})
		if err != nil {
			return err
		}
		child := c.DamageContext
		projected := *s
		projected.Actors = append([]LifetimeActor(nil), s.Actors...)
		if projected.actor(c.ActorID).categorySealed(converted.Category) {
			projected.actor(c.ActorID).Status = ""
		}
		return projected.ValidateDamage(&child, rosterEffect("", float64(100+85*(len(c.PartyIDs)-1)), CategoryMagic, true), rng)
	case "special":
		return s.validateCallingSpecial(c, job, helperID, targetID, rng)
	case "double-tail":
		return s.validateDoubleTail(c, rng)
	case "venom":
		return s.validateVenom(c, helperID, rng)
	case "grave", "drain-barrage":
		// #1365 status/defeat, #1366 fields, #1337 MP drain.
		return ErrUnsupportedEffect
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

func validateSummonRequester(c *RosterContext) error {
	if c.RequesterLevel == nil || *c.RequesterLevel < 0 {
		return ErrUnsupportedEffect
	}
	return nil
}

func rosterEffect(target string, power float64, category CallbackCategory, direct bool) TargetEffect {
	e := TargetEffect{TargetID: target, Payload: CallbackPayload{Number: power}, Category: category}
	if direct {
		e.Arguments = []string{"1"}
	}
	return e
}

func (s *LifetimeState) validateRosterHealing(c *RosterContext, power float64, category CallbackCategory, rng random.Generator) error {
	for _, id := range c.PartyIDs {
		if err := s.validatePartyTarget(c.TargetContext, rosterEffect(id, power, category, false), rng); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) fallenPower(c *RosterContext) int {
	power := 100
	for _, id := range c.PartyIDs {
		if s.actor(id).HP <= 0 {
			power += 420
		}
	}
	return power
}

// ApplyRoster never schedules an NPC action or finishes a player/NPC tail.
// A command shares its existing operation counters and restores the actor even
// on errors or roster growth. Only resulting facts are serialized.
func (s *LifetimeState) ApplyRoster(c *RosterContext, id, targetID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{}
	if err := s.ValidateRoster(c, id, targetID, rng); err != nil {
		return r, err
	}
	if id == "job-66-revive" {
		v, err := s.ApplyRevival(&RevivalContext{TargetContext: c.TargetContext}, id, targetID, rng)
		return RosterResult{Applied: v.Applied, HelperID: s.FindCallingHelper(c.TargetContext, 66), TargetConsumed: v.TargetConsumed, Effects: v.Applications}, err
	}
	if id == "tool-78" || id == "gem-82" || npcRosterCommand(id) == "summon" {
		return s.summonEnemy(c, id, rng)
	}
	if npcRosterCommand(id) == "escape" {
		s.actor(c.ActorID).HP = 0
		return RosterResult{Applied: true}, nil
	}
	if p, ok := toolHelper(id); ok {
		if id == "extension-244" && s.rosterIdentity(p.Identity) == nil {
			if rng.Float64()*3 >= 1 {
				p.Form = 1
				if rng.Float64()*2 >= 1 {
					p.Form = 2
				}
			}
		}
		return s.AddHelper(c, p, rng)
	}
	job, command, _ := rosterCommand(id)
	if command == "summon" {
		return s.summonCalling(c, job, rng)
	}
	if command == "duplicate" {
		return s.duplicateHelper(c, rng)
	}
	helperID := s.FindCallingHelper(c.TargetContext, job)
	if command == "dismiss" {
		helperID = ""
		if a := s.rosterIdentity(callingIdentity(job)); a != nil {
			helperID = a.ID
		}
	}
	h := s.actor(helperID)
	if command == "distribution" {
		v, err := s.ApplyDistributedAttack(&c.EffectContext, helperID, c.Cursor, rng)
		return RosterResult{Applied: len(v.RaisedIDs) > 0 || len(v.Tails) > 0, HelperID: helperID, Effects: v.Tails}, err
	}
	if h == nil || h.HP <= 0 {
		return r, nil
	}
	r.HelperID = helperID
	var err error
	switch command {
	case "dismiss":
		s.dismissHelper(c, helperID)
		r.Applied = true
	case "defend":
		h.Temporary = TemporaryDefense
		r.Applied = true
		if job == 87 {
			s.growHelper(c, helperID, 5, rng)
		}
	case "merge":
		return s.mergeHelper(c, helperID, job), nil
	case "attack":
		r.TargetConsumed = true
		actor, npc := c.ActorID, c.ActorNPC
		c.ActorID, c.ActorNPC = helperID, h.NPC
		defer func() { c.ActorID, c.ActorNPC = actor, npc }()
		r.Hits, err = s.ApplyNormalAttack(&c.DamageContext, targetID, rng)
		c.ActorID, c.ActorNPC = actor, npc
		r.Applied = len(r.Hits) > 0
		if err == nil && job == 87 {
			s.growHelper(c, helperID, 4, rng)
		}
	case "small":
		return s.smallHelpers(c, helperID, rng)
	case "revive-party":
		r.Applied = s.reviveCallingParty(c)
	case "party-heal":
		r.Effects, err = s.ApplyPartyHealing(&c.EffectContext, 999, CategoryMagic, rng)
	case "rain":
		r.Effects, err = s.ApplyAreaDamage(&c.DamageContext, 170, CategoryMagic, true, rng)
		if err == nil {
			var heals []EffectApplication
			heals, err = s.ApplyPartyHealing(&c.EffectContext, 150, CategoryMagic, rng)
			r.Effects = append(r.Effects, heals...)
		}
	case "barrage":
		hits := 3 + int(rng.Float64()*2)
		for range hits {
			if s.actor(c.ActorID).HP <= 0 {
				break
			}
			var hit EffectApplication
			hit, err = s.ApplyDamage(&c.DamageContext, rosterEffect("", float64(s.actor(helperID).Agility)*2, CategoryPhysical, false), rng)
			if err != nil {
				break
			}
			r.Effects = append(r.Effects, hit)
		}
	case "fallen-power":
		r.Effects, err = s.ApplyAreaDamage(&c.DamageContext, float64(s.fallenPower(c)), CategoryUntyped, true, rng)
	case "ride":
		r.TargetConsumed = true
		a := s.actor(c.ActorID)
		hits := 1
		if s.actor(targetID) == nil {
			hits = 2
		}
		for i := range hits {
			e := rosterEffect(targetID, float64(a.Agility+s.actor(helperID).Agility)*.6, CategoryPhysical, true)
			if hits == 2 {
				power := float64(a.Agility) * .7
				if i == 1 {
					power = float64(s.actor(helperID).Agility) * .7
				}
				e = rosterEffect("", power, CategoryPhysical, true)
			}
			var hit EffectApplication
			hit, err = s.ApplyDamage(&c.DamageContext, e, rng)
			if err != nil {
				break
			}
			r.Effects = append(r.Effects, hit)
		}
	case "share-mp":
		r.TargetConsumed = true
		target, resolveErr := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: targetID, Category: CategoryMagic, Payload: CallbackPayload{Symbol: "party-power"}}, rng)
		r.Effects = append(r.Effects, EffectApplication{TargetResolution: target})
		if resolveErr != nil || !target.Resolved {
			return r, resolveErr
		}
		power := 100
		for _, id := range c.PartyIDs {
			a := s.actor(id)
			if id != c.ActorID && a.MP >= 5 {
				a.MP -= 5
				power += 85
			}
		}
		var hit EffectApplication
		hit, err = s.ApplyDamage(&c.DamageContext, rosterEffect(target.TargetID, float64(power), CategoryMagic, true), rng)
		r.Effects = append(r.Effects, hit)
	case "special":
		return s.applyCallingSpecial(c, job, helperID, targetID, rng)
	case "double-tail":
		return s.applyDoubleTail(c, rng)
	case "venom":
		return s.applyVenom(c, helperID, rng)
	}
	r.Applied = r.Applied || len(r.Effects) > 0
	return r, err
}
