package dungeon

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

// map15State keeps the legacy digit flags (pool outcomes 2–7, open walls 9),
// the treasure _2 killed, and the run's default field.
type map15State struct {
	Flags string `json:"flags,omitempty"`
	// shortcut: the batch resolver has no named fields; explicit combat (#1865) applies it.
	Field string `json:"field,omitempty"`
}

func (st map15State) has(flag byte) bool { return strings.IndexByte(st.Flags, flag) >= 0 }

func decodeMap15(raw json.RawMessage) (map15State, error) {
	var st map15State
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("dungeon: map15 state: %w", err)
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
	order    string
	outcomes map[byte]nativeEvent
}

func (ev exclusiveEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	st, err := decodeMap15(n.exp.MapState)
	if err != nil {
		return err
	}
	var left []byte
	for i := range len(ev.order) {
		if !st.has(ev.order[i]) {
			left = append(left, ev.order[i])
		}
	}
	n.step.EventKey = n.key
	if len(left) == 0 {
		return nil // unreachable: each floor has as many triggers as outcomes
	}
	pick := left[n.rng.Intn(len(left))]
	st.Flags += string(pick)
	n.step.MapState = st.encode()
	return ev.outcomes[pick].apply(n)
}

// gateEvent is a wall that opens with flag `open`. A closed one returns the
// party and refunds the round; the pre-move reset has already run
// (_1.cgi:30–37, _2.cgi:31–38).
type gateEvent struct{ open byte }

func (ev gateEvent) visible(exp *ActiveExpedition, _ byte) (bool, error) {
	st, err := decodeMap15(exp.MapState)
	return !st.has(ev.open), err
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
type fortressBoss struct{ boss DungeonMonster }

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
	return fixedBoss(ev.boss).apply(n)
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

// nm15Enemy builds a map-owned enemy from its legacy battle fields.
func nm15Enemy(id, name string, hp, at, df, ag, exp, gold int, a corebattle.LifetimeActor) DungeonMonster {
	return DungeonMonster{ID: id, Name: name, HP: hp, Attack: at, Defense: df, Agility: ag, ExpReward: exp, GoldReward: gold, combat: enemyCombat(a)}
}

func nm15Jobs(job string, sp int, old string, oldSP int) corebattle.BattleJobs {
	return corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: old, OldSP: oldSP}
}

// nm15Pool is __data.cgi's @monsters with @appears; _1 and _2 use it too.
var nm15Pool = EncounterPool{
	Monsters: []DungeonMonster{
		nm15Enemy("nm15-private", "PR-4", 200, 180, 150, 97, 54, 20, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 140, CMP: 40},
			Jobs:        nm15Jobs("job-30", 999, "", 0),
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/__data.cgi:75", CustomName: "custom-gem_summon_sky",
				Gems: [3]string{"gem_summon_sky"}, GemCosts: [3]int{18}},
		}),
		nm15Enemy("nm15-roughneck", "RA-28", 480, 330, 160, 190, 98, 120, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 680, ItemDefinitionIDs: []string{"item-147"}},
			Jobs:        nm15Jobs("job-81", 999, "job-50", 999),
			Temporary:   corebattle.TemporaryNullMagic,
		}),
		nm15Enemy("nm15-sky-slaughter", "SK-51", 400, 400, 80, 380, 122, 36, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 300},
			Jobs:        nm15Jobs("job-502", 999, "job-52", 999),
		}),
		nm15Enemy("nm15-assailant", "PO-45", 500, 320, 250, 160, 100, 100, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 421, CMP: 70},
			Jobs:        nm15Jobs("job-24", 999, "job-01", 999),
			NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/__data.cgi:127", CustomName: "custom-gem_buff_atk_1-gem_atk_2-gem_atk_2",
				Gems: [3]string{"gem_buff_atk_1", "gem_atk_2", "gem_atk_2"}, GemCosts: [3]int{16, 10, 10}},
		}),
		nm15Enemy("nm15-goost", "GA-67", 370, 480, 380, 350, 128, 77, corebattle.LifetimeActor{
			Participant: corebattle.Participant{MP: 555, ItemDefinitionIDs: []string{"item-193"}},
			Jobs:        nm15Jobs("job-85", 999, "job-20", 999),
			Temporary:   corebattle.TemporaryReduceAttack,
		}),
	},
	Appears: []int{0, 0, 0, 0, 1, 1, 1, 2, 2, 3, 3, 3, 4},
}

