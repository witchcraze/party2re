package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type applySealRequest struct {
	SealID int `json:"seal_id"`
}

type nameEquipmentRequest struct {
	Target string `json:"target"`
	Name   string `json:"name"`
}

type withdrawWeaponRequest struct {
	Slot int `json:"slot"`
}

// BlacksmithService defines the blacksmith operations exposed over HTTP.
type BlacksmithService interface {
	ApplySeal(ctx context.Context, characterID string, sealID int) (blacksmith.Seal, error)
	NameEquipment(ctx context.Context, characterID string, target string, customName string) error
	DepositWeapon(ctx context.Context, characterID string) (blacksmith.Deposit, error)
	WithdrawWeapon(ctx context.Context, characterID string, slot int) error
	Observe(ctx context.Context, characterID string) (blacksmith.Observation, error)
}

// WithBlacksmith configures the scene observation and four equipment commands.
func WithBlacksmith(service BlacksmithService) Option {
	return func(h *Handler) {
		h.blacksmith = service
		if service == nil {
			withEchoCommand[applySealRequest, blacksmithSealResult]("blacksmith_seal", nil, nil)(h)
			withEchoCommand[nameEquipmentRequest, blacksmithNameResult]("blacksmith_name", nil, nil)(h)
			withEchoCommand[struct{}, blacksmithDepositResult]("blacksmith_deposit", nil, nil)(h)
			withEchoCommand[withdrawWeaponRequest, blacksmithWithdrawResult]("blacksmith_withdraw", nil, nil)(h)
			return
		}
		withEchoCommand("blacksmith_seal", func(ctx context.Context, actor string, p applySealRequest) (blacksmithSealResult, error) {
			seal, err := service.ApplySeal(ctx, actor, p.SealID)
			if err != nil {
				return blacksmithSealResult{}, err
			}
			message := "よっしゃ！武器に【" + seal.Name + "】を刻印したぜ！"
			return blacksmithSealResult{seal, message, commandEcho{target: seal.Name, reply: message}}, nil
		}, blacksmithActionRejection)(h)
		withEchoCommand("blacksmith_name", func(ctx context.Context, actor string, p nameEquipmentRequest) (blacksmithNameResult, error) {
			if err := service.NameEquipment(ctx, actor, p.Target, p.Name); err != nil {
				return blacksmithNameResult{}, err
			}
			target, label := "weapon", "武器"
			switch strings.ToLower(strings.TrimSpace(p.Target)) {
			case "armor", "arm", "防具":
				target, label = "armor", "防具"
			}
			message := label + "を" + p.Name + "と名づけたぜ"
			return blacksmithNameResult{target, p.Name, message, commandEcho{target: label + "＠なまえ>" + p.Name, reply: message}}, nil
		}, blacksmithActionRejection)(h)
		withEchoCommand("blacksmith_deposit", func(ctx context.Context, actor string, _ struct{}) (blacksmithDepositResult, error) {
			// Read the display name before mutation (blacksmith.cgi:177 get_weapon_name).
			seen, err := service.Observe(ctx, actor)
			if err != nil {
				return blacksmithDepositResult{}, err
			}
			deposit, err := service.DepositWeapon(ctx, actor)
			if err != nil {
				return blacksmithDepositResult{}, err
			}
			name := ""
			if seen.Weapon != nil {
				name = seen.Weapon.Name // effective name
			}
			message := name + "をお預かっておくぜ"
			return blacksmithDepositResult{deposit, message, commandEcho{target: "武器", reply: message}}, nil
		}, blacksmithActionRejection)(h)
		withEchoCommand("blacksmith_withdraw", func(ctx context.Context, actor string, p withdrawWeaponRequest) (blacksmithWithdrawResult, error) {
			seen, err := service.Observe(ctx, actor)
			if err != nil {
				return blacksmithWithdrawResult{}, err
			}
			name := ""
			for _, stored := range seen.Deposits {
				if stored.Slot == p.Slot {
					name = stored.Name
				}
			}
			err = service.WithdrawWeapon(ctx, actor, p.Slot)
			if errors.Is(err, blacksmith.ErrWeaponSlotOccupied) && name != "" {
				// Legacy answers a held weapon with a logged $npc_com that changes nothing (blacksmith.cgi:206-209).
				const message = "おっと、武器を外してからにしてくれ！"
				return blacksmithWithdrawResult{p.Slot, message, commandEcho{target: name, reply: message}}, nil
			}
			if err != nil {
				return blacksmithWithdrawResult{}, err
			}
			message := name + "を返すぜ"
			return blacksmithWithdrawResult{p.Slot, message, commandEcho{target: name, reply: message}}, nil
		}, blacksmithActionRejection)(h)
	}
}

