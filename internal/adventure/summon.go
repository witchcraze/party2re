package adventure

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// SummonPool preserves the current (including seasonal) stage table order.
// The normal-floor filter is inappropriate here: the caller supplies separate
// source appearance positions, including repeats, to RosterContext. Plain
// Monster stats cannot certify omitted job/custom/status payloads (#1342).
func (s Stage) SummonPool(payloads map[string]corebattle.LifetimeActor) ([]corebattle.LifetimeActor, error) {
	if len(s.MonsterIDs) == 0 {
		return nil, corebattle.ErrUnsupportedEffect
	}
	pool := make([]corebattle.LifetimeActor, 0, len(s.MonsterIDs))
	for _, id := range s.MonsterIDs {
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
