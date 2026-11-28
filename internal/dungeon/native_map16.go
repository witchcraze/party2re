package dungeon

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

// nativeMap16ID is the crystal mine (legacy map/16), unlisted until its sealed
// floors land (#2943).
const nativeMap16ID = "native-map-16"

// nativeMap16Floors transcribe map/16/1.cgi, _1.cgi, __2.cgi and __1.cgi.
var nativeMap16Floors = [4][]string{
	{"11111111", "111D1111", "111d0011", "1100101C", "1X00100c", "12101100", "111S0000", "111110b1", "111111B1"},
	{"1111R111", "1Gg00001", "11001001", "1101Ee01", "11011101", "10Y1210R", "11010001", "110000f1", "111R11F1"},
	{"11111111", "11111111", "11111111", "11111111", "11113111", "11113111", "11113111", "1111V111", "1z33v111"},
	{"11191111", "111H1111", "111h1111", "11131111", "11131111", "111Z1111", "11131111", "11131111", "1333Q111"},
}

// nm16Kinds are the gem-boss kinds; get_kind_from_wday maps Monday–Saturday
// to them in order (_1.cgi:45–52).
var nm16Kinds = []string{"i", "j", "k", "l", "m", "n"}

// map16State is the run's map16 progress; X and Y use per-symbol once-keys.
type map16State struct {
	// Site is the active mining site (B–H): entering it sets it and its
	// approach cell clears it.
	Site string `json:"site,omitempty"`
	// SpringUsed means the three springs (R), which share one flag, are empty.
	SpringUsed bool `json:"spring_used,omitempty"`
	// Kinds are the gem-boss kinds spawned this run, recorded at spawn.
	Kinds []string `json:"kinds,omitempty"`
}

// map16StateOf reads the state an earlier event of this step saved, or the run's.
func map16StateOf(n *nativeStep) (map16State, error) {
	raw := n.step.MapState
	if raw == nil {
		raw = n.exp.MapState
	}
	var st map16State
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("dungeon: map16 state: %w", err)
	}
	return st, nil
}

func (st map16State) save(n *nativeStep) {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err) // plain fields always marshal
	}
	n.step.MapState = raw
}

// nm16Site is a mining site (B–G): entering it activates the site (1.cgi:25,27,29).
type nm16Site struct{}

func (nm16Site) apply(n *nativeStep) error {
	st, err := map16StateOf(n)
	if err != nil {
		return err
	}
	st.Site = string(n.cell)
	st.save(n)
	return nil
}

// nm16Approach is b–g: it clears its own site, then spawns like the base
// event_0 (1.cgi:24,26,28).
type nm16Approach struct{}

func (nm16Approach) apply(n *nativeStep) error {
	st, err := map16StateOf(n)
	if err != nil {
		return err
	}
	if st.Site == strings.ToUpper(string(n.cell)) {
		st.Site = ""
		st.save(n)
	}
	return spawnEvent{}.apply(n)
}

// nm16Refill is the check that starts every _1 event but the wall and 2: a
// step landing on a multiple of 20 refills the springs (_1.cgi:34–43).
type nm16Refill struct{ nativeEvent }

func (r nm16Refill) apply(n *nativeStep) error {
	if err := nm16RefillSprings(n); err != nil {
		return err
	}
	return r.nativeEvent.apply(n)
}

func nm16RefillSprings(n *nativeStep) error {
	if (n.exp.Round+n.step.RoundDelta)%20 != 0 {
		return nil
	}
	st, err := map16StateOf(n)
	if err != nil || !st.SpringUsed {
		return err
	}
	st.SpringUsed = false
	st.save(n)
	return nil
}

// nm16Spring is R: after the refill check, while the springs are full, it
// empties them and heals the party's HP and MP with base 100 (_1.cgi:34). It
// runs the check itself so its dynamic mark stays visible.
type nm16Spring struct{}

func (nm16Spring) mark(exp *ActiveExpedition, _ byte) (string, error) {
	var st map16State
	if len(exp.MapState) > 0 {
		if err := json.Unmarshal(exp.MapState, &st); err != nil {
			return "", fmt.Errorf("dungeon: map16 state: %w", err)
		}
	}
	if st.SpringUsed {
		return "□", nil
	}
	return "○", nil
}

func (nm16Spring) apply(n *nativeStep) error {
	if err := nm16RefillSprings(n); err != nil {
		return err
	}
	st, err := map16StateOf(n)
	if err != nil || st.SpringUsed {
		return err
	}
	hp, err := partyHeal(n.exp, n.actor, 100, n.rng)
	if err != nil {
		return err
	}
	mp, err := partyMPHeal(n.exp, n.actor, 100, n.rng)
	if err != nil {
		return err
	}
	st.SpringUsed = true
	st.save(n)
	n.step.MemberHPDeltas, n.step.MemberMPDeltas, n.step.HPDelta = hp, mp, hp[n.leader.ID]
	n.recovery = true
	return nil
}

