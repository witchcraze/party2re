package dungeon

import (
	"encoding/json"
	"fmt"
	"slices"
)

// NativeMap17ID is the pharaoh's tomb (legacy map/17).
const NativeMap17ID = "native-map-17"

// nativeMap17Floors transcribe map/17/1.cgi, _1.cgi, _2.cgi, _3.cgi and __1.cgi.
var nativeMap17Floors = [5][]string{
	{"U000000000009000900U", "01111111111111111110", "01111111111111111110", "011U000000009000U110", "01100090079000900110", "01190111111111109110", "01100100000000100110", "01109100000000190110", "01100100111100100110", "011081001w0100160110", "011001001Z0100100110", "01109100100100190110", "011001001z1100100110", "0119010b10000U109110", "0110011B121111100110", "01100000011000900110", "011U0090011S0000U110", "91111111111111111119", "01111111111111111110", "U000000011000000000U"},
	{"u011111111111111iI0u", "00111111111111111000", "H0111111111111111111", "h11u001111111110u111", "11100Kk1111111100111", "11111111111111100111", "1111111111111111V111", "1111111111111111v111", "1111111111111111v111", "1111111110011111v111", "111111111Y011111V111", "111111111n0111110111", "11111111101111110111", "111j11111Xxxxu110111", "111J0111111111100111", "11100111111111100110", "111u011111111110u111", "11111111111111111111", "0000000Vvvvv00000000", "u000000111110000000u"},
	{"u011111111111111iI0u", "00111111111111111000", "H0111111111111111111", "h11u001111111100u111", "111000VvvvV000000111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111100111111111", "111111111Y0111111111", "111111111n0111110111", "11111111101111111111", "111j11111Xxxxu11k111", "111J011111111110K111", "11100111111111100110", "111u011111111110u111", "11111111111111111111", "0000000Vvvvv00000000", "u000000111110000000u"},
	{"u011111111111111iI0u", "00111111111111111000", "H0111111111111111111", "h11u011111111100u111", "1110011111111jJ00111", "11100111111111111111", "11101111111111111111", "11101111111111111111", "11101111111111111111", "111V1111100111111111", "111v11111Y0111111111", "111v11111n0111111111", "111v1111101111111111", "111V11111Xxxxu11k111", "1110011111111110K111", "11100111111111100110", "111u011111111110u111", "11111111111111111111", "0000000Vvvvv00000000", "u000000111110000000u"},
	{"11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111", "11111111D0OE11111111", "11111111L00011111111", "11111111000M11111111", "11111111GQ0F11111111", "11111111111111111111", "11111111111111111111", "111111111U1111111111", "11111111100111111111", "111111111N0111111111", "11111111111111111111", "11111111111111111111", "11111111111111111111"},
}

// map17State is the run's map17 progress; once-events use per-symbol keys.
type map17State struct {
	// Under is the underground floor (1–3) the first stair drew.
	Under int `json:"under,omitempty"`
	// Raised records that the switch walls (9) stand.
	Raised bool `json:"raised,omitempty"`
	// shortcut: the batch resolver has no named fields; explicit combat (#1865) applies it.
	Field string `json:"field,omitempty"`
}

func decodeMap17(raw json.RawMessage) (map17State, error) {
	var st map17State
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("dungeon: map17 state: %w", err)
	}
	return st, nil
}

func (st map17State) encode() json.RawMessage {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err) // plain fields always marshal
	}
	return raw
}

// nm17Spawn is the main floor's event_0: an encounter when 5r ≤ t, where t is
// 1 plus the j/k treasures visited, or 0 once the key is held (1.cgi:51–53,62).
// Passing b switches the pool to ___data.cgi (1.cgi:172–177).
type nm17Spawn struct{}

func (nm17Spawn) apply(n *nativeStep) error {
	t := 1 + nm17Visits(n.exp)
	if slices.Contains(n.exp.ConsumedEvents, "Y") {
		t = 0
	}
	if 5*n.rng.Float64() > float64(t) {
		return nil
	}
	pool := nm17Outer
	if slices.Contains(n.exp.ConsumedEvents, "b") {
		pool = nm17Inner
	}
	n.fight(pool.monster(n.rng), EventBattle, "", "")
	return nil
}

