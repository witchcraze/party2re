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

// WithBlacksmith configures retained reads and the four equipment commands.
func WithBlacksmith(service BlacksmithService) Option {
	return func(h *Handler) {
		h.blacksmith = service
		if service == nil {
			withActionCommand[applySealRequest]("blacksmith_seal", nil, nil)(h)
			withActionCommand[nameEquipmentRequest]("blacksmith_name", nil, nil)(h)
			withActionCommand[struct{}]("blacksmith_deposit", nil, nil)(h)
			withActionCommand[withdrawWeaponRequest]("blacksmith_withdraw", nil, nil)(h)
			return
		}
		withActionCommand("blacksmith_seal", func(ctx context.Context, actor string, p applySealRequest) (any, error) {
			seal, err := service.ApplySeal(ctx, actor, p.SealID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"seal": seal, "message": "よっしゃ！武器に【" + seal.Name + "】を刻印したぜ！"}, nil
		}, blacksmithActionRejection)(h)
		withActionCommand("blacksmith_name", func(ctx context.Context, actor string, p nameEquipmentRequest) (any, error) {
			if err := service.NameEquipment(ctx, actor, p.Target, p.Name); err != nil {
				return nil, err
			}
			target, label := "weapon", "武器"
			switch strings.ToLower(strings.TrimSpace(p.Target)) {
			case "armor", "arm", "防具":
				target, label = "armor", "防具"
			}
			name := strings.TrimSpace(p.Name)
			message := label + "を" + name + "と名づけたぜ"
			if name == "" {
				message = label + "の名前を元に戻したぜ"
			}
			return map[string]any{"target": target, "name": name, "message": message}, nil
		}, blacksmithActionRejection)(h)
		withActionCommand("blacksmith_deposit", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			deposit, err := service.DepositWeapon(ctx, actor)
			if err != nil {
				return nil, err
			}
			return map[string]any{"deposit": deposit, "message": "武器をお預かっておくぜ"}, nil
		}, blacksmithActionRejection)(h)
		withActionCommand("blacksmith_withdraw", func(ctx context.Context, actor string, p withdrawWeaponRequest) (any, error) {
			if err := service.WithdrawWeapon(ctx, actor, p.Slot); err != nil {
				return nil, err
			}
			return map[string]any{"slot": p.Slot, "message": "武器を返すぜ"}, nil
		}, blacksmithActionRejection)(h)
	}
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
