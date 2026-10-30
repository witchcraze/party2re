package battle

import (
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// NamedFieldContext holds submitted, frozen command facts and the existing
// operation owners. Catalog is rebuilt after reload, never serialized.
type NamedFieldContext struct {
	DamageContext
	Arguments                                     []string
	Death                                         *DefeatContext
	Roster                                        *RosterContext
	Catalog                                       CallbackCatalog
	Discovered                                    []CallbackEntry
	SubmittedCommand, SubmittedTarget, CustomName string
	RequesterGender, RequesterIcon                string
	PartyMPHealCalls, CureCalls                   int
	ResponseSuppressed                            bool
}

// NamedFieldEntries certifies positions, independently of complete job tables.
func NamedFieldEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "job-22-entry-5", Name: "darkness-field", RequiredSP: 140, MPCost: 7, Extended: true},
		{ID: "job-29-entry-7", Name: "cosmic-field", RequiredSP: 150, MPCost: 1, Extended: true},
		{ID: "job-53-entry-6", Name: "trap-field", RequiredSP: 120, MPCost: 1, Extended: true},
		{ID: "job-55-entry-7", Name: "rainbow-light", RequiredSP: 80, MPCost: 24, Extended: true},
		{ID: "job-56-entry-3", Name: "finite-random", RequiredSP: 46, MPCost: 10},
		{ID: "job-64-grave", Name: "grave", RequiredSP: 150, MPCost: 1, Extended: true},
		{ID: "job-84-entry-4", Name: "burn-field", RequiredSP: 140, MPCost: 35},
		{ID: "job-86-entry-1", Name: "library-field", RequiredSP: 5, MPCost: 10},
	}
}

func namedFieldSpec(id string) (string, int, int) {
	switch id {
	case "job-22-entry-5":
		return "darkness", 5, 5
	case "job-29-entry-7":
		return "cosmic", 5, 3
	case "job-53-entry-6":
		return "antlion", 6, 5
	case "job-64-grave":
		return "grave", 6, 3
	case "job-84-entry-4":
		return "burn", 5, 0
	case "job-86-entry-1":
		return "library", 11, 6
	}
	return "", 0, 0
}

