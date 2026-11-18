package dungeon

import "strings"

// NativeMap2ID is the four-floor tower (legacy map/2).
const NativeMap2ID = "native-map-2"

// nativeMap2Floors transcribe legacy map/2 in variant order: 1–3 are the
// initial floors (1.cgi–3.cgi), 4–7 the second floor (_1–_4), 8 the third
// (__1) and 9–11 the treasure vault (___1–___3). F leaves the floor and T is a
// pitfall back down; Q is a passage without encounters.
var nativeMap2Floors = [11][5]string{
	{"F70", "710", "000", "110", "S00"},
	{"F70", "710", "010", "010", "0S0"},
	{"F70", "110", "000", "011", "00S"},
	{"000", "010", "000", "01T", "08F"},
	{"000", "010", "000", "T10", "F80"},
	{"000", "110", "000", "01T", "08F"},
	{"000", "110", "000", "T10", "F80"},
	{"1F1", "0B0", "QTQ", "000", "000"},
	{"101", "000", "000", "T2T", "304"},
	{"101", "000", "000", "020", "3T4"},
	{"101", "000", "0T0", "020", "304"},
}

var nativeMap2FloorOf = [11]int{1, 1, 1, 2, 2, 2, 2, 3, 4, 4, 4}

// nativeStage3Pool is legacy stage 3 (uniform appearance) under the adventure
// catalog's names; its boss is the leading member of the legacy boss group.
func nativeStage3Pool() EncounterPool {
	return EncounterPool{
		Monsters: []DungeonMonster{
			mon("nm-s3-01", "ヒーラースライム", 50, 30, 15, 50, 14, 5), mon("nm-s3-02", "しびれクラゲ", 60, 36, 18, 35, 13, 6),
			mon("nm-s3-03", "シェルスライム", 40, 20, 80, 20, 16, 9), mon("nm-s3-04", "メイジバット", 54, 40, 20, 75, 14, 6),
			mon("nm-s3-05", "亀戦士", 48, 64, 70, 5, 13, 4), mon("nm-s3-06", "チビイエティ", 64, 46, 15, 80, 7, 3),
			mon("nm-s3-07", "イエティ", 150, 78, 25, 22, 20, 8),
		},
		Boss: &DungeonMonster{ID: "nm-s3-boss", Name: "キラーシェル", HP: 450, Attack: 90, Defense: 90, Agility: 30, ExpReward: 100, GoldReward: 150},
	}
}

// nativeStage4Pool is legacy stage 4 under the adventure catalog's names.
func nativeStage4Pool() EncounterPool {
	return EncounterPool{
		Monsters: []DungeonMonster{
			mon("nm-s4-01", "泥の魔手", 70, 85, 36, 50, 10, 8), mon("nm-s4-02", "おおさそり", 34, 76, 130, 45, 11, 9),
			mon("nm-s4-03", "ボム", 50, 50, 60, 50, 15, 10), mon("nm-s4-04", "まどうし", 55, 35, 30, 70, 16, 6),
			mon("nm-s4-05", "トゲサボテン", 100, 70, 50, 150, 20, 5), mon("nm-s4-06", "紅トゲサボテン", 100, 70, 50, 150, 20, 5),
			mon("nm-s4-07", "トゲボウズ", 64, 55, 125, 15, 23, 7), mon("nm-s4-08", "チビベリー", 50, 55, 15, 120, 15, 5),
			mon("nm-s4-09", "ベロロン", 120, 85, 40, 10, 28, 10), mon("nm-s4-10", "チビドラゴン", 150, 85, 35, 20, 24, 7),
		},
		Appears: []int{0, 0, 0, 1, 1, 1, 2, 2, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 9},
		Boss:    &DungeonMonster{ID: "nm-s4-boss", Name: "ゴーレム", HP: 1500, Attack: 100, Defense: 60, Agility: 50, ExpReward: 150, GoldReward: 100},
	}
}

// nativeMap2 has the menu threshold for dungeon 2 (job level 5). Each floor
// loads one of its stage files per request; floor bosses 7/8 always load
// stages 3/4, while the third floor's B uses that request's stage.
func nativeMap2() Dungeon {
	m3 := nativeMap3Pools()
	s2, s3, s4, s5, s6 := nativeMap1Pool(), nativeStage3Pool(), nativeStage4Pool(), m3[0], m3[1]
	up := map[byte]string{'F': "凸"}
	floors := [4]Floor{
		{FloorNumber: 1, Pools: []EncounterPool{s2, s3, s4}, Marks: up,
			Exits:  map[byte]FloorExit{'F': {Variants: []int{4, 5, 6}}},
			Bosses: map[byte]DungeonMonster{'7': *s3.Boss}},
		{FloorNumber: 2, RoundLimit: 40, Pools: []EncounterPool{s3, s4, s5}, Marks: up,
			Exits:  map[byte]FloorExit{'F': {Variants: []int{8}}, 'T': {Variants: []int{1, 2, 3}, Trap: 30}},
			Bosses: map[byte]DungeonMonster{'8': *s4.Boss}},
		{FloorNumber: 3, RoundLimit: 50, Pools: []EncounterPool{s4, s5, s6},
			Marks: map[byte]string{'F': "凸", cellBoss: "◎", 'Q': "　", 'T': "　"},
			Exits: map[byte]FloorExit{'F': {Variants: []int{9, 10, 11}}, 'T': {Variants: []int{4, 5, 6, 7}, Trap: 70}}},
		{FloorNumber: 4, RoundLimit: 50, Pools: []EncounterPool{s3, s4, s5, s6},
			Marks: map[byte]string{'2': "宝", '3': "宝", '4': "宝"},
			Exits: map[byte]FloorExit{'T': {Variants: []int{8}, Trap: 40}}},
	}
	variants := make([]Floor, len(nativeMap2Floors))
	for i, rows := range nativeMap2Floors {
		f := floors[nativeMap2FloorOf[i]-1]
		f.Width, f.Height, f.Grid = 3, 5, rows[:]
		for y, row := range rows {
			if x := strings.IndexByte(row, cellStart); x >= 0 {
				f.StartX, f.StartY = x, y
			}
		}
		variants[i] = f
	}
	return Dungeon{
		ID:             NativeMap2ID,
		Name:           "落穴の塔",
		Description:    "四層の塔。階段の先は選べず、落とし穴は下の階へ戻す。最上階は宝物庫。",
		RoundLimit:     30,
		LeaderJobLevel: 5,
		Variants:       variants,
		Initial:        3,
	}
}
