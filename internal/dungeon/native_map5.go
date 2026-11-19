package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap5ID is the lever temple (legacy map/5, four initial variants).
const NativeMap5ID = "native-map-5"

// jobRemedy (legacy job 43) deciphers the H table for the acting character only.
const jobRemedy = "job-43"

// nativeMap5Grids transcribe map/5/1.cgi–4.cgi with their legacy symbols.
var nativeMap5Grids = [][]string{
	{"S0000IT140t", "0111t111110", "01000000010", "01011101010", "0I01L101000", "01010001101", "0T01I111100", "0101I12100I", "0001IIB1H13"},
	{"S0000ItI40T", "011T1111110", "01000000010", "0T011101010", "0101L101000", "00010001010", "010111I1010", "01t121I10II", "0001BII1H13"},
	{"3000L000000", "01111111110", "01S000010T0", "01011100010", "0101000I000", "t101011T141", "0I0T0H11111", "01010110II1", "000000001B2"},
	{"30000t00000", "01111111110", "01S00001000", "L101110001T", "0101000I000", "0t0101141T1", "01010H11111", "0101t110II1", "0T0000001B2"},
}

// The lever is a once-only boss whose flag also reveals every I.
var nativeMap5Events = map[byte]mapEvent{
	'I': {action: hiddenEvent{key: 'L'}},
	'L': {action: bossEvent{}, perSymbol: true, keepMark: true},
	'T': {action: mistEvent{status: corebattle.StatusDeadlyPoison}},
	't': {action: trapEvent{base: 80}},
	'H': {action: remedyEvent{}, perSymbol: true},
	'2': {action: treasureEvent{}, perSymbol: true},
	'3': {action: treasureEvent{}, perSymbol: true},
	'4': {action: treasureEvent{}, perSymbol: true},
}

// nativeMap5 has round limit 60 and the menu threshold for dungeon 5 (job level 11).
// Each event loads one of stages 5–8 (int(rand(4) + 5)), as map3 does.
func nativeMap5() Dungeon {
	pools := stagePools(5, 8)
	marks := map[byte]string{'L': "★"}
	variants := variantFloors(Floor{FloorNumber: 1, Pools: pools, Marks: marks, events: nativeMap5Events}, nativeMap5Grids)
	return Dungeon{
		ID:             NativeMap5ID,
		Name:           "仕掛けの神殿",
		Description:    "レバー仕掛けの神殿。毒ガスと矢の罠を越えた先に主が潜む。",
		RoundLimit:     60,
		LeaderJobLevel: 11,
		Variants:       variants,
	}
}

// fullRestore is map5 event_H: every roster member, fallen included, returns
// to full HP and MP. Unknown maxima (older buffers) are left unchanged.
func fullRestore(exp *ActiveExpedition) (hp, mp map[string]int) {
	hp, mp = map[string]int{}, map[string]int{}
	for _, m := range exp.Members {
		if d := m.MaxHP - m.CurrentHP; d > 0 {
			hp[m.CharacterID] = d
		}
		if d := m.MaxMP - m.CurrentMP; d > 0 {
			mp[m.CharacterID] = d
		}
	}
	return hp, mp
}