// nm17Visits counts the j/k treasures visited, whatever they held.
func nm17Visits(exp *ActiveExpedition) int {
	t := 0
	for _, k := range exp.ConsumedEvents {
		if k == "j" || k == "k" {
			t++
		}
	}
	return t
}

// nm17Raisable is a 9 cell: a passage without encounters, or a wall while
// the switch walls stand (1.cgi:4,63–68).
type nm17Raisable struct{}

func (nm17Raisable) apply(*nativeStep) error { return nil }

func (nm17Raisable) closed(exp *ActiveExpedition) (bool, error) {
	st, err := decodeMap17(exp.MapState)
	return st.Raised, err
}

func (ev nm17Raisable) wallShown(exp *ActiveExpedition) (bool, error) { return ev.closed(exp) }

// nm17Gate is B: its boss once, then after b a wall that puts the party one
// row north (--$py) with the round spent (1.cgi:117–131).
type nm17Gate struct{ boss DungeonMonster }

func (ev nm17Gate) apply(n *nativeStep) error {
	if slices.Contains(n.exp.ConsumedEvents, "b") {
		n.step.NewY--
		return nil
	}
	if n.consumed {
		return nil
	}
	return fixedBoss(ev.boss).apply(n)
}

func (nm17Gate) wallShown(exp *ActiveExpedition) (bool, error) {
	return slices.Contains(exp.ConsumedEvents, "b"), nil
}

// nm17Seal is b: once, it installs den as the run's default field (1.cgi:133).
type nm17Seal struct{}

func (nm17Seal) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap17(n.exp.MapState)
	if err != nil {
		return err
	}
	st.Field = "den"
	n.step.EventKey = n.key
	n.step.MapState = st.encode()
	return nil
}

// nm17Stairs is U: the first use draws _1–_3 for the run and every stair
// reuses it, keeping the coordinate (1.cgi:162–170).
type nm17Stairs struct{}

func (nm17Stairs) apply(n *nativeStep) error {
	st, err := decodeMap17(n.exp.MapState)
	if err != nil {
		return err
	}
	if st.Under == 0 {
		st.Under = 1 + n.rng.Intn(3)
		n.step.MapState = st.encode()
	}
	n.step.MapVariant = 1 + st.Under
	n.step.RoundLimit = n.dungeon.limitOf(n.dungeon.Variants[n.step.MapVariant-1])
	return nil
}

// nm17Switch is switch(num, x, y) (1.cgi:77–98): once, the first true branch
// of lower walls, nothing, raise walls, warp, or falling treasure.
type nm17Switch struct{ x, y int }

func (ev nm17Switch) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap17(n.exp.MapState)
	if err != nil {
		return err
	}
	n.step.EventKey = n.key
	switch {
	case st.Raised && 2*n.rng.Float64() < 1:
		st.Raised = false
	case 3*n.rng.Float64() < 1:
	case !st.Raised && 4*n.rng.Float64() < 1:
		st.Raised = true
	case 2*n.rng.Float64() < 1:
		n.step.NewX, n.step.NewY = ev.x, ev.y
	default:
		n.step.Encounter = treasureEncounter(n.exp, n.actor.JobID, n.rng, n.step.NewX, n.step.NewY)
	}
	n.step.MapState = st.encode()
	return nil
}

// onceEvent fires its event only while the cell's once-key is unspent.
type onceEvent struct{ nativeEvent }

func (o onceEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	return o.nativeEvent.apply(n)
}

// nm17Pair is h/i or j/k: the visit is recorded either way, then a treasure
// if rand(2) < 1 or the partner was visited, else the boss (_1.cgi:57–99).
type nm17Pair struct {
	partner string
	boss    DungeonMonster
}

func (ev nm17Pair) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	if 2*n.rng.Float64() < 1 || slices.Contains(n.exp.ConsumedEvents, ev.partner) {
		return treasure.apply(n)
	}
	return fixedBoss(ev.boss).apply(n)
}

