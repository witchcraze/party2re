package dungeon

import (
	"encoding/json"
	"fmt"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// NativeMap15ID is the fortress branches (legacy map/15).
const NativeMap15ID = "native-map-15"

// nativeMap15Floors transcribe map/15/1.cgi, _1.cgi and _2.cgi.
var nativeMap15Floors = [3][]string{
	{"00200", "01010", "01010", "B1A1C", "01010", "00000", "11S11", "11011", "10401"},
	{"11011", "00000", "01010", "E1D1F", "01d10", "d0a0d", "11311", "11111", "11111"},
	{"11111", "11z11", "11011", "10Y01", "1d1d1", "1j1k1", "10101", "1J1K1", "10001"},
}

// map15State records the pool outcomes already drawn on any floor.
type map15State struct {
	Drawn string `json:"drawn,omitempty"`
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
	var st map15State
	if len(n.exp.MapState) > 0 {
		if err := json.Unmarshal(n.exp.MapState, &st); err != nil {
			return fmt.Errorf("dungeon: map15 state: %w", err)
		}
	}
	var left []byte
	for i := range len(ev.order) {
		if strings.IndexByte(st.Drawn, ev.order[i]) < 0 {
			left = append(left, ev.order[i])
		}
	}
	n.step.EventKey = n.key
	if len(left) == 0 {
		return nil // unreachable: each floor has as many triggers as outcomes
	}
	pick := left[n.rng.Intn(len(left))]
	st.Drawn += string(pick)
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	n.step.MapState = raw
	return ev.outcomes[pick].apply(n)
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

// nm15MainPool is the main floor's A–C outcomes (1.cgi:33–56).
var nm15MainPool = exclusiveEvent{order: "234", outcomes: map[byte]nativeEvent{
	'2': choiceEvent{odds: 2, outcomes: [2]nativeEvent{fixedBoss(nm15Inferno), fixedBoss(nm15Blaze)}},
	'3': fixedBoss(nm15Bomb),
	'4': stageMonster,
}}

// nativeMap15 has the leader job level 50 of @ex_dungeon. The main floor's
// stairs 2 and 4 lead to _1 and _2, keeping the coordinate.
func nativeMap15() Dungeon {
	pool := []EncounterPool{nm15Pool}
	floors := [3]Floor{
		{Pools: pool,
			Exits:  map[byte]FloorExit{'2': {Variants: []int{2}}, '4': {Variants: []int{3}}},
			Marks:  map[byte]string{'A': "◎", 'B': "◎", 'C': "◎", '2': "凸", '4': "凸"},
			events: map[byte]mapEvent{'A': {action: nm15MainPool, perSymbol: true}, 'B': {action: nm15MainPool, perSymbol: true}, 'C': {action: nm15MainPool, perSymbol: true}}},
		// shortcut: _1 and _2 events are plain passages until #2442 and #2443 bind them.
		{Pools: pool, RoundLimit: 60},
		{Pools: pool, RoundLimit: 50},
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
