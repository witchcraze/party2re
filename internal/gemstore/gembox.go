package gemstore

import (
	"context"
	"errors"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

const (
	MinGemBoxCapacity = 5
	MaxGemBoxCapacity = 100
)

var (
	ErrGemBoxFull          = errors.New("gem box is full")
	ErrRecipientGemBoxFull = errors.New("recipient gem box is full")
	ErrGemBoxNotFound      = errors.New("gem box not found")
)

// CalculateGemBoxCapacity calculates dynamic gem box capacity according to legacy Party2 formula (system.cgi:get_you_gem_box_c):
// $max_gem_box = $m{job_lv} >= 20 ? 100 : $m{job_lv} * 5 + 5;
func CalculateGemBoxCapacity(jobLevel int) int {
	if jobLevel >= 20 {
		return MaxGemBoxCapacity
	}
	if jobLevel <= 0 {
		return MinGemBoxCapacity
	}
	return jobLevel*5 + 5
}

// GemBox represents the character's dedicated gem storage.
type GemBox struct {
	CharacterID string              `json:"character_id"`
	Capacity    int                 `json:"capacity"`
	Items       []coreitem.Instance `json:"items"`
}

// Count returns the current count of gems in the gem box.
func (b GemBox) Count() int {
	return len(b.Items)
}

// IsFull returns true if the gem box has reached or exceeded its capacity.
func (b GemBox) IsFull() bool {
	return len(b.Items) >= b.Capacity
}

// AddItem adds an item instance to the gem box.
func (b *GemBox) AddItem(instance coreitem.Instance) error {
	if b.IsFull() {
		return ErrGemBoxFull
	}
	b.Items = append(b.Items, instance)
	return nil
}

// RemoveItem removes a gem from the gem box by instance ID or definition ID.
func (b *GemBox) RemoveItem(target string) (coreitem.Instance, error) {
	for i, inst := range b.Items {
		if inst.ID == target || inst.DefinitionID == target {
			removed := inst
			b.Items = append(b.Items[:i], b.Items[i+1:]...)
			return removed, nil
		}
	}
	return coreitem.Instance{}, ErrItemNotOwned
}

// FindItem looks up an item in the gem box by instance ID or definition ID.
func (b GemBox) FindItem(target string) (coreitem.Instance, bool) {
	for _, inst := range b.Items {
		if inst.ID == target || inst.DefinitionID == target {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

// GemBoxRepository defines the persistence contract for character gem boxes.
type GemBoxRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (GemBox, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (GemBox, error)
	Save(ctx context.Context, box GemBox) error
}
