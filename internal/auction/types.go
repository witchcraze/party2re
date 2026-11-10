package auction

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// NPCName is the venue NPC (auction.cgi $npc_name).
const NPCName = "@ワイルド"

var (
	ErrCannotSendToSelf  = errors.New("cannot send to yourself")
	ErrTargetNotFound    = errors.New("target player does not exist")
	ErrInsufficientMoney = errors.New("insufficient money")
	ErrInvalidSendAmount = errors.New("send amount must be at least 1 G")
	ErrDepotFull         = errors.New("target player's depot is full")
	ErrTabooItem         = errors.New("this item cannot be sent")
	ErrItemNotEquipped   = errors.New("no item equipped in specified slot")
	ErrItemNotFound      = errors.New("item not found")
	ErrInvalidSendTarget = errors.New("target character ID or name required")
	ErrNothingToSend     = errors.New("must specify gold or item to send")
	ErrInvalidSlot       = errors.New("invalid equipment slot")
)

type SendRequest struct {
	SenderCharacterID   string `json:"sender_character_id"`
	TargetCharacterID   string `json:"target_character_id,omitempty"`
	TargetCharacterName string `json:"target_character_name,omitempty"`
	Gold                int    `json:"gold,omitempty"`
	Slot                string `json:"slot,omitempty"`
	InstanceID          string `json:"instance_id,omitempty"`
}

type SendResult struct {
	SenderCharacterID   string               `json:"sender_character_id"`
	TargetCharacterID   string               `json:"target_character_id"`
	TargetCharacterName string               `json:"target_character_name"`
	TransferredGold     int                  `json:"transferred_gold,omitempty"`
	TransferredItem     *TransferredItemInfo `json:"transferred_item,omitempty"`
	Message             string               `json:"message"`
}

type TransferredItemInfo struct {
	InstanceID       string `json:"instance_id"`
	DefinitionID     string `json:"definition_id"`
	ItemName         string `json:"item_name"`
	EnhancementLevel int    `json:"enhancement_level"`
}

// InspectResult is the public status other players see; wallet money is not
// part of legacy shiraberu for another player.
type InspectResult struct {
	CharacterID string            `json:"character_id"`
	Name        string            `json:"name"`
	Gender      string            `json:"gender"`
	Level       int               `json:"level"`
	JobID       string            `json:"job_id"`
	SP          int               `json:"sp"`
	OldJobID    string            `json:"old_job_id"`
	OldSP       int               `json:"old_sp"`
	Tired       int               `json:"tired"`
	MaxHP       int               `json:"max_hp"`
	MaxMP       int               `json:"max_mp"`
	Attack      int               `json:"attack"`
	Defense     int               `json:"defense"`
	Agility     int               `json:"agility"`
	Weapon      *EquippedItemInfo `json:"weapon,omitempty"`
	Armor       *EquippedItemInfo `json:"armor,omitempty"`
	Accessory   *EquippedItemInfo `json:"accessory,omitempty"`
	Self        bool              `json:"self"`
}

type EquippedItemInfo struct {
	InstanceID       string `json:"instance_id"`
	DefinitionID     string `json:"definition_id"`
	Name             string `json:"name"`
	CustomName       string `json:"custom_name,omitempty"`
	EnhancementLevel int    `json:"enhancement_level"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	FindByName(ctx context.Context, name string) (corecharacter.Character, error)
	FindByNameForUpdate(ctx context.Context, name string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

type EquipmentRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, value coreequipment.Equipment) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, d depot.Depot) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
