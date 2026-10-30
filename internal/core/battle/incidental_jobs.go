package battle

import "github.com/witchcraze/party2re/internal/core/random"

// IncidentalJobContext contains operation facts; consumers rebuild effect ports
// after reload. JokeOutcomes contains native presentation indices, never prose.
type IncidentalJobContext struct {
	DamageContext
	Arguments    []string
	Status       StatusContext
	JokeOutcomes []int
}

func (c *IncidentalJobContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func incidentalTaunt(id string) string {
	switch id {
	case "job-8-entry-5":
		return "job-55-entry-3"
	case "job-8-entry-6":
		return "job-55-entry-5"
	}
	return ""
}

func incidentalAreaStatus(id string) (string, CallbackCategory, float64) {
	switch id {
	case "job-7-entry-7":
		return StatusConfusion, CategoryDance, 60
	case "job-9-entry-3":
		return StatusConfusion, CategoryBreath, 45
	case "job-9-entry-5":
		return StatusSleep, CategoryBreath, 35
	}
	return "", CategoryUntyped, 0
}

func incidentalRaw(id string) bool {
	return id == "job-8-entry-2" || id == "job-8-entry-3" || id == "job-8-entry-4" || id == "job-8-entry-7" || id == "job-8-entry-8"
}

// ProjectIncidentalJob projects the exact enemy tension reset only.
func (s *LifetimeState) ProjectIncidentalJob(c *IncidentalJobContext, id string) (bool, error) {
	if err := s.ValidateIncidentalJob(c, id, random.Default()); err != nil {
		return false, err
	}
	if id != "job-8-entry-8" {
		return false, nil
	}
	for _, target := range c.EnemyIDs {
		s.actor(target).Tension = 1
	}
	return true, nil
}

// ApplyIncidentalJob executes intrinsic bodies; the invocation/consumer owns
// ordinary costs, random MP restoration and committed receipt replay.
func (s *LifetimeState) ApplyIncidentalJob(c *IncidentalJobContext, id string, rng random.Generator) error {
	if err := s.ValidateIncidentalJob(c, id, rng); err != nil {
		return err
	}
	if taunt := incidentalTaunt(id); taunt != "" {
		child := TreatmentMischiefContext{DamageContext: c.DamageContext, Arguments: c.Arguments, Status: c.Status}
		err := s.ApplyTreatmentMischief(&child, taunt, rng)
		c.DamageContext, c.Arguments, c.Status = child.DamageContext, child.Arguments, child.Status
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	defer func() { c.Status.EffectContext = c.EffectContext }()
	if id == "job-7-entry-5" || id == "job-9-entry-7" || id == "job-9-entry-9" {
		previousReset, previousDefeat := c.Reset, c.Defeat
		damage := s.nativeDamageContext(c.DamageContext, c.Status.Defeat, rng)
		c.Reset, c.Defeat = damage.Reset, damage.Defeat
		defer func() { c.Reset, c.Defeat = previousReset, previousDefeat }()
	}
	if symbol, category, chance := incidentalAreaStatus(id); symbol != "" {
		c.Status.EffectContext = c.EffectContext
		_, err := s.ApplyAreaStatus(&c.Status, controlSymbol("", symbol, category), chance, false, false, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-7-entry-1", "job-9-entry-2":
		target, category, stat := c.ActorID, CategoryPhysical, StatDefense
		if id == "job-9-entry-2" {
			target, category, stat = c.shift(), CategoryMagic, StatAgility
		}
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(target, .4, category), stat, rng)
		return err
	case "job-7-entry-3":
		_, err := s.ApplyPartyStat(&c.EffectContext, .2, CategoryBreath, StatHit, true, rng)
		return err
	case "job-7-entry-5":
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Defense)*1.6, CategoryPhysical, false), rng)
		if err != nil || r.Amount <= 0 {
			return err
		}
		_, err = s.ApplyRecoil(&c.EffectContext, &c.End, float64(r.Amount)*.07, rng)
		return err
	case "job-7-entry-6":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "share-mana", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		amount := min(100, a.MP/2)
		a.MP -= amount
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(r.TargetID, float64(amount), CategoryMagic), rng)
		return err
	case "job-8-entry-1":
		a.Status = StatusSleep
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(a.ID, float64(a.MaxHP)*.5, CategoryUntyped), rng)
		return err
	case "job-8-entry-2", "job-8-entry-3", "job-8-entry-4":
		t := s.actor(s.randomMember(rng))
		t.Status = map[string]string{"job-8-entry-2": StatusParalyze, "job-8-entry-3": StatusDofuu, "job-8-entry-4": StatusDeadlyPoison}[id]
		t.Tension = 1
	case "job-8-entry-7":
		c.JokeOutcomes = append(c.JokeOutcomes, int(rng.Float64()*7))
		for i := range s.Actors {
			t := &s.Actors[i]
			if t.ID == c.ActorID || rng.Float64()*3 < 1 {
				continue
			}
			t.Status, t.Tension = StatusDofuu, 1
		}
	case "job-8-entry-8":
		for _, target := range c.EnemyIDs {
			s.actor(target).Tension = 1
		}
	case "job-9-entry-0":
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Agility)*1.4, CategoryPhysical, false), rng)
		return err
	case "job-9-entry-1":
		_, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .4, CategoryMagic), StatAgility, rng)
		return err
	case "job-9-entry-7":
		c.AdditionalEffect = true
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack)*1.2, CategoryPhysical, false), rng)
		if err != nil || !r.Returned || r.TargetID == "" || s.actor(r.TargetID).HP <= 0 {
			return err
		}
		c.Status.EffectContext = c.EffectContext
		_, err = s.ApplyStatus(&c.Status, controlSymbol(r.TargetID, StatusParalyze, CategoryPhysical), 30, false, false, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-9-entry-8":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "armor-break", CategoryPhysical), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		t.Temporary, t.Tension, t.Defense = "", 1, t.BaseStats.Defense
		_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(t.ID, .1, CategoryPhysical), StatDefense, rng)
		return err
	case "job-9-entry-9":
		return s.applyIncidentalDrain(c, resources, rng)
	}
	return nil
}

func (s *LifetimeState) incidentalDrainPrepare(c *IncidentalJobContext, target string) (int, int) {
	a, t := s.actor(c.ActorID), s.actor(target)
	hit, defense, agility := a.Hit, 0, 0
	if t != nil {
		defense, agility = t.Defense, t.Agility
		t.Defense = min(t.Defense, t.Agility)
	}
	if a.Agility >= agility {
		a.Hit = 100
	}
	return hit, defense
}

func (s *LifetimeState) incidentalDrainRestore(c *IncidentalJobContext, target string, hit, defense int) {
	if t := s.actor(target); t != nil {
		t.Defense = defense
	}
	s.actor(c.ActorID).Hit = hit
}

func (s *LifetimeState) applyIncidentalDrain(c *IncidentalJobContext, resources func(TargetResolution), rng random.Generator) error {
	r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "drain", CategoryUntyped), rng)
	if err != nil {
		return err
	}
	resources(r)
	target := ""
	if r.Resolved {
		target = r.TargetID
	}
	hit, defense := s.incidentalDrainPrepare(c, target)
	result, err := func() (EffectApplication, error) {
		defer s.incidentalDrainRestore(c, target, hit, defense)
		return s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, float64(s.actor(c.ActorID).Agility)*1.7, CategoryPhysical, false), rng)
	}()
	if err != nil || !result.Returned || result.TargetID == "" {
		return err
	}
	_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(result.Amount), CategoryPhysical), rng)
	return err
}
