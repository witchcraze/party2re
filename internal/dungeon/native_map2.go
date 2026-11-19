package dungeon

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

// nativeMap2 has the menu threshold for dungeon 2 (job level 5). Each floor
// loads one of its stage files per request; floor bosses 7/8 always load
// stages 3/4, while the third floor's B uses that request's stage.
func nativeMap2() Dungeon {
	s2, s3, s4, s5, s6 := stagePool(2), stagePool(3), stagePool(4), stagePool(5), stagePool(6)
	up := map[byte]string{'F': "凸"}
	floors := [4]Floor{
		{FloorNumber: 1, Pools: []EncounterPool{s2, s3, s4}, Marks: up,
			Exits:  map[byte]FloorExit{'F': {Variants: []int{4, 5, 6}}},
			Bosses: map[byte]DungeonMonster{'7': *s3.Boss}},
		{FloorNumber: 2, RoundLimit: 40, Pools: []EncounterPool{s3, s4, s5}, Marks: up,
			Exits:  map[byte]FloorExit{'F': {Variants: []int{8}}, 'T': {Variants: []int{1, 2, 3}, Trap: 30}},
			Bosses: map[byte]DungeonMonster{'8': *s4.Boss}},
		{FloorNumber: 3, RoundLimit: 50, Pools: []EncounterPool{s4, s5, s6},
			Marks: map[byte]string{'F': "凸", 'B': "◎", 'Q': "　", 'T': "　"},
			Exits: map[byte]FloorExit{'F': {Variants: []int{9, 10, 11}}, 'T': {Variants: []int{4, 5, 6, 7}, Trap: 70}}},
		{FloorNumber: 4, RoundLimit: 50, Pools: []EncounterPool{s3, s4, s5, s6},
			Marks: map[byte]string{'2': "宝", '3': "宝", '4': "宝"},
			Exits: map[byte]FloorExit{'T': {Variants: []int{8}, Trap: 40}}},
	}
	// Flags carry across floors, so the repeated 7 and the vault treasures key by symbol.
	events := map[byte]mapEvent{
		'7': {action: bossEvent{}, perSymbol: true}, '8': {action: bossEvent{}, perSymbol: true},
		'2': {action: treasureEvent{}, perSymbol: true}, '3': {action: treasureEvent{}, perSymbol: true}, '4': {action: treasureEvent{}, perSymbol: true},
	}
	variants := make([]Floor, len(nativeMap2Floors))
	for i, rows := range nativeMap2Floors {
		f := floors[nativeMap2FloorOf[i]-1]
		f.events = events
		variants[i] = gridFloor(f, rows[:])
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
