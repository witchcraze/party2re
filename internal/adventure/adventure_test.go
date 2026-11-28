package adventure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type repositoryStub struct {
	mu      sync.Mutex
	value   Adventure
	saveErr error
}

func (r *repositoryStub) Save(_ context.Context, value Adventure) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return r.saveErr
	}
	r.value = value
	return nil
}

func (r *repositoryStub) FindByID(_ context.Context, id string) (Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" || r.value.ID != id {
		return Adventure{}, ErrNotFound
	}
	return r.value, nil
}

func (r *repositoryStub) ListByCharacterID(_ context.Context, characterID string, limit, offset int) ([]Adventure, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.CharacterID == characterID {
		return []Adventure{r.value}, 1, nil
	}
	return nil, 0, nil
}

func (r *repositoryStub) ListByCharacterIDByCursor(_ context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.CharacterID == characterID {
		return []Adventure{r.value}, nil
	}
	return nil, nil
}

func (r *repositoryStub) GetAggregatedStats(_ context.Context, characterID string) (AggregatedStats, error) {
	return AggregatedStats{}, nil
}

type characterRepositoryStub struct {
	mu        sync.Mutex
	value     corecharacter.Character
	chars     map[string]corecharacter.Character
	updateErr error
}

func (r *characterRepositoryStub) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.chars) > 0 {
		if c, ok := r.chars[id]; ok {
			return c, nil
		}
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if r.value.ID == "" || r.value.ID != id {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.value, nil
}

func (r *characterRepositoryStub) Update(_ context.Context, value corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateErr != nil {
		return r.updateErr
	}
	r.value = value
	return nil
}

func TestAdventureNewServiceNilDependencies(t *testing.T) {
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{}
	if _, err := NewService(nil, characters); err == nil {
		t.Fatal("NewService(nil, ...) expected error, got nil")
	}
	if _, err := NewService(adventures, nil); err == nil {
		t.Fatal("NewService(..., nil) expected error, got nil")
	}
	if svc, err := NewService(adventures, characters); err != nil || svc == nil {
		t.Fatalf("NewService() = %v, %v", svc, err)
	}
}

func TestAdventureRealClock(t *testing.T) {
	clock := RealClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("RealClock.Now() returned zero time")
	}
}

func TestValidateCombatItem(t *testing.T) {
	tests := []struct {
		name    string
		def     item.Definition
		wantErr bool
	}{
		{
			name: "combat only item allowed",
			def: item.Definition{
				ID:            "item-001",
				Name:          "薬草",
				UsageCategory: item.UsageCategoryCombatOnly,
			},
			wantErr: false,
		},
		{
			name: "anytime seed item rejected",
			def: item.Definition{
				ID:            "item-060",
				Name:          "力の種",
				UsageCategory: item.UsageCategoryAnytime,
			},
			wantErr: true,
		},
		{
			name: "small medal rejected",
			def: item.Definition{
				ID:            "item-100",
				Name:          "小さなメダル",
				UsageCategory: item.UsageCategoryAnytime,
			},
			wantErr: true,
		},
		{
			name: "passive item rejected",
			def: item.Definition{
				ID:            "item-037",
				Name:          "闘気の盾",
				UsageCategory: item.UsageCategoryCombatPassive,
			},
			wantErr: true,
		},
		{
			name: "material item rejected",
			def: item.Definition{
				ID:            "item-200",
				Name:          "鉄のインゴット",
				UsageCategory: item.UsageCategoryNone,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCombatItem(tt.def)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for item %s, got nil", tt.def.Name)
				} else if !errors.Is(err, ErrCannotUseInCombat) {
					t.Errorf("expected ErrCannotUseInCombat, got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error for item %s, got %v", tt.def.Name, err)
				}
			}
		})
	}
}
