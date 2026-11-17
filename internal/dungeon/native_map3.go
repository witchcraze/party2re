package dungeon

import "strings"

// NativeMap3ID is the warp maze (legacy map/3, eight initial variants).
const NativeMap3ID = "native-map-3"

// nativeMap3Grids are independently authored 9×9 layouts. Lowercase letters
// are warps resolved through nativeMap3Warps; M confuses the party before an
// encounter. Legacy variants have no hidden passage: event_I in map/3/5–8.cgi
// has no cell.
var nativeMap3Grids = [8][9]string{
	{"S00a10C00", "11010110B", "M0001000M", "0110b1011", "0001000c0", "1101011d1", "0e0001000", "0110f1B10", "C00000001"},
	{"0000C0000", "01110111a", "0b0M00010", "01011110B", "000cSd000", "B1011110M", "0f00e0010", "01110C110", "000000000"},
	{"B0001000C", "010a0010M", "0001b1000", "11000e011", "M0c0S0d0B", "110f00011", "000101000", "0101C1010", "000000000"},
	{"000C10000", "0110101a0", "0M00b00B0", "01110110c", "d00S10000", "0110e1101", "0B0001M00", "01101f110", "C00000000"},
	{"C00001B00", "0110001M0", "00a1b1000", "1101c1011", "000dSe000", "1101f1011", "0001010C0", "0M1000110", "B00001000"},
	{"000010M00", "0B1010110", "0000c000C", "0111a1110", "00bdSf000", "0111e1110", "C00000000", "0110101B0", "M00010000"},
	{"M0000000B", "0101a1010", "000101000", "110b0c011", "C00dSe00C", "110f00011", "000000000", "0101M1010", "B00010000"},
	{"0B000000M", "010110100", "00C0a00b0", "0111c1110", "000dSe000", "0111f1110", "0M0000C00", "0101101B0", "000000000"},
}

// nativeMap3Warps gives each variant's (x, y) destinations. Some land on their
// own cell, on another warp, or on an event cell, which then stays untriggered.
var nativeMap3Warps = [8]map[byte][2]int{
	{'a': {8, 4}, 'b': {0, 8}, 'c': {7, 4}, 'd': {4, 4}, 'e': {6, 2}, 'f': {7, 1}},
	{'a': {4, 0}, 'b': {5, 4}, 'c': {3, 4}, 'd': {8, 6}, 'e': {4, 4}, 'f': {0, 5}},
	{'a': {8, 0}, 'b': {4, 7}, 'c': {2, 4}, 'd': {0, 4}, 'e': {3, 1}, 'f': {8, 4}},
	{'a': {2, 2}, 'b': {4, 2}, 'c': {1, 6}, 'd': {8, 8}, 'e': {3, 4}, 'f': {3, 0}},
	{'a': {6, 0}, 'b': {4, 8}, 'c': {4, 6}, 'd': {5, 4}, 'e': {3, 4}, 'f': {4, 2}},
	{'a': {4, 5}, 'b': {0, 0}, 'c': {2, 4}, 'd': {5, 4}, 'e': {4, 3}, 'f': {1, 1}},
	{'a': {0, 8}, 'b': {5, 3}, 'c': {3, 3}, 'd': {3, 4}, 'e': {0, 0}, 'f': {8, 4}},
	{'a': {4, 6}, 'b': {1, 0}, 'c': {4, 4}, 'd': {8, 8}, 'e': {0, 0}, 'f': {7, 7}},
}

func mon(id, name string, hp, at, df, ag, exp, gold int) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold}
}