// The main floor's outcome 2 groups (1.cgi:58–192) are represented by their
// leaders, the bomb group (194–239) by its first member.
// shortcut: one member stands in for each three-member group until #1954 adds groups.
var (
	nm15Inferno = nm15Enemy("nm15-inferno", "IF-31", 1500, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 800, CMP: 100, ItemDefinitionIDs: []string{"item-158"}},
		Jobs:          nm15Jobs("job-54", 999, "job-84", 999),
		Hit:           1000,
		CrystalReward: 3,
		NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/1.cgi:80", CustomName: "custom-gem_buff_atk_1-gem_sky_atk_2",
			Gems: [3]string{"gem_buff_atk_1", "gem_sky_atk_2"}, GemCosts: [3]int{16, 40}},
	})
	nm15Blaze = nm15Enemy("nm15-blaze", "BZ-13", 1500, 350, 300, 250, 300, 100, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 400, CMP: 100, ItemDefinitionIDs: []string{"item-158"}},
		Jobs:          nm15Jobs("job-54", 999, "job-47", 999),
		Hit:           1000,
		CrystalReward: 3,
		NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/15/1.cgi:148", CustomName: "custom-gem_buff_atk_1-gem_divine_tension_sky",
			Gems: [3]string{"gem_buff_atk_1", "gem_divine_tension_sky"}, GemCosts: [3]int{16, 57}},
	})
	nm15Bomb = nm15Enemy("nm15-bomb", "チビボムA", 100, 50, 600, 900, 50, 1, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 42},
		Jobs:        nm15Jobs("job-94", 10, "", 0),
	})
)

// crossBossEvent is _2's J and K (_2.cgi:28–29,43–57): the first of the two
// kills j or k, the walls open by draw or once both fired, then one of four
// groups spawns.
type crossBossEvent struct{ groups [4]DungeonMonster }

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
	if !st.has('j') && !st.has('k') && !bothFired {
		kill := byte('j')
		if 2*n.rng.Float64() >= 1 {
			kill = 'k'
		}
		st.Flags += string(kill)
	}
	if !st.has('9') && (bothFired || 2*n.rng.Float64() < 1) {
		st.Flags += "9"
	}
	pick := 2
	if 2*n.rng.Float64() < 1 {
		pick = 0
	}
	if 2*n.rng.Float64() >= 1 {
		pick++
	}
	n.step.MapState = st.encode()
	return fixedBoss(ev.groups[pick]).apply(n)
}

// liveTreasure is a _2 treasure that the first boss may have killed.
type liveTreasure struct{}

func (liveTreasure) visible(exp *ActiveExpedition, cell byte) (bool, error) {
	st, err := decodeMap15(exp.MapState)
	return !st.has(cell), err
}

func (ev liveTreasure) apply(n *nativeStep) error {
	live, err := ev.visible(n.exp, n.cell)
	if err != nil || !live {
		return err
	}
	return treasure.apply(n)
}

// The _1 outcome 5 group is __data.cgi's @bosses; __1's X group is _data.cgi's.
var (
	nm15Hazard = nm15Enemy("nm15-grand-hazard", "GN-2000", 8000, 430, 450, 260, 1800, 1800, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 2000, ItemDefinitionIDs: []string{"item-161"}},
		Jobs:          nm15Jobs("job-501", 999, "", 0),
		CrystalReward: 10,
		Temporary:     corebattle.TemporaryReflectAttack,
	})
	nm15ShieldedBomb = nm15Enemy("nm15-shielded-bomb", "チビボムA", 100, 50, 600, 900, 50, 1, corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 42},
		Jobs:        nm15Jobs("job-94", 10, "", 0),
		Temporary:   corebattle.TemporaryNullMagic,
	})
	nm15Core = nm15Enemy("nm15-core", "XS-0", 33000, 460, 500, 180, 8000, 8000, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 12000, MaxMP: 20000, ItemDefinitionIDs: []string{"item-208"}},
		Jobs:          nm15Jobs("job-503", 999, "", 0),
		CrystalReward: 17,
		Temporary:     corebattle.TemporaryNullAttack,
	})
)

