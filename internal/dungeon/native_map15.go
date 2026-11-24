package dungeon

import (
	"encoding/json"
	"fmt"
	"slices"
)

// NativeMap15ID is the fortress branches (legacy map/15).
const NativeMap15ID = "native-map-15"

// nativeMap15Floors transcribe map/15/1.cgi, _1.cgi, _2.cgi and __1.cgi.
var nativeMap15Floors = [4][]string{
	{"00200", "01010", "01010", "B1A1C", "01010", "00000", "11S11", "11011", "10401"},
	{"11011", "00000", "01010", "E1D1F", "01d10", "d0a0d", "11311", "11111", "11111"},
	{"11111", "11z11", "11011", "10Y01", "1d1d1", "1j1k1", "10101", "1J1K1", "10001"},
	{"11111", "1wzv1", "11X11", "10001", "10G01", "10001", "11011", "11111", "11111"},
}

// map15State is the run's map15 progress.
type map15State struct {
	// Outcomes are the exclusive-pool outcomes drawn so far (2–4 main, 5–7 _1).
	Outcomes []int `json:"outcomes,omitempty"`
	// Killed is the _2 treasure symbol ('j' or 'k') the first boss destroyed.
	Killed string `json:"killed,omitempty"`
	// SouthOpen records that _2's walls opened.
	SouthOpen bool `json:"south_open,omitempty"`
	// shortcut: the batch resolver has no named fields; explicit combat (#1865) applies it.
	Field string `json:"field,omitempty"`
}

func (st map15State) drew(outcome int) bool { return slices.Contains(st.Outcomes, outcome) }