// nm16Field is _1's event_0: before Y a spawn at 1/3; after Y, at 1/6 a gem
// boss of an unrecorded kind (an ordinary group once all are recorded), else
// a spawn at 1/3 (_1.cgi:35,63–70).
type nm16Field struct{}

func (nm16Field) apply(n *nativeStep) error {
	if slices.Contains(n.exp.ConsumedEvents, "Y") && 6*n.rng.Float64() < 1 {
		st, err := map16StateOf(n)
		if err != nil {
			return err
		}
		left := slices.DeleteFunc(slices.Clone(nm16Kinds), func(k string) bool { return slices.Contains(st.Kinds, k) })
		if len(left) == 0 {
			n.fightGroup(n.encounterPool(), "", "")
			return nil
		}
		kind := left[n.rng.Intn(len(left))]
		st.Kinds = append(st.Kinds, kind)
		st.save(n)
		n.fightRoster([]DungeonMonster{nm16GemBosses[kind]}, "", "")
		return nil
	}
	if 3*n.rng.Float64() < 1 {
		n.fightGroup(n.encounterPool(), "", "")
	}
	return nil
}

// nm16Weekday is get_kind_from_wday in JST: Monday–Saturday give i–n; Sunday
// draws int(rand(6)), where 0 gives no kind and 1–5 give i–m (_1.cgi:72–76).
func nm16Weekday(t time.Time, rng RandomSource) string {
	w := int(t.In(timer.JST).Weekday())
	if w == 0 {
		w = rng.Intn(6)
	}
	if w == 0 {
		return ""
	}
	return nm16Kinds[w-1]
}

// nm16Herald is Y: once, the weekday's gem boss, or __data.cgi's knights
// without a kind (_1.cgi:43).
type nm16Herald struct{}

func (nm16Herald) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	roster := nm16Knights
	if kind := nm16Weekday(n.step.Now, n.rng); kind != "" {
		st, err := map16StateOf(n)
		if err != nil {
			return err
		}
		st.Kinds = append(st.Kinds, kind)
		st.save(n)
		roster = []DungeonMonster{nm16GemBosses[kind]}
	}
	n.fightRoster(roster, n.key, "")
	return nil
}

// nativeMap16 starts on the main floor; every floor has limit 150. Variant 2
// is _1, 3 is __2 and 4 is __1.
// shortcut: _1's 2 and the __2/__1 events stay inert, and the map unlisted, until #2943.
func nativeMap16() Dungeon {
	site := mapEvent{action: nm16Site{}}
	approach := mapEvent{action: nm16Approach{}}
	main := Floor{
		Pools: []EncounterPool{nm16MainPool},
		Exits: map[byte]FloorExit{'2': {Variants: []int{2}}},
		Marks: map[byte]string{'X': "◎", 'B': "★", 'C': "★", 'D': "★", '2': "凸"},
		events: map[byte]mapEvent{
			'B': site, 'C': site, 'D': site, 'b': approach, 'c': approach, 'd': approach,
			'X': {action: onceEvent{monsterOutcome{monsters: []DungeonMonster{nm16MainCrystal}}}, perSymbol: true},
		},
	}
	refill := func(ev nativeEvent) mapEvent { return mapEvent{action: nm16Refill{ev}} }
	spring := Floor{
		FloorNumber: 2,
		Pools:       []EncounterPool{nm16SpringPool},
		Marks:       map[byte]string{'Y': "◎", 'E': "★", 'F': "★", 'G': "★", '2': "凸"},
		events: map[byte]mapEvent{
			'0': refill(nm16Field{}), 'R': {action: nm16Spring{}},
			'E': refill(nm16Site{}), 'F': refill(nm16Site{}), 'G': refill(nm16Site{}),
			'e': refill(nm16Approach{}), 'f': refill(nm16Approach{}), 'g': refill(nm16Approach{}),
			'Y': {action: nm16Refill{nm16Herald{}}, perSymbol: true},
		},
	}
	tmpls := [4]Floor{main, spring, {FloorNumber: 3}, {FloorNumber: 4, Name: "封印結界"}}
	variants := make([]Floor, len(tmpls))
	for i, f := range tmpls {
		variants[i] = gridFloor(f, nativeMap16Floors[i])
	}
	return Dungeon{
		ID:          nativeMap16ID,
		Name:        "水晶鉱山",
		Description: "水晶の眠る古い鉱山。掘り進めた奥に封印が待つ。",
		RoundLimit:  150,
		Variants:    variants,
		Initial:     1,
	}
}