// nm17Ambush is v (every entry) or, once the key is held, x: _trap_d(100)
// when rand(odds) < 1, then the boss (_1.cgi:52,54).
type nm17Ambush struct {
	odds    float64
	needKey bool
	boss    DungeonMonster
}

func (ev nm17Ambush) apply(n *nativeStep) error {
	if ev.needKey && !slices.Contains(n.exp.ConsumedEvents, "Y") {
		return nil
	}
	if ev.odds*n.rng.Float64() < 1 {
		partyTrap(&n.step, n.exp, n.leader.ID, 100, n.rng)
	}
	n.fight(ev.boss, EventBoss, "", "")
	return nil
}

// nm17Rest is n and N: once, _heals(base, '無') then _item_mp_heals(base).
type nm17Rest struct{ base float64 }

func (ev nm17Rest) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	hp, err := partyHeal(n.exp, n.actor, ev.base, n.rng)
	if err != nil {
		return err
	}
	n.step.EventKey = n.key
	n.step.MemberHPDeltas, n.step.MemberMPDeltas, n.step.HPDelta = hp, itemMPHeal(n.exp, ev.base, n.rng), hp[n.leader.ID]
	n.recovery = true
	return nil
}

// itemMPHeal is _item_mp_heals(base) without an element: each living member
// regains int(base × (0.9 + rand(0.3))) MP up to max, with no field or status
// processing, unlike _mp_hs (lib/_skill.cgi:10570–10586).
func itemMPHeal(exp *ActiveExpedition, base float64, rng RandomSource) map[string]int {
	out := make(map[string]int, len(exp.Members))
	for _, m := range exp.Members {
		if m.CurrentHP <= 0 {
			continue
		}
		v := int(base * (0.9 + 0.3*rng.Float64()))
		if d := min(m.CurrentMP+v, m.MaxMP) - m.CurrentMP; d > 0 {
			out[m.CharacterID] = d
		}
	}
	return out
}

// nm17Ascent is w: after Z, its boss and the move to the secret room at the
// same coordinate, saved together (1.cgi:153–160).
type nm17Ascent struct{}

func (nm17Ascent) visible(exp *ActiveExpedition, _ byte) (bool, error) {
	return slices.Contains(exp.ConsumedEvents, "Z"), nil
}

func (nm17Ascent) apply(n *nativeStep) error {
	if !slices.Contains(n.exp.ConsumedEvents, "Z") {
		return nil
	}
	n.step.MapVariant = 5
	n.step.RoundLimit = n.dungeon.limitOf(n.dungeon.Variants[4])
	n.fight(nm17Necro, EventBoss, "", "")
	return nil
}

// nm17Vault is a secret-room treasure shown from `show` visited j/k and
// opened only from `open`; a closed one stays shown and unspent (__1.cgi:32–55).
type nm17Vault struct{ open, show int }

func (ev nm17Vault) visible(exp *ActiveExpedition, _ byte) (bool, error) {
	return nm17Visits(exp) >= ev.show, nil
}

func (ev nm17Vault) apply(n *nativeStep) error {
	if n.consumed || nm17Visits(n.exp) < ev.open {
		return nil
	}
	return treasure.apply(n)
}

// nm17Underground binds the three underground floors' shared events.
var nm17Underground = map[byte]mapEvent{
	'0': {action: passageEvent{}},
	'H': {action: onceEvent{fixedMonster(nm17Robber)}, perSymbol: true},
	'I': {action: onceEvent{fixedMonster(nm17Robber)}, perSymbol: true},
	'J': {action: onceEvent{fixedMonster(nm17RobberChief)}, perSymbol: true},
	'K': {action: onceEvent{fixedMonster(nm17RobberChief)}, perSymbol: true},
	'h': {action: nm17Pair{partner: "i", boss: nm17Outer.Monsters[4]}, perSymbol: true},
	'i': {action: nm17Pair{partner: "h", boss: nm17Outer.Monsters[4]}, perSymbol: true},
	'j': {action: nm17Pair{partner: "k", boss: nm17Inner.Monsters[3]}, perSymbol: true},
	'k': {action: nm17Pair{partner: "j", boss: nm17Inner.Monsters[3]}, perSymbol: true},
	'V': {action: keyEvent{}, perSymbol: true},
	'v': {action: nm17Ambush{odds: 2, boss: nm17Mask}},
	'x': {action: nm17Ambush{odds: 4, needKey: true, boss: nm17GreatMask}},
	'Y': {action: keyEvent{}, perSymbol: true},
	'n': {action: nm17Rest{base: 200}, perSymbol: true},
}

