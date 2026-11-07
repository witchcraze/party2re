package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// EquippedExtensionContext retains operation facts; rebuild ports after reload.
// Death owns the reset/defeat descendants of damage; nil keeps them whole gates.
type EquippedExtensionContext struct {
	DamageContext
	Arguments []string
	Death     *DefeatContext
}

func (c *EquippedExtensionContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	target := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return target
}

type EquippedExtensionEntry struct {
	CallbackEntry
	DefinitionID string
}

// EquippedExtensionEntries is the certified subset of the source table at
// ff97fc5:_skill.cgi:321–477. The roster and revival positions keep their owners.
func EquippedExtensionEntries() []EquippedExtensionEntry {
	entry := func(number string, mp int) EquippedExtensionEntry {
		id := "extension-" + number
		return EquippedExtensionEntry{CallbackEntry{ID: id, Name: id, MPCost: mp}, "item-" + number}
	}
	return []EquippedExtensionEntry{
		entry("165", 30), entry("201", 30), entry("163", 12), entry("212", 5),
		entry("247", 5), entry("248", 2), entry("146", 37), entry("200", 10),
	}
}

// projectileVariant separates the equipped and ordinary tool bodies of one source
// ID (_skill.cgi:413–434; _data.cgi:2013). Only job26 adds agility and the tail.
type projectileVariant struct{ base, spread, agility, chance float64 }

var (
	extensionProjectile = projectileVariant{2, .5, .75, 50}
	toolProjectile      = projectileVariant{4, 2, 1.5, 70}
)

// The first enemy check only binds the later helpers' target and carries no power.
const projectileSymbol = "projectile"

