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

func NewInstance(definitionID string, quantity int) (Instance, error) {
	return NewInstanceWithEnhancement(definitionID, quantity, 0)
}

func NewInstanceWithEnhancement(definitionID string, quantity int, enhancementLevel int) (Instance, error) {
	if strings.TrimSpace(definitionID) == "" || quantity <= 0 || enhancementLevel < 0 {
		return Instance{}, ErrInvalidInstance
	}
	return Instance{
		ID:               id.New(),
		DefinitionID:     strings.TrimSpace(definitionID),
		Quantity:         quantity,
		EnhancementLevel: enhancementLevel,
	}, nil
}
