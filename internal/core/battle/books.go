package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BookContext retains operation facts; consumers rebuild ports after reload.
type BookContext struct {
	DamageContext
	Arguments []string
}

func (c *BookContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// BookEntries is the certified subset of the source book table
// (ff97fc5:_skill.cgi:479–736). Revival and copy keep their separate owners.
// MPCost is the listed cost before the item249 modifier.
func BookEntries() []CallbackEntry {
	entry := func(number string, name string, mp int) CallbackEntry {
		return CallbackEntry{ID: "book-" + number, Name: name, MPCost: mp}
	}
	return []CallbackEntry{
		entry("169", "book-heal-30", 6), entry("170", "book-heal-90", 20),
		entry("171", "book-party-heal-120", 50), entry("172", "book-heal-180", 36),
		entry("175", "book-defense-up", 9), entry("176", "book-party-defense-up", 21),
		entry("177", "book-agility-up", 9), entry("178", "book-party-agility-up", 18),
		entry("179", "book-attack-up", 48), entry("250", "book-party-heal-999", 120),
		entry("174", "book-magic-reflection", 15), entry("218", "book-poison-cure", 8),
		entry("219", "book-party-paralysis-cure", 15), entry("220", "book-party-sleep-cure", 12),
		entry("221", "book-current-status-cure", 35),
		entry("222", "book-magic-20", 6), entry("223", "book-magic-70", 17), entry("224", "book-magic-220", 46),
		entry("225", "book-magic-wave-25", 11), entry("226", "book-magic-wave-60", 24), entry("227", "book-magic-wave-125", 40),
		entry("228", "book-sleep", 19), entry("229", "book-area-confusion", 24), entry("230", "book-area-magic-seal", 24),
	}
}

type bookKind int

const (
	bookHeal bookKind = iota
	bookStat
	bookDamage
	bookStatus
	bookCure
	bookReflect
)

// party means all allies, or all enemies for damage/status. power is the heal
// or damage base, stat coefficient or status chance. An empty cure symbol is
// book221's current-state cure.
type bookBody struct {
	kind   bookKind
	power  float64
	stat   BattleStat
	symbol string
	party  bool
}

func bookSpec(id string) (bookBody, bool) {
	switch id {
	case "book-169":
		return bookBody{kind: bookHeal, power: 30}, true
	case "book-170":
		return bookBody{kind: bookHeal, power: 90}, true
	case "book-171":
		return bookBody{kind: bookHeal, power: 120, party: true}, true
	case "book-172":
		return bookBody{kind: bookHeal, power: 180}, true
	case "book-175":
		return bookBody{kind: bookStat, power: .4, stat: StatDefense}, true
	case "book-176":
		return bookBody{kind: bookStat, power: .25, stat: StatDefense, party: true}, true
	case "book-177":
		return bookBody{kind: bookStat, power: .4, stat: StatAgility}, true
	case "book-178":
		return bookBody{kind: bookStat, power: .25, stat: StatAgility, party: true}, true
	case "book-179":
		return bookBody{kind: bookStat, power: 1, stat: StatAttack}, true
	case "book-250":
		return bookBody{kind: bookHeal, power: 999, party: true}, true
	case "book-174":
		return bookBody{kind: bookReflect}, true
	case "book-218":
		return bookBody{kind: bookCure, symbol: StatusDeadlyPoison}, true
	case "book-219":
		return bookBody{kind: bookCure, symbol: StatusParalyze, party: true}, true
	case "book-220":
		return bookBody{kind: bookCure, symbol: StatusSleep, party: true}, true
	case "book-221":
		return bookBody{kind: bookCure}, true
	case "book-222":
		return bookBody{kind: bookDamage, power: 20}, true
	case "book-223":
		return bookBody{kind: bookDamage, power: 70}, true
	case "book-224":
		return bookBody{kind: bookDamage, power: 220}, true
	case "book-225":
		return bookBody{kind: bookDamage, power: 25, party: true}, true
	case "book-226":
		return bookBody{kind: bookDamage, power: 60, party: true}, true
	case "book-227":
		return bookBody{kind: bookDamage, power: 125, party: true}, true
	case "book-228":
		return bookBody{kind: bookStatus, power: 65, symbol: StatusSleep}, true
	case "book-229":
		return bookBody{kind: bookStatus, power: 50, symbol: StatusConfusion, party: true}, true
	case "book-230":
		return bookBody{kind: bookStatus, power: 70, symbol: StatusMagicSeal, party: true}, true
	}
	return bookBody{}, false
}

// bookPower multiplies heal, stat and damage bases before modifiers; book250
// and the reflection, cure and status bodies are excluded.
func (s *LifetimeState) bookPower(id string, b bookBody) float64 {
	if id != "book-250" && b.kind <= bookDamage && s.Fields.Current() == "library" {
		return b.power * 1.5
	}
	return b.power
}

// bookMagician lists the books whose names are on the item249 magician list
// (_battle.cgi:1804–1825); item249 halves their cost and doubles the others.
func bookMagician(id string) bool {
	switch id {
	case "book-172", "book-174", "book-222", "book-223", "book-224", "book-225",
		"book-226", "book-227", "book-228", "book-229", "book-230":
		return true
	}
	return false
}

func (c *BookContext) status() *StatusContext {
	return &StatusContext{EffectContext: c.EffectContext}
}

func (c *BookContext) keepStatus(sc *StatusContext) {
	c.EffectContext = sc.EffectContext
}

// ValidateBookAction is the ordinary held-book discovery and submission gate:
// player actor, enemies, the held book and item249-adjusted MP, but neither SP
// nor an uncursed actor (_skill.cgi:34,136–145,260–283). The consumer debits
// the returned cost after the callback.
// Job86's double-use wrapper uses ValidateBook with its own cost instead.
func (s *LifetimeState) ValidateBookAction(c *BookContext, id string, rng random.Generator) (CallbackEntry, error) {
	if s == nil || c == nil {
		return CallbackEntry{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return CallbackEntry{}, err
	}
	a := s.actor(c.ActorID)
	for _, entry := range BookEntries() {
		if entry.ID != id {
			continue
		}
		if a.NPC || c.ActorNPC || !hasItem(a.ItemDefinitionIDs, "item-"+id[len("book-"):]) || len(c.EnemyIDs) == 0 {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		if hasItem(a.ItemDefinitionIDs, "item-249") {
			if bookMagician(id) {
				entry.MPCost /= 2
			} else {
				entry.MPCost *= 2
			}
		}
		if a.MP < entry.MPCost {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		return entry, s.ValidateBook(c, id, rng)
	}
	return CallbackEntry{}, ErrUnsupportedEffect
}

// ValidateBook admits the intrinsic body on isolated state, without argument
// consumption, resource writes, external Apply hooks or RNG draws.
func (s *LifetimeState) ValidateBook(c *BookContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	b, ok := bookSpec(id)
	if !ok {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	power := p.bookPower(id, b)
	switch b.kind {
	case bookDamage:
		if b.party {
			return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power, CategoryMagic, true), rng)
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), power, CategoryMagic, true), rng)
	case bookStatus:
		e := controlSymbol("", b.symbol, CategoryMagic)
		if b.party {
			return p.ValidateAreaStatus(child.status(), e, power, false, false, rng)
		}
		e.TargetID = child.shift()
		return p.ValidateStatus(child.status(), e, power, false, false, rng)
	case bookReflect:
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "reflection", CategoryMagic), rng)
	case bookCure:
		if !b.party {
			target := child.shift()
			if b.symbol == "" {
				// Book221 resolves with an untyped intent, then cures as magic.
				if err := p.validatePartyTarget(child.TargetContext, controlSymbol(target, "current_status_cure", CategoryUntyped), rng); err != nil {
					return err
				}
			}
			return p.validatePartyTarget(child.TargetContext, controlSymbol(target, b.symbol, CategoryMagic), rng)
		}
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(target, b.symbol, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	}
	switch {
	case b.stat != "" && b.party:
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, power, CategoryMagic)
			if err := p.validateStatUp(&child.EffectContext, e, b.stat, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, b.stat, &admission)
		}
	case b.stat != "":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), power, CategoryMagic), b.stat, rng)
	case b.party:
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, power, CategoryMagic), rng); err != nil {
				return err
			}
		}
	default:
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	}
	return nil
}

