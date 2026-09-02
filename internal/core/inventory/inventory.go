package inventory

import (
	"errors"
	"strings"

	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrInvalidInventory = errors.New("inventory is invalid")
	ErrInvalidQuantity  = errors.New("item quantity is invalid")
	ErrDuplicateItem    = errors.New("item instance already exists")
	ErrItemNotFound     = errors.New("item instance not found")
)

type Inventory struct {
	CharacterID string
	Items       []item.Instance
}

func New(characterID string) (Inventory, error) {
	if strings.TrimSpace(characterID) == "" {
		return Inventory{}, ErrInvalidInventory
	}
	return Inventory{CharacterID: strings.TrimSpace(characterID)}, nil
}

func (i *Inventory) Add(value item.Instance) error {
	if i == nil || strings.TrimSpace(i.CharacterID) == "" || value.ID == "" ||
		value.DefinitionID == "" || value.Quantity <= 0 {
		return ErrInvalidInventory
	}
	for _, existing := range i.Items {
		if existing.ID == value.ID {
			return ErrDuplicateItem
		}
	}
	i.Items = append(i.Items, value)
	return nil
}

func (i *Inventory) Quantity(definitionID string) int {
	if i == nil {
		return 0
	}

	total := 0
	for _, value := range i.Items {
		if value.DefinitionID == definitionID {
			total += value.Quantity
		}
	}
	return total
}

func (i *Inventory) Find(instanceID string) (item.Instance, bool) {
	if i == nil {
		return item.Instance{}, false
	}
	for _, value := range i.Items {
		if value.ID == instanceID {
			return value, true
		}
	}
	return item.Instance{}, false
}

func (i *Inventory) Consume(instanceID string, quantity int) error {
	if i == nil || quantity <= 0 {
		return ErrInvalidQuantity
	}
	for index, value := range i.Items {
		if value.ID != instanceID {
			continue
		}
		if value.Quantity < quantity {
			return ErrInvalidQuantity
		}
		value.Quantity -= quantity
		if value.Quantity == 0 {
			i.Items = append(i.Items[:index], i.Items[index+1:]...)
		} else {
			i.Items[index] = value
		}
		return nil
	}
	return ErrItemNotFound
}

// Update replaces an existing item instance in the inventory matching by ID.
func (i *Inventory) Update(value item.Instance) error {
	if i == nil || strings.TrimSpace(i.CharacterID) == "" || value.ID == "" {
		return ErrInvalidInventory
	}
	for index, existing := range i.Items {
		if existing.ID == value.ID {
			i.Items[index] = value
			return nil
		}
	}
	return ErrItemNotFound
}
