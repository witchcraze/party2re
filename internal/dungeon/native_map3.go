package dungeon

import "strings"

// NativeMap3ID is the warp maze (legacy map/3, eight initial variants).
const NativeMap3ID = "native-map-3"

// nativeMap3Grids transcribes map/3/1.cgi–8.cgi. Warp letters A, C, D, E,
// F and G use lowercase aliases, and T uses the native mist cell M.
var nativeMap3Grids = [8][9]string{
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

func mon(id, name string, hp, at, df, ag, exp, gold int) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold}
}

// nativeMap3Pools mirror the stat magnitudes and appearance weights of the four
// legacy stage files from map/3/*.cgi. Names follow the Adventure catalog
// apart from the two approved generic names (#2373).
func nativeMap3Pools() []EncounterPool {
	return []EncounterPool{
		{
			Monsters: []DungeonMonster{
				mon("nm3-a-01", "まどうし", 55, 45, 40, 70, 18, 6), mon("nm3-a-02", "魔法使い", 67, 40, 45, 76, 20, 9),
				mon("nm3-a-03", "メイジスライム", 70, 35, 30, 100, 19, 8), mon("nm3-a-04", "デビルエッグ", 66, 66, 66, 66, 11, 6),
				mon("nm3-a-05", "小悪魔インプ", 124, 64, 35, 110, 15, 6), mon("nm3-a-06", "巨大眼球", 90, 78, 55, 60, 21, 8),
				mon("nm3-a-07", "浮遊眼", 100, 65, 50, 64, 20, 11), mon("nm3-a-08", "ジャイアントバット", 180, 98, 10, 180, 40, 8),
				mon("nm3-a-09", "ミミック", 300, 100, 30, 300, 50, 100), mon("nm3-a-10", "銀滴スライム", 8, 40, 1500, 1500, 250, 10),
			},
			Appears: []int{0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5, 6, 6, 6, 7, 8, 9},
			Boss:    &DungeonMonster{ID: "nm3-a-boss", Name: "闇の魔術士", HP: 1400, Attack: 70, Defense: 20, Agility: 100, ExpReward: 180, GoldReward: 120},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-b-01", "メイジマッシュ", 80, 65, 65, 80, 23, 18), mon("nm3-b-02", "キングコブラ", 100, 85, 35, 60, 21, 8),
				mon("nm3-b-03", "レッドアイ", 80, 50, 30, 40, 24, 10), mon("nm3-b-04", "ダークアイ", 70, 60, 30, 50, 28, 11),
				mon("nm3-b-05", "ビーストマスター", 160, 120, 50, 35, 27, 14), mon("nm3-b-06", "ベビーレオ", 90, 75, 25, 125, 18, 12),
				mon("nm3-b-07", "シャドウパンサー", 130, 105, 30, 74, 30, 10), mon("nm3-b-08", "ニョロロ", 75, 96, 20, 80, 28, 26),
				mon("nm3-b-09", "ラフレシア", 95, 86, 27, 40, 30, 26), mon("nm3-b-10", "モサモサ", 100, 60, 40, 70, 15, 100),
				mon("nm3-b-11", "デスローパー", 80, 90, 80, 60, 32, 16), mon("nm3-b-12", "サンダーシープ", 150, 70, 50, 20, 50, 50),
			},
			Appears: []int{0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11},
			Boss:    &DungeonMonster{ID: "nm3-b-boss", Name: "一つ目の巨人", HP: 2500, Attack: 200, Defense: 20, Agility: 20, ExpReward: 200, GoldReward: 5},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-c-01", "ファイアースライム", 100, 77, 58, 82, 25, 12), mon("nm3-c-02", "マグマスライム", 130, 93, 70, 30, 30, 27),
				mon("nm3-c-03", "爆弾岩", 100, 86, 40, 10, 50, 15), mon("nm3-c-04", "チクリ", 111, 111, 75, 20, 34, 25),
				mon("nm3-c-05", "ボム", 70, 60, 80, 60, 35, 20), mon("nm3-c-06", "チビドラゴン", 120, 78, 65, 66, 30, 12),
				mon("nm3-c-07", "ブラックドラコ", 110, 90, 100, 50, 32, 20), mon("nm3-c-08", "ドラゴン", 200, 140, 75, 68, 60, 24),
				mon("nm3-c-09", "チビエッグ", 80, 70, 80, 100, 20, 14), mon("nm3-c-10", "ミニタウルス", 140, 112, 50, 90, 26, 18),
				mon("nm3-c-11", "炎の戦士", 110, 152, 30, 60, 28, 10),
			},
			Boss: &DungeonMonster{ID: "nm3-c-boss", Name: "火炎鳥", HP: 1800, Attack: 160, Defense: 70, Agility: 80, ExpReward: 300, GoldReward: 180},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-d-01", "コボルト勇者", 100, 80, 60, 60, 30, 15), mon("nm3-d-02", "コボルト戦士", 120, 90, 70, 10, 30, 15),
				mon("nm3-d-03", "コボルト魔道士", 80, 50, 30, 80, 30, 15), mon("nm3-d-04", "コボルト僧侶", 90, 60, 40, 60, 30, 15),
				mon("nm3-d-05", "爆弾岩", 100, 86, 40, 10, 40, 5), mon("nm3-d-06", "守護の岩石", 110, 75, 50, 44, 36, 10),
				mon("nm3-d-07", "ものまね妖魔", 133, 99, 33, 99, 33, 33), mon("nm3-d-08", "イタズラ妖精", 140, 100, 40, 120, 27, 20),
				mon("nm3-d-09", "ギャンブル妖精", 104, 120, 40, 130, 28, 22), mon("nm3-d-10", "マウサギ", 104, 120, 40, 130, 28, 22),
			},
			Appears: []int{0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 5, 6, 6, 7, 8, 9},
			Boss:    &DungeonMonster{ID: "nm3-d-boss", Name: "ベヒーモス", HP: 2800, Attack: 185, Defense: 125, Agility: 145, ExpReward: 350, GoldReward: 200},
		},
	}
}

// nativeMap3 has round limit 60 and the menu threshold for dungeon 3 (job level 7).
func nativeMap3() Dungeon {
	pools := nativeMap3Pools()
	variants := make([]Floor, len(nativeMap3Grids))
	for i, rows := range nativeMap3Grids {
		f := Floor{FloorNumber: 1, Width: 9, Height: 9, Grid: rows[:], Warps: nativeMap3Warps[i], Pools: pools}
		for y, row := range rows {
			if x := strings.IndexByte(row, cellStart); x >= 0 {
				f.StartX, f.StartY = x, y
			}
		}
		variants[i] = f
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
