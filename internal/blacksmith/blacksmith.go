package blacksmith

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	MaxStorageSlots = 3
)

var (
	ErrInvalidCharacterID   = errors.New("invalid character ID")
	ErrNoWeaponEquipped     = errors.New("no weapon equipped")
	ErrNoArmorEquipped      = errors.New("no armor equipped")
	ErrInvalidSeal          = errors.New("invalid seal ID")
	ErrSealNotApplicable    = errors.New("seal is not applicable to this weapon")
	ErrInsufficientCrystals = errors.New("insufficient crystals")
	ErrStorageFull          = errors.New("blacksmith storage is full (maximum 3 items)")
	ErrDuplicateStoredName  = errors.New("a weapon with the same name is already in storage")
	ErrWeaponSlotOccupied   = errors.New("unequip weapon before withdrawing from blacksmith storage")
	ErrDepositNotFound      = errors.New("deposit not found in blacksmith storage")
	ErrInvalidSlot          = errors.New("invalid storage slot (must be 1..3)")
	ErrInvalidTarget        = errors.New("target must be weapon or armor")
)

// Deposit represents an authentic 3-slot blacksmith storage record preserving seals and custom names (party2/lib/blacksmith.cgi:144-237).
type Deposit struct {
	ID               string    `json:"id"`
	CharacterID      string    `json:"character_id"`
	Slot             int       `json:"slot"`
	ItemDefinitionID string    `json:"item_definition_id"`
	SealID           int       `json:"seal_id"`
	CustomName       string    `json:"custom_name"`
	CreatedAt        time.Time `json:"created_at"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

type EquipmentRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, equipment coreequipment.Equipment) error
}

type StorageRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]Deposit, error)
	ListByCharacterIDForUpdate(ctx context.Context, characterID string) ([]Deposit, error)
	Save(ctx context.Context, deposit Deposit) error
	Delete(ctx context.Context, characterID string, slot int) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	equipment   EquipmentRepository
	storage     StorageRepository
	catalog     item.DefinitionProvider
	txProvider  TransactionProvider
}

type Option func(*Service)

func WithEquipmentRepository(equipment EquipmentRepository) Option {
	return func(s *Service) {
		s.equipment = equipment
	}
}

func WithStorageRepository(storage StorageRepository) Option {
	return func(s *Service) {
		s.storage = storage
	}
}

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	catalog item.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		catalog:     catalog,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

// ApplySeal attaches one of the 12 authentic seals to the currently equipped weapon using crystals (party2/lib/blacksmith.cgi:110-140).
func (s *Service) ApplySeal(ctx context.Context, characterID string, sealID int) (Seal, error) {
	if strings.TrimSpace(characterID) == "" {
		return Seal{}, ErrInvalidCharacterID
	}
	seal, ok := GetSeal(sealID)
	if !ok {
		return Seal{}, ErrInvalidSeal
	}

	var appliedSeal Seal
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if s.equipment == nil {
			return errors.New("equipment repository not configured")
		}
		equip, err := s.equipment.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		instID, ok := equip.Equipped(item.SlotMainHand)
		if !ok || instID == "" {
			return ErrNoWeaponEquipped
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, found := inv.Find(instID)
		if !found {
			return ErrNoWeaponEquipped
		}

		def, err := s.catalog.FindByID(inst.DefinitionID)
		if err != nil {
			return err
		}

		if !CanApplySeal(def.ID, sealID) {
			return ErrSealNotApplicable
		}

		if char.Crystal < seal.CrystalCost {
			return ErrInsufficientCrystals
		}

		char.Crystal -= seal.CrystalCost
		char.WeaponSeal = sealID

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		appliedSeal = seal
		return nil
	})
	if err != nil {
		return Seal{}, err
	}
	return appliedSeal, nil
}

// NameEquipment assigns a custom name (up to 20 characters) to the currently equipped weapon or armor (party2/lib/blacksmith.cgi:55-105).
func (s *Service) NameEquipment(ctx context.Context, characterID string, target string, customName string) error {
	if strings.TrimSpace(characterID) == "" {
		return ErrInvalidCharacterID
	}
	t := strings.ToLower(strings.TrimSpace(target))
	if t != "weapon" && t != "wea" && t != "武器" && t != "armor" && t != "arm" && t != "防具" {
		return ErrInvalidTarget
	}

	trimmedName := strings.TrimSpace(customName)
	if trimmedName != "" {
		if err := ValidateCustomName(trimmedName); err != nil {
			return err
		}
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if s.equipment == nil {
			return errors.New("equipment repository not configured")
		}
		equip, err := s.equipment.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		switch t {
		case "weapon", "wea", "武器":
			instID, ok := equip.Equipped(item.SlotMainHand)
			if !ok || instID == "" {
				return ErrNoWeaponEquipped
			}
			char.WeaponCustomName = trimmedName
		case "armor", "arm", "防具":
			instID, ok := equip.Equipped(item.SlotBody)
			if !ok || instID == "" {
				return ErrNoArmorEquipped
			}
			char.ArmorCustomName = trimmedName
		}

		return s.characters.Update(txCtx, char)
	})
}

// DepositWeapon deposits the equipped weapon into the dedicated 3-slot blacksmith storage (party2/lib/blacksmith.cgi:144-188).
func (s *Service) DepositWeapon(ctx context.Context, characterID string) (Deposit, error) {
	if strings.TrimSpace(characterID) == "" {
		return Deposit{}, ErrInvalidCharacterID
	}
	if s.storage == nil {
		return Deposit{}, errors.New("storage repository not configured")
	}
	if s.equipment == nil {
		return Deposit{}, errors.New("equipment repository not configured")
	}

	var dep Deposit
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		equip, err := s.equipment.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		instID, ok := equip.Equipped(item.SlotMainHand)
		if !ok || instID == "" {
			return ErrNoWeaponEquipped
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, found := inv.Find(instID)
		if !found {
			return ErrNoWeaponEquipped
		}

		stored, err := s.storage.ListByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if len(stored) >= MaxStorageSlots {
			return ErrStorageFull
		}

		def, err := s.catalog.FindByID(inst.DefinitionID)
		if err != nil {
			return err
		}

		effectiveName := char.WeaponCustomName
		if effectiveName == "" {
			effectiveName = def.Name
		}

		for _, item := range stored {
			sName := item.CustomName
			if sName == "" {
				sDef, err := s.catalog.FindByID(item.ItemDefinitionID)
				if err == nil {
					sName = sDef.Name
				}
			}
			if sName == effectiveName {
				return ErrDuplicateStoredName
			}
		}

		usedSlots := make(map[int]bool)
		for _, item := range stored {
			usedSlots[item.Slot] = true
		}
		freeSlot := 1
		for freeSlot <= MaxStorageSlots && usedSlots[freeSlot] {
			freeSlot++
		}

		dep = Deposit{
			ID:               id.New(),
			CharacterID:      characterID,
			Slot:             freeSlot,
			ItemDefinitionID: inst.DefinitionID,
			SealID:           char.WeaponSeal,
			CustomName:       char.WeaponCustomName,
			CreatedAt:        time.Now().UTC(),
		}

		if _, err := equip.Unequip(item.SlotMainHand); err != nil {
			return err
		}
		if err := inv.Consume(instID, inst.Quantity); err != nil {
			return err
		}
		char.ClearWeaponCustomization()

		if err := s.storage.Save(txCtx, dep); err != nil {
			return err
		}
		if err := s.equipment.Save(txCtx, equip); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}
		return s.characters.Update(txCtx, char)
	})
	if err != nil {
		return Deposit{}, err
	}
	return dep, nil
}

// WithdrawWeapon retrieves a stored weapon from the blacksmith storage into the character's main hand (party2/lib/blacksmith.cgi:192-238).
func (s *Service) WithdrawWeapon(ctx context.Context, characterID string, slot int) error {
	if strings.TrimSpace(characterID) == "" {
		return ErrInvalidCharacterID
	}
	if slot < 1 || slot > MaxStorageSlots {
		return ErrInvalidSlot
	}
	if s.storage == nil {
		return errors.New("storage repository not configured")
	}
	if s.equipment == nil {
		return errors.New("equipment repository not configured")
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		equip, err := s.equipment.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if _, ok := equip.Equipped(item.SlotMainHand); ok {
			return ErrWeaponSlotOccupied
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		stored, err := s.storage.ListByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		var targetDep *Deposit
		for i := range stored {
			if stored[i].Slot == slot {
				targetDep = &stored[i]
				break
			}
		}
		if targetDep == nil {
			return ErrDepositNotFound
		}

		def, err := s.catalog.FindByID(targetDep.ItemDefinitionID)
		if err != nil {
			return err
		}

		newInstance := item.Instance{
			ID:           id.New(),
			DefinitionID: targetDep.ItemDefinitionID,
			Quantity:     1,
		}
		if err := inv.Add(newInstance); err != nil {
			return err
		}

		if _, err := equip.Equip(&inv, def, newInstance.ID); err != nil {
			return err
		}

		char.WeaponSeal = targetDep.SealID
		char.WeaponCustomName = targetDep.CustomName

		if err := s.storage.Delete(txCtx, characterID, slot); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}
		if err := s.equipment.Save(txCtx, equip); err != nil {
			return err
		}
		return s.characters.Update(txCtx, char)
	})
}

// ListDeposits returns all stored weapons for the character in the blacksmith storage.
func (s *Service) ListDeposits(ctx context.Context, characterID string) ([]Deposit, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.storage == nil {
		return nil, errors.New("storage repository not configured")
	}
	return s.storage.ListByCharacterID(ctx, characterID)
}
