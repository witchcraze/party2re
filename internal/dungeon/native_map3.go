package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap3ID is the warp maze (legacy map/3, eight initial variants).
const NativeMap3ID = "native-map-3"

// nativeMap3Grids transcribes map/3/1.cgi–8.cgi. Warp letters A, C, D, E,
// F and G use lowercase aliases, and T uses the native mist cell M.
var nativeMap3Grids = [][]string{
	{"f0000000d", "0101c1010", "003101b00", "011101110", "0a00S0000", "011101110", "0Me101200", "010101M10", "B0000000g"},
	{"b0000000f", "010101010", "00a1013M0", "011101110", "0000S00d0", "011101110", "0M2101B00", "0101e1010", "c0000000g"},
	{"a00000003", "01M101010", "00b101f00", "011101110", "0000S00d0", "011101110", "00c101BM0", "0101e1010", "20000000g"},
	{"a0000000f", "010101010", "00b1013M0", "011101110", "0000S00d0", "011101110", "002101g00", "0101e1M10", "c0000000B"},
	{"M00000000", "010101010", "0031c1B00", "011101110", "00a0S0d00", "011101110", "00b1e12M0", "010101010", "f0000000g"},
	{"f0000000g", "010101010", "0Mb1c1200", "011101110", "00a0S0d00", "011101110", "0031e1B00", "010101010", "00000000M"},
	{"f00000002", "010101M10", "00b1c1g00", "011101110", "00a0S0d00", "011101110", "0031e1000", "010101010", "M0000000B"},
	{"b0000000g", "01M101010", "00f1c1200", "011101110", "00a0S0d00", "011101110", "0031e1B00", "010101010", "00000000M"},
}

// nativeMap3Warps gives each variant's (x, y) destinations. Some land on their
// own cell, on another warp, or on an event cell, which then stays untriggered.
var nativeMap3Warps = [8]map[byte][2]int{
	{'a': {5, 4}, 'c': {4, 5}, 'd': {2, 6}, 'e': {8, 0}, 'f': {6, 6}, 'g': {2, 2}},
	{'a': {8, 8}, 'c': {6, 2}, 'd': {3, 4}, 'e': {4, 3}, 'f': {2, 6}, 'g': {2, 2}},
	{'a': {6, 6}, 'c': {8, 0}, 'd': {3, 4}, 'e': {4, 3}, 'f': {0, 8}, 'g': {2, 2}},
	{'a': {6, 6}, 'c': {6, 2}, 'd': {3, 4}, 'e': {4, 3}, 'f': {2, 6}, 'g': {0, 0}},
	{'a': {6, 4}, 'c': {4, 2}, 'd': {2, 4}, 'e': {4, 6}, 'f': {6, 2}, 'g': {2, 2}},
	{'a': {6, 4}, 'c': {4, 6}, 'd': {2, 4}, 'e': {4, 2}, 'f': {6, 6}, 'g': {2, 6}},
	{'a': {6, 4}, 'c': {4, 6}, 'd': {2, 4}, 'e': {4, 2}, 'f': {6, 6}, 'g': {0, 8}},
	{'a': {6, 4}, 'c': {4, 6}, 'd': {2, 4}, 'e': {4, 2}, 'f': {8, 8}, 'g': {2, 6}},
}

// b is a boss independent of B; M (legacy T) confuses on every entry.
var nativeMap3Events = map[byte]mapEvent{
	'b': {action: bossEvent{}},
	'M': {action: mistEvent{status: corebattle.StatusConfusion}},
	'2': {action: treasureEvent{}, perSymbol: true},
	'3': {action: treasureEvent{}, perSymbol: true},
}

// nativeMap3 has round limit 60 and the menu threshold for dungeon 3 (job level 7).
func nativeMap3() Dungeon {
	pools := stagePools(5, 8)
	variants := make([]Floor, len(nativeMap3Grids))
	for i, rows := range nativeMap3Grids {
		variants[i] = gridFloor(Floor{Warps: nativeMap3Warps[i], Pools: pools, events: nativeMap3Events}, rows)
	}
	return Dungeon{
		ID:             NativeMap3ID,
		Name:           "惑いの樹海",
		Description:    "転移の罠と幻の霧が旅人を惑わせる樹海。主が二体潜む。",
		RoundLimit:     60,
		LeaderJobLevel: 7,
		Variants:       variants,
	}
}
