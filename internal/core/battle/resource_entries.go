package battle

import "github.com/witchcraze/party2re/internal/core/random"

type ResourceEntry struct {
	CallbackEntry
	SourceID, CMPCost int
	TargetScope       string
}

// ResourceEntries contains intrinsic bodies; full job tables and late custom
// charging remain the invoking consumer's responsibility.
func ResourceEntries() []ResourceEntry {
	return []ResourceEntry{
		{CallbackEntry: CallbackEntry{ID: "gem_mp_heal_sky_1", Name: "ally-mp-50"}, SourceID: 24, CMPCost: 24, TargetScope: TargetScopeSingleAlly},
		{CallbackEntry: CallbackEntry{ID: "gem_mp_heal_sky_2", Name: "self-mp-100"}, SourceID: 25, CMPCost: 37, TargetScope: TargetScopeSelf},
		{CallbackEntry: CallbackEntry{ID: "gem_drain_mp_sky", Name: "mp-drain"}, SourceID: 27, CMPCost: 42, TargetScope: TargetScopeSingleEnemy},
		{CallbackEntry: CallbackEntry{ID: "gem_debuff_mp", Name: "area-mp-down"}, SourceID: 97, CMPCost: 17, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_mp_burst", Name: "mp-burst"}, SourceID: 98, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_cmp_burst", Name: "cmp-burst"}, SourceID: 99, CMPCost: 1, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_status_cmp_3", Name: "status-cmp-3"}, SourceID: 103, CMPCost: -3, TargetScope: TargetScopeSelf},
		{CallbackEntry: CallbackEntry{ID: "gem_status_cmp_10", Name: "status-cmp-10"}, SourceID: 104, CMPCost: -10, TargetScope: TargetScopeSelf},
		{CallbackEntry: CallbackEntry{ID: "job-50-entry-5", Name: "mp-restoration-50", RequiredSP: 110, MPCost: 50}, TargetScope: TargetScopeSingleAlly},
	}
}

func resourceEntry(id string) (ResourceEntry, bool) {
	for _, e := range ResourceEntries() {
		if e.ID == id {
			return e, true
		}
	}
	return ResourceEntry{}, false
}

// ValidateResource shares the execution gate without draws, hooks or writes.
func (s *LifetimeState) ValidateResource(c *DamageContext, id, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	entry, ok := resourceEntry(id)
	if !ok {
		return ErrUnsupportedEffect
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	e := controlEffect(targetID, 50, CategoryMagic)
	switch entry.SourceID {
	case 0, 24, 25:
		if entry.SourceID == 25 {
			e.TargetID = c.ActorID
			e.Payload.Number = 100
		}
		return s.validatePartyTarget(c.TargetContext, e, rng)
	case 27, 97:
		e.Payload.Number = .2
		_, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
		if err != nil {
			return err
		}
		if entry.SourceID == 27 {
			p := s.callbackCopy()
			if p.actor(c.ActorID).categorySealed(CategoryMagic) {
				p.actor(c.ActorID).Status = ""
			}
			return p.validatePartyTarget(c.TargetContext, controlEffect(c.ActorID, 1, CategoryMagic), rng)
		}
	case 98, 99:
		e.Payload.Number = float64(s.actor(c.ActorID).MP) * 1.5
		if entry.SourceID == 99 {
			e.Payload.Number = float64(s.actor(c.ActorID).CMP) * 3.5
		}
		e.Arguments = []string{"1"}
		return s.validateAreaDamage(c, e, rng)
	}
	return nil
}

// ProjectResource applies only deterministic103/104 bodies on private state.
// They inspect the current status; they never assign poison or consume a target.
func (s *LifetimeState) ProjectResource(c *DamageContext, id string) bool {
	entry, ok := resourceEntry(id)
	if !ok || (entry.SourceID != 103 && entry.SourceID != 104) {
		return false
	}
	a := s.actor(c.ActorID)
	if a.Status != "" {
		a.CMP = max(0, a.CMP+entry.CMPCost)
	}
	return true
}

func (s *LifetimeState) ApplyResource(c *DamageContext, id, targetID string, rng random.Generator) ([]EffectApplication, error) {
	if err := s.ValidateResource(c, id, targetID, rng); err != nil {
		return nil, err
	}
	if s.ProjectResource(c, id) {
		return nil, nil
	}
	entry, _ := resourceEntry(id)
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	e := controlEffect(targetID, 50, CategoryMagic)
	switch entry.SourceID {
	case 0, 24, 25:
		if entry.SourceID == 25 {
			e.TargetID = c.ActorID
			e.Payload.Number = 100
		}
		r, err := s.ApplyMPHealing(&c.EffectContext, e, rng)
		return []EffectApplication{r}, err
	case 27:
		e.Payload.Number = .2
		r, err := s.ApplyStatDown(&c.EffectContext, e, StatMP, rng)
		results := []EffectApplication{r}
		if err != nil || !r.Returned {
			return results, err
		}
		tail, err := s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
		return append(results, tail), err
	case 97:
		return s.ApplyPartyStat(&c.EffectContext, .2, CategoryMagic, StatMP, true, rng)
	case 98, 99:
		a := s.actor(c.ActorID)
		power := float64(a.MP) * 1.5
		if entry.SourceID == 99 {
			power = float64(a.CMP) * 3.5
		}
		r, err := s.ApplyAreaDamage(c, power, CategoryMagic, true, rng)
		if err != nil {
			return r, err
		}
		a = s.actor(c.ActorID)
		if entry.SourceID == 98 {
			a.MP = 0
		} else {
			a.CMP = 1
		}
		return r, nil
	}
	return nil, ErrUnsupportedEffect
}
