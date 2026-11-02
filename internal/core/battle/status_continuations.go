package battle

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
)

// These bound admission work, never the source's independent outcomes. An
// incomplete enumeration rejects the entire action before execution or RNG.
const maxStatusContinuations = 4096

type statusContinuations struct {
	branches  []StatusContinuation
	byState   map[[32]byte][]int
	remaining *int
}

func (set *statusContinuations) add(s LifetimeState, c StatusContext) error {
	encoded, err := json.Marshal(s)
	if err != nil {
		return ErrUnsupportedEffect
	}
	key := sha256.Sum256(encoded)
	facts := func(c StatusContext) StatusContext { c.Damage, c.targetResources, c.Defeat = nil, nil, nil; return c }
	cursor := func(c StatusContext) TargetCursor {
		if c.Defeat != nil {
			return c.Defeat.Cursor
		}
		return TargetCursor{}
	}
	for _, index := range set.byState[key] {
		old := set.branches[index]
		// Serialization is only an index: omitted facts and hash collisions must
		// still compare exactly before any reachable outcome is coalesced.
		if cursor(old.Context) == cursor(c) && reflect.DeepEqual(facts(old.Context), facts(c)) && reflect.DeepEqual(old.State, s) {
			return nil
		}
	}
	if len(set.branches) >= maxStatusContinuations {
		return ErrUnsupportedEffect
	}
	if set.remaining != nil {
		if *set.remaining <= 0 {
			return ErrUnsupportedEffect
		}
		*set.remaining -= 1
	}
	if set.byState == nil {
		set.byState = make(map[[32]byte][]int)
	}
	f := cloneCallbackContext(CallbackContext{Status: &c})
	set.byState[key] = append(set.byState[key], len(set.branches))
	set.branches = append(set.branches, StatusContinuation{State: s.callbackCopy(), Context: *f.Status})
	return nil
}
