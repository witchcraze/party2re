package battle

import "github.com/witchcraze/party2re/internal/core/random"

// SongDanceDrainContext shares operation counters and effect ports. Rebuild
// runtime hooks from saved facts; Arguments belong to the invocation frame.
type SongDanceDrainContext struct {
	DamageContext
	Arguments []string
	Status    *StatusContext
}

// SongDanceDrainEntries preserves complete source order, including the
// separately registered sacrifice14.4. Registration never narrows a random pool.
func SongDanceDrainEntries(job int) []CallbackEntry {
	switch job {
	case 13:
		return []CallbackEntry{
			{ID: "job-13-entry-0", Name: "mana-song", RequiredSP: 5, MPCost: 3},
			{ID: "job-13-entry-1", Name: "healing-song", RequiredSP: 15, MPCost: 7},
			{ID: "job-13-entry-2", Name: "waking-song", RequiredSP: 30, MPCost: 5},
			{ID: "job-13-entry-3", Name: "defense-song", RequiredSP: 40, MPCost: 6},
			{ID: "job-13-entry-4", Name: "sleep-song", RequiredSP: 60, MPCost: 9},
			{ID: "job-13-entry-5", Name: "attack-song", RequiredSP: 90, MPCost: 10},
		}
	case 14:
		return []CallbackEntry{
			{ID: "job-14-entry-0", Name: "evasion-dance", RequiredSP: 4, MPCost: 3},
			{ID: "job-14-entry-1", Name: "mana-dance", RequiredSP: 9, MPCost: 3},
			{ID: "job-14-entry-2", Name: "deflect-stance", RequiredSP: 16, MPCost: 5},
			{ID: "job-14-entry-3", Name: "physical-sweep", RequiredSP: 30, MPCost: 9},
			{ID: "job-14-entry-4", Name: "献命の舞", RequiredSP: 45, MPCost: 1},
			{ID: "job-14-entry-5", Name: "blade-dance", RequiredSP: 70, MPCost: 16},
			{ID: "job-14-entry-6", Name: "healing-dance", RequiredSP: 100, MPCost: 12},
		}
	case 20:
		return []CallbackEntry{
			{ID: "job-20-entry-0", Name: "binding-dance", RequiredSP: 4, MPCost: 4},
			{ID: "job-20-entry-1", Name: "mana-whip", RequiredSP: 9, MPCost: 6},
			{ID: "job-20-entry-2", Name: "party-magic-mitigation", RequiredSP: 16, MPCost: 6},
			{ID: "job-20-entry-3", Name: "sleep-breath", RequiredSP: 20, MPCost: 9},
			{ID: "job-20-entry-4", Name: "confusion-dance", RequiredSP: 36, MPCost: 7},
			{ID: "job-20-entry-5", Name: "ending-dance", RequiredSP: 60, MPCost: 24},
			{ID: "job-20-entry-6", Name: "healing-whip", RequiredSP: 100, MPCost: 18},
		}
	}
	return nil
}

func (c *SongDanceDrainContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ProjectSongDanceDrain carries only the complete deterministic inline writes
// into later admission. Neither body has a category/living caster guard.
func (s *LifetimeState) ProjectSongDanceDrain(c *SongDanceDrainContext, id string) bool {
	switch id {
	case "job-14-entry-2":
		s.actor(c.ActorID).Temporary = TemporaryDeflect
	case "job-20-entry-2":
		// Source20.2 omits the magic-seal check present in guardian17.2.
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReduceMagic
			}
		}
	default:
		return false
	}
	return true
}

// ApplySongDanceDrain executes intrinsic bodies only. Ordinary listed charges
// and random/delegated cost handling remain with the invocation/consumer owner.
func (s *LifetimeState) ApplySongDanceDrain(c *SongDanceDrainContext, id string, rng random.Generator) error {
	if err := s.ValidateSongDanceDrain(c, id, rng); err != nil {
		return err
	}
	if s.ProjectSongDanceDrain(c, id) {
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
	switch id {
	case "job-13-entry-0":
		_, err := s.ApplyPartyStat(&c.EffectContext, .25, CategorySong, StatMP, true, rng)
		return err
	case "job-13-entry-1", "job-14-entry-6":
		power, category := 30.0, CategorySong
		if id == "job-14-entry-6" {
			power, category = 140, CategoryDance
		}
		_, err := s.ApplyPartyHealing(&c.EffectContext, power, category, rng)
		return err
	case "job-13-entry-3", "job-13-entry-5":
		coefficient, stat := .4, StatDefense
		if id == "job-13-entry-5" {
			coefficient, stat = .6, StatAttack
		}
		_, err := s.ApplyPartyStat(&c.EffectContext, coefficient, CategorySong, stat, false, rng)
		return err
	case "job-14-entry-0":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .4, CategoryDance), StatAgility, rng)
		return err
	case "job-14-entry-1":
		_, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .4, CategoryDance), StatMP, rng)
		return err
	case "job-14-entry-3":
		_, err := s.ApplyAreaDamage(&c.DamageContext, float64(s.actor(c.ActorID).Attack)*.8, CategoryPhysical, false, rng)
		return err
	case "job-14-entry-5":
		count := 2 + int(rng.Float64()*3)
		for range count {
			a := s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			if _, err := s.ApplyDamage(&c.DamageContext, controlEffect("", float64(a.Attack)*.8, CategoryDance), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-20-entry-1", "job-20-entry-6":
		coefficient, recovery := .9, .1
		if id == "job-20-entry-6" {
			coefficient, recovery = 1.7, .5
		}
		hit, err := s.ApplyDamage(&c.DamageContext, controlEffect(c.shift(), float64(s.actor(c.ActorID).Attack)*coefficient, CategoryPhysical), rng)
		if err != nil || !hit.Returned || hit.TargetID == "" {
			return err
		}
		e := controlEffect(c.ActorID, float64(int(float64(hit.Amount)*recovery)), CategoryPhysical)
		if id == "job-20-entry-1" {
			_, err = s.ApplyMPHealing(&c.EffectContext, e, rng)
		} else {
			_, err = s.ApplyHealing(&c.EffectContext, e, rng)
		}
		return err
	default:
		child := *c.Status
		child.EffectContext = c.EffectContext
		var err error
		switch id {
		case "job-13-entry-2":
			_, err = s.ApplyPartyCure(&child, controlSymbol("", StatusSleep, CategorySong), rng)
		case "job-13-entry-4":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusSleep, CategorySong), 50, false, false, rng)
		case "job-20-entry-0":
			_, err = s.ApplyStatus(&child, controlSymbol(c.shift(), StatusDofuu, CategoryDance), 85, false, false, rng)
		case "job-20-entry-3":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusSleep, CategoryBreath), 35, false, false, rng)
		case "job-20-entry-4":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusConfusion, CategoryDance), 60, false, false, rng)
		case "job-20-entry-5":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", "instant_death", CategoryDance), 17, false, true, rng)
		}
		child.EffectContext.targetResources = c.Status.EffectContext.targetResources
		*c.Status = child
		c.EffectContext = child.EffectContext
		return err
	}
}
