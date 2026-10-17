package dungeon

import (
	"context"
	"sync"
	"time"
)

// MemoryExpeditionRepository provides an in-memory, thread-safe implementation
// of ActiveExpeditionStore matching the exact semantics and error codes of the Valkey Lua script.
type MemoryExpeditionRepository struct {
	mu           sync.RWMutex
	expeditions  map[string]ActiveExpedition
	expirations  map[string]time.Time
	memberLeader map[string]string
	ttl          time.Duration
}

// NewMemoryExpeditionRepository creates a new thread-safe in-memory store.
func NewMemoryExpeditionRepository(opts ...ValkeyOption) *MemoryExpeditionRepository {
	repo := &MemoryExpeditionRepository{
		expeditions:  make(map[string]ActiveExpedition),
		expirations:  make(map[string]time.Time),
		memberLeader: make(map[string]string),
		ttl:          DefaultExpeditionTTL,
	}
	for _, opt := range opts {
		cfg := &valkeyConfig{ttl: repo.ttl}
		opt(cfg)
		repo.ttl = cfg.ttl
	}
	return repo
}

func (m *MemoryExpeditionRepository) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.expeditions[characterID]
	if ok {
		expTime, hasExp := m.expirations[characterID]
		if hasExp && time.Now().UTC().After(expTime) {
			m.deleteExpeditionLocked(characterID)
			return nil, nil
		}

		cp := exp
		cp.AccumulatedItems = append([]string(nil), exp.AccumulatedItems...)
		cp.Members = append([]ExpeditionMember(nil), exp.Members...)
		return &cp, nil
	}

	leaderID, hasLeader := m.memberLeader[characterID]
	if !hasLeader {
		return nil, nil
	}

	exp, ok = m.expeditions[leaderID]
	if !ok {
		delete(m.memberLeader, characterID)
		return nil, nil
	}

	expTime, hasExp := m.expirations[leaderID]
	if hasExp && time.Now().UTC().After(expTime) {
		m.deleteExpeditionLocked(leaderID)
		return nil, nil
	}

	// Privacy check: verify characterID is an authorized member of this expedition
	found := false
	for _, mem := range exp.Members {
		if mem.CharacterID == characterID {
			found = true
			break
		}
	}
	if !found {
		delete(m.memberLeader, characterID)
		return nil, nil
	}

	cp := exp
	cp.AccumulatedItems = append([]string(nil), exp.AccumulatedItems...)
	cp.Members = append([]ExpeditionMember(nil), exp.Members...)
	return &cp, nil
}

func (m *MemoryExpeditionRepository) SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := exp
	cp.AccumulatedItems = append([]string(nil), exp.AccumulatedItems...)
	cp.Members = append([]ExpeditionMember(nil), exp.Members...)
	m.expeditions[exp.CharacterID] = cp
	m.expirations[exp.CharacterID] = time.Now().UTC().Add(m.ttl)

	for _, mem := range exp.Members {
		if mem.CharacterID != exp.CharacterID {
			m.memberLeader[mem.CharacterID] = exp.CharacterID
		}
	}
	return nil
}

func (m *MemoryExpeditionRepository) DeleteActiveExpedition(ctx context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.expeditions[characterID]; ok {
		m.deleteExpeditionLocked(characterID)
		return nil
	}
	if leaderID, ok := m.memberLeader[characterID]; ok {
		m.deleteExpeditionLocked(leaderID)
		return nil
	}
	return nil
}

func (m *MemoryExpeditionRepository) deleteExpeditionLocked(leaderID string) {
	if exp, ok := m.expeditions[leaderID]; ok {
		for _, mem := range exp.Members {
			delete(m.memberLeader, mem.CharacterID)
		}
	}
	delete(m.expeditions, leaderID)
	delete(m.expirations, leaderID)
}

func (m *MemoryExpeditionRepository) Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetID := characterID
	if _, ok := m.expeditions[targetID]; !ok {
		if lid, hasLeader := m.memberLeader[targetID]; hasLeader {
			targetID = lid
		}
	}

	exp, ok := m.expeditions[targetID]
	if !ok {
		return StepOutcome{}, ErrExpeditionNotFound
	}

	expTime, hasExp := m.expirations[targetID]
	if hasExp && time.Now().UTC().After(expTime) {
		m.deleteExpeditionLocked(targetID)
		return StepOutcome{}, ErrExpeditionNotFound
	}

	if exp.ID != params.ExpectedExpeditionID {
		return StepOutcome{}, ErrExpeditionIDMismatch
	}

	if exp.Status != StatusExploring {
		return StepOutcome{}, ErrExpeditionNotActive
	}

	currentHP := exp.CurrentHP + params.HPDelta
	turnsRemaining := exp.TurnsRemaining + params.TurnsDelta

	if len(params.MemberHPDeltas) > 0 {
		allDead := true
		for i := range exp.Members {
			if delta, ok := params.MemberHPDeltas[exp.Members[i].CharacterID]; ok {
				exp.Members[i].CurrentHP += delta
				if exp.Members[i].CurrentHP < 0 {
					exp.Members[i].CurrentHP = 0
				}
			}
			if exp.Members[i].CurrentHP > 0 {
				allDead = false
			}
		}
		if len(exp.Members) > 0 && allDead {
			currentHP = 0
		}
	}

	finalStatus := StatusExploring
	if currentHP <= 0 {
		currentHP = 0
		finalStatus = StatusWipedOut
	} else if turnsRemaining <= 0 {
		turnsRemaining = 0
		finalStatus = StatusWipedOut
	}

	exp.CurrentFloor = params.NewFloor
	exp.PosX = params.NewX
	exp.PosY = params.NewY
	exp.CurrentHP = currentHP
	exp.TurnsRemaining = turnsRemaining
	exp.Status = finalStatus
	exp.AccumulatedExp += params.ExpDelta
	exp.AccumulatedGold += params.GoldDelta
	exp.AccumulatedMedals += params.MedalsDelta
	if params.RewardItemID != "" {
		exp.AccumulatedItems = append(exp.AccumulatedItems, params.RewardItemID)
	}
	exp.UpdatedAt = params.Now

	m.expeditions[targetID] = exp
	m.expirations[targetID] = time.Now().UTC().Add(m.ttl)

	cp := exp
	cp.AccumulatedItems = append([]string(nil), exp.AccumulatedItems...)
	cp.Members = append([]ExpeditionMember(nil), exp.Members...)
	return StepOutcome{
		Expedition: cp,
		Status:     finalStatus,
	}, nil
}
