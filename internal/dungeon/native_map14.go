package dungeon

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NativeMap14ID is the sunken routes (legacy map/14).
const NativeMap14ID = "native-map-14"

// nativeMap14Floors transcribe map/14/1.cgi, _P.cgi, _K.cgi and _T.cgi; the
// variant number is the legacy destination type + 1.
var nativeMap14Floors = [4][]string{
	{"11010001101", "0B01D000000", "10011100001", "01000000001", "011010100E0", "00010S00110", "00110000100", "00000101100", "0C010000A00", "00000110000"},
	{"11111101111", "0010000P9R1", "10100101ab1", "10110001111", "10000101001", "11110111011", "00000000010", "00111911010", "00100001010", "00100R01010"},
	{"111111R1111", "0000019t111", "11011110u11", "0001010v110", "001R0001100", "00091K10101", "11000000000", "11111011110", "00111011000", "00000011000"},
	{"11f011011R1", "001000100j0", "1010010k011", "i000h0000R0", "10100101000", "1R00010o011", "01n10e0000d", "0001l010110", "00000010000", "c01110R101g"},
}

// map14Points are the main floor's points 4–8 in _allot_point's list order.
const map14Points = "ABCDE"

const (
	map14ShipVariant = 4
	// _allot_tre omits k, which is always eligible.
	map14DisablableSites = "cdefghijlno"
)

// routeState holds the active points, their assigned destinations and the
// treasure sites disabled when the ship is first entered.
type routeState struct {
	Active           string         `json:"active"`
	Dest             map[string]int `json:"dest,omitempty"`
	DisabledTreasure string         `json:"disabled_treasure,omitempty"`
}

func decodeRouteState(raw json.RawMessage) (routeState, error) {
	var st routeState
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("dungeon: map14 route state: %w", err)
	}
	return st, nil
}

func (st routeState) encode() json.RawMessage {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err) // a string and a map of ints always marshal
	}
	return raw
}

// allotPoints is _allot_point: three of the five points without replacement.
func allotPoints(rng RandomSource) json.RawMessage {
	return routeState{Active: drawMap14Sites(map14Points, 3, rng)}.encode()
}

func drawMap14Sites(candidates string, count int, rng RandomSource) string {
	left := []byte(candidates)
	drawn := make([]byte, 0, count)
	for range count {
		i := rng.Intn(len(left))
		drawn = append(drawn, left[i])
		left = append(left[:i], left[i+1:]...)
	}
	return string(drawn)
}

func routeActive(exp *ActiveExpedition, cell byte) (bool, error) {
	st, err := decodeRouteState(exp.MapState)
	return strings.IndexByte(st.Active, cell) >= 0, err
}

func shipTreasureActive(exp *ActiveExpedition, cell byte) (bool, error) {
	st, err := decodeRouteState(exp.MapState)
	return strings.IndexByte(st.DisabledTreasure, cell) < 0, err
}

// routeDestination returns the point's destination variant, drawing it on the
// first visit uniformly from the types no point holds yet (get_sub_stage_num).
// The first ship transfer also allocates its disabled sites.
func routeDestination(exp *ActiveExpedition, cell byte, rng RandomSource) (int, json.RawMessage, error) {
	st, err := decodeRouteState(exp.MapState)
	if err != nil {
		return 0, nil, err
	}
	v, assigned := st.Dest[string(cell)]
	if !assigned {
		var free []int
		for candidate := 2; candidate <= map14ShipVariant; candidate++ {
			held := false
			for _, d := range st.Dest {
				held = held || d == candidate
			}
			if !held {
				free = append(free, candidate)
			}
		}
		v = free[rng.Intn(len(free))]
		if st.Dest == nil {
			st.Dest = map[string]int{}
		}
		st.Dest[string(cell)] = v
	}
	if v == map14ShipVariant && st.DisabledTreasure == "" {
		st.DisabledTreasure = drawMap14Sites(map14DisablableSites, 5, rng)
	} else if assigned {
		return v, nil, nil
	}
	return v, st.encode(), nil
}

// Distinctive names are replaced (rule 00 §3), reusing the shared catalog's.
var (
	nm14Helmfish = DungeonMonster{ID: "nm14-helmfish", Name: "ヘルムフィッシュ", HP: 138, Attack: 98, Defense: 132, Agility: 95, ExpReward: 41, GoldReward: 8}
	nm14Diver    = DungeonMonster{ID: "nm14-diver-chick", Name: "ダイバーピヨコ", HP: 250, Attack: 150, Defense: 101, Agility: 108, ExpReward: 54, GoldReward: 54}
	nm14Turtle   = DungeonMonster{ID: "nm14-turtle-warrior", Name: "亀戦士", HP: 118, Attack: 101, Defense: 111, Agility: 42, ExpReward: 37, GoldReward: 22}
	nm14Marine   = DungeonMonster{ID: "nm14-sea-slime", Name: "海原スライム", HP: 142, Attack: 88, Defense: 138, Agility: 53, ExpReward: 38, GoldReward: 25}
	nm14Mimic    = DungeonMonster{ID: "nm14-ship-mimic", Name: "ミミック", HP: 300, Attack: 280, Defense: 65, Agility: 600, ExpReward: 40, GoldReward: 100}
	// shortcut: the strongest chest monster represents the group until #1954 adds groups.
	nm14ArmoredChest = DungeonMonster{ID: "nm14-armored-chest", Name: "魔装の宝箱", HP: 500, Attack: 300, Defense: 95, Agility: 400, ExpReward: 70, GoldReward: 200}
)

