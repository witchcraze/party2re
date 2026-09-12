package dungeon

import (
	"context"
	"sync"
	"time"
)

// MemoryExpeditionRepository provides an in-memory, thread-safe implementation
// of ActiveExpeditionStore matching the exact semantics and error codes of the Valkey Lua script.
type MemoryExpeditionRepository struct {
	mu          sync.RWMutex
	expeditions map[string]ActiveExpedition
	expirations map[string]time.Time
	ttl         time.Duration
}

// NewMemoryExpeditionRepository creates a new thread-safe in-memory store.
func NewMemoryExpeditionRepository(opts ...ValkeyOption) *MemoryExpeditionRepository {
	repo := &MemoryExpeditionRepository{
		expeditions: make(map[string]ActiveExpedition),
		expirations: make(map[string]time.Time),
		ttl:         DefaultExpeditionTTL,
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
	if !ok {
		return nil, nil
	}

	expTime, hasExp := m.expirations[characterID]
	if hasExp && time.Now().UTC().After(expTime) {
		delete(m.expeditions, characterID)
		delete(m.expirations, characterID)
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
	return nil
}

func (m *MemoryExpeditionRepository) DeleteActiveExpedition(ctx context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.expeditions, characterID)
	delete(m.expirations, characterID)
	return nil
}

func (m *MemoryExpeditionRepository) Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.expeditions[characterID]
	if !ok {
		return StepOutcome{}, ErrExpeditionNotFound
	}

	expTime, hasExp := m.expirations[characterID]
	if hasExp && time.Now().UTC().After(expTime) {
		delete(m.expeditions, characterID)
		delete(m.expirations, characterID)
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

	m.expeditions[characterID] = exp
	m.expirations[characterID] = time.Now().UTC().Add(m.ttl)

	cp := exp
	cp.AccumulatedItems = append([]string(nil), exp.AccumulatedItems...)
	cp.Members = append([]ExpeditionMember(nil), exp.Members...)
	return StepOutcome{
		Expedition: cp,
		Status:     finalStatus,
	}, nil
}
