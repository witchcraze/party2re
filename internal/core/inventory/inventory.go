package inventory

import (
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrInvalidInventory = errors.New("inventory is invalid")
	ErrInvalidQuantity  = errors.New("item quantity is invalid")
	ErrDuplicateItem    = errors.New("item instance already exists")
	ErrItemNotFound     = errors.New("item instance not found")
	ErrInventoryFull    = errors.New("inventory is full")
)

// HandCount is 3, matching the legacy Party2 Perl CGI hand fields ($m{wea}, $m{arm}, $m{ite}).
const (
	HandCount          = 3
	DefaultMaxCapacity = HandCount
)

type Inventory struct {
	CharacterID string
	Items       []item.Instance
}

// Clone returns an inventory whose item mutations are isolated from the original.
func (i Inventory) Clone() Inventory {
	i.Items = slices.Clone(i.Items)
	return i
}

// MaxCapacity returns the maximum number of items an inventory can hold.
func (i *Inventory) MaxCapacity() int {
	return DefaultMaxCapacity
}

// IsHandOccupied reports whether the legacy hand field for the given kind
// (1 = weapon, 2 = armor, 3 = tool/shield/accessory) already holds an item.
func (i *Inventory) IsHandOccupied(kind int) bool {
	if i == nil {
		return false
	}
	for _, inst := range i.Items {
		if item.LegacyHandKindByID(inst.DefinitionID) == kind {
			return true
		}
	}
	return false
}

// IsFull reports whether the inventory cannot accept an item.
// If definitionID is provided, it reports whether the target legacy hand field is occupied.
// Otherwise, it reports whether all hand fields are occupied.
func (i *Inventory) IsFull(definitionID ...string) bool {
	if i == nil {
		return false
	}
	if len(definitionID) > 0 && strings.TrimSpace(definitionID[0]) != "" {
		return i.IsHandOccupied(item.LegacyHandKindByID(definitionID[0]))
	}
	return len(i.Items) >= i.MaxCapacity()
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

// Consume removes quantity of the item instance.
// Returns ErrInvalidQuantity if quantity <= 0 or if available quantity < quantity.
func (i *Inventory) Consume(instanceID string, quantity int) error {
	_, err := i.ConsumeItem(instanceID, quantity)
	return err
}

// ConsumeOne removes one unit of the item instance.
// Delegates to Consume(instanceID, 1).
func (i *Inventory) ConsumeOne(instanceID string) error {
	return i.Consume(instanceID, 1)
}

// ConsumeItem removes quantity of the item instance and returns a copy of the consumed item instance.
// Returns ErrInvalidQuantity if quantity <= 0 or if available quantity < quantity.
// If remaining quantity reaches 0, the item slot is removed from inventory.
func (i *Inventory) ConsumeItem(instanceID string, quantity int) (item.Instance, error) {
	if i == nil || quantity <= 0 {
		return item.Instance{}, ErrInvalidQuantity
	}
	for index, value := range i.Items {
		if value.ID != instanceID {
			continue
		}
		if value.Quantity < quantity {
			return item.Instance{}, ErrInvalidQuantity
		}
		consumed := value
		consumed.Quantity = quantity

		value.Quantity -= quantity
		if value.Quantity == 0 {
			i.Items = append(i.Items[:index], i.Items[index+1:]...)
		} else {
			i.Items[index] = value
		}
		return consumed, nil
	}
	return item.Instance{}, ErrItemNotFound
}

// ConsumeOneItem removes one unit of the item instance and returns it.
// Delegates to ConsumeItem(instanceID, 1).
func (i *Inventory) ConsumeOneItem(instanceID string) (item.Instance, error) {
	return i.ConsumeItem(instanceID, 1)
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

// HeldToolID projects the legacy single-hand tool definition from inventory evidence.
// An empty inventory proves an empty tool hand (pointer to "").
// A single-row inventory containing a quantity-1 tool or accessory proves that held item.
// Inventories containing equipped weapons, armor, stacked rows, or ambiguous multiple rows remain unknown (nil).
func (i Inventory) HeldToolID() *string {
	if len(i.Items) == 0 {
		held := ""
		return &held
	}
	if len(i.Items) == 1 && i.Items[0].Quantity == 1 && item.IsToolID(i.Items[0].DefinitionID) {
		held := i.Items[0].DefinitionID
		return &held
	}
	return nil
}