// ValidateEquippedExtension is the shared discovery and submission gate. Listing
// needs enemies, the equipped item and adjusted MP, but neither SP nor an uncursed
// actor (_skill.cgi:34,124–134,260–284). The MP cost is the consumer's outer debit.
func (s *LifetimeState) ValidateEquippedExtension(c *EquippedExtensionContext, id string, rng random.Generator) (CallbackEntry, error) {
	if s == nil || c == nil {
		return CallbackEntry{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return CallbackEntry{}, err
	}
	a := s.actor(c.ActorID)
	for _, entry := range EquippedExtensionEntries() {
		if entry.ID != id {
			continue
		}
		if a.NPC || c.ActorNPC || !hasItem(a.ItemDefinitionIDs, entry.DefinitionID) || len(c.EnemyIDs) == 0 {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		// None of these source names is in _is_magician_skill (_battle.cgi:1804–1825).
		if hasItem(a.ItemDefinitionIDs, "item-249") {
			entry.MPCost *= 2
		}
		if a.MP < entry.MPCost {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		return entry.CallbackEntry, s.validateExtensionBody(c, id, rng)
	}
	return CallbackEntry{}, ErrUnsupportedEffect
}

func (s *LifetimeState) validateExtensionBody(c *EquippedExtensionContext, id string, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	a := p.actor(c.ActorID)
	switch id {
	case "extension-165":
		h := p.extensionPartner(a)
		if h == nil {
			return nil
		}
		target := child.shift()
		for _, stat := range []int{h.Attack, h.Defense, h.Agility} {
			power := float64(a.Attack+stat) * .85
			e := dualAgilityEffect(target, power, CategoryPhysical, false)
			if _, err := p.projectCallbackDamage(&child.DamageContext, c.Death, e, power, false, rng); err != nil {
				return err
			}
		}
		return nil
	case "extension-201":
		target := child.shift()
		damage := p.nativeDamageContext(child.DamageContext, c.Death, rng)
		if err := p.ValidateExtension201(&damage, target, rng); err != nil || a.Agility <= a.BaseStats.Agility {
			return err
		}
		power := float64(a.Agility) * 1.6
		_, err := p.projectCallbackDamage(&child.DamageContext, c.Death, dualAgilityEffect(target, power, CategoryUntyped, true), power, false, rng)
		return err
	case "extension-163":
		if !a.Temporary.reflection() {
			return nil
		}
		// The copy can change a later healing target, so admit it on the result.
		p.actor(p.mirrorRecipient(a, child.shift())).Temporary = a.Temporary
		for _, member := range c.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(member, 50, CategoryUntyped), rng); err != nil {
				return err
			}
		}
		return nil
	case "extension-248":
		return p.validateProjectile(&child.DamageContext, c.Death, child.shift(), extensionProjectile, rng)
	case "extension-146":
		if a.categorySealed(CategoryMagic) {
			a.Status = "" // The sealed outcome has no descendants; admit the cured path.
		}
		e := controlSymbol(child.shift(), string(TemporaryRegeneration), CategoryUntyped)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(candidate.TargetID, 50, CategoryMagic), rng); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyEquippedExtension executes the intrinsic body; the consumer debits its MP.
func (s *LifetimeState) ApplyEquippedExtension(c *EquippedExtensionContext, id string, rng random.Generator) error {
	if _, err := s.ValidateEquippedExtension(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	defer s.bindNativeDamage(&c.DamageContext, c.Death, rng)()
	a := s.actor(c.ActorID)
	switch id {
	case "extension-165":
		h := s.extensionPartner(a)
		if h == nil {
			return nil
		}
		stat := []int{h.Attack, h.Defense, h.Agility}[int(rng.Float64()*3)]
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack+stat)*.85, CategoryPhysical, false), rng)
		return err
	case "extension-201":
		_, err := s.ApplyExtension201(&c.DamageContext, c.shift(), rng)
		return err
	case "extension-163":
		if !a.Temporary.reflection() {
			return nil
		}
		s.actor(s.mirrorRecipient(a, c.shift())).Temporary = a.Temporary
		count := 0
		for _, member := range c.PartyIDs {
			if s.actor(member).Temporary == a.Temporary {
				count++
			}
		}
		_, err := s.ApplyPartyHealing(&c.EffectContext, float64(count*50), CategoryUntyped, rng)
		return err
	case "extension-212":
		a.Extended, a.ExtendedTurns = ExtendedInterception, 7
	case "extension-247":
		if a.HP > 0 {
			a.Extended, a.ExtendedTurns = ExtendedIncense, 3+int(rng.Float64()*5)
		}
	case "extension-248":
		return s.applyProjectile(&c.DamageContext, c.Death, c.shift(), extensionProjectile, rng)
	case "extension-146":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), string(TemporaryRegeneration), CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(r.TargetID, float64(50+int(rng.Float64()*50)), CategoryMagic), rng); err != nil {
			return err
		}
		// The write keeps the first check's target when healing redirects.
		s.actor(r.TargetID).Temporary = TemporaryRegeneration
	case "extension-200":
		if a.Extended != ExtendedWarGod {
			a.Extended, a.ExtendedTurns = ExtendedWarGod, 2+int(rng.Float64()*3)
		}
	}
	return nil
}

// extensionPartner reads the living helper by its exact identity, previous job
// first without fallback. Unlike FindCallingHelper it never matches appearances.
func (s *LifetimeState) extensionPartner(a *LifetimeActor) *LifetimeActor {
	job := callingJob(a.Jobs)
	if job == 0 {
		return nil
	}
	if h := s.rosterIdentity(callingIdentity(job)); h != nil && h.HP > 0 {
		return h
	}
	return nil
}

// mirrorRecipient accepts any participant, including enemies and the fallen.
func (s *LifetimeState) mirrorRecipient(a *LifetimeActor, target string) string {
	if s.actor(target) == nil {
		return a.ID
	}
	return target
}

