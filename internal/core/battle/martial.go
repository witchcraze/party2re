package battle

import "github.com/witchcraze/party2re/internal/core/random"

// MartialContext holds operation facts; consumers rebuild effect ports on load.
type MartialContext struct {
	DamageContext
	Status    *StatusContext
	Arguments []string
}

// MartialEntries preserves source positions, including both job4 extensions.
func MartialEntries(job int) []CallbackEntry {
	switch job {
	case 4:
		return []CallbackEntry{
			{ID: "job-4-entry-0", Name: "sweeping-strike", Extended: true},
			{ID: "job-4-entry-1", Name: "restore-accuracy", RequiredSP: 1},
			{ID: "job-4-entry-2", Name: "evasive-footwork", RequiredSP: 5, MPCost: 3},
			{ID: "job-4-entry-3", Name: "knee-strike", RequiredSP: 14, MPCost: 4},
			{ID: "job-4-entry-4", Name: "leg-sweep", RequiredSP: 25, MPCost: 3},
			{ID: "job-4-entry-5", Name: "vital-strike", RequiredSP: 45, MPCost: 11},
			{ID: "job-4-entry-6", Name: "focused-punch", RequiredSP: 70, MPCost: 15},
			{ID: "job-4-entry-7", Name: "flurry", RequiredSP: 100, MPCost: 20},
			{ID: "job-4-entry-8", Name: "flame-stance", RequiredSP: 100, MPCost: 10, Extended: true},
		}
	case 23:
		return []CallbackEntry{
			{ID: "job-23-entry-0", Name: "leaping-strike", RequiredSP: 10, MPCost: 5},
			{ID: "job-23-entry-1", Name: "martial-power", RequiredSP: 30, MPCost: 25},
			{ID: "job-23-entry-2", Name: "draining-strike", RequiredSP: 50, MPCost: 18},
			{ID: "job-23-entry-3", Name: "twin-leaps", RequiredSP: 70, MPCost: 14},
			{ID: "job-23-entry-4", Name: "piercing-spear", RequiredSP: 130, MPCost: 35},
		}
	case 25:
		return []CallbackEntry{
			{ID: "job-25-entry-0", Name: "roundhouse", RequiredSP: 5, MPCost: 8},
			{ID: "job-25-entry-1", Name: "centering", RequiredSP: 15, MPCost: 3},
			{ID: "job-25-entry-2", Name: "reckless-strike", RequiredSP: 30, MPCost: 15},
			{ID: "job-25-entry-3", Name: "physical-counter", RequiredSP: 50, MPCost: 5},
			{ID: "job-25-entry-4", Name: "major-defense", RequiredSP: 70, MPCost: 3},
			{ID: "job-25-entry-5", Name: "pressure-wave", RequiredSP: 90, MPCost: 12},
			{ID: "job-25-entry-6", Name: "party-cover", RequiredSP: 110},
			{ID: "job-25-entry-7", Name: "revival-stance", RequiredSP: 130, MPCost: 7},
		}
	}
	return nil
}

func (c *MartialContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func applyMartialInline(s *LifetimeState, c *MartialContext, id string) bool {
	a := s.actor(c.ActorID)
	switch id {
	case "job-4-entry-1":
		a.Hit = 95
	case "job-25-entry-3":
		a.Temporary = TemporaryReflectAttack
	case "job-25-entry-4":
		a.Temporary = TemporaryStrongDefense
	case "job-25-entry-6":
		a.Temporary = TemporaryCovering
		for _, member := range c.PartyIDs {
			if member != a.ID {
				s.actor(member).Temporary = TemporaryCover
			}
		}
	case "job-25-entry-7":
		a.Temporary = TemporaryRevival
	default:
		return false
	}
	return true
}

// ApplyMartial executes intrinsic bodies; the enclosing invocation owns costs.
func (s *LifetimeState) ApplyMartial(c *MartialContext, id string, rng random.Generator) error {
	if err := s.ValidateMartial(c, id, rng); err != nil {
		return err
	}
	if applyMartialInline(s, c, id) {
		return nil
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
	case "job-4-entry-0", "job-25-entry-0", "job-25-entry-5":
		power, direct := float64(a.Attack)*.75, false
		if id == "job-25-entry-0" {
			power = float64(a.Attack) * .8
		} else if id == "job-25-entry-5" {
			power, direct = 120, true
		}
		_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryPhysical, direct, rng)
		return err
	case "job-4-entry-2", "job-4-entry-8", "job-23-entry-1":
		stats := []BattleStat{StatAttack, StatDefense}
		if id == "job-4-entry-2" {
			stats = []BattleStat{StatAgility}
		}
		for _, stat := range stats {
			if _, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .4, CategoryPhysical), stat, rng); err != nil {
				return err
			}
		}
		if id == "job-4-entry-8" {
			a = s.actor(c.ActorID)
			a.Extended, a.ExtendedTurns = ExtendedFlame, 6
		}
		return nil
	case "job-4-entry-4", "job-4-entry-5":
		child := *c.Status
		child.EffectContext = c.EffectContext
		status, chance, death := StatusDofuu, 75.0, false
		if id == "job-4-entry-5" {
			status, chance, death = "death", 19, true
		}
		_, err := s.ApplyStatus(&child, controlSymbol(c.shift(), status, CategoryPhysical), chance, false, death, rng)
		child.targetResources = c.Status.targetResources
		*c.Status = child
		c.EffectContext = child.EffectContext
		c.EffectContext.targetResources = resources
		return err
	case "job-25-entry-1":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, 100, CategoryPhysical), rng)
		if err == nil {
			s.actor(c.ActorID).Hit = 95
		}
		return err
	}
	count := 1
	if id == "job-4-entry-7" {
		count = 3 + int(rng.Float64()*3)
	} else if id == "job-23-entry-3" {
		count = 2
	}
	for range count {
		a = s.actor(c.ActorID)
		if (id == "job-4-entry-7" || id == "job-23-entry-3") && a.HP <= 0 {
			break
		}
		e := martialDamage(c, a, id)
		r, err := s.ApplyDamage(&c.DamageContext, e, rng)
		if err != nil {
			return err
		}
		if id == "job-23-entry-2" && r.Returned && r.TargetID != "" {
			_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryPhysical), rng)
		}
		if err != nil {
			return err
		}
		if id == "job-25-entry-2" {
			s.actor(c.ActorID).Temporary = TemporaryDouble
		}
	}
	return nil
}

func martialDamage(c *MartialContext, a *LifetimeActor, id string) TargetEffect {
	e := dualAgilityEffect("", float64(a.Attack)*1.2, CategoryPhysical, false)
	if id == "job-4-entry-7" {
		e.Payload.Number = float64(a.Attack) * .8
		return e
	}
	e.TargetID = c.shift()
	switch id {
	case "job-4-entry-6":
		e.Payload.Number = float64(a.Attack) * 1.5
	case "job-23-entry-0":
		e.Payload.Number = float64(a.Agility) * 1.8
	case "job-23-entry-2":
		e.Payload.Number = float64(a.Attack) * .9
	case "job-23-entry-3":
		e.Payload.Number = float64(a.Agility) * 2.2
	case "job-23-entry-4":
		e.Payload.Number, e.Arguments = float64(a.Attack)*1.4, []string{"1"}
	case "job-25-entry-2":
		e.Payload.Number = float64(a.Attack) * 2
	}
	return e
}
