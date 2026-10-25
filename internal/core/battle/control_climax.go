package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ControlMPDrain is the resource owner's complete MP stat-down operation.
// Validate is pure; Apply shares state/counters and runs only after admission.
type ControlMPDrain struct {
	Validate func(*LifetimeState, *EffectContext, TargetEffect) error
	Apply    func(*LifetimeState, *EffectContext, TargetEffect, random.Generator) (EffectApplication, error)
}

// ControlClimaxContext retains operation facts, including the pre-action
// temporary snapshot (_battle.cgi:587–594). Rebuild hooks after loading facts.
type ControlClimaxContext struct {
	DamageContext
	Arguments      []string
	PriorTemporary map[string]TemporaryEffect
	MPDrain        *ControlMPDrain
	StatusCalls    int
}

// ControlClimaxEntries preserves complete tables, including the field/revival
// positions supplied by their own owners; registration never narrows a pool.
func ControlClimaxEntries(job int) []CallbackEntry {
	switch job {
	case 53:
		return []CallbackEntry{
			{ID: "job-53-entry-0", Name: "binding-thread", RequiredSP: 5, MPCost: 3},
			{ID: "job-53-entry-1", Name: "poison-thread", RequiredSP: 15, MPCost: 4},
			{ID: "job-53-entry-2", Name: "affliction-breath", RequiredSP: 30, MPCost: 16},
			{ID: "job-53-entry-3", Name: "counter-web", RequiredSP: 55, MPCost: 15},
			{ID: "job-53-entry-4", Name: "sealing-web", RequiredSP: 80, MPCost: 8},
			{ID: "job-53-entry-5", Name: "possession-thread", RequiredSP: 120, MPCost: 7},
			{ID: "job-53-entry-6", Name: "trap-field", RequiredSP: 120, MPCost: 1, Extended: true},
		}
	case 58:
		return []CallbackEntry{
			{ID: "job-58-entry-0", Name: "life-reduction", RequiredSP: 20, MPCost: 26},
			{ID: "job-58-entry-1", Name: "confused-trance", RequiredSP: 50, MPCost: 9},
			{ID: "job-58-entry-2", Name: "random-seal", RequiredSP: 80, MPCost: 16},
			{ID: "job-58-entry-3", Name: "mana-reduction", RequiredSP: 110, MPCost: 6},
			{ID: "job-58-entry-4", Name: "double-focus", RequiredSP: 140, MPCost: 33},
			{ID: "job-58-entry-5", Name: "area-life-reduction", RequiredSP: 160, MPCost: 66},
			{ID: "job-58-entry-6", Name: "affliction-arrow", RequiredSP: 160, MPCost: 38, Extended: true},
		}
	case 70:
		return []CallbackEntry{
			{ID: "job-70-entry-0", Name: "meditation", RequiredSP: 50, MPCost: 25},
			{ID: "job-70-entry-1", Name: "paired-strength", RequiredSP: 100, MPCost: 25},
			{ID: "job-70-entry-2", Name: "magic-storm", RequiredSP: 150, MPCost: 40},
			{ID: "job-70-entry-3", Name: "蘇生の声", RequiredSP: 200, MPCost: 27},
			{ID: "job-70-entry-4", Name: "threefold-halo", RequiredSP: 200, MPCost: 66, Extended: true},
		}
	}
	return nil
}

