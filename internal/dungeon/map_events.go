package dungeon

import (
	"fmt"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Each floor binds its own symbols, with unbound symbols falling back to
// the base events of vs_dungeon.cgi.
type nativeEvent interface {
	apply(*nativeStep) error
}

type mapEvent struct {
	action nativeEvent
	// flag overrides the cell key so different symbols can share once-state.
	flag string
	// perSymbol shares once-state across cells and floors with the same symbol.
	perSymbol bool
	// keepMark preserves the mark after the event is spent (map5 lever).
	keepMark bool
}

const (
	cellWall  = '1'
	cellStart = 'S'
)

var baseEvents = map[byte]mapEvent{'0': {action: spawnEvent{}}, 'B': {action: bossEvent{}}}

func (f Floor) event(cell byte) mapEvent {
	if ev, ok := f.events[cell]; ok {
		return ev
	}
	if ev, ok := baseEvents[cell]; ok {
		return ev
	}
	return mapEvent{action: passageEvent{}}
}

func (ev mapEvent) stateKey(cell byte, x, y int) string {
	if ev.flag != "" {
		return ev.flag
	}
	if ev.perSymbol {
		return string(cell)
	}
	return fmt.Sprintf("%c:%d:%d", cell, x, y)
}

// conditionalWall blocks movement like a wall while closed (map17 9).
type conditionalWall interface {
	closed(*ActiveExpedition) (bool, error)
}

func (ev mapEvent) tileSymbol(exp *ActiveExpedition, cell byte, x, y int, mark, wall string) (string, error) {
	if shown, ok := ev.action.(interface {
		wallShown(*ActiveExpedition) (bool, error)
	}); ok {
		if isWall, err := shown.wallShown(exp); err != nil || isWall {
			return wall, err
		}
	}
	if dynamic, ok := ev.action.(interface {
		mark(*ActiveExpedition, byte) (string, error)
	}); ok {
		return dynamic.mark(exp, cell)
	}
	if gated, ok := ev.action.(interface {
		visible(*ActiveExpedition, byte) (bool, error)
	}); ok {
		visible, err := gated.visible(exp, cell)
		if err != nil || !visible {
			return "□", err
		}
	}
	hidden, isHidden := ev.action.(hiddenEvent)
	switch {
	case cell == cellWall || isHidden && (hidden.key == 0 || !slices.Contains(exp.ConsumedEvents, string(hidden.key))):
		return wall, nil
	case mark != "" && (ev.keepMark || !slices.Contains(exp.ConsumedEvents, ev.stateKey(cell, x, y))):
		return mark, nil
	default:
		return "□", nil
	}
}

type passageEvent struct{}

func (passageEvent) apply(n *nativeStep) error {
	if to, ok := n.floor.Warps[n.cell]; ok {
		n.step.NewX, n.step.NewY = to[0], to[1]
	}
	if exit, ok := n.floor.Exits[n.cell]; ok {
		// The destination is drawn first, then each member's damage (map/2 event_T).
		n.step.MapVariant = exit.Variants[n.rng.Intn(len(exit.Variants))]
		n.step.RoundLimit = n.dungeon.limitOf(n.dungeon.Variants[n.step.MapVariant-1])
		if exit.Trap > 0 {
			partyTrap(&n.step, n.exp, n.leader.ID, float64(exit.Trap), n.rng)
		}
	}
	return nil
}

type hiddenEvent struct{ key byte }

func (hiddenEvent) apply(*nativeStep) error { return nil }

type spawnEvent struct{ status corebattle.StatusEffect }

func (ev spawnEvent) apply(n *nativeStep) error {
	if 2*n.rng.Float64() <= 1 {
		if pool := n.encounterPool(); len(pool.Monsters) > 0 {
			n.fightGroup(pool, "", ev.status)
		}
	}
	return nil
}

type mistEvent struct{ status corebattle.StatusEffect }

func (ev mistEvent) apply(n *nativeStep) error {
	n.fightGroup(n.encounterPool(), "", ev.status)
	return nil
}

type bossEvent struct {
	status corebattle.StatusEffect
	pools  []EncounterPool
}

func (ev bossEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	if roster, ok := n.floor.Bosses[n.cell]; ok {
		n.fightRoster(roster, n.key, ev.status)
		return nil
	}
	var pool EncounterPool
	if len(ev.pools) > 0 {
		pool = ev.pools[n.rng.Intn(len(ev.pools))]
	} else {
		pool = n.encounterPool()
	}
	if len(pool.Bosses) > 0 {
		n.fightRoster(pool.Bosses, n.key, ev.status)
	}
	return nil
}

type keyEvent struct{}

func (keyEvent) apply(n *nativeStep) error {
	if !n.consumed {
		n.step.EventKey = n.key
	}
	return nil
}

type messageEvent struct{ keyEvent }

type doorEvent struct {
	key  byte
	jobs []string
}

func (ev doorEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	if slices.Contains(ev.jobs, n.actor.JobID) || slices.Contains(n.exp.ConsumedEvents, string(ev.key)) {
		n.step.EventKey = n.key
	} else {
		// Locked: the round is spent and the party returns to its only approach.
		n.step.NewX, n.step.NewY = n.exp.PosX, n.exp.PosY
	}
	return nil
}

type treasureEvent struct{}

func (treasureEvent) apply(n *nativeStep) error {
	if !n.consumed {
		n.step.EventKey = n.key
		n.step.Encounter = treasureEncounter(n.exp, n.actor.JobID, n.rng, n.step.NewX, n.step.NewY)
	}
	return nil
}

type attritionEvent struct{}

func (attritionEvent) apply(n *nativeStep) error {
	if loss := attritionLoss(n.exp, n.actor); loss > 0 {
		n.step.MemberHPDeltas = map[string]int{n.actor.ID: -loss}
		if n.actor.ID == n.leader.ID {
			n.step.HPDelta = -loss
		}
	}
	return nil
}

type recoveryEvent struct{}

func (recoveryEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	hp, mp, err := recoverOne(n.exp, n.actor, n.rng)
	if err != nil {
		return err
	}
	n.step.EventKey = n.key
	n.step.MemberHPDeltas, n.step.MemberMPDeltas = hp, mp
	n.step.HPDelta = hp[n.leader.ID]
	n.recovery = true
	return nil
}

type trapEvent struct {
	base   int
	random bool
}

func (ev trapEvent) apply(n *nativeStep) error {
	base := float64(ev.base)
	if ev.random {
		base *= n.rng.Float64()
	}
	partyTrap(&n.step, n.exp, n.leader.ID, base, n.rng)
	return nil
}

type remedyEvent struct{}

func (remedyEvent) apply(n *nativeStep) error {
	if n.consumed || n.actor.JobID != jobRemedy {
		return nil
	}
	n.step.EventKey = n.key
	n.step.MemberHPDeltas, n.step.MemberMPDeltas = fullRestore(n.exp)
	n.step.HPDelta = n.step.MemberHPDeltas[n.leader.ID]
	n.recovery = true
	return nil
}

type choiceEvent struct {
	odds     int
	outcomes [2]nativeEvent
}

func (ev choiceEvent) apply(n *nativeStep) error {
	if n.consumed {
		return nil
	}
	outcome := ev.outcomes[0]
	if ev.odds > 0 && float64(ev.odds)*n.rng.Float64() >= 1 {
		outcome = ev.outcomes[1]
	}
	n.step.EventKey = n.key
	if outcome != nil {
		return outcome.apply(n)
	}
	return nil
}

// monsterOutcome is _add_monster over a map-owned @monsters list, or over
// the drawn pool when empty.
type monsterOutcome struct{ monsters []DungeonMonster }

func (o monsterOutcome) apply(n *nativeStep) error {
	pool := EncounterPool{Monsters: o.monsters}
	if len(o.monsters) == 0 {
		pool = n.encounterPool()
	}
	n.fightGroup(pool, n.key, "")
	return nil
}

// bossOutcome is add_boss over a map-owned @bosses roster, or over the drawn
// pool's when empty.
type bossOutcome struct{ roster []DungeonMonster }

func (o bossOutcome) apply(n *nativeStep) error {
	roster := o.roster
	if len(roster) == 0 {
		roster = n.encounterPool().Bosses
	}
	n.fightRoster(roster, n.key, "")
	return nil
}

type healOutcome struct{ base int }

func (o healOutcome) apply(n *nativeStep) error {
	hp, err := partyHeal(n.exp, n.actor, float64(o.base)*n.rng.Float64(), n.rng)
	if err != nil {
		return err
	}
	n.step.MemberHPDeltas, n.step.HPDelta = hp, hp[n.leader.ID]
	n.recovery = true
	return nil
}

type goldOutcome int

func (goldOutcome) apply(*nativeStep) error {
	// shortcut: the wallet change lands with #2417; map6 stays unlisted until then.
	return nil
}
