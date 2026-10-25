package battle

import "github.com/witchcraze/party2re/internal/core/random"

// HighPowerContext holds operation facts; consumers rebuild reset/defeat ports
// and registrations after loading the frozen command and lifetime state.
type HighPowerContext struct {
	DamageContext
	EquipmentReset                          *EquipmentResetContext
	Arguments                               []string
	StatusCalls, AreaStatusCalls, CureCalls int
}

// HighPowerEntries retains every position and listed cost, including extensions.
func HighPowerEntries(job int) []CallbackEntry {
	switch job {
	case 47:
		return []CallbackEntry{
			{ID: "job-47-entry-0", Name: "heavy-strike", RequiredSP: 20, MPCost: 5},
			{ID: "job-47-entry-1", Name: "recoil-strike", RequiredSP: 50, MPCost: 9},
			{ID: "job-47-entry-2", Name: "exhausting-strike", RequiredSP: 80, MPCost: 20},
			{ID: "job-47-entry-3", Name: "wound-strike", RequiredSP: 120, MPCost: 36},
			{ID: "job-47-entry-4", Name: "four-strikes", RequiredSP: 160, MPCost: 40},
			{ID: "job-47-entry-5", Name: "vital-weapon", RequiredSP: 160, MPCost: 47, Extended: true},
		}
	case 48:
		return []CallbackEntry{
			{ID: "job-48-entry-0", Name: "poison-wave", RequiredSP: 40, MPCost: 56},
			{ID: "job-48-entry-1", Name: "sleep-wave", RequiredSP: 80, MPCost: 66},
			{ID: "job-48-entry-2", Name: "blinding-flare", RequiredSP: 120, MPCost: 66},
			{ID: "job-48-entry-3", Name: "life-collapse", RequiredSP: 160, MPCost: 44},
			{ID: "job-48-entry-4", Name: "sweeping-strike", RequiredSP: 200, MPCost: 46},
			{ID: "job-48-entry-5", Name: "renewal-barrier", RequiredSP: 200, MPCost: 20, Extended: true},
		}
	case 49:
		return []CallbackEntry{
			{ID: "job-49-entry-0", Name: "status-ribbon", RequiredSP: 100, MPCost: 10},
			{ID: "job-49-entry-1", Name: "guarding-shield", RequiredSP: 200, MPCost: 20},
			{ID: "job-49-entry-2", Name: "heavy-sword", RequiredSP: 300, MPCost: 30},
		}
	case 52:
		return []CallbackEntry{
			{ID: "job-52-entry-0", Name: "knee-strike", RequiredSP: 20, MPCost: 4},
			{ID: "job-52-entry-1", Name: "gather-strength", RequiredSP: 50, MPCost: 8},
			{ID: "job-52-entry-2", Name: "armor-break", RequiredSP: 80, MPCost: 15},
			{ID: "job-52-entry-3", Name: "vulnerable-focus", RequiredSP: 130, MPCost: 20},
			{ID: "job-52-entry-4", Name: "reckless-wave", RequiredSP: 150, MPCost: 30},
		}
	}
	return nil
}

