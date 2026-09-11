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

const (
	VenueName = "オークション会場"
	NPCName   = "@ワイルド"
)

var DialogueWords = []string{
	"ここはオークション会場です。他のプレイヤーとアイテム交換やアイテム売買をする場所です。",
	"入札や出品のようなシステムはないです。自由に競りをしてください。",
	"相手が実際にそのアイテムや落札金を持っているのか「＠しらべる」で見ることができます。",
}

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

type InspectResult struct {
	CharacterID string            `json:"character_id"`
	Name        string            `json:"name"`
	Level       int               `json:"level"`
	JobID       string            `json:"job_id"`
	JobLevel    int               `json:"job_level"`
	Money       int               `json:"money"`
	Weapon      *EquippedItemInfo `json:"weapon,omitempty"`
	Armor       *EquippedItemInfo `json:"armor,omitempty"`
	Accessory   *EquippedItemInfo `json:"accessory,omitempty"`
	Message     string            `json:"message,omitempty"`
}

type EquippedItemInfo struct {
	InstanceID       string `json:"instance_id"`
	DefinitionID     string `json:"definition_id"`
	Name             string `json:"name"`
	EnhancementLevel int    `json:"enhancement_level"`
}

type VenueInfo struct {
	Title    string   `json:"title"`
	NPCName  string   `json:"npc_name"`
	Dialogue []string `json:"dialogue"`
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
