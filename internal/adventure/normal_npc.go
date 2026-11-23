package adventure

import (
	"slices"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// NextNPC selects in semantic order, including skipped heads. It is a read:
// rejected/ineligible operations and GET never change order or timestamps.
// Treasure-room NPC response is disabled (vs_monster.cgi:38–41).
func (r *NormalRun) NextNPC(now time.Time) string {
	if isTreasureFloor(r.Round) {
		return ""
	}
	for _, id := range r.Enemies {
		a := r.Actor(id)
		if a != nil && a.NPC && a.HP > 0 && now.Sub(r.LastActions[id]) >= time.Duration(r.Admission.Speed)*time.Second {
			return id
		}
	}
	return ""
}

// RecordNPCResponse preserves source head rotation, not acted-member removal.
// Call after successful execution, within the command's existing transaction.
func (r *NormalRun) RecordNPCResponse(id string, now time.Time) error {
	a := r.Actor(id)
	if id == "" || !slices.Contains(r.Enemies, id) || a == nil || !a.NPC {
		return runhost.ErrConflict
	}
	if r.LastActions == nil {
		r.LastActions = make(map[string]time.Time)
	}
	r.LastActions[id] = now
	if len(r.Enemies) > 1 {
		r.Enemies = append(append([]string(nil), r.Enemies[1:]...), r.Enemies[0])
		// The next formation reads the same opposing order from the member scan.
		ordered := make([]corebattle.LifetimeActor, 0, len(r.Enemies))
		for _, id := range r.Enemies {
			if a := r.Actor(id); a != nil {
				ordered = append(ordered, *a)
			}
		}
		for i, a := range r.Battle.Actors {
			if slices.Contains(r.Enemies, a.ID) {
				r.Battle.Actors[i], ordered = ordered[0], ordered[1:]
			}
		}
	}
	return nil
}