func (c *HighPowerContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (s *LifetimeState) highPowerWeapon(c *HighPowerContext) {
	a := s.actor(c.ActorID)
	cap := 999
	if a.OverLevel {
		cap = max(999, min(1998, a.BaseStats.Attack*3))
	}
	a.Attack = min(cap, a.Attack+int(float64(a.HP)*.25))
	a.Temporary, a.Hit = TemporaryRegeneration, 95
}

func (s *LifetimeState) highPowerReset(c *HighPowerContext, rng random.Generator) error {
	child := *c.EquipmentReset
	child.TargetContext = c.TargetContext
	for _, member := range s.Actors {
		tension := s.actor(member.ID).Tension
		if err := s.ApplyEquipmentReset(&child, member.ID, rng); err != nil {
			return err
		}
		a := s.actor(member.ID)
		a.Tension = tension
		if a.HP <= 0 {
			a.HP = 1
		}
	}
	c.Cursor = child.Cursor
	*c.EquipmentReset = child
	s.actor(c.ActorID).Temporary = TemporaryAbsorbMagic
	return nil
}

// ApplyHighPower executes intrinsic bodies only. Outer costs and indirect MP
// restoration belong to ApplyInvocations/the submitting consumer.
func (s *LifetimeState) ApplyHighPower(c *HighPowerContext, id string, rng random.Generator) error {
	if err := s.ValidateHighPower(c, id, rng); err != nil {
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
	case "job-47-entry-0", "job-47-entry-1", "job-47-entry-2", "job-49-entry-2", "job-52-entry-0":
		coefficient := 1.2
		if id == "job-47-entry-1" || id == "job-49-entry-2" {
			coefficient = 1.6
		}
		if id == "job-47-entry-2" {
			coefficient = 1.8
		}
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack)*coefficient, CategoryPhysical, false), rng)
		if err != nil {
			return err
		}
		if id == "job-47-entry-1" && r.Returned && r.Amount > 0 {
			_, err = s.ApplyRecoil(&c.EffectContext, &c.End, float64(r.Amount)*.06, rng)
		}
		if id == "job-47-entry-2" && r.Returned && r.TargetID != "" {
			a = s.actor(c.ActorID)
			a.Attack -= int(float64(a.Attack) * .1)
		}
		return err
	case "job-47-entry-3", "job-48-entry-3", "job-52-entry-2":
		category := CategoryPhysical
		if id == "job-48-entry-3" {
			category = CategoryMagic
		}
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "collapse", category), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		switch id {
		case "job-47-entry-3":
			_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(t.ID, float64(min(400, t.MaxHP-t.HP+10)), CategoryPhysical, true), rng)
		case "job-48-entry-3":
			if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 && rng.Float64()*3 < 1 {
				t.HP = 1
			}
		case "job-52-entry-2":
			t.Temporary, t.Tension, t.Defense = "", 1, t.BaseStats.Defense
			_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(t.ID, .1, CategoryPhysical), StatDefense, rng)
		}
		return err
	case "job-47-entry-4":
		for range 4 {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("", float64(a.Attack), CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
	case "job-47-entry-5":
		s.highPowerWeapon(c)
	case "job-48-entry-0", "job-48-entry-1":
		c.AdditionalEffect = true
		power, chance, status := 120.0, 40.0, StatusDeadlyPoison
		if id == "job-48-entry-1" {
			power, chance, status = 70, 30, StatusSleep
		}
		if _, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryMagic, true, rng); err != nil {
			return err
		}
		child := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls}
		_, err := s.ApplyAreaStatus(&child, controlSymbol("", status, CategoryMagic), chance, false, false, rng)
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls
		return err
	case "job-48-entry-2":
		c.AdditionalEffect = true
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 300, CategoryMagic, true), rng)
		if err == nil && r.Returned && r.TargetID != "" && s.actor(r.TargetID).HP > 0 {
			_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .08, CategoryMagic), StatHit, rng)
		}
		return err
	case "job-48-entry-4":
		_, err := s.ApplyAreaDamage(&c.DamageContext, float64(a.Attack)*1.2, CategoryPhysical, false, rng)
		return err
	case "job-48-entry-5":
		return s.highPowerReset(c, rng)
	case "job-49-entry-0":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "cure", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		child := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
		_, err = s.ApplyCure(&child, controlSymbol(r.TargetID, s.actor(r.TargetID).Status, CategoryUntyped), rng)
		c.EffectContext, c.CureCalls = child.EffectContext, child.CureCalls
		return err
	case "job-49-entry-1", "job-52-entry-1":
		stat, coefficient := StatDefense, .6
		if id == "job-52-entry-1" {
			stat, coefficient = StatAttack, 1
		}
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, coefficient, CategoryPhysical), stat, rng)
		if err == nil && id == "job-49-entry-1" {
			s.actor(c.ActorID).Temporary = TemporaryReduceAttack
		}
		return err
	case "job-52-entry-3":
		child := ControlClimaxContext{DamageContext: c.DamageContext}
		err := s.ApplyControlClimax(&child, "job-58-entry-4", rng)
		c.DamageContext = child.DamageContext
		return err
	case "job-52-entry-4":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 200, CategoryPhysical, true, rng)
		if err == nil {
			s.actor(c.ActorID).Temporary = TemporaryDouble
		}
		return err
	}
	return nil
}