// Blacksmith results carry the smith's reply (blacksmith.cgi:76,96,125,177,213).
type blacksmithSealResult struct {
	Seal    blacksmith.Seal `json:"seal"`
	Message string          `json:"message"`
	commandEcho
}

type blacksmithNameResult struct {
	Target  string `json:"target"`
	Name    string `json:"name"`
	Message string `json:"message"`
	commandEcho
}

type blacksmithDepositResult struct {
	Deposit blacksmith.Deposit `json:"deposit"`
	Message string             `json:"message"`
	commandEcho
}

type blacksmithWithdrawResult struct {
	Slot    int    `json:"slot"`
	Message string `json:"message"`
	commandEcho
}

func blacksmithActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, blacksmith.ErrDepositNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "BLACKSMITH_DEPOSIT_NOT_FOUND", Message: "Stored weapon not found."}
	case errors.Is(err, blacksmith.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVALID_CHARACTER_ID", Message: "Invalid character ID."}
	case errors.Is(err, blacksmith.ErrNoWeaponEquipped):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_NO_WEAPON", Message: "Equip a weapon first."}
	case errors.Is(err, blacksmith.ErrNoArmorEquipped):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_NO_ARMOR", Message: "Equip armor first."}
	case errors.Is(err, blacksmith.ErrInvalidSeal):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVALID_SEAL", Message: "Invalid seal ID."}
	case errors.Is(err, blacksmith.ErrSealNotApplicable):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_SEAL_NOT_APPLICABLE", Message: "Seal cannot be applied to this weapon."}
	case errors.Is(err, blacksmith.ErrInsufficientCrystals):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INSUFFICIENT_CRYSTALS", Message: "Insufficient crystals."}
	case errors.Is(err, blacksmith.ErrStorageFull):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_STORAGE_FULL", Message: "Weapon storage is full."}
	case errors.Is(err, blacksmith.ErrDuplicateStoredName):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_DUPLICATE_STORED_NAME", Message: "A weapon with this name is already stored."}
	case errors.Is(err, blacksmith.ErrWeaponSlotOccupied):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_WEAPON_SLOT_OCCUPIED", Message: "Unequip your weapon before withdrawing."}
	case errors.Is(err, blacksmith.ErrInvalidSlot):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVALID_SLOT", Message: "Storage slot must be 1..3."}
	case errors.Is(err, blacksmith.ErrInvalidTarget):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVALID_TARGET", Message: "Select weapon or armor."}
	case errors.Is(err, blacksmith.ErrNameWhitespace), errors.Is(err, blacksmith.ErrNameForbiddenChar),
		errors.Is(err, blacksmith.ErrNameAtSymbol), errors.Is(err, blacksmith.ErrNameTooLong), errors.Is(err, blacksmith.ErrEmptyName):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVALID_NAME", Message: "Invalid equipment name."}
	case errors.Is(err, coreinventory.ErrInventoryFull):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKSMITH_INVENTORY_FULL", Message: "Inventory is full."}
	default:
		return 0, ErrorDetail{}
	}
}