// nativeMap3Pools mirror the stat magnitudes and appearance weights of the four
// legacy stage files the map draws from (map/3/*.cgi: int(rand(4)+5)) under
// generic names. Each boss is the leading member of its legacy boss group.
func nativeMap3Pools() []EncounterPool {
	return []EncounterPool{
		{
			Monsters: []DungeonMonster{
				mon("nm3-a-01", "見習い術師", 55, 45, 40, 70, 18, 6), mon("nm3-a-02", "森の術師", 67, 40, 45, 76, 20, 9),
				mon("nm3-a-03", "魔力の粘体", 70, 35, 30, 100, 19, 8), mon("nm3-a-04", "悪魔の卵", 66, 66, 66, 66, 11, 6),
				mon("nm3-a-05", "小悪魔", 124, 64, 35, 110, 15, 6), mon("nm3-a-06", "大目玉", 90, 78, 55, 60, 21, 8),
				mon("nm3-a-07", "浮遊眼", 100, 65, 50, 64, 20, 11), mon("nm3-a-08", "大吸血蝙蝠", 180, 98, 10, 180, 40, 8),
				mon("nm3-a-09", "擬態箱", 300, 100, 30, 300, 50, 100), mon("nm3-a-10", "鋼の粘体", 8, 40, 1500, 1500, 250, 10),
			},
			Appears: []int{0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5, 6, 6, 6, 7, 8, 9},
			Boss:    &DungeonMonster{ID: "nm3-a-boss", Name: "闇の大術師", HP: 1400, Attack: 70, Defense: 20, Agility: 100, ExpReward: 180, GoldReward: 120},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-b-01", "魔茸", 80, 65, 65, 80, 23, 18), mon("nm3-b-02", "王蛇", 100, 85, 35, 60, 21, 8),
				mon("nm3-b-03", "赤眼", 80, 50, 30, 40, 24, 10), mon("nm3-b-04", "黒眼", 70, 60, 30, 50, 28, 11),
				mon("nm3-b-05", "斧の荒くれ", 160, 120, 50, 35, 27, 14), mon("nm3-b-06", "子豹", 90, 75, 25, 125, 18, 12),
				mon("nm3-b-07", "殺し豹", 130, 105, 30, 74, 30, 10), mon("nm3-b-08", "長虫", 75, 96, 20, 80, 28, 26),
				mon("nm3-b-09", "毒花", 95, 86, 27, 40, 30, 26), mon("nm3-b-10", "毛玉獣", 100, 60, 40, 70, 15, 100),
				mon("nm3-b-11", "死の踊り手", 80, 90, 80, 60, 32, 16), mon("nm3-b-12", "雷羊", 150, 70, 50, 20, 50, 50),
			},
			Appears: []int{0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11},
			Boss:    &DungeonMonster{ID: "nm3-b-boss", Name: "一つ目の巨人", HP: 2500, Attack: 200, Defense: 20, Agility: 20, ExpReward: 200, GoldReward: 5},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-c-01", "火の粘体", 100, 77, 58, 82, 25, 12), mon("nm3-c-02", "溶岩の粘体", 130, 93, 70, 30, 30, 27),
				mon("nm3-c-03", "爆ぜ岩", 100, 86, 40, 10, 50, 15), mon("nm3-c-04", "棘玉", 111, 111, 75, 20, 34, 25),
				mon("nm3-c-05", "火の玉", 70, 60, 80, 60, 35, 20), mon("nm3-c-06", "子竜", 120, 78, 65, 66, 30, 12),
				mon("nm3-c-07", "黒竜", 110, 90, 100, 50, 32, 20), mon("nm3-c-08", "竜", 200, 140, 75, 68, 60, 24),
				mon("nm3-c-09", "竜の卵", 80, 70, 80, 100, 20, 14), mon("nm3-c-10", "小牛鬼", 140, 112, 50, 90, 26, 18),
				mon("nm3-c-11", "炎の剣士", 110, 152, 30, 60, 28, 10),
			},
			Boss: &DungeonMonster{ID: "nm3-c-boss", Name: "火喰い鳥", HP: 1800, Attack: 160, Defense: 70, Agility: 80, ExpReward: 300, GoldReward: 180},
		},
		{
			Monsters: []DungeonMonster{
				mon("nm3-d-01", "闘技の剣士", 100, 80, 60, 60, 30, 15), mon("nm3-d-02", "闘技の戦士", 120, 90, 70, 10, 30, 15),
				mon("nm3-d-03", "闘技の術師", 80, 50, 30, 80, 30, 15), mon("nm3-d-04", "闘技の僧侶", 90, 60, 40, 60, 30, 15),
				mon("nm3-d-05", "爆ぜ岩", 100, 86, 40, 10, 40, 5), mon("nm3-d-06", "身代わり岩", 110, 75, 50, 44, 36, 10),
				mon("nm3-d-07", "写し身", 133, 99, 33, 99, 33, 33), mon("nm3-d-08", "悪戯妖精", 140, 100, 40, 120, 27, 20),
				mon("nm3-d-09", "博打妖精", 104, 120, 40, 130, 28, 22), mon("nm3-d-10", "魔兎", 104, 120, 40, 130, 28, 22),
			},
			Appears: []int{0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 5, 6, 6, 7, 8, 9},
			Boss:    &DungeonMonster{ID: "nm3-d-boss", Name: "大角獣", HP: 2800, Attack: 185, Defense: 125, Agility: 145, ExpReward: 350, GoldReward: 200},
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
