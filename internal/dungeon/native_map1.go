package dungeon

import "strings"

// NativeMap1ID is the key-and-door maze (legacy map/1, seven initial variants).
const NativeMap1ID = "native-map-1"

// nativeMap1Grids are independently authored 9×9 layouts. The door seals the
// boss and treasure 2; a locked door has one open neighbour, entered moving
// west in variants 1, 2 and 5 and moving south in the others, like legacy.
var nativeMap1Grids = [7][9]string{
	{"3000K0000", "1H1101110", "000001000", "011101011", "0001S0000", "110101110", "000100010", "111100010", "2B0D00010"},
	{"0000K0003", "0111110H1", "000000000", "111101101", "B20D0S001", "111101100", "00000H000", "010111010", "0K0000010"},
	{"3H0000000", "010110110", "0000S0000", "011010101", "000010000", "1K10001D1", "000000101", "0H11001B1", "30000H121"},
	{"300000H03", "011101110", "000100000", "010S01110", "010000010", "011100010", "0001D1100", "K1H1B1000", "0001210K0"},
	{"111000K00", "2BD000110", "111H10000", "000010110", "01110S010", "010000010", "0101110H0", "000100010", "3K0100003"},
	{"3000000H0", "011110100", "0000S01D1", "0110101B1", "010000121", "01111H010", "000000000", "1H1101110", "3K0000K00"},
	{"000030000", "011101110", "000S00010", "1011010H0", "000001000", "1D1100011", "101K00100", "1B1011H10", "1210000K3"},
}

// nativeMap1Pool mirrors the stats and appearance weights of legacy stage 2,
// which map1 always loads, under generic names.
func nativeMap1Pool() EncounterPool {
	return EncounterPool{
		Monsters: []DungeonMonster{
			mon("nm1-01", "亡霊", 20, 36, 20, 30, 7, 4), mon("nm1-02", "術師の亡霊", 26, 32, 26, 36, 12, 6),
			mon("nm1-03", "影", 21, 20, 35, 44, 11, 5), mon("nm1-04", "包帯男", 50, 40, 6, 16, 10, 7),
			mon("nm1-05", "骸骨剣士", 45, 50, 15, 20, 12, 6), mon("nm1-06", "夜の鬼火", 25, 30, 25, 35, 9, 5),
			mon("nm1-07", "小さな番犬", 41, 31, 8, 61, 8, 4), mon("nm1-08", "南瓜頭", 65, 55, 20, 14, 16, 8),
			mon("nm1-09", "擬態箱", 120, 55, 10, 200, 30, 50),
		},
		Appears: []int{0, 0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5, 6, 6, 7, 8},
		Boss:    &DungeonMonster{ID: "nm1-boss", Name: "亡者の剣士", HP: 1000, Attack: 60, Defense: 20, Agility: 50, ExpReward: 160, GoldReward: 50},
	}
}

// nativeMap1 has round limit 50 and the menu threshold for dungeon 1 (job level 3).
func nativeMap1() Dungeon {
	pools := []EncounterPool{nativeMap1Pool()}
	variants := make([]Floor, len(nativeMap1Grids))
	for i, rows := range nativeMap1Grids {
		f := Floor{FloorNumber: 1, Width: 9, Height: 9, Grid: rows[:], Pools: pools}
		for y, row := range rows {
			if x := strings.IndexByte(row, cellStart); x >= 0 {
				f.StartX, f.StartY = x, y
			}
		}
		variants[i] = f
	}
	return Dungeon{
		ID:             NativeMap1ID,
		Name:           "封じの地下墓所",
		Description:    "鍵のかかった扉の奥に主が眠る地下墓所。隠し通路もある。",
		RoundLimit:     50,
		LeaderJobLevel: 3,
		Variants:       variants,
	}
}
