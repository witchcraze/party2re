package adventure

import (
	"slices"
	"time"
)

// NextNPC selects in semantic order, including skipped heads. It is a read:
// rejected/ineligible operations and GET never change order or timestamps.
func (r *NormalRun) NextNPC(now time.Time) string {
	for _, id := range r.Enemies {
		a := r.actor(id)
		if a != nil && a.NPC && a.HP > 0 && now.Sub(r.LastActions[id]) >= time.Duration(r.Admission.Speed)*time.Second {
			return id
		}
	}
	return ""
}

// RecordNPCResponse preserves source head rotation, not acted-member removal.
// Call after successful execution, within the command's existing transaction.
func (r *NormalRun) RecordNPCResponse(id string, now time.Time) error {
	a := r.actor(id)
	if id == "" || !slices.Contains(r.Enemies, id) || a == nil || !a.NPC {
		return ErrRunConflict
	}
	if r.LastActions == nil {
		r.LastActions = make(map[string]time.Time)
	}
	r.LastActions[id] = now
	if len(r.Enemies) > 1 {
		r.Enemies = append(append([]string(nil), r.Enemies[1:]...), r.Enemies[0])
	}
	return nil
}