func (c *NamedFieldContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// Last discovered name wins, just as the submitted actions map does. A name
// whitelisted by this producer never supplies a substitute callback identity.
func (c *NamedFieldContext) darknessContinuation() (string, string, error) {
	name := strings.TrimLeft(c.SubmittedCommand, "＠")
	if c.SubmittedCommand == c.CustomName {
		return "", "", nil
	}
	allowed := false
	for _, entry := range RecoilDarkStateEntries(22)[:5] {
		allowed = allowed || name == entry.Name
	}
	if !allowed {
		return "", "", nil
	}
	id := ""
	for _, entry := range c.Discovered {
		if strings.TrimLeft(entry.Name, "＠") == name {
			id = entry.ID
		}
	}
	// Perl's list-operator precedence feeds the custom-name filtered list into
	// the outer grep, excluding a matching custom command before map lookup.
	if id == "" {
		return "", "", ErrUnsupportedEffect
	}
	return id, strings.TrimPrefix(strings.TrimSpace(c.SubmittedTarget), "&gt;"), nil
}

func (s *LifetimeState) createDarkness(c *NamedFieldContext) {
	for _, id := range c.EnemyIDs {
		a := s.actor(id)
		a.Attack, a.Defense, a.Agility = a.BaseStats.Attack, a.BaseStats.Defense, a.BaseStats.Agility
	}
	a := s.actor(c.ActorID)
	cap := 999
	if a.OverLevel {
		cap = max(999, min(1998, a.BaseStats.Attack*3))
	}
	a.Attack = min(cap, a.Attack+int(float64(a.BaseStats.Attack)*.5))
}

// ApplyNamedField runs the complete intrinsic body; ordinary listed costs and
// random/delegated restoration remain with the existing invocation owner.
func (s *LifetimeState) ApplyNamedField(c *NamedFieldContext, id string, rng random.Generator) error {
	if err := s.ValidateNamedField(c, id, rng); err != nil {
		return err
	}
	previous := c.targetResources
	previousEffect := c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previous, previousEffect }()
	if id == "job-56-entry-3" {
		s.applyFiniteChaos(rng)
		return nil
	}
	if id == "job-55-entry-7" {
		return s.applyRainbow(c, int(rng.Float64()*7), rng)
	}
	if id == "job-64-grave" {
		h := s.actor(s.FindCallingHelper(c.TargetContext, 64))
		if h == nil || h.HP <= 0 {
			return nil
		}
	}
	name, base, width := namedFieldSpec(id)
	turns := base
	if width > 0 {
		turns += int(rng.Float64() * float64(width))
	}
	if !s.Fields.Create(name, turns) {
		return nil
	}
	switch name {
	case "darkness":
		s.createDarkness(c)
		child, target, err := c.darknessContinuation()
		if err != nil {
			return err
		}
		if child != "" {
			c.GuaranteedHit = true
			frame := CallbackContext{TargetContext: c.TargetContext, Arguments: []string{target}, Damage: &c.DamageContext}
			if err := s.ApplyInvocations(&frame, c.Catalog, []CallbackInvocation{{Kind: "entry", EntryID: child}}, rng); err != nil {
				return err
			}
			c.TargetContext = frame.TargetContext
			a := s.actor(c.ActorID)
			a.HP = max(1, a.HP-a.HP/2)
		}
	case "cosmic":
		if _, err := s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(s.actor(c.ActorID).MaxMP)*.075, CategoryMagic), rng); err != nil {
			return err
		}
		team := s.actor(c.ActorID).TeamID
		if s.RecordFirstUse(team, "job-29-entry-7") {
			c.ResponseSuppressed = true
		}
	case "antlion", "grave":
		if rng.Float64()*2 < 1 {
			symbol := "antlion"
			if name == "grave" {
				symbol = "possession"
			}
			r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), symbol, CategoryUntyped), rng)
			if err != nil {
				return err
			}
			resources(r)
			if !r.Resolved {
				return ErrUnsupportedEffect
			}
			if name == "antlion" {
				s.actor(r.TargetID).Status = StatusDofuu
				return nil
			}
			original := c.TargetContext
			originalNPC, originalReset, originalDefeat := c.ActorNPC, c.Reset, c.Defeat
			defer func() {
				c.TargetContext, c.ActorNPC, c.Reset, c.Defeat = original, originalNPC, originalReset, originalDefeat
				if c.Death != nil {
					c.Death.TargetContext = original
				}
			}()
			c.TargetContext = possessionContext(original, r.TargetID)
			c.ActorNPC = s.actor(r.TargetID).NPC
			c.DamageContext = s.nativeDamageContext(c.DamageContext, c.Death, rng)
			for range 2 {
				if _, err := s.applyNormalStrike(&c.DamageContext, "", rng); err != nil {
					return err
				}
			}
			s.actor(r.TargetID).Status = StatusConfusion
		}
	case "burn":
		reset := c.Death.EquipmentResetContext
		reset.TargetContext = c.TargetContext
		for _, a := range s.Actors {
			if err := s.ApplyEquipmentReset(&reset, a.ID, rng); err != nil {
				return err
			}
		}
		c.Cursor, c.Death.Cursor = reset.Cursor, reset.Cursor
		for _, id := range c.PartyIDs {
			a := s.actor(id)
			if id != c.ActorID && a.HP > 0 {
				a.HP = max(1, a.HP-300)
			}
		}
		damage := s.nativeDamageContext(c.DamageContext, c.Death, rng)
		if _, err := s.ApplyAreaDamage(&damage, 300, CategoryBreath, true, rng); err != nil {
			return err
		}
		c.DamageContext = damage
		a := s.actor(c.ActorID)
		if a.Jobs.JobID == "job-84" && a.Icon == c.RequesterIcon {
			a.Icon = nativeFormIcon("job-84-ex", c.RequesterGender)
		}
		return s.statTension(&c.EffectContext, a, false, rng)
	}
	return nil
}
