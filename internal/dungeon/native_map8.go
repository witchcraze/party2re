package dungeon

// NativeMap8ID is the warping desert (legacy map/8, twelve initial variants).
const NativeMap8ID = "native-map-8"

// nativeMap8Grids transcribe map/8/1.cgi–12.cgi with their legacy symbols.
// They contain no 1: every wall is the map edge.
var nativeMap8Grids = [][]string{
	{"2A00D0000A3", "A00000000D0", "0000000000B", "000W0000000", "0000000000C", "0W000SW0000", "B0000000000", "00000000000", "00000W00W00", "0C000000040", "D03000C000B"},
	{"0200000B300", "A000D000D00", "00000000000", "00000W0000B", "00W00000000", "00000SW000C", "00000000W00", "000000W0000", "00000000004", "B00W00000B0", "0C0000A00C0", "30C00020000"},
	{"2A00D0000A3", "A000000W0D0", "0000000000B", "0000W000000", "0000000000C", "00000000000", "B00W0S00000", "00000W00000", "00000000000", "0C000000000", "D03000000B0", "00000C00040"},
	{"0000030000A3", "000B000000D0", "0A000000000B", "000000000000", "00000W00000C", "00W00S000000", "B00000W00000", "000000000000", "00000W000000", "00C000000WB0", "0030D0C00040"},
	{"00000D00003A", "000300000000", "000000000000", "00A000000000", "00000W0000W4", "00000S000000", "200W00000000", "000000000000", "0W0000W00000", "002000000040", "00A0D0C000B0"},
	{"03000000000", "00000002000", "D00000W0B00", "0000W000000", "00000000000", "000WS0000W0", "00000000000", "0W000000000", "00000000004", "0C000003000", "000000D000A"},
	{"000000000A0", "0B000000004", "0000W000000", "00000000000", "3W000000000", "00000S0W00C", "00000000000", "00000000000", "00000W00000", "0W0000000D0", "A000C000002"},
	{"0000000030", "020000000D", "0000W00000", "0000000000", "A000000000", "00000000W2", "000W0S0000", "0000000000", "00000W0000", "00W0000000", "0000B00C0D", "0300000040"},
	{"4000000002", "0A00000000", "0000000W00", "0000W00000", "D000S00000", "000000W000", "00W000000C", "0000000000", "0000000000", "0000W00000", "B000000000", "0004000003"},
	{"400000000A", "B000000000", "0000000000", "000W000000", "0000000000", "0000000000", "0000SW0003", "00W0000000", "0000000000", "0000W00000", "200000W000", "0C0000400C"},
	{"C00000000B0", "0000000000A", "00000000W02", "000W0000000", "0000S0W0000", "0A0W0000000", "00000000000", "00000000000", "00W000000W0", "030000W0040", "00A00000D00"},
	{"20000000000", "0C000W0002A", "D0000000W00", "00000000000", "00000000000", "0A00WS00000", "00000W00000", "0000000W000", "W0A00000000", "000000000D0", "30000B00400"},
}

// A wall hit and W both scatter the party; the map edge displays as passage
// ($map_imgs{1}) and no symbol has a mark, so the view shows only passage.
var nativeMap8Events = map[byte]mapEvent{
	cellWall: {action: scatterEvent{}},
	'W':      {action: scatterEvent{}},
	'A':      {action: bossEvent{}, perSymbol: true},
	'B':      {action: bossEvent{}, perSymbol: true},
	'C':      {action: bossEvent{}, perSymbol: true},
	'D':      {action: bossEvent{}, perSymbol: true},
	'2':      {action: treasureEvent{}, perSymbol: true},
	'3':      {action: treasureEvent{}, perSymbol: true},
	'4':      {action: treasureEvent{}, perSymbol: true},
}

type scatterEvent struct{}

func (ev scatterEvent) apply(n *nativeStep) error {
	// The landing cell is not dispatched.
	n.step.NewX, n.step.NewY = ev.landing(n.rng)
	return nil
}

// event_W/event_1 draw y, then x, each int(rand(5) + 3), inside every map8 grid.
func (scatterEvent) landing(rng RandomSource) (x, y int) {
	y = 3 + rng.Intn(5)
	return 3 + rng.Intn(5), y
}

// nativeMap8 has round limit 50 and the menu threshold for dungeon 8 (job level 17).
// Each event loads one of stages 10–14 (int(rand(5) + 10)).
func nativeMap8() Dungeon {
	marks := map[byte]string{cellWall: "□"}
	variants := variantFloors(Floor{FloorNumber: 1, Pools: stagePools(10, 14), Marks: marks, events: nativeMap8Events}, nativeMap8Grids)
	return Dungeon{
		ID:             NativeMap8ID,
		Name:           "惑わしの砂漠",
		Description:    "どこまでも同じ景色が続く砂漠。果てに触れると、どこかへ運ばれる。",
		RoundLimit:     50,
		LeaderJobLevel: 17,
		Variants:       variants,
	}
}
