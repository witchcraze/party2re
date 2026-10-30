package battle

import "github.com/witchcraze/party2re/internal/core/random"

// RecoilDarkStateContext holds operation facts. RawDefeat supplies the native
// defeat/reset closure for the raw strike, which bypasses all damage helpers.
type RecoilDarkStateContext struct {
	DamageContext
	RawDefeat                    *DefeatContext
	Arguments                    []string
	StatusCalls, AreaStatusCalls int
}

// RecoilDarkStateEntries preserves source order, including the separately
// owned darkness field at22.5. Its presence never certifies its callback.
func RecoilDarkStateEntries(job int) []CallbackEntry {
	switch job {
	case 21:
		return []CallbackEntry{
			{ID: "job-21-entry-0", Name: "defense-recoil-strike", RequiredSP: 10, MPCost: 4},
			{ID: "job-21-entry-1", Name: "deflection", RequiredSP: 20, MPCost: 5},
			{ID: "job-21-entry-2", Name: "immobilizing-roar", RequiredSP: 40, MPCost: 12},
			{ID: "job-21-entry-3", Name: "reckless-strike", RequiredSP: 60, MPCost: 5},
			{ID: "job-21-entry-4", Name: "double-edge-strike", RequiredSP: 80, MPCost: 6},
			{ID: "job-21-entry-5", Name: "indiscriminate-strike", RequiredSP: 110, MPCost: 9},
			{ID: "job-21-entry-6", Name: "frenzy", RequiredSP: 110, MPCost: 20, Extended: true},
		}
	case 22:
		return []CallbackEntry{
			{ID: "job-22-entry-0", Name: "dark-strike", RequiredSP: 10, MPCost: 5},
			{ID: "job-22-entry-1", Name: "defense-pact", RequiredSP: 20, MPCost: 9},
			{ID: "job-22-entry-2", Name: "confusing-wave", RequiredSP: 40, MPCost: 16},
			{ID: "job-22-entry-3", Name: "blinding-strike", RequiredSP: 70, MPCost: 10},
			{ID: "job-22-entry-4", Name: "dark-heavy-strike", RequiredSP: 140, MPCost: 40},
			{ID: "job-22-entry-5", Name: "darkness-field", RequiredSP: 140, MPCost: 7, Extended: true},
		}
	case 38:
		return []CallbackEntry{
			{ID: "job-38-entry-0", Name: "wound-drain", RequiredSP: 10, MPCost: 12},
			{ID: "job-38-entry-1", Name: "mp-drain", RequiredSP: 20, MPCost: 3},
			{ID: "job-38-entry-2", Name: "magic-immunity", RequiredSP: 30, MPCost: 6},
			{ID: "job-38-entry-3", Name: "defense-pact", RequiredSP: 60, MPCost: 9},
			{ID: "job-38-entry-4", Name: "sleeping-breath", RequiredSP: 90, MPCost: 9},
			{ID: "job-38-entry-5", Name: "capped-life-drain", RequiredSP: 130, MPCost: 37},
		}
	}
	return nil
}