// nativeMap17 has the leader job level 40 of @ex_dungeon and limit 200 on
// every floor. Variants 2–4 are the underground floors, 5 the secret room.
// shortcut: chests hold no contents until #2455.
func nativeMap17() Dungeon {
	passage := mapEvent{action: passageEvent{}}
	main := Floor{
		Exits: map[byte]FloorExit{'2': {Variants: []int{5}}},
		Marks: map[byte]string{'6': "◇", '7': "◇", '8': "◇", 'B': "◎", 'U': "凹", 'z': "扉", '2': "■", 'Z': "◎", 'w': "凸"},
		events: map[byte]mapEvent{
			'0': {action: nm17Spawn{}}, '9': {action: nm17Raisable{}},
			'B': {action: nm17Gate{boss: nm17Guardians}, perSymbol: true}, 'b': {action: nm17Seal{}, perSymbol: true},
			'U': {action: nm17Stairs{}}, 'z': {action: doorEvent{key: 'Y'}, perSymbol: true},
			'6': {action: nm17Switch{x: 6, y: 3}, perSymbol: true},
			'7': {action: nm17Switch{x: 4, y: 10}, perSymbol: true},
			'8': {action: nm17Switch{x: 8, y: 16}, perSymbol: true},
			'Z': {action: onceEvent{fixedBoss(nm17Pharaohs)}, perSymbol: true}, 'w': {action: nm17Ascent{}},
		},
	}
	// The three underground floors share their events, and their passages
	// spawn nothing (event_0 is empty), as in the secret room.
	under := Floor{
		Exits:  map[byte]FloorExit{'u': {Variants: []int{1}}},
		Marks:  map[byte]string{'u': "凸", 'V': "◇", 'Y': "○", 'h': "宝", 'i': "宝", 'j': "宝", 'k': "宝", 'n': "☆"},
		events: nm17Underground,
	}
	room := Floor{
		Name:  "王墓の秘室",
		Exits: map[byte]FloorExit{'U': {Variants: []int{1}}},
		Marks: map[byte]string{'D': "宝", 'E': "宝", 'F': "宝", 'G': "宝", 'L': "宝", 'M': "宝", 'Q': "宝", 'O': "宝", 'N': "☆", 'U': "凹"},
		events: map[byte]mapEvent{
			'0': passage, 'D': {action: fixedTreasureEvent{}, perSymbol: true},
			'E': {action: treasure, perSymbol: true}, 'F': {action: treasure, perSymbol: true}, 'G': {action: treasure, perSymbol: true},
			'L': {action: nm17Vault{open: 1, show: 1}, perSymbol: true}, 'M': {action: nm17Vault{open: 2, show: 1}, perSymbol: true},
			'Q': {action: nm17Vault{open: 3, show: 2}, perSymbol: true}, 'O': {action: nm17Vault{open: 4, show: 2}, perSymbol: true},
			'N': {action: nm17Rest{base: 300}, perSymbol: true},
		},
	}
	tmpls := [5]Floor{main, under, under, under, room}
	variants := make([]Floor, len(tmpls))
	for i, f := range tmpls {
		f.FloorNumber = 1
		variants[i] = gridFloor(f, nativeMap17Floors[i])
	}
	return Dungeon{
		ID:             NativeMap17ID,
		Name:           "王墓",
		Description:    "砂に埋もれた王の墓。仕掛けの奥に秘室が眠る。",
		RoundLimit:     200,
		LeaderJobLevel: 40,
		Variants:       variants,
		Initial:        1,
	}
}
