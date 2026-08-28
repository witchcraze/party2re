package item

import (
	"errors"
	"strings"

	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrInvalidDefinition = errors.New("item definition is invalid")
	ErrInvalidInstance   = errors.New("item instance is invalid")
)

type Definition struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
	Slot  Slot   `json:"slot,omitempty"`
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
