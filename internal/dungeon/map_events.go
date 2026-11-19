package dungeon

import "fmt"

// Like a legacy map file overriding event_<symbol>, each floor binds its own
// symbols to events; a symbol has no meaning beyond its floor. Unbound symbols
// fall back to the base events of vs_dungeon.cgi.
type eventKind uint8

const (
	_              eventKind = iota // zero value: no event
	eventHidden                     // traversable, displayed as a wall
	eventSpawn                      // event_0: rand(2) <= 1 spawns an encounter
	eventBoss                       // once: Floor.Bosses[symbol], else the event's pool boss
	eventTreasure                   // once: unopened chests
	eventKey                        // once; never consumed by its door
	eventDoor                       // opens with its key or an acting job 9; locked spends the round in place
	eventMist                       // every entry: party confusion, then an encounter
	eventAttrition                  // the mover loses int(MaxHP × 0.02 + 0.5) HP
	eventRecovery                   // once: one random member's HP or MP
	eventMessage                    // once: narration only
)

type cellEvent struct {
	kind eventKind
	// perSymbol shares one once-state across every cell and floor with this
	// symbol, like a legacy flag; otherwise each cell has its own.
	perSymbol bool
	key       byte // eventDoor: the key event's symbol
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
	if ev.perSymbol {
		return string(cell)
	}
	return fmt.Sprintf("%c:%d:%d", cell, x, y)
}
