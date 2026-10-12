package battle

import "github.com/witchcraze/party2re/internal/core/random"

type TemporaryEffect string

const (
	TemporaryDefense       TemporaryEffect = "defense"
	TemporaryStrongDefense TemporaryEffect = "strong_defense"
	TemporaryOneHitDefense TemporaryEffect = "one_hit_defense"
	TemporaryReflectAttack TemporaryEffect = "reflect_attack"
	TemporaryReflectMagic  TemporaryEffect = "reflect_magic"
	TemporaryReflectBreath TemporaryEffect = "reflect_breath"
	TemporaryReflectDance  TemporaryEffect = "reflect_dance"
	TemporaryCover         TemporaryEffect = "cover"
	TemporaryCovering      TemporaryEffect = "covering"
	TemporaryRevival       TemporaryEffect = "revival"
	TemporaryRegeneration  TemporaryEffect = "regeneration"
	TemporaryReduceAttack  TemporaryEffect = "reduce_attack"
	TemporaryReduceMagic   TemporaryEffect = "reduce_magic"
	TemporaryReduceBreath  TemporaryEffect = "reduce_breath"
	TemporaryDouble        TemporaryEffect = "double"
	TemporaryDeflect       TemporaryEffect = "deflect"
	TemporaryAbsorbMagic   TemporaryEffect = "absorb_magic"
	TemporaryNullAttack    TemporaryEffect = "null_attack"
	TemporaryNullMagic     TemporaryEffect = "null_magic"
	TemporaryPhantom       TemporaryEffect = "phantom"
	TemporaryRage          TemporaryEffect = "rage"
)

func (e TemporaryEffect) reflection() bool {
	return e == TemporaryReflectAttack || e == TemporaryReflectMagic || e == TemporaryReflectBreath || e == TemporaryReflectDance
}

// BeginNormalAction expires effects only after incapacitation checks, before
// the callback. Player mirror-sharing preserves reflection; item163 makes its
// expiry random. NPCs have neither exception. _battle.cgi:748–755;
// _npc_action.cgi:315. nil RNG is invalid for a probabilistic expiry.
func (a *LifetimeActor) BeginNormalAction(npc, mirrorSharing, reflectionAccessory bool, rng random.Generator) error {
	reflecting := a.Temporary.reflection()
	if !npc && mirrorSharing && reflecting {
		return nil
	}
	defense := a.Temporary == TemporaryDefense || a.Temporary == TemporaryStrongDefense || a.Temporary == TemporaryOneHitDefense
	if defense || (reflecting && (npc || !reflectionAccessory)) {
		a.Temporary = ""
		return nil
	}
	if rng == nil {
		return ErrInvalidRequest
	}
	if rng.Float64() < 1.0/3 {
		a.Temporary = ""
	}
	return nil
}

// ConsumeOneHitDefense is called at the damage helper's documented point,
// after recording DamageCalls and before hit/evasion checks. Reflection and
// cover are redirects, not single-use consumption (_skill.cgi:9464–9715).
func (a *LifetimeActor) ConsumeOneHitDefense() bool {
	if a.Temporary != TemporaryOneHitDefense {
		return false
	}
	a.Temporary = ""
	return true
}

// ResetEffects restores lifetime baselines and the supplied, freshly evaluated
// equipment facts. Equipment must be evaluated for this reset's requester,
// actor and target context; an encounter-start snapshot is insufficient.
// HP/MP/CMP/items/history/bans are deliberately outside this reset.
// _battle.cgi:1390–1408; #1365 owns equipment formula certification.
func (a *LifetimeActor) ResetEffects(equipment LifetimeReset) {
	a.Status = equipment.Status
	a.Temporary = equipment.Temporary
	a.Extended, a.ExtendedTurns = equipment.Extended, equipment.ExtendedTurns
	a.Tension, a.Hit = 1, 95
	if equipment.Tension != 0 {
		a.Tension = equipment.Tension
	}
	if equipment.Hit != nil {
		a.Hit = *equipment.Hit
	}
	a.Attack, a.Defense, a.Agility = equipment.Attack, equipment.Defense, max(0, equipment.Agility)
	a.Defending = false
}

// LifetimeReset contains the result of equipment reapplication, never code.
// Hit is optional because zero is a meaningful accuracy override.
type LifetimeReset struct {
	Status                   string
	Attack, Defense, Agility int
	Temporary                TemporaryEffect
	Extended                 ExtendedEffect
	ExtendedTurns            int
	Tension                  float64
	Hit                      *int
}

// ReviveOnAdvance checks item188's party-wide pre-reset pass. The first living
// holder supplies independent draws for fallen allies. Reset all members after
// this pass, then apply TemporaryRevival to each living holder.
func (s *LifetimeState) ReviveOnAdvance(partyIDs []string, rng random.Generator) error {
	party := make([]*LifetimeActor, 0, len(partyIDs))
	for _, id := range partyIDs {
		var found *LifetimeActor
		for i := range s.Actors {
			if s.Actors[i].ID == id {
				found = &s.Actors[i]
				break
			}
		}
		if found == nil {
			return ErrInvalidParticipant
		}
		party = append(party, found)
	}
	for _, a := range party {
		if a.HP <= 0 || !hasItem(a.ItemDefinitionIDs, "item-188") {
			continue
		}
		if rng == nil {
			return ErrInvalidRequest
		}
		for _, other := range party {
			if other.HP <= 0 && rng.Float64() < 0.5 {
				other.HP = other.MaxHP / 2
			}
		}
		break
	}
	return nil
}
