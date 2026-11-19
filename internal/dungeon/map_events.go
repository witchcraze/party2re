package dungeon

import (
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Like a legacy map file overriding event_<symbol>, each floor binds its own
// symbols to events; a symbol has no meaning beyond its floor. Unbound symbols
// fall back to the base events of vs_dungeon.cgi.
type eventKind uint8

const (
	_              eventKind = iota // zero value: no event
	eventHidden                     // traversable, displayed as a wall until its key event is spent
	eventSpawn                      // event_0: rand(2) <= 1 spawns an encounter
	eventBoss                       // once: Floor.Bosses[symbol], else the event's pool boss
	eventTreasure                   // once: unopened chests
	eventKey                        // once; never consumed by its door
	eventDoor                       // opens with its key or an acting job in jobs; locked spends the round in place
	eventMist                       // every entry: a party status for the ensuing encounter
	eventAttrition                  // the mover loses int(MaxHP × 0.02 + 0.5) HP
	eventRecovery                   // once: one random member's HP or MP
	eventMessage                    // once: narration only
	eventTrap                       // every entry: _trap_d on the whole roster
	eventRemedy                     // once, by an acting job 43: the roster to full HP and MP
	eventChoice                     // once per flag: draws one of two outcomes (map6 pairs)
)

type cellEvent struct {
	kind eventKind
	// perSymbol shares one once-state across every cell and floor with this
	// symbol, like a legacy flag; otherwise each cell has its own.
	perSymbol bool
	// keepMark keeps the floor's mark after the event is spent (map5 lever).
	keepMark bool
	key      byte                    // eventDoor/eventHidden: the symbol whose flag opens/reveals it
	jobs     []string                // eventDoor: the acting character's jobs that open it
	status   corebattle.StatusEffect // eventMist
	base     int                     // eventTrap: _trap_d base damage
	// flag, when set, is the once-state shared by every symbol carrying it
	// (map6 A/B share "AB").
	flag string
	// eventChoice: outcomes[0] when rand(odds) < 1, else outcomes[1]; odds 0
	// always takes outcomes[0].
	odds     int
	outcomes [2]choiceOutcome
}

type outcomeKind uint8

const (
	_               outcomeKind = iota // zero value: flag only
	outcomeMonster                     // add_monster: the fixed monster, else the event's stage pool
	outcomeBoss                        // add_boss: the fixed group's representative, else the pool boss
	outcomeTreasure                    // _add_treasure
	outcomeTrap                        // _trap_d on the whole roster
	outcomeHeal                        // _heals(base, '無') on the living roster
	outcomeGold                        // the mover's wallet; see #2417
)

type choiceOutcome struct {
	kind    outcomeKind
	monster *DungeonMonster
	// base is the trap/heal base, with random the bound of rand(base); for
	// gold, +1 is a pickup and -1 a loss.
	base   int
	random bool
}

// The base wall and start; every other symbol is passable.
const (
	cellWall  = '1'
	cellStart = 'S'
)

var baseEvents = map[byte]cellEvent{'0': {kind: eventSpawn}, 'B': {kind: eventBoss}}

func (f Floor) event(cell byte) cellEvent {
	if ev, ok := f.events[cell]; ok {
		return ev
	}
	return baseEvents[cell]
}

func (ev cellEvent) stateKey(cell byte, x, y int) string {
	if ev.flag != "" {
		return ev.flag
	}
	if ev.perSymbol {
		return string(cell)
	}
	return fmt.Sprintf("%c:%d:%d", cell, x, y)
}
