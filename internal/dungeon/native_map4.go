package dungeon

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// NativeMap4ID is the attrition maze (legacy map/4, seven initial variants).
const NativeMap4ID = "native-map-4"

// nativeMap4Grids transcribe map/4/1.cgi–7.cgi with their legacy symbols.
var nativeMap4Grids = [][]string{
	{"SM000010bB2", "11011010134", "00010010111", "01110110100", "00010H10001", "11011111101", "00000000100", "01101010001", "01101011111", "0000100000H"},
	{"SM000010Bb2", "11011010113", "00010010014", "01110111011", "00010001000", "11011101101", "00001000100", "01101010001", "01101011111", "0000100000H"},
	{"SM000010Bb2", "11011010113", "00010010014", "01110111011", "00010001000", "11011101110", "00001000110", "01101010010", "01101011110", "00H01000000"},
	{"SM000010bB2", "11011010134", "00010010111", "01110110100", "00010H10001", "11011111100", "00000000101", "0111111H100", "01100011110", "00001000000"},
	{"SM00001432B", "1101101111b", "00010010H10", "01110110110", "00010H10000", "11011111110", "00000000100", "0110111H101", "01111011101", "00000000000"},
	{"SM000010Bb2", "11011010113", "00010010014", "01110111011", "00010001000", "11011101101", "00000001000", "01111111011", "0100000101H", "00010100000"},
	{"SM000010Bb2", "11011010113", "00010010014", "01110111011", "00010001000", "11011101110", "00000001H10", "01111111010", "01000001010", "00010100000"},
}

// Passages charge the mover instead of spawning, and every H shares one flag.
var nativeMap4Events = map[byte]mapEvent{
	'0': {action: attritionEvent{}},
	'M': {action: messageEvent{}, perSymbol: true},
	'H': {action: recoveryEvent{}, perSymbol: true},
	'b': {action: bossEvent{}},
	'2': {action: treasureEvent{}, perSymbol: true},
	'3': {action: treasureEvent{}, perSymbol: true},
	'4': {action: treasureEvent{}, perSymbol: true},
}

// nativeMap4 has round limit 70 and the menu threshold for dungeon 4 (job level 9).
// Each event loads one of stages 4–8 (int(rand(5) + 4)).
func nativeMap4() Dungeon {
	pools := stagePools(4, 8)
	marks := map[byte]string{'B': "◎", 'b': "◎", '2': "宝", '3': "宝", '4': "宝"}
	variants := variantFloors(Floor{FloorNumber: 1, Pools: pools, Marks: marks, events: nativeMap4Events}, nativeMap4Grids)
	return Dungeon{
		ID:             NativeMap4ID,
		Name:           "命削りの大迷路",
		Description:    "敵の出ない大迷路。一歩ごとに進む者の命が削られていく。",
		RoundLimit:     70,
		LeaderJobLevel: 9,
		Variants:       variants,
	}
}

// attritionLoss is int(MaxHP × 0.02 + 0.5), exact in integers.
func attritionLoss(exp *ActiveExpedition, actor *corecharacter.Character) int {
	maxHP := actor.Stats.MaxHP
	for _, m := range exp.Members {
		if m.CharacterID == actor.ID {
			maxHP = m.MaxHP
		}
	}
	return (2*maxHP + 50) / 100
}

// recoverOne is map4 event_H. Its call forwards the direction name as the
// target, which _check_party treats as unspecified: a uniform roster member,
// fallen ones redirected to the actor. Half the time HP, otherwise MP, base 40.
func recoverOne(exp *ActiveExpedition, actor *corecharacter.Character, rng RandomSource) (hp, mp map[string]int, err error) {
	healHP := rng.Float64() < .5
	return rosterHeal(exp, actor, func(state *corebattle.LifetimeState, c *corebattle.EffectContext, g randomSourceAdapter) error {
		effect := corebattle.TargetEffect{Payload: corebattle.CallbackPayload{Number: 40}}
		apply := state.ApplyMPHealing
		if healHP {
			apply = state.ApplyHealing
		}
		_, err := apply(c, effect, g)
		return err
	}, rng)
}

// partyHeal is _heals(base, '無'): every living member through the shared
// party-heal routine with the actor's modifiers (map6).
func partyHeal(exp *ActiveExpedition, actor *corecharacter.Character, base float64, rng RandomSource) (map[string]int, error) {
	hp, _, err := rosterHeal(exp, actor, func(state *corebattle.LifetimeState, c *corebattle.EffectContext, g randomSourceAdapter) error {
		_, err := state.ApplyPartyHealing(c, base, corebattle.CategoryUntyped, g)
		return err
	}, rng)
	return hp, err
}

// rosterState builds a LifetimeState and ordered member IDs from the saved roster.
func rosterState(exp *ActiveExpedition) (corebattle.LifetimeState, []string) {
	state := corebattle.LifetimeState{Actors: make([]corebattle.LifetimeActor, len(exp.Members))}
	ids := make([]string, len(exp.Members))
	for i, m := range exp.Members {
		ids[i] = m.CharacterID
		p := corebattle.Participant{ID: m.CharacterID, Name: m.Name, TeamID: "allies", HP: m.CurrentHP, MaxHP: m.MaxHP, MP: m.CurrentMP, MaxMP: m.MaxMP, ItemDefinitionIDs: m.ItemIDs}
		state.Actors[i] = corebattle.LifetimeActor{Participant: p, Tension: 1, Hit: 95}
	}
	return state, ids
}

// rosterHeal runs one healing effect on the saved roster and returns the
// per-member HP and MP changes.
func rosterHeal(exp *ActiveExpedition, actor *corecharacter.Character, heal func(*corebattle.LifetimeState, *corebattle.EffectContext, randomSourceAdapter) error, rng RandomSource) (hp, mp map[string]int, err error) {
	state, ids := rosterState(exp)
	c := &corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: actor.ID, PartyIDs: ids}}
	if err := heal(&state, c, randomSourceAdapter{rng}); err != nil {
		return nil, nil, err
	}
	hp, mp = map[string]int{}, map[string]int{}
	for i, m := range exp.Members {
		if d := state.Actors[i].HP - m.CurrentHP; d != 0 {
			hp[m.CharacterID] = d
		}
		if d := state.Actors[i].MP - m.CurrentMP; d != 0 {
			mp[m.CharacterID] = d
		}
	}
	return hp, mp, nil
}
