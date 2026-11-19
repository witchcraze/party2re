package dungeon

// NativeMap1ID is the key-and-door maze (legacy map/1, seven initial variants).
const NativeMap1ID = "native-map-1"

// nativeMap1Grids transcribe legacy map/1/1.cgi–7.cgi; legacy I is H here.
// A locked door has one open neighbour, entered moving west in variants 1, 2
// and 5 and moving south in the others.
var nativeMap1Grids = [][]string{
	{"2B0D0101K", "111100010", "000101010", "010001000", "01H111110", "010000010", "0001S1000", "1H111111H", "100030000"},
	{"2B0D00000", "111110110", "100010110", "001000000", "011101H10", "00K1013H0", "011101110", "010000010", "0001S1000"},
	{"1K1000013", "10101101H", "000010000", "011110110", "000S00010", "01101111D", "011000110", "11001001B", "3H0111K12"},
	{"3100000H3", "H101D1011", "0001B1000", "011121110", "000111000", "0100S0011", "01010111K", "010100010", "K10001000"},
	{"3100S0000", "H00101110", "001100010", "011001010", "000011000", "K11000010", "11D011010", "2B011001H", "1111K0113"},
	{"0000K0000", "011111110", "010000000", "0001S111H", "011111000", "000000010", "0101D1000", "1101B101H", "3HK121013"},
	{"000030000", "011111110", "010000000", "0001S1111", "011111000", "000000010", "01D101000", "01011101H", "K10B21K13"},
}

// Variants repeat K and 3 under one legacy flag, so they key by symbol.
var nativeMap1Events = map[byte]cellEvent{
	'H': {kind: eventHidden},
	'K': {kind: eventKey, perSymbol: true},
	'D': {kind: eventDoor, perSymbol: true, key: 'K'},
	'2': {kind: eventTreasure, perSymbol: true},
	'3': {kind: eventTreasure, perSymbol: true},
}

// nativeMap1 has round limit 50 and the menu threshold for dungeon 1 (job level 3).
func nativeMap1() Dungeon {
	pools := []EncounterPool{stagePool(2)}
	variants := variantFloors(Floor{FloorNumber: 1, Pools: pools, Marks: map[byte]string{'K': "★", 'D': "扉"}, events: nativeMap1Events}, nativeMap1Grids)
	return Dungeon{
		ID:             NativeMap1ID,
		Name:           "封じの地下墓所",
		Description:    "鍵のかかった扉の奥に主が眠る地下墓所。隠し通路もある。",
		RoundLimit:     50,
		LeaderJobLevel: 3,
		Variants:       variants,
	}
}
