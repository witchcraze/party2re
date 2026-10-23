package battle

import "github.com/witchcraze/party2re/internal/core/random"

// DualAgilityContext shares the existing damage/status owners and ordered
// operation facts. Arguments belong to this callback's invocation frame.
type DualAgilityContext struct {
	DamageContext
	Status    *StatusContext
	Arguments []string
}

// DualAgilityEntries is the full source order at _skill.cgi:5929–5983,
// 6620–6667. The final job77 identity freezes its table-build-time variant.
func DualAgilityEntries(job int, temporary bool) []CallbackEntry {
	if job == 77 {
		entries := []CallbackEntry{
			{ID: "job-77-entry-0", Name: "dual-strike", RequiredSP: 10, MPCost: 4},
			{ID: "job-77-entry-1", Name: "tension-strike", RequiredSP: 40, MPCost: 8},
			{ID: "job-77-entry-2", Name: "agility-sweep", RequiredSP: 70, MPCost: 42},
			{ID: "job-77-entry-3", Name: "neutral-pair", RequiredSP: 90, MPCost: 22},
			{ID: "job-77-entry-4", Name: "triple-strike", RequiredSP: 130, MPCost: 37},
		}
		if temporary {
			entries[4].ID, entries[4].Name = "job-77-entry-4-temporary", "release-six-strikes"
		}
		return entries
	}
	if job == 83 {
		return []CallbackEntry{
			{ID: "job-83-entry-0", Name: "agile-strike", RequiredSP: 5, MPCost: 3},
			{ID: "job-83-entry-1", Name: "party-accuracy", RequiredSP: 15, MPCost: 9},
			{ID: "job-83-entry-2", Name: "vital-strike", RequiredSP: 35, MPCost: 11},
			{ID: "job-83-entry-3", Name: "three-thrusts", RequiredSP: 60, MPCost: 24},
			{ID: "job-83-entry-4", Name: "rage-stance", RequiredSP: 85, MPCost: 8},
			{ID: "job-83-entry-5", Name: "strongest-thrust", RequiredSP: 100, MPCost: 21},
		}
	}
	return nil
}

func (c *DualAgilityContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	target := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return target
}

func dualAgilityEffect(target string, power float64, category CallbackCategory, direct bool) TargetEffect {
	e := TargetEffect{TargetID: target, Payload: CallbackPayload{Number: power}, Category: category}
	if direct {
		e.Arguments = []string{"1"}
	}
	return e
}

func applyDualAgilityInline(s *LifetimeState, c *DualAgilityContext, id string) bool {
	switch id {
	case "job-83-entry-1":
		for _, member := range c.PartyIDs {
			a := s.actor(member)
			a.Hit = min(95, a.Hit+20)
		}
	case "job-83-entry-4":
		s.actor(c.ActorID).Temporary = TemporaryRage
	default:
		return false
	}
	return true
}

// ProjectDualAgility carries exact inline state writes into later admission.
// Random damage/death transitions remain with the existing effect owners.
func (s *LifetimeState) ProjectDualAgility(c *DualAgilityContext, id string) (bool, error) {
	if err := s.ValidateDualAgility(c, id, random.Default()); err != nil {
		return false, err
	}
	return applyDualAgilityInline(s, c, id), nil
}

// ApplyDualAgility executes intrinsic callbacks only. Ordinary listed costs and
// random/delegated MP restoration remain with the enclosing invocation owner.
func (s *LifetimeState) ApplyDualAgility(c *DualAgilityContext, id string, rng random.Generator) error {
	if err := s.ValidateDualAgility(c, id, rng); err != nil {
		return err
	}
	if applyDualAgilityInline(s, c, id) {
		return nil
	}
	previous := c.targetResources
	c.targetResources = func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	defer func() { c.targetResources = previous }()
	a := s.actor(c.ActorID)
	switch id {
	case "job-77-entry-0", "job-83-entry-0":
		target := c.shift()
		attack, agility, hit := a.Attack, a.Agility, a.Hit
		repeats := 1
		if id == "job-77-entry-0" {
			a.Attack = int(float64(a.Attack) * .75)
			c.ToolCategory = ToolCategoryContext{Source: "job77", SavedAttack: float64(attack)}
			repeats = 2
		} else {
			a.Agility *= 5
			a.Hit += 5
		}
		defer func() {
			a := s.actor(c.ActorID)
			if id == "job-77-entry-0" {
				a.Attack = attack
			} else {
				a.Agility, a.Hit = agility, hit
			}
		}()
		for range repeats { // No living guard between the two ordinary helpers.
			if _, err := s.applyNormalStrike(&c.DamageContext, target, rng); err != nil {
				return err
			}
		}
		return nil
	case "job-77-entry-1":
		target, err := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: c.shift(), Category: CategoryUntyped, Payload: CallbackPayload{Symbol: "tension"}}, rng)
		if err != nil {
			return err
		}
		c.targetResources(target)
		tension := 0.0
		if t := s.actor(target.TargetID); target.Resolved && t != nil {
			tension = t.Tension
		}
		id := ""
		if target.Resolved {
			id = target.TargetID
		}
		_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(id, float64(a.Attack)*(tension+1)/2, CategoryPhysical, false), rng)
		return err
	case "job-83-entry-2":
		child := *c.Status
		child.EffectContext = c.EffectContext
		child.targetResources = c.targetResources
		_, err := s.ApplyStatus(&child, TargetEffect{TargetID: c.shift(), Category: CategoryPhysical, Payload: CallbackPayload{Symbol: "death"}}, 19, false, true, rng)
		child.targetResources = c.Status.targetResources
		*c.Status = child
		c.EffectContext = child.EffectContext
		return err
	}
	count := 1
	captured := ""
	switch id {
	case "job-77-entry-2":
		count = 2 + int(rng.Float64()*2)
	case "job-77-entry-3":
		count, captured = 2, c.shift()
	case "job-77-entry-4", "job-83-entry-3":
		count = 3
	case "job-77-entry-4-temporary":
		a.Temporary, count = "", 6
	}
	for range count {
		a = s.actor(c.ActorID)
		if id != "job-83-entry-5" && a.HP <= 0 {
			break
		}
		if id == "job-77-entry-2" {
			if _, err := s.ApplyAreaDamage(&c.DamageContext, float64(a.Agility)*1.5, CategoryPhysical, false, rng); err != nil {
				return err
			}
			continue
		}
		e := dualAgilityEffect("", float64(a.Attack), CategoryPhysical, false)
		switch id {
		case "job-77-entry-3":
			e.TargetID, e.Category = captured, CategoryUntyped
		case "job-77-entry-4-temporary":
			e.Payload.Number *= .55
			e.Arguments = []string{"1"}
		case "job-83-entry-3":
			e.TargetID, e.Payload.Number = c.shift(), e.Payload.Number*.85
		case "job-83-entry-5":
			e.TargetID = c.shift()
			e.Payload.Number = max(float64(a.Attack)*.85, float64(a.Agility)*1.5)
			e.Arguments = []string{"1"}
		}
		if _, err := s.ApplyDamage(&c.DamageContext, e, rng); err != nil {
			return err
		}
	}
	return nil
}
