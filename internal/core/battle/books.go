package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BookContext retains operation facts; consumers rebuild ports after reload.
type BookContext struct {
	EffectContext
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

// BookEntries is the restorative/proportional-stat subset of the source book
// table (ff97fc5:_skill.cgi:479–736). Revival, copy and the other bodies keep
// their separate owners. MPCost is the listed cost before the item249 modifier.
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
	}
}

type bookBody struct {
	power float64
	stat  BattleStat // empty for healing
	party bool
}

func bookSpec(id string) (bookBody, bool) {
	switch id {
	case "book-169":
		return bookBody{power: 30}, true
	case "book-170":
		return bookBody{power: 90}, true
	case "book-171":
		return bookBody{power: 120, party: true}, true
	case "book-172":
		return bookBody{power: 180}, true
	case "book-175":
		return bookBody{power: .4, stat: StatDefense}, true
	case "book-176":
		return bookBody{power: .25, stat: StatDefense, party: true}, true
	case "book-177":
		return bookBody{power: .4, stat: StatAgility}, true
	case "book-178":
		return bookBody{power: .25, stat: StatAgility, party: true}, true
	case "book-179":
		return bookBody{power: 1, stat: StatAttack}, true
	case "book-250":
		return bookBody{power: 999, party: true}, true
	}
	return bookBody{}, false
}

// bookPower multiplies the base value before modifiers; book250 is excluded.
func (s *LifetimeState) bookPower(id string, b bookBody) float64 {
	if id != "book-250" && s.Fields.Current() == "library" {
		return b.power * 1.5
	}
	return b.power
}

// ValidateBookAction is the ordinary held-book discovery and submission gate:
// player actor, enemies, the held book and item249-adjusted MP, but neither SP
// nor an uncursed actor (_skill.cgi:34,136–145,260–283). Of these names only
// book172 is on the magician list (_battle.cgi:1804–1825), so it halves; the
// others double. The consumer debits the returned cost after the callback.
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
			if id == "book-172" {
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
	previous := c.targetResources
	c.targetResources = func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	defer func() { c.targetResources = previous }()
	b, _ := bookSpec(id)
	power := s.bookPower(id, b)
	var err error
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
