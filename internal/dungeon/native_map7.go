package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap7ID is the rock switch cave (legacy map/7, six initial variants).
const NativeMap7ID = "native-map-7"

// nativeMap7Grids transcribe map/7/1.cgi–6.cgi with their legacy symbols.
// D has one open neighbour, so a denied entry returns to the cell it came from.
var nativeMap7Grids = [][]string{
	{"01213", "T0B1A", "10110", "00000", "0101T", "01S10", "01011", "00001", "D1101", "01t0b", "4101K"},
	{"21014", "B0T10", "1011D", "00000", "0101T", "01S10", "01011", "00001", "01101", "A1t0b", "3101K"},
	{"01014", "t0T10", "1011D", "00000", "0101B", "01S12", "01011", "00001", "01101", "A1b0t", "31K10"},
	{"21014", "B0T10", "1011D", "00000", "0101t", "01S10", "01011", "00001", "01101", "b1A0t", "K1310"},
	{"31210", "A0B1T", "10110", "00000", "0101t", "01S10", "01011", "00001", "D1101", "01b0T", "41K10"},
	{"K1012", "b0t1B", "10110", "00000", "0101t", "01S10", "01011", "00001", "D1101", "01T0A", "41013"},
}

// The switch K and the rock D share one flag: either one opens the rock.
func nativeMap7Events(trapBase int) map[byte]cellEvent {
	return map[byte]cellEvent{
		'K': {kind: eventKey, flag: "DK"},
		'D': {kind: eventDoor, flag: "DK", jobs: []string{"job-04", "job-25"}},
		'T': {kind: eventMist, status: corebattle.StatusParalyze},
		't': {kind: eventTrap, base: trapBase},
		'A': {kind: eventBoss, perSymbol: true},
		'b': {kind: eventBoss, perSymbol: true},
		'2': {kind: eventTreasure, perSymbol: true},
		'3': {kind: eventTreasure, perSymbol: true},
		'4': {kind: eventTreasure, perSymbol: true},
	}
}

// nm7FixedBoss is _data.cgi's fixed group, which replaces the stage bosses
// with rand(3) < 1 on every event.
var nm7FixedBoss = strongest([]DungeonMonster{
	{ID: "nm7-ifrit", Name: "イフリート", HP: 7000, Attack: 360, Defense: 140, Agility: 300, ExpReward: 1500, GoldReward: 300},
	{ID: "nm7-red-stone", Name: "レッドストーン", HP: 10, Attack: 200, Defense: 6000, Agility: 1000, ExpReward: 70, GoldReward: 500},
})

// nativeMap7Pools draws stages 8–11 (int(rand(4) + 8)) and the fixed boss with
// odds 1/3: each stage appears twice as is and once with the fixed boss.
func nativeMap7Pools() []EncounterPool {
	var pools []EncounterPool
	for _, p := range stagePools(8, 11) {
		fixed := p
		fixed.Boss = &nm7FixedBoss
		pools = append(pools, p, p, fixed)
	}
	return pools
}

// nativeMap7 has round limit 60 and the menu threshold for dungeon 7 (job level 15).
func nativeMap7() Dungeon {
	pools := nativeMap7Pools()
	variants := make([]Floor, len(nativeMap7Grids))
	for i, rows := range nativeMap7Grids {
		trapBase := 150
		if i == 2 {
			trapBase = 140 // map/7/3.cgi event_t
		}
		marks := map[byte]string{'K': "★", 'D': "岩"}
		if i == 5 {
			delete(marks, 'K') // map/7/6.cgi defines no switch mark
		}
		variants[i] = gridFloor(Floor{FloorNumber: 1, Pools: pools, Marks: marks, events: nativeMap7Events(trapBase)}, rows)
	}
	return Dungeon{
		ID:             NativeMap7ID,
		Name:           "炎の岩窟",
		Description:    "大岩に塞がれた灼熱の洞窟。仕掛けを踏むか、拳で岩を砕いて奥へ進む。",
		RoundLimit:     60,
		LeaderJobLevel: 15,
		Variants:       variants,
	}
}
