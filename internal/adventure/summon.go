package adventure

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// SummonPool preserves the current (including seasonal) stage table order.
// The normal-floor filter is inappropriate here: the caller supplies separate
// source appearance positions, including repeats, to RosterContext. A dynamic
// stage15 table uses the frozen NormalRun.EncounterPool directly.
func (s Stage) SummonPool(payloads map[string]corebattle.LifetimeActor) ([]corebattle.LifetimeActor, error) {
	ids := s.MonsterIDs
	if s.Ordinary != nil {
		ids = s.Ordinary.MonsterIDs
	}
	if len(ids) == 0 {
		return nil, corebattle.ErrUnsupportedEffect
	}
	pool := make([]corebattle.LifetimeActor, 0, len(ids))
	for _, id := range ids {
		a, ok := payloads[id]
		if !ok {
			return nil, corebattle.ErrUnsupportedEffect
		}
		if a.ID != id {
			return nil, ErrInvalidMonster
		}
		pool = append(pool, a)
	}
	return pool, nil
}
