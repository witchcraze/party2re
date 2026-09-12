package item

import (
	"errors"
	"fmt"
	"strings"

	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrInvalidDefinition = errors.New("item definition is invalid")
	ErrInvalidInstance   = errors.New("item instance is invalid")
)

// UsageCategory specifies where and how an item can be used based on legacy Party2 Perl CGI ($ites[no][3]).
type UsageCategory int

const (
	// UsageCategoryNone (0) indicates items that cannot be actively used via menu (e.g. materials, coins, job emblems, automatic).
	UsageCategoryNone UsageCategory = 0
	// UsageCategoryCombatOnly (1) indicates items usable only in combat via @どうぐ (e.g. herbs, drops, status recovery, magic waters).
	UsageCategoryCombatOnly UsageCategory = 1
	// UsageCategoryAnytime (2) indicates items usable anytime / at Home (@ほーむ) (e.g. seeds, medals, fight elixir, costumes, recipes).
	UsageCategoryAnytime UsageCategory = 2
	// UsageCategoryCombatPassive (3) indicates items that trigger automatically in combat or passive charms/accessories.
	UsageCategoryCombatPassive UsageCategory = 3
	// UsageCategoryDepotAfterAction (4) indicates items processed during quest join or depot after-action.
	UsageCategoryDepotAfterAction UsageCategory = 4
)

func (c UsageCategory) String() string {
	switch c {
	case UsageCategoryNone:
		return "none"
	case UsageCategoryCombatOnly:
		return "combat_only"
	case UsageCategoryAnytime:
		return "anytime"
	case UsageCategoryCombatPassive:
		return "combat_passive"
	case UsageCategoryDepotAfterAction:
		return "depot_after_action"
	default:
		return fmt.Sprintf("UsageCategory(%d)", c)
	}
}

// IsValidUsageCategory returns whether the usage category is a recognized legacy category.
func IsValidUsageCategory(c UsageCategory) bool {
	switch c {
	case UsageCategoryNone, UsageCategoryCombatOnly, UsageCategoryAnytime, UsageCategoryCombatPassive, UsageCategoryDepotAfterAction:
		return true
	default:
		return false
	}
}

// IsUsableAtHome returns whether the item can be actively used at Home (@ほーむ).
func (c UsageCategory) IsUsableAtHome() bool {
	return c == UsageCategoryAnytime || c == UsageCategoryDepotAfterAction
}

// IsCombatOnly returns whether the item can only be used during battle.
func (c UsageCategory) IsCombatOnly() bool {
	return c == UsageCategoryCombatOnly
}

// IsUsableInCombatCommand returns whether the item category can be actively used via combat command (@どうぐ).
// In legacy Party2 CGI (_skill.cgi), only UsageCategoryCombatOnly (1) items can be registered and used in battle commands.
func (c UsageCategory) IsUsableInCombatCommand() bool {
	return c == UsageCategoryCombatOnly
}

// UsageLocation represents the execution context where an item usage attempt occurs.
type UsageLocation string

const (
	UsageLocationHome   UsageLocation = "home"
	UsageLocationCombat UsageLocation = "combat"
)

var (
	ErrCannotUseInCombat    = errors.New("item cannot be used in combat")
	ErrCannotUseAtHome      = errors.New("item cannot be used at home")
	ErrInvalidUsageLocation = errors.New("invalid usage location")
)

// ValidateUsageLocation checks whether an item with the given category is permitted for use at the specified location.
func ValidateUsageLocation(category UsageCategory, location UsageLocation) error {
	switch location {
	case UsageLocationCombat:
		if !category.IsUsableInCombatCommand() {
			return ErrCannotUseInCombat
		}
		return nil
	case UsageLocationHome:
		if !category.IsUsableAtHome() {
			return ErrCannotUseAtHome
		}
		return nil
	default:
		return ErrInvalidUsageLocation
	}
}

type Definition struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Price         int           `json:"price"`
	Slot          Slot          `json:"slot,omitempty"`
	UsageCategory UsageCategory `json:"usage_category,omitempty"`
}

// IsUsableAtHome returns whether the item definition can be actively consumed at Home.
func (d Definition) IsUsableAtHome() bool {
	return d.UsageCategory.IsUsableAtHome()
}

// IsCombatOnly returns whether the item definition can only be consumed in combat.
func (d Definition) IsCombatOnly() bool {
	return d.UsageCategory.IsCombatOnly()
}

// CanUseInCombat returns whether the item definition can be actively used in combat command (@どうぐ).
func (d Definition) CanUseInCombat() bool {
	return d.UsageCategory.IsUsableInCombatCommand()
}