// nm14Pools are _data.cgi (main floor and _T), _dataP.cgi and _dataK.cgi with
// their appearance weights; P and K bosses stand in for their groups.
var nm14Pools = [3]EncounterPool{
	{
		Monsters: []DungeonMonster{
			{ID: "nm14-numb-jelly", Name: "麻痺クラゲ", HP: 106, Attack: 97, Defense: 43, Agility: 67, ExpReward: 32, GoldReward: 23},
			nm14Marine,
			{ID: "nm14-shell-slime", Name: "シェルスライム", HP: 85, Attack: 90, Defense: 130, Agility: 40, ExpReward: 34, GoldReward: 29},
			nm14Helmfish, nm14Diver, nm14Turtle,
		},
		Appears: []int{0, 0, 0, 1, 1, 2, 2, 2, 3, 3, 3, 4, 5, 5, 5},
	},
	{
		Monsters: []DungeonMonster{
			{ID: "nm14-pirate-skeleton", Name: "カイゾクガイコツ", HP: 154, Attack: 108, Defense: 20, Agility: 76, ExpReward: 41, GoldReward: 36},
			nm14Helmfish, nm14Diver, nm14Turtle,
		},
		Appears: []int{0, 0, 0, 0, 0, 1, 1, 2, 3},
		Boss:    &DungeonMonster{ID: "nm14-captain-skeleton", Name: "キャプテンガイコツ", HP: 1500, Attack: 248, Defense: 82, Agility: 140, ExpReward: 500, GoldReward: 320},
	},
	{
		Monsters: []DungeonMonster{
			nm14Marine,
			{ID: "nm14-cleio", Name: "クレイオー", HP: 15, Attack: 35, Defense: 1200, Agility: 600, ExpReward: 200, GoldReward: 500},
			nm14Helmfish,
			{ID: "nm14-luminous-tentacles", Name: "ルミナステンタクルス", HP: 147, Attack: 118, Defense: 72, Agility: 15, ExpReward: 45, GoldReward: 20},
			nm14Diver,
			{ID: "nm14-light-jaws-shell", Name: "ライトジョーズシェル", HP: 320, Attack: 170, Defense: 200, Agility: 10, ExpReward: 62, GoldReward: 90},
		},
		Appears: []int{0, 0, 0, 1, 2, 2, 2, 3, 3, 4, 5},
		Boss:    &DungeonMonster{ID: "nm14-leviakel", Name: "リヴァイケル", HP: 5800, Attack: 300, Defense: 200, Agility: 160, ExpReward: 800, GoldReward: 1000},
	},
}

// nativeMap14 has round limit 200 on every floor and the leader job level 30
// of @ex_dungeon. Points route to a destination; R returns to the main floor
// at the same coordinate; 9 is narration only.
func nativeMap14() Dungeon {
	back := map[byte]FloorExit{'R': {Variants: []int{1}}}
	ship := Floor{Pools: nm14Pools[:1], Exits: back, Marks: map[byte]string{'R': "★"}, events: map[byte]cellEvent{}}
	sites := map14DisablableSites + "k"
	for i := range len(sites) {
		ship.Marks[sites[i]] = "宝"
		ship.events[sites[i]] = cellEvent{kind: eventShipSite, perSymbol: true}
	}
	floors := [4]Floor{
		{Pools: nm14Pools[:1], Marks: map[byte]string{'A': "★", 'B': "★", 'C': "★", 'D': "★", 'E': "★"},
			events: map[byte]cellEvent{'A': {kind: eventRoute}, 'B': {kind: eventRoute}, 'C': {kind: eventRoute}, 'D': {kind: eventRoute}, 'E': {kind: eventRoute}}},
		{Pools: nm14Pools[1:2], Exits: back, Marks: map[byte]string{'P': "◎", 'R': "★", 'a': "宝", 'b': "宝"},
			events: map[byte]cellEvent{'P': {kind: eventBoss, perSymbol: true}, 'a': {kind: eventTreasure, perSymbol: true}, 'b': {kind: eventTreasure, perSymbol: true}}},
		{Pools: nm14Pools[2:3], Exits: back, Marks: map[byte]string{'K': "◎", 'R': "★", 't': "宝", 'u': "宝", 'v': "宝"},
			events: map[byte]cellEvent{'K': {kind: eventBoss, perSymbol: true}, 't': {kind: eventTreasure, perSymbol: true}, 'u': {kind: eventTreasure, perSymbol: true}, 'v': {kind: eventTreasure, perSymbol: true}}},
		ship,
	}
	variants := make([]Floor, len(floors))
	for i, f := range floors {
		f.FloorNumber = 1
		variants[i] = gridFloor(f, nativeMap14Floors[i])
	}
	return Dungeon{
		ID:             NativeMap14ID,
		Name:           "沈没の海峡",
		Description:    "海底に沈む船と深い峡谷。光る目印の先は、潜るたびに行き先が定まる。",
		RoundLimit:     200,
		LeaderJobLevel: 30,
		Variants:       variants,
		Initial:        1,
		newState:       allotPoints,
	}
}
