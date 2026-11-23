package runhost

import (
	"slices"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func (r *State) Actor(id string) *corebattle.LifetimeActor {
	return FindActor(r.Battle.Actors, id)
}

// Ready rejects an actor whose action interval has not elapsed at now.
func (r *State) Ready(actor string, now time.Time) error {
	if now.Before(r.LastActions[actor].Add(time.Duration(r.Admission.Speed) * time.Second)) {
		return ErrCooldown
	}
	return nil
}

// LivingEnemies uses persisted encounter membership, never coordinate-derived IDs.
func (r *State) LivingEnemies() bool {
	for _, box := range r.Boxes {
		if box.IsUnopened() {
			return true
		}
	}
	for _, id := range r.Enemies {
		if a := r.Actor(id); a != nil && a.HP > 0 {
			return true
		}
	}
	return false
}

// PruneFallenNPCs is write_member's save boundary: fallen NPCs and helpers leave
// the room, fallen real players keep their place (_battle.cgi:516–523).
func (r *State) PruneFallenNPCs(actorID string) {
	frame := corebattle.TargetContext{ActorID: actorID}
	r.Battle.PruneRoster(&frame)
	r.Enemies = slices.DeleteFunc(r.Enemies, func(id string) bool {
		if r.Actor(id) != nil {
			return false
		}
		delete(r.LastActions, id)
		return true
	})
}

// DiscardEncounter removes the current enemies, their timing and any boxes.
func (r *State) DiscardEncounter() {
	for _, id := range r.Enemies {
		delete(r.LastActions, id)
	}
	r.Battle.Actors = slices.DeleteFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return slices.Contains(r.Enemies, a.ID) })
	r.Enemies = nil
	r.Boxes = nil
}

func FindActor(actors []corebattle.LifetimeActor, id string) *corebattle.LifetimeActor {
	for i := range actors {
		if actors[i].ID == id {
			return &actors[i]
		}
	}
	return nil
}

func Reward(e Earnings) corebattle.Reward {
	return corebattle.Reward{Experience: e.EXP, Currency: e.Gold, Crystals: e.Crystals}
}