func decodeMap15(raw json.RawMessage) (map15State, error) {
	if len(raw) == 0 {
		return map15State{}, nil
	}
	// Read legacy flags snapshots alongside typed fields; encode writes only typed fields.
	var payload struct {
		map15State
		Flags string `json:"flags"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return map15State{}, fmt.Errorf("dungeon: map15 state: %w", err)
	}
	st := payload.map15State
	for i := range len(payload.Flags) {
		switch ch := payload.Flags[i]; ch {
		case '2', '3', '4', '5', '6', '7':
			if outcome := int(ch - '0'); !st.drew(outcome) {
				st.Outcomes = append(st.Outcomes, outcome)
			}
		case 'j', 'k':
			if st.Killed == "" {
				st.Killed = string(ch)
			}
		case '9':
			st.SouthOpen = true
		}
	}
	return st, nil
}

func (st map15State) encode() json.RawMessage {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err) // two strings always marshal
	}
	return raw
}

// exclusiveEvent is get_event: a uniform draw from the outcomes in order that
// no trigger has drawn yet.
type exclusiveEvent struct {
	order    []int
	outcomes map[int]nativeEvent
}

func (ev exclusiveEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap15(n.exp.MapState)
	if err != nil {
		return err
	}
	var left []int
	for _, o := range ev.order {
		if !st.drew(o) {
			left = append(left, o)
		}
	}
	n.step.EventKey = n.key
	if len(left) == 0 {
		return nil // unreachable: each floor has as many triggers as outcomes
	}
	pick := left[n.rng.Intn(len(left))]
	st.Outcomes = append(st.Outcomes, pick)
	n.step.MapState = st.encode()
	return ev.outcomes[pick].apply(n)
}

// gateEvent is a wall that stays while `open` is false. A closed one returns
// the party and refunds the round; the pre-move reset has already run
// (_1.cgi:30–37, _2.cgi:31–38).
type gateEvent struct{ open func(map15State) bool }

func (ev gateEvent) visible(exp *ActiveExpedition, _ byte) (bool, error) {
	st, err := decodeMap15(exp.MapState)
	return !ev.open(st), err
}

func (ev gateEvent) apply(n *nativeStep) error {
	closed, err := ev.visible(n.exp, n.cell)
	if closed {
		n.step.NewX, n.step.NewY, n.step.RoundDelta = n.exp.PosX, n.exp.PosY, 0
	}
	return err
}

// recoveryOutcome is _heals then _mp_hs with a fixed base and element none.
type recoveryOutcome struct{ base float64 }

func (o recoveryOutcome) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	hp, err := partyHeal(n.exp, n.actor, o.base, n.rng)
	if err != nil {
		return err
	}
	mp, err := partyMPHeal(n.exp, n.actor, o.base, n.rng)
	if err != nil {
		return err
	}
	n.step.EventKey = n.key
	n.step.MemberHPDeltas, n.step.MemberMPDeltas, n.step.HPDelta = hp, mp, hp[n.leader.ID]
	n.recovery = true
	return nil
}

// fortressBoss installs fortress as the run's default field, then spawns the
// boss (__1.cgi:28, _2.cgi:30).
type fortressBoss struct{ roster []DungeonMonster }

func (ev fortressBoss) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap15(n.exp.MapState)
	if err != nil {
		return err
	}
	st.Field = "fortress"
	n.step.MapState = st.encode()
	return fixedBoss(ev.roster...).apply(n)
}

// fixedTreasureEvent is add_fixed_treasure: one chest per allied player,
// fallen included, with no job-78 bonus (_npc_action.cgi:555–598).
// shortcut: chests hold no contents until #2451 binds map15's sources (here item 265).
type fixedTreasureEvent struct{}

func (fixedTreasureEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	n.step.EventKey = n.key
	n.step.Encounter = &ActiveEncounter{EventType: EventTreasure, X: n.step.NewX, Y: n.step.NewY, Chests: make([]TreasureChest, max(1, len(n.exp.Members)))}
	return nil
}

// crossBossEvent is _2's J and K (_2.cgi:28–29,43–57): the first of the two
// kills j or k, the walls open by draw or once both fired, then one of four
// groups spawns.
type crossBossEvent struct{ groups [4][]DungeonMonster }

func (ev crossBossEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap15(n.exp.MapState)
	if err != nil {
		return err
	}
	other := "K"
	if n.cell == 'K' {
		other = "J"
	}
	bothFired := slices.Contains(n.exp.ConsumedEvents, other)
	if st.Killed == "" && !bothFired {
		st.Killed = "j"
		if 2*n.rng.Float64() >= 1 {
			st.Killed = "k"
		}
	}
	if !st.SouthOpen && (bothFired || 2*n.rng.Float64() < 1) {
		st.SouthOpen = true
	}
	pick := 2
	if 2*n.rng.Float64() < 1 {
		pick = 0
	}
	if 2*n.rng.Float64() >= 1 {
		pick++
	}
	n.step.MapState = st.encode()
	return fixedBoss(ev.groups[pick]...).apply(n)
}

// liveTreasure is a _2 treasure that the first boss may have killed.
type liveTreasure struct{}

func (liveTreasure) visible(exp *ActiveExpedition, cell byte) (bool, error) {
	st, err := decodeMap15(exp.MapState)
	return st.Killed != string(cell), err
}

func (ev liveTreasure) apply(n *nativeStep) error {
	live, err := ev.visible(n.exp, n.cell)
	if err != nil || !live {
		return err
	}
	return treasure.apply(n)
}

// nm15MainPool is the main floor's A–C outcomes (1.cgi:33–56).
var nm15MainPool = exclusiveEvent{order: []int{2, 3, 4}, outcomes: map[int]nativeEvent{
	2: choiceEvent{odds: 2, outcomes: [2]nativeEvent{fixedBoss(nm15Main2[0]...), fixedBoss(nm15Main2[1]...)}},
	3: fixedBoss(nm15Bombs...),
	4: stageMonster,
}}

// nm15NorthPool is _1's D–F outcomes (_1.cgi:43–67).
var nm15NorthPool = exclusiveEvent{order: []int{5, 6, 7}, outcomes: map[int]nativeEvent{
	5: fixedBoss(nm15Hazard...),
	6: fixedBoss(nm15ShieldedBombs...),
	7: recoveryOutcome{base: 100},
}}

// nativeMap15 has the leader job level 50 of @ex_dungeon. The main floor's
// stairs 2 and 4 lead to _1 and _2, and _1's 3 to __1, keeping the coordinate.
func nativeMap15() Dungeon {
	pool := []EncounterPool{nm15Pool}
	floors := [4]Floor{
		{Pools: pool,
			Exits:  map[byte]FloorExit{'2': {Variants: []int{2}}, '4': {Variants: []int{3}}},
			Marks:  map[byte]string{'A': "◎", 'B': "◎", 'C': "◎", '2': "凸", '4': "凸"},
			events: map[byte]mapEvent{'A': {action: nm15MainPool, perSymbol: true}, 'B': {action: nm15MainPool, perSymbol: true}, 'C': {action: nm15MainPool, perSymbol: true}}},
		{FloorNumber: 2, Pools: pool, RoundLimit: 60,
			Exits: map[byte]FloorExit{'3': {Variants: []int{4}}},
			Marks: map[byte]string{'D': "◎", 'E': "◎", 'F': "◎", 'a': "宝", 'd': "■", '3': "凸"},
			events: map[byte]mapEvent{'D': {action: nm15NorthPool, perSymbol: true}, 'E': {action: nm15NorthPool, perSymbol: true}, 'F': {action: nm15NorthPool, perSymbol: true},
				'a': {action: treasure, perSymbol: true}, 'd': {action: gateEvent{open: func(st map15State) bool { return st.drew(5) }}}}},
		// shortcut: chests hold no contents until #2451 binds map15's sources, _2's own pool included.
		// _2 and __1 share the z flag safely: the branches never meet.
		{FloorNumber: 2, Pools: pool, RoundLimit: 50,
			Marks: map[byte]string{'J': "◎", 'K': "◎", 'Y': "◎", 'z': "宝", 'j': "宝", 'k': "宝", 'd': "■"},
			events: map[byte]mapEvent{'J': {action: crossBossEvent{groups: nm15SouthGroups}, perSymbol: true}, 'K': {action: crossBossEvent{groups: nm15SouthGroups}, perSymbol: true},
				'Y': {action: fortressBoss{roster: nm15Panzer}, perSymbol: true}, 'd': {action: gateEvent{open: func(st map15State) bool { return st.SouthOpen }}},
				'z': {action: treasure, perSymbol: true}, 'j': {action: liveTreasure{}, perSymbol: true}, 'k': {action: liveTreasure{}, perSymbol: true}}},
		// __1's passages spawn nothing (event_0 is empty).
		{FloorNumber: 3, RoundLimit: 90,
			Marks: map[byte]string{'w': "宝", 'z': "宝", 'v': "宝", 'G': "○", 'X': "◎"},
			events: map[byte]mapEvent{'0': {action: passageEvent{}}, 'G': {action: recoveryOutcome{base: 300}, perSymbol: true},
				'X': {action: fortressBoss{roster: nm15Core}, perSymbol: true}, 'z': {action: fixedTreasureEvent{}, perSymbol: true},
				'w': {action: treasure, perSymbol: true}, 'v': {action: treasure, perSymbol: true}}},
	}
	variants := make([]Floor, len(floors))
	for i, f := range floors {
		variants[i] = gridFloor(f, nativeMap15Floors[i])
	}
	return Dungeon{
		ID:             NativeMap15ID,
		Name:           "機兵の砦",
		Description:    "機械兵が守る砦。三つの扉の先に何が待つかは、開けるまで分からない。",
		RoundLimit:     30,
		LeaderJobLevel: 50,
		Variants:       variants,
		Initial:        1,
	}
}