// IsStackable returns whether items of this definition can be stacked in storage.
// In Party2 game rules, equipment items (weapons, armor, shields, accessories with Slot != SlotNone)
// are non-stackable discrete gear, while non-equipment items (Slot == SlotNone) are stackable consumables/materials.
func (d Definition) IsStackable() bool {
	return d.Slot == SlotNone
}

// NewInstance creates an item instance from this definition, enforcing stackability invariants.
// For equipment items (!d.IsStackable()), quantity must be 1.
func (d Definition) NewInstance(quantity int) (Instance, error) {
	return d.NewInstanceWithEnhancement(quantity, 0)
}

// NewInstanceWithEnhancement creates an item instance with enhancement from this definition,
// enforcing stackability invariants. For equipment items or enhanced items, quantity must be 1.
func (d Definition) NewInstanceWithEnhancement(quantity int, enhancementLevel int) (Instance, error) {
	if (!d.IsStackable() || enhancementLevel > 0) && quantity > 1 {
		return Instance{}, ErrInvalidInstance
	}
	return NewInstanceWithEnhancement(d.ID, quantity, enhancementLevel)
}

// ValidateInstance verifies that an instance satisfies the domain invariants for this definition.
func (d Definition) ValidateInstance(inst Instance) error {
	if inst.DefinitionID != d.ID {
		return ErrInvalidInstance
	}
	if inst.Quantity <= 0 || inst.EnhancementLevel < 0 {
		return ErrInvalidInstance
	}
	if (!d.IsStackable() || inst.EnhancementLevel > 0) && inst.Quantity > 1 {
		return ErrInvalidInstance
	}
	return nil
}

// CanStackInstances reports whether instances a and b of this definition can be stacked together.
func (d Definition) CanStackInstances(a, b Instance) bool {
	return a.CanStackWith(b, d.IsStackable())
}

// ValidateUsageLocation checks whether this item definition is permitted for use at the specified location.
func (d Definition) ValidateUsageLocation(location UsageLocation) error {
	return ValidateUsageLocation(d.UsageCategory, location)
}

type Slot string

const (
	SlotNone      Slot = ""
	SlotMainHand  Slot = "main-hand"
	SlotOffHand   Slot = "off-hand"
	SlotBody      Slot = "body"
	SlotAccessory Slot = "accessory"
)

func IsValidSlot(slot Slot) bool {
	switch slot {
	case SlotNone, SlotMainHand, SlotOffHand, SlotBody, SlotAccessory:
		return true
	default:
		return false
	}
}

func NewDefinition(id, name string, price int) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || price < 0 {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name), Price: price}, nil
}

func NewConsumableDefinition(id, name string, price int, category UsageCategory) (Definition, error) {
	if !IsValidUsageCategory(category) {
		return Definition{}, ErrInvalidDefinition
	}
	value, err := NewDefinition(id, name, price)
	if err != nil {
		return Definition{}, err
	}
	value.UsageCategory = category
	return value, nil
}

func NewEquipmentDefinition(id, name string, price int, slot Slot) (Definition, error) {
	value, err := NewDefinition(id, name, price)
	if err != nil || slot == SlotNone || !IsValidSlot(slot) {
		return Definition{}, ErrInvalidDefinition
	}
	value.Slot = slot
	return value, nil
}

type Instance struct {
	ID               string
	DefinitionID     string
	Quantity         int
	EnhancementLevel int
}

// CanStackWith reports whether this instance can stack with another instance,
// given whether the underlying item definition is stackable.
// Invariants enforced:
// 1. Definition must be stackable (equipment cannot stack).
// 2. Both instances must share the same DefinitionID.
// 3. Neither instance may have an EnhancementLevel > 0.
func (i Instance) CanStackWith(other Instance, isStackable bool) bool {
	if !isStackable {
		return false
	}
	if i.DefinitionID != other.DefinitionID {
		return false
	}
	if i.EnhancementLevel > 0 || other.EnhancementLevel > 0 {
		return false
	}
	return i.EnhancementLevel == other.EnhancementLevel
}

func NewInstance(definitionID string, quantity int) (Instance, error) {
	return NewInstanceWithEnhancement(definitionID, quantity, 0)
}

func NewInstanceWithEnhancement(definitionID string, quantity int, enhancementLevel int) (Instance, error) {
	if strings.TrimSpace(definitionID) == "" || quantity <= 0 || enhancementLevel < 0 {
		return Instance{}, ErrInvalidInstance
	}
	if enhancementLevel > 0 && quantity > 1 {
		return Instance{}, ErrInvalidInstance
	}
	return Instance{
		ID:               id.New(),
		DefinitionID:     strings.TrimSpace(definitionID),
		Quantity:         quantity,
		EnhancementLevel: enhancementLevel,
	}, nil
}
