package dungeon

import (
	"encoding/json"
	"fmt"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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
	t := 1
	for _, k := range n.exp.ConsumedEvents {
		if k == "j" || k == "k" {
			t++
		}
	}
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

// nm17Raisable is a 9 cell: a passage without encounters, or a wall while
// the switch walls stand (1.cgi:4,63–68).
type nm17Raisable struct{}

func (nm17Raisable) apply(*nativeStep) error { return nil }

func (nm17Raisable) closed(exp *ActiveExpedition) (bool, error) {
	st, err := decodeMap17(exp.MapState)
	return st.Raised, err
}

func (ev nm17Raisable) wallShown(exp *ActiveExpedition) (bool, error) { return ev.closed(exp) }

// nm17Gate is B: its boss once, then after b a door that returns the party
// north with the round spent (1.cgi:117–131).
type nm17Gate struct{ boss DungeonMonster }

func (ev nm17Gate) apply(n *nativeStep) error {
	if slices.Contains(n.exp.ConsumedEvents, "b") {
		n.step.NewX, n.step.NewY = n.exp.PosX, n.exp.PosY
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

func nm17Enemy(id, name string, hp, at, df, ag, exp, gold int, a corebattle.LifetimeActor) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold, combat: enemyCombat(a)}
}

func nm17Jobs(job string, sp int, old string, oldSP int) corebattle.BattleJobs {
	return corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: old, OldSP: oldSP}
}

// nm17Outer is __data.cgi's @monsters with @appears; distinctive names are
// replaced (rule 00 §3).
var nm17Outer = EncounterPool{
	Monsters: []DungeonMonster{
		nm17Enemy("nm17-blood-hand", "血塗れの手", 160, 130, 50, 60, 41, 10, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 41},
			Jobs:        nm17Jobs("job-09", 10, "job-61", 10),
		}),
		nm17Enemy("nm17-mummy", "マミー", 170, 150, 60, 60, 43, 16, corebattle.LifetimeActor{
			Jobs: nm17Jobs("", 0, "", 20),
		}),
		// The source lists old_sp twice; Perl keeps the later 40 (__data.cgi:129–130).
		nm17Enemy("nm17-stinger", "スティンガー", 180, 180, 150, 120, 54, 28, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-90", 20, "", 40),
		}),
		nm17Enemy("nm17-wraith", "死霊", 199, 119, 199, 66, 45, 13, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 42},
			Jobs:        nm17Jobs("job-31", 60, "", 0),
		}),
		nm17Enemy("nm17-mimic", "ミミック", 300, 100, 30, 300, 50, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 68},
			Jobs:        nm17Jobs("job-93", 10, "", 0),
			Temporary:   corebattle.TemporaryDouble,
		}),
		nm17Enemy("nm17-ooze-pharaoh", "粘体の王", 444, 144, 144, 144, 144, 144, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 144},
			Jobs:        nm17Jobs("job-19", 60, "", 0),
		}),
	},
	Appears: []int{0, 0, 1, 1, 1, 2, 2, 3, 3, 4, 5},
}

// nm17Inner is ___data.cgi's @monsters with @appears.
var nm17Inner = EncounterPool{
	Monsters: []DungeonMonster{
		nm17Enemy("nm17-sphinx-a", "スフィンクス", 500, 400, 480, 100, 250, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-37", 999, "job-30", 999),
			Hit:         150,
		}),
		nm17Enemy("nm17-sphinx-b", "スフィンクス", 500, 400, 480, 100, 250, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500},
			Jobs:        nm17Jobs("job-19", 999, "job-27", 999),
			Hit:         150,
		}),
		nm17Enemy("nm17-sand-golem", "サンドゴーレム", 350, 320, 450, 200, 200, 200, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 200},
			Jobs:        nm17Jobs("job-16", 999, "", 30),
			Temporary:   corebattle.TemporaryRevival,
		}),
		nm17Enemy("nm17-calamity-box", "災いの箱", 900, 300, 95, 800, 100, 500, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 69},
			Jobs:        nm17Jobs("job-93", 20, "", 0),
			Temporary:   corebattle.TemporaryDouble,
		}),
		nm17Enemy("nm17-envoy", "ファラオのつかい", 450, 300, 800, 900, 130, 800, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 500, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:        nm17Jobs("job-15", 200, "job-91", 200),
		}),
	},
	Appears: []int{0, 0, 1, 1, 2, 2, 3, 4},
}

// nm17Guardians is __data.cgi's @bosses, fought at B.
// shortcut: the strongest member stands in for the group until #1954 adds groups.
var nm17Guardians = strongest([]DungeonMonster{
	nm17Enemy("nm17-golem-a", "サンドゴーレムA", 1300, 220, 400, 110, 250, 100, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 700},
		Jobs:        nm17Jobs("job-16", 999, "", 0),
		Hit:         150,
		Temporary:   corebattle.TemporaryRevival,
	}),
	nm17Enemy("nm17-sphinx-guard-a", "スフィンクスA", 3200, 260, 320, 150, 500, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 999},
		Jobs:          nm17Jobs("job-37", 999, "job-30", 999),
		Hit:           150,
		CrystalReward: 4,
		Temporary:     corebattle.TemporaryReflectAttack,
	}),
	nm17Enemy("nm17-sphinx-guard-b", "スフィンクスB", 3200, 260, 320, 150, 500, 200, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 999},
		Jobs:          nm17Jobs("job-19", 999, "job-27", 999),
		Hit:           150,
		CrystalReward: 4,
		Temporary:     corebattle.TemporaryReflectMagic,
	}),
	nm17Enemy("nm17-golem-b", "サンドゴーレムB", 1300, 220, 400, 110, 250, 100, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 700},
		Jobs:        nm17Jobs("job-16", 999, "", 0),
		Hit:         150,
		Temporary:   corebattle.TemporaryRevival,
	}),
})

// nativeMap17 has the leader job level 40 of @ex_dungeon and limit 200 on
// every floor. Variants 2–4 are the underground floors, 5 the secret room.
// shortcut: underground and secret-room events, 2, Z and w stay plain
// passages until their units land; chests hold no contents until #2455.
func nativeMap17() Dungeon {
	passage := mapEvent{action: passageEvent{}}
	main := Floor{
		Marks: map[byte]string{'6': "◇", '7': "◇", '8': "◇", 'B': "◎", 'U': "凹", 'z': "扉", '2': "■", 'Z': "◎"},
		events: map[byte]mapEvent{
			'0': {action: nm17Spawn{}}, '9': {action: nm17Raisable{}},
			'B': {action: nm17Gate{boss: nm17Guardians}, perSymbol: true}, 'b': {action: nm17Seal{}, perSymbol: true},
			'U': {action: nm17Stairs{}}, 'z': {action: doorEvent{key: 'Y'}, perSymbol: true},
			'6': {action: nm17Switch{x: 6, y: 3}, perSymbol: true},
			'7': {action: nm17Switch{x: 4, y: 10}, perSymbol: true},
			'8': {action: nm17Switch{x: 8, y: 16}, perSymbol: true},
		},
	}
	// Underground passages and the secret room spawn nothing (event_0 is empty).
	under := Floor{
		Exits:  map[byte]FloorExit{'u': {Variants: []int{1}}},
		Marks:  map[byte]string{'u': "凸"},
		events: map[byte]mapEvent{'0': passage},
	}
	room := Floor{events: map[byte]mapEvent{'0': passage}}
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