func (c *RecoilDarkStateContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func recoilDarkStrike(a *LifetimeActor, id, target string) TargetEffect {
	power := float64(a.Attack) * 1.6
	switch id {
	case "job-21-entry-0":
		power = float64(a.Defense) * 1.6
	case "job-21-entry-3":
		power = float64(a.Attack) * 2
	case "job-22-entry-0":
		power = float64(a.Attack) * 1.5
	case "job-22-entry-3":
		power = float64(a.Attack) * 1.2
	case "job-22-entry-4":
		power = float64(a.Attack) * 2.5
	case "job-38-entry-0":
		return dualAgilityEffect(target, float64(a.MaxHP-a.HP)*.5+5, CategoryMagic, true)
	}
	return dualAgilityEffect(target, power, CategoryPhysical, false)
}

func recoilDarkStatus(id string) (string, CallbackCategory, float64) {
	switch id {
	case "job-21-entry-2":
		return StatusDofuu, CategoryPhysical, 60
	case "job-22-entry-2":
		return StatusConfusion, CategoryMagic, 50
	default:
		return StatusSleep, CategoryBreath, 35
	}
}

// ApplyRecoilDarkState runs intrinsic bodies; the invocation/command owner
// retains listed costs and random/delegated MP restoration.
func (s *LifetimeState) ApplyRecoilDarkState(c *RecoilDarkStateContext, id string, rng random.Generator) error {
	if err := s.ValidateRecoilDarkState(c, id, rng); err != nil {
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
	case "job-21-entry-1":
		a.Temporary = TemporaryDeflect
	case "job-21-entry-2", "job-22-entry-2", "job-38-entry-4":
		symbol, category, chance := recoilDarkStatus(id)
		child := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls}
		_, err := s.ApplyAreaStatus(&child, controlSymbol("", symbol, category), chance, false, false, rng)
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls
		return err
	case "job-21-entry-5":
		return s.applyRawRecoilStrike(c, rng)
	case "job-21-entry-6":
		for range 2 {
			if err := s.statTension(&c.EffectContext, s.actor(c.ActorID), false, rng); err != nil {
				return err
			}
		}
		a = s.actor(c.ActorID)
		a.Status, a.Extended, a.ExtendedTurns = StatusConfusion, ExtendedFrenzy, 5
	case "job-22-entry-1", "job-38-entry-3":
		a.Defense -= int(float64(a.Defense) * .5)
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAttack, rng)
		return err
	case "job-38-entry-1":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .2, CategoryMagic), StatMP, rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
		return err
	case "job-38-entry-2":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err == nil && allowed {
			a.Temporary = TemporaryNullMagic
		}
		return err
	case "job-38-entry-5":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "drain", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		power := float64(min(300, s.actor(r.TargetID).HP/2))
		damage, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(r.TargetID, power, CategoryMagic, true), rng)
		if err != nil || !damage.Returned || damage.TargetID == "" {
			return err
		}
		_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, power, CategoryMagic), rng)
		return err
	default:
		if id == "job-22-entry-3" {
			c.AdditionalEffect = true
		}
		r, err := s.ApplyDamage(&c.DamageContext, recoilDarkStrike(a, id, c.shift()), rng)
		if err != nil {
			return err
		}
		a = s.actor(c.ActorID)
		switch id {
		case "job-21-entry-3":
			a.Temporary = TemporaryDouble // Even a miss or a lethal reflection reaches this write.
		case "job-22-entry-3":
			if r.Returned && r.TargetID != "" && s.actor(r.TargetID).HP > 0 {
				_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .1, CategoryPhysical), StatHit, rng)
			}
		case "job-38-entry-0":
			if r.Returned && r.TargetID != "" {
				_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
			}
		default:
			if r.Returned && r.Amount > 0 {
				power := float64(r.Amount) * .05
				if id == "job-21-entry-0" {
					power = float64(r.Amount) * .07
				}
				if id == "job-22-entry-0" || id == "job-22-entry-4" {
					base := r.Amount
					if a.HP <= 999 {
						base = a.HP
					}
					power = float64(base) * .1
					if id == "job-22-entry-4" {
						power *= 2
					}
				}
				_, err = s.ApplyRecoil(&c.EffectContext, &c.End, power, rng)
			}
		}
		return err
	}
	return nil
}

func (s *LifetimeState) applyRawRecoilStrike(c *RecoilDarkStateContext, rng random.Generator) error {
	ids := c.EnemyIDs
	if rng.Float64()*4 < 1 {
		ids = c.PartyIDs
	}
	i := int(rng.Float64() * float64(max(1, len(ids))))
	t := s.actor(c.ActorID)
	if len(ids) > 0 && s.actor(ids[i]).HP > 0 {
		t = s.actor(ids[i])
	}
	a := s.actor(c.ActorID)
	power := int((float64(a.Attack)*3 - float64(t.Defense)*.4) * ((.9 + .3*rng.Float64()) * a.Tension))
	if power < 1 {
		power = int(rng.Float64()*2) + 1
	}
	a.Tension = 1
	t.HP = max(0, t.HP-power)
	if t.HP > 0 {
		return nil
	}
	child := *c.RawDefeat
	child.TargetContext = c.TargetContext
	_, err := s.ApplyDefeat(&child, t.ID, rng)
	c.Cursor = child.Cursor
	*c.RawDefeat = child
	return err
}