// _2's J/K groups 1–4 and Y's group 5 (_2.cgi:66–363); group 4 has one member.
var (
	nm15SouthGroups = [4]DungeonMonster{
		nm15Enemy("nm15-grand-hazard-2", "GN-2000", 10000, 500, 520, 260, 1800, 1800, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 2000, ItemDefinitionIDs: []string{"item-159"}},
			Jobs:          nm15Jobs("job-501", 999, "", 0),
			CrystalReward: 10,
			Temporary:     corebattle.TemporaryReflectAttack,
		}),
		nm15Enemy("nm15-blaze-2", "BZ-13", 4000, 300, 280, 250, 300, 100, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 400, ItemDefinitionIDs: []string{"item-158"}},
			Jobs:          nm15Jobs("job-54", 999, "job-47", 999),
			Hit:           1000,
			CrystalReward: 4,
		}),
		nm15Enemy("nm15-cyber-armor", "CB-40B", 4000, 380, 400, 130, 500, 222, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 811, ItemDefinitionIDs: []string{"item-192"}},
			Jobs:          nm15Jobs("job-501", 50, "job-04", 999),
			CrystalReward: 3,
			Temporary:     corebattle.TemporaryReflectAttack,
		}),
		nm15Enemy("nm15-sky-slaughter-2", "SK-51", 2000, 400, 80, 400, 200, 56, corebattle.LifetimeActor{
			Participant:   corebattle.Participant{MP: 1200, ItemDefinitionIDs: []string{"item-183"}},
			Jobs:          nm15Jobs("job-502", 999, "job-52", 999),
			CrystalReward: 2,
		}),
	}
	nm15Panzer = nm15Enemy("nm15-devil-panzer", "EV-9", 15000, 540, 460, 200, 1800, 500, corebattle.LifetimeActor{
		Participant:   corebattle.Participant{MP: 8000, MaxMP: 10000, ItemDefinitionIDs: []string{"item-204"}},
		Jobs:          nm15Jobs("job-503", 10, "job-501", 200),
		CrystalReward: 13,
	})
)

// nm15MainPool is the main floor's A–C outcomes (1.cgi:33–56).
var nm15MainPool = exclusiveEvent{order: "234", outcomes: map[byte]nativeEvent{
	'2': choiceEvent{odds: 2, outcomes: [2]nativeEvent{fixedBoss(nm15Inferno), fixedBoss(nm15Blaze)}},
	'3': fixedBoss(nm15Bomb),
	'4': stageMonster,
}}

// nm15NorthPool is _1's D–F outcomes (_1.cgi:43–67).
var nm15NorthPool = exclusiveEvent{order: "567", outcomes: map[byte]nativeEvent{
	'5': fixedBoss(nm15Hazard),
	'6': fixedBoss(nm15ShieldedBomb),
	'7': recoveryOutcome{base: 100},
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
		{Pools: pool, RoundLimit: 60,
			Exits: map[byte]FloorExit{'3': {Variants: []int{4}}},
			Marks: map[byte]string{'D': "◎", 'E': "◎", 'F': "◎", 'a': "宝", 'd': "■", '3': "凸"},
			events: map[byte]mapEvent{'D': {action: nm15NorthPool, perSymbol: true}, 'E': {action: nm15NorthPool, perSymbol: true}, 'F': {action: nm15NorthPool, perSymbol: true},
				'a': {action: treasure, perSymbol: true}, 'd': {action: gateEvent{open: '5'}}}},
		// shortcut: chests hold no contents until #2451 binds map15's sources, _2's own pool included.
		// _2 and __1 share the z flag safely: the branches never meet.
		{Pools: pool, RoundLimit: 50,
			Marks: map[byte]string{'J': "◎", 'K': "◎", 'Y': "◎", 'z': "宝", 'j': "宝", 'k': "宝", 'd': "■"},
			events: map[byte]mapEvent{'J': {action: crossBossEvent{groups: nm15SouthGroups}, perSymbol: true}, 'K': {action: crossBossEvent{groups: nm15SouthGroups}, perSymbol: true},
				'Y': {action: fortressBoss{boss: nm15Panzer}, perSymbol: true}, 'd': {action: gateEvent{open: '9'}},
				'z': {action: treasure, perSymbol: true}, 'j': {action: liveTreasure{}, perSymbol: true}, 'k': {action: liveTreasure{}, perSymbol: true}}},
		// __1's passages spawn nothing (event_0 is empty).
		{RoundLimit: 90,
			Marks: map[byte]string{'w': "宝", 'z': "宝", 'v': "宝", 'G': "○", 'X': "◎"},
			events: map[byte]mapEvent{'0': {action: passageEvent{}}, 'G': {action: recoveryOutcome{base: 300}, perSymbol: true},
				'X': {action: fortressBoss{boss: nm15Core}, perSymbol: true}, 'z': {action: fixedTreasureEvent{}, perSymbol: true},
				'w': {action: treasure, perSymbol: true}, 'v': {action: treasure, perSymbol: true}}},
	}
	variants := make([]Floor, len(floors))
	for i, f := range floors {
		f.FloorNumber = 1
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