// bindNativeDamage routes damage descendants to their owner and carries the
// shared cursor both ways, so a nested item159 reset reaches later cursor reads.
func (s *LifetimeState) bindNativeDamage(c *DamageContext, defeat *DefeatContext, rng random.Generator) func() {
	reset, kill := c.Reset, c.Defeat
	native := s.nativeDamageContext(*c, defeat, rng)
	shared := func(hook func(string) error) func(string) error {
		return func(id string) error {
			if defeat == nil {
				return hook(id)
			}
			defeat.Cursor = c.Cursor
			err := hook(id)
			c.Cursor = defeat.Cursor
			return err
		}
	}
	c.Reset, c.Defeat = shared(native.Reset), shared(native.Defeat)
	return func() { c.Reset, c.Defeat = reset, kill }
}

// applyProjectile uses the persistent requester's level, never the actor's. The
// first check's target binds both later helpers, which repeat their own checks.
// The caller owns target resources and an admitted, bound damage context.
func (s *LifetimeState) applyProjectile(c *DamageContext, defeat *DefeatContext, target string, v projectileVariant, rng random.Generator) error {
	previousConversion := c.symbolicConversion
	c.symbolicConversion = true // Seal12 zeroes both symbolic checks; the tail then ends.
	defer func() { c.symbolicConversion = previousConversion }()
	r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(target, projectileSymbol, CategoryPhysical), rng)
	if err != nil {
		return err
	}
	c.targetResources(r)
	target = ""
	if r.Resolved {
		target = r.TargetID
	}
	a := s.actor(c.ActorID)
	power := float64(*c.RequesterLevel) * (v.base + rng.Float64()*v.spread)
	job26 := a.Jobs.JobID == "job-26"
	if job26 {
		power += float64(a.Agility) * v.agility
		c.AdditionalEffect = true
	}
	if _, err := s.ApplyDamage(c, dualAgilityEffect(target, math.Trunc(power), CategoryPhysical, true), rng); err != nil || !job26 {
		return err
	}
	status := StatusContext{EffectContext: c.EffectContext, Defeat: defeat}
	_, err = s.ApplyStatus(&status, controlSymbol(target, StatusDeadlyPoison, CategoryPhysical), v.chance, false, false, rng)
	c.EffectContext = status.EffectContext
	return err
}

// validateProjectile runs on the caller's private projection, which it may change.
func (s *LifetimeState) validateProjectile(c *DamageContext, defeat *DefeatContext, target string, v projectileVariant, rng random.Generator) error {
	if c.RequesterLevel == nil || *c.RequesterLevel < 0 {
		return ErrUnsupportedEffect
	}
	if len(c.EnemyIDs) == 0 {
		return ErrNoBattleTarget
	}
	child := *c
	child.symbolicConversion = true
	a := s.actor(c.ActorID)
	targets, err := s.controlTargets(&ControlClimaxContext{DamageContext: child}, controlSymbol(target, projectileSymbol, CategoryPhysical), false)
	if err != nil {
		return err
	}
	if a.categorySealed(CategoryPhysical) {
		a.Status = "" // A still-sealed actor reaches only no-effect helpers.
	}
	low, high := float64(*c.RequesterLevel)*v.base, float64(*c.RequesterLevel)*(v.base+v.spread)
	job26 := a.Jobs.JobID == "job-26"
	if job26 {
		low, high = low+float64(a.Agility)*v.agility, high+float64(a.Agility)*v.agility
		child.AdditionalEffect = true
	}
	low, high = math.Trunc(low), math.Trunc(high)
	for _, id := range targets {
		states, err := s.projectCallbackDamage(&child, defeat, dualAgilityEffect(id, low, CategoryPhysical, true), high, job26, rng)
		if err != nil {
			return err
		}
		if !job26 {
			continue
		}
		for _, next := range states {
			status := StatusContext{EffectContext: next.Context.EffectContext, Defeat: defeat}
			if err := next.State.ValidateStatus(&status, controlSymbol(id, StatusDeadlyPoison, CategoryPhysical), v.chance, false, false, rng); err != nil {
				return err
			}
		}
	}
	return nil
}
