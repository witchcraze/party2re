package dungeon

import "strings"

// NativeMap1ID is the key-and-door maze (legacy map/1, seven initial variants).
const NativeMap1ID = "native-map-1"

// nativeMap1Grids transcribe legacy map/1/1.cgi–7.cgi; legacy I is H here.
// A locked door has one open neighbour, entered moving west in variants 1, 2
// and 5 and moving south in the others.
var nativeMap1Grids = [7][9]string{
	{"2B0D0101K", "111100010", "000101010", "010001000", "01H111110", "010000010", "0001S1000", "1H111111H", "100030000"},
	{"2B0D00000", "111110110", "100010110", "001000000", "011101H10", "00K1013H0", "011101110", "010000010", "0001S1000"},
	{"1K1000013", "10101101H", "000010000", "011110110", "000S00010", "01101111D", "011000110", "11001001B", "3H0111K12"},
	{"3100000H3", "H101D1011", "0001B1000", "011121110", "000111000", "0100S0011", "01010111K", "010100010", "K10001000"},
	{"3100S0000", "H00101110", "001100010", "011001010", "000011000", "K11000010", "11D011010", "2B011001H", "1111K0113"},
	{"0000K0000", "011111110", "010000000", "0001S111H", "011111000", "000000010", "0101D1000", "1101B101H", "3HK121013"},
	{"000030000", "011111110", "010000000", "0001S1111", "011111000", "000000010", "01D101000", "01011101H", "K10B21K13"},
}

// nativeMap1Pool is legacy stage 2, which map1 always loads, named as in the
// adventure catalog.
func nativeMap1Pool() EncounterPool {
	return EncounterPool{
		Monsters: []DungeonMonster{
			mon("nm1-01", "ゴースト", 20, 36, 20, 30, 7, 4), mon("nm1-02", "メイジゴースト", 26, 32, 26, 36, 12, 6),
			mon("nm1-03", "シャドー", 21, 20, 35, 44, 11, 5), mon("nm1-04", "ミイラ男", 50, 40, 6, 16, 10, 7),
			mon("nm1-05", "ガイコツ剣士", 45, 50, 15, 20, 12, 6), mon("nm1-06", "ナイトウィスプ", 25, 30, 25, 35, 9, 5),
			mon("nm1-07", "プチケルベロス", 41, 31, 8, 61, 8, 4), mon("nm1-08", "パンプキン", 65, 55, 20, 14, 16, 8),
			mon("nm1-09", "人食い箱", 120, 55, 10, 200, 30, 50),
		},
		Appears: []int{0, 0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5, 6, 6, 7, 8},
		Boss:    &DungeonMonster{ID: "nm1-boss", Name: "亡霊剣士", HP: 1000, Attack: 60, Defense: 20, Agility: 50, ExpReward: 160, GoldReward: 50},
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
