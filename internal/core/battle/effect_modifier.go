package battle

import "github.com/witchcraze/party2re/internal/core/random"

// EffectContext is operation-local, shared by ordered helpers. It is not a
// battle round or durable lifetime counter. Resource writes from target helpers
// remain explicit results for the MP owner. Damage must be fully admitted by
// its owner before supplying the item201 hook; nil retains whole-effect gating.
type EffectContext struct {
	TargetContext
	DamageCalls, AreaDamageCalls         int
	HealCalls, PartyHealCalls            int
	StatUpCalls, StatDownCalls           int
	PartyStatUpCalls, PartyStatDownCalls int
	TensionCalls                         int
	Item185Activated, BowEffect          bool
	AdditionalEffect, ActorNPC           bool
	RequesterLevel                       *int
	// Damage's owner updates this context's DamageCalls after target/zero gates
	// (_damage:8826–8831), including misses. The caller must not count again.
	Damage func(TargetContext, TargetEffect, random.Generator) error
	// Complete native bodies apply target resource facts before effect tails.
	targetResources func(TargetResolution)
}

type ModifierKind string

const (
	ModifierHPHealing ModifierKind = "hp_healing"
	ModifierMPHealing ModifierKind = "mp_healing"
	ModifierStatUp    ModifierKind = "stat_up"
	ModifierStatDown  ModifierKind = "stat_down"
)

// EffectApplication distinguishes helper rejection, a returned no-effect tuple,
// requested amount and actual main-stat change. Incidental recovery and target
// resource facts must be handled even when Applied is false.
type EffectApplication struct {
	TargetResolution
	Returned          bool
	Applied           bool
	Amount            int
	Delta             int
	IncidentalHealing []HealingFact
}

type HealingFact struct {
	TargetID      string
	Amount, Delta int
}

// ModifyEffect is the nondamage shared modifier segment at
// _skill.cgi:10084–10148. MP recovery calls this but owns its separate writes,
// exclusions and overflow. Stat-up's tool heal precedes the item239 zero gate.
func (s *LifetimeState) ModifyEffect(c *EffectContext, e TargetEffect, kind ModifierKind, stat BattleStat, rng random.Generator) (float64, []HealingFact, error) {
	if c == nil {
		return 0, nil, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return 0, nil, err
	}
	if err := e.validate(); err != nil || e.Payload.Symbol != "" {
		return 0, nil, ErrUnsupportedEffect
	}
	target := s.actor(e.TargetID)
	if target == nil {
		return 0, nil, ErrInvalidParticipant
	}
	switch kind {
	case ModifierHPHealing, ModifierMPHealing, ModifierStatUp, ModifierStatDown:
	default:
		return 0, nil, ErrUnsupportedEffect
	}
	if kind == ModifierStatUp && !stat.canRaise() {
		return 0, nil, ErrUnsupportedEffect
	}
	a := s.actor(c.ActorID)
	v := s.initialEffectPower(c, e, kind)
	var heals []HealingFact
	if hasItem(a.ItemDefinitionIDs, "item-186") && kind == ModifierStatUp {
		r, err := s.ApplyToolHealing(c.TargetContext, e.TargetID, float64(int(float64(target.baseStat(stat))*v)), false, rng)
		if err != nil {
			return 0, nil, err
		}
		heals = append(heals, HealingFact{TargetID: e.TargetID, Amount: r.Amount, Delta: r.Delta})
	}
	v = s.finalEffectPower(c, v)
	if kind == ModifierHPHealing || kind == ModifierMPHealing {
		if a.Status == StatusSabaku {
			v *= .7 + rng.Float64()*.2
		}
		if target.Status == StatusVirulentPoison {
			v = float64(int(v * .1))
		}
	}
	if kind == ModifierMPHealing && s.Fields.Current() == "den" {
		v = float64(int(v*.025) + 1)
	}
	return v, heals, nil
}

// Admission reuses the pure modifiers on either side of incidental186 healing.
func (s *LifetimeState) initialEffectPower(c *EffectContext, e TargetEffect, kind ModifierKind) float64 {
	a := s.actor(c.ActorID)
	v := e.Payload.Number
	if s.Fields.Current() == "sanctuary" && e.Category == CategoryMagic && kind != ModifierStatDown {
		v *= 1.2
	}
	if s.Fields.Current() == "darkness" && kind == ModifierStatUp {
		v = 0
	}
	if c.Item185Activated {
		v *= .5
	}
	if c.BowEffect {
		v *= 1.5
	}
	if hasItem(a.ItemDefinitionIDs, "item-190") && e.Category == CategoryDance && (kind == ModifierHPHealing || kind == ModifierStatUp) {
		v *= 1.5
	}
	return v
}

func (s *LifetimeState) finalEffectPower(c *EffectContext, v float64) float64 {
	a := s.actor(c.ActorID)
	if hasItem(a.ItemDefinitionIDs, "item-144") && c.AreaDamageCalls > 0 && v < 300 {
		v *= 1.3
	}
	if hasItem(a.ItemDefinitionIDs, "item-239") && c.DamageCalls >= 1 {
		v = 0
	}
	return v
}