func (c *ControlClimaxContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func controlEffect(target string, power float64, category CallbackCategory) TargetEffect {
	return TargetEffect{TargetID: target, Payload: CallbackPayload{Number: power}, Category: category}
}

func controlSymbol(target, symbol string, category CallbackCategory) TargetEffect {
	return TargetEffect{TargetID: target, Payload: CallbackPayload{Symbol: symbol}, Category: category}
}

func (s *LifetimeState) consumeArrow(c *ControlClimaxContext) (bool, TemporaryEffect) {
	a := s.actor(c.ActorID)
	if a.Status != "" {
		a.Status = ""
		return true, ""
	}
	tmp := a.Temporary
	if tmp == "" {
		tmp = c.PriorTemporary[a.ID]
	}
	a.Temporary = ""
	return false, tmp
}

// ApplyControlClimax executes full intrinsic bodies. Ordinary costs and random
// wrapper MP restoration stay at the enclosing invocation boundary.
func (s *LifetimeState) ApplyControlClimax(c *ControlClimaxContext, id string, rng random.Generator) error {
	if err := s.ValidateControlClimax(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	switch id {
	case "job-53-entry-0", "job-53-entry-1", "job-53-entry-4":
		status := StatusDofuu
		if id == "job-53-entry-1" {
			status = StatusDeadlyPoison
		}
		if id == "job-53-entry-4" {
			status = StatusPhysicalSeal
		}
		return s.controlStatus(c, controlSymbol(c.shift(), status, CategoryBreath), 80, rng)
	case "job-53-entry-2", "job-58-entry-2":
		statuses, category, chance := []string{StatusDeadlyPoison, StatusParalyze, StatusSleep}, CategoryBreath, 60.0
		if id == "job-58-entry-2" {
			statuses, category, chance = []string{StatusMagicSeal, StatusPhysicalSeal, StatusDofuu}, CategoryMagic, 70
		}
		for _, target := range c.EnemyIDs {
			if err := s.controlStatus(c, controlSymbol(target, statuses[int(rng.Float64()*3)], category), chance, rng); err != nil {
				return err
			}
		}
	case "job-53-entry-3":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "counter", CategoryBreath), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			s.actor(r.TargetID).Temporary = TemporaryReflectAttack
		}
	case "job-53-entry-5":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "possession", CategoryBreath), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		original, npc := c.TargetContext, c.ActorNPC
		c.TargetContext = possessionContext(original, r.TargetID)
		c.ActorNPC = s.actor(r.TargetID).NPC
		defer func() { original.Item195Activated = c.Item195Activated; c.TargetContext, c.ActorNPC = original, npc }()
		_, err = s.applyNormalStrike(&c.DamageContext, "", rng)
		return err
	case "job-58-entry-0":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "life-reduction", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved && rng.Float64()*5 > 1 {
			_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .7, CategoryMagic), StatHP, rng)
		}
		return err
	case "job-58-entry-1":
		a.Status = ""
		for _, stat := range []BattleStat{StatAttack, StatAgility} {
			if _, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, 1, CategoryPhysical), stat, rng); err != nil {
				return err
			}
		}
		a = s.actor(c.ActorID)
		a.Temporary, a.Status = TemporaryAbsorbMagic, StatusConfusion
	case "job-58-entry-3":
		_, err := c.MPDrain.Apply(s, &c.EffectContext, controlEffect(c.shift(), .4, CategoryMagic), rng)
		return err
	case "job-58-entry-4":
		a.Temporary = TemporaryDouble
		for range 2 {
			if err := s.statTension(&c.EffectContext, s.actor(c.ActorID), false, rng); err != nil {
				return err
			}
		}
	case "job-58-entry-5":
		for _, target := range c.EnemyIDs {
			if rng.Float64()*3 > 1 {
				if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(target, .75, CategoryMagic), StatHP, rng); err != nil {
					return err
				}
			}
		}
	case "job-58-entry-6":
		status, tmp := s.consumeArrow(c)
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "arrow", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		target := ""
		if r.Resolved {
			target = r.TargetID
		}
		if status {
			s.actor(target).Status = []string{StatusMagicSeal, StatusPhysicalSeal, StatusDofuu}[int(rng.Float64()*3)]
		}
		if tmp != "" {
			s.actor(target).Temporary = tmp
		}
		e := controlEffect(target, float64(s.actor(c.ActorID).Attack)*.7, CategoryMagic)
		e.Arguments = []string{"1"}
		_, err = s.ApplyDamage(&c.DamageContext, e, rng)
		return err
	case "job-70-entry-0":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, 300, CategoryUntyped), rng)
		return err
	case "job-70-entry-1":
		for _, stat := range []BattleStat{StatAttack, StatDefense} {
			if _, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .4, CategoryPhysical), stat, rng); err != nil {
				return err
			}
		}
	case "job-70-entry-2":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 180, CategoryMagic, true, rng)
		return err
	case "job-70-entry-4":
		return s.applyClimax(c, rng)
	}
	return nil
}

func (s *LifetimeState) controlStatus(c *ControlClimaxContext, e TargetEffect, chance float64, rng random.Generator) error {
	status := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls}
	_, err := s.ApplyStatus(&status, e, chance, false, false, rng)
	c.EffectContext = status.EffectContext
	c.StatusCalls = status.StatusCalls
	return err
}

func possessionContext(c TargetContext, id string) TargetContext {
	c.possessionOriginID, c.ActorID = c.ActorID, id
	return c
}

func (s *LifetimeState) applyClimax(c *ControlClimaxContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	power := max(a.Attack, a.Defense, a.Agility)
	r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "halo", CategoryUntyped), rng)
	if err != nil {
		return err
	}
	c.targetResources(r)
	target := r.TargetID
	tmp := s.actor(target).Temporary
	s.actor(target).Temporary = ""
	e := controlEffect(target, float64(power), CategoryMagic)
	e.Arguments = []string{"1"}
	hit, err := s.ApplyDamage(&c.DamageContext, e, rng)
	if err != nil {
		return err
	}
	if s.actor(target).HP > 0 {
		s.actor(target).Temporary = tmp
	}
	a = s.actor(c.ActorID)
	if a.HP <= 0 || hit.Amount <= 0 {
		return nil
	}
	switch power {
	case a.Attack:
		_, err = s.ApplyAreaDamage(&c.DamageContext, float64(hit.Amount)*.3, CategoryUntyped, true, rng)
	case a.Defense:
		_, err = s.ApplyPartyHealing(&c.EffectContext, float64(a.MaxHP)*.3, CategoryUntyped, rng)
	default:
		if _, err = s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .3, CategoryUntyped), StatAgility, rng); err != nil {
			return err
		}
		if rng.Float64()*2 < 1 {
			s.actor(target).Status, s.actor(target).Tension = StatusDofuu, 1
		}
	}
	return err
}