// ApplyBook executes the intrinsic body; ordinary costs and book assets remain
// with the invoking consumer. Party bodies leave the argument unconsumed.
func (s *LifetimeState) ApplyBook(c *BookContext, id string, rng random.Generator) error {
	if err := s.ValidateBook(c, id, rng); err != nil {
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
	b, _ := bookSpec(id)
	power := s.bookPower(id, b)
	var err error
	switch b.kind {
	case bookDamage:
		if b.party {
			_, err = s.ApplyAreaDamage(&c.DamageContext, power, CategoryMagic, true, rng)
		} else {
			_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
		}
		return err
	case bookStatus:
		sc, e := c.status(), controlSymbol("", b.symbol, CategoryMagic)
		if b.party {
			_, err = s.ApplyAreaStatus(sc, e, power, false, false, rng)
		} else {
			e.TargetID = c.shift()
			_, err = s.ApplyStatus(sc, e, power, false, false, rng)
		}
		c.keepStatus(sc)
		return err
	case bookReflect:
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "reflection", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			s.actor(r.TargetID).Temporary = TemporaryReflectMagic
		}
		return nil
	case bookCure:
		sc, e := c.status(), controlSymbol("", b.symbol, CategoryMagic)
		switch {
		case b.party:
			_, err = s.ApplyPartyCure(sc, e, rng)
		case b.symbol != "":
			e.TargetID = c.shift()
			_, err = s.ApplyCure(sc, e, rng)
		default:
			// Book221 resolves with an untyped intent, then cures that status as magic.
			var r TargetResolution
			r, err = s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "current_status_cure", CategoryUntyped), rng)
			if err == nil {
				resources(r)
				if r.Resolved {
					e.TargetID, e.Payload.Symbol = r.TargetID, s.actor(r.TargetID).Status
					_, err = s.ApplyCure(sc, e, rng)
				}
			}
		}
		c.keepStatus(sc)
		return err
	}
	switch {
	case b.stat != "" && b.party:
		_, err = s.ApplyPartyStat(&c.EffectContext, power, CategoryMagic, b.stat, false, rng)
	case b.stat != "":
		_, err = s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), b.stat, rng)
	case b.party:
		_, err = s.ApplyPartyHealing(&c.EffectContext, power, CategoryMagic, rng)
	default:
		_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
	}
	// Book250 seals its caster after any heal outcome, outside the library.
	if err == nil && id == "book-250" && s.Fields.Current() != "library" {
		s.actor(c.ActorID).Status = StatusMagicSeal
	}
	return err
}
