package challenge

import (
	"context"
	"encoding/json"
	"sync"
)

// MemoryTransitionRepository is an explicitly selected test adapter.
type MemoryTransitionRepository struct {
	mu      sync.Mutex
	states  map[string]Transition
	claims  map[string]string
	results map[string]map[string]map[int]CommandResult
}
type memoryRunLock struct{ repo *MemoryTransitionRepository }

func NewMemoryTransitionRepository() *MemoryTransitionRepository {
	return &MemoryTransitionRepository{states: map[string]Transition{}, claims: map[string]string{}, results: map[string]map[string]map[int]CommandResult{}}
}
func (r *MemoryTransitionRepository) lock(ctx context.Context) func() {
	if ctx.Value(memoryRunLock{r}) != nil {
		return func() {}
	}
	r.mu.Lock()
	return r.mu.Unlock
}
func (r *MemoryTransitionRepository) WithRunLock(ctx context.Context, id string, fn func(context.Context) error) error {
	unlock := r.lock(ctx)
	defer unlock()
	data, err := json.Marshal(struct {
		States  map[string]Transition
		Claims  map[string]string
		Results map[string]map[string]map[int]CommandResult
	}{r.states, r.claims, r.results})
	if err != nil {
		return err
	}
	err = fn(context.WithValue(ctx, memoryRunLock{r}, true))
	if err != nil {
		var old struct {
			States  map[string]Transition
			Claims  map[string]string
			Results map[string]map[string]map[int]CommandResult
		}
		if decodeErr := json.Unmarshal(data, &old); decodeErr != nil {
			return decodeErr
		}
		r.states, r.claims, r.results = old.States, old.Claims, old.Results
	}
	return err
}
func (r *MemoryTransitionRepository) FindTransition(ctx context.Context, id string) (Transition, error) {
	unlock := r.lock(ctx)
	defer unlock()
	data, err := json.Marshal(r.states[id])
	if err != nil {
		return Transition{}, err
	}
	var cp Transition
	if err := json.Unmarshal(data, &cp); err != nil {
		return Transition{}, err
	}
	return cp, nil
}
func (r *MemoryTransitionRepository) FindClaim(ctx context.Context, actor string) (string, error) {
	unlock := r.lock(ctx)
	defer unlock()
	return r.claims[actor], nil
}
func (r *MemoryTransitionRepository) FindCommandResult(ctx context.Context, run, actor string, rev int) (*CommandResult, error) {
	unlock := r.lock(ctx)
	defer unlock()
	x, ok := r.results[run][actor][rev]
	if !ok {
		return nil, nil
	}
	data, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	var cp CommandResult
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
func (r *MemoryTransitionRepository) EnrollRun(ctx context.Context, s ChallengeSession) error {
	unlock := r.lock(ctx)
	defer unlock()
	for _, m := range s.Members {
		if r.claims[m.CharacterID] != "" {
			return ErrActiveSessionExists
		}
	}
	for _, m := range s.Members {
		r.claims[m.CharacterID] = s.ID
	}
	return nil
}
func (r *MemoryTransitionRepository) SaveTransition(ctx context.Context, t Transition, result *CommandResult) error {
	unlock := r.lock(ctx)
	defer unlock()
	old := r.states[t.RunID]
	if old.Pending || old.Revision+1 != t.Revision {
		return ErrRevisionConflict
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	var cp Transition
	if err := json.Unmarshal(data, &cp); err != nil {
		return err
	}
	r.states[t.RunID] = cp
	if result != nil {
		if r.results[t.RunID] == nil {
			r.results[t.RunID] = map[string]map[int]CommandResult{}
		}
		if r.results[t.RunID][result.ActorID] == nil {
			r.results[t.RunID][result.ActorID] = map[int]CommandResult{}
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		var cp CommandResult
		if err := json.Unmarshal(data, &cp); err != nil {
			return err
		}
		r.results[t.RunID][result.ActorID][result.ExpectedRevision] = cp
	}
	return nil
}
func (r *MemoryTransitionRepository) AcknowledgePublication(ctx context.Context, id string, rev int) error {
	unlock := r.lock(ctx)
	defer unlock()
	t := r.states[id]
	if t.Revision != rev {
		return ErrRevisionConflict
	}
	for _, actor := range t.RemovedIDs {
		if r.claims[actor] == id {
			delete(r.claims, actor)
		}
	}
	t.Pending = false
	t.Publication = nil
	r.states[id] = t
	return nil
}
