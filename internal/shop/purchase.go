package shop

import (
	"context"
	"errors"
	"math/rand"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

type PurchaseResult struct {
	Character          corecharacter.Character
	Inventory          coreinventory.Inventory
	Depot              *depot.Depot
	ItemInstance       item.Instance
	TotalPrice         int
	TransferredToDepot bool
	NPCMessage         string
}

type BatchPurchaseItemRequest struct {
	ItemDefinitionID string `json:"item_definition_id"`
	Quantity         int    `json:"quantity"`
}

type BatchPurchaseResult struct {
	Character  corecharacter.Character `json:"character"`
	Depot      depot.Depot             `json:"depot"`
	Purchased  []item.Instance         `json:"purchased"`
	TotalPrice int                     `json:"total_price"`
	NPCMessage string                  `json:"npc_message"`
}

func itemKindFromSlot(slot item.Slot) int {
	switch slot {
	case item.SlotMainHand:
		return 1 // Weapon
	case item.SlotOffHand, item.SlotBody, item.SlotAccessory:
		return 2 // Armor / Shield / Accessory
	default:
		return 3 // Consumable / Other item
	}
}

func shopTypeFromKind(kind int) ShopType {
	switch kind {
	case 1:
		return ShopTypeWeapon
	case 2:
		return ShopTypeArmor
	default:
		return ShopTypeItem
	}
}

func categoryForSlot(slot item.Slot) string {
	switch slot {
	case item.SlotMainHand:
		return "weapon"
	case item.SlotOffHand, item.SlotBody, item.SlotAccessory:
		return "armor"
	default:
		return "item"
	}
}

func (s *Service) isItemInActiveHelper(ctx context.Context, itemID string) bool {
	if s.helper == nil {
		return false
	}
	active, err := s.helper.GetActiveHelperItemIDs(ctx, s.now())
	if err != nil {
		return false
	}
	for _, actID := range active {
		if actID == itemID {
			return true
		}
	}
	return false
}

// Purchase buys an item at 2x retail price.
// If the target slot is empty in inventory and quantity is 1, it is delivered to inventory.
// If the slot is already occupied, quantity > 1, or depot transfer occurs, it is sent to depot.
func (s *Service) Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (PurchaseResult, error) {
	if quantity <= 0 || quantity > MaxTransactionQuantity {
		return PurchaseResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return PurchaseResult{}, corecharacter.ErrNotFound
	}

	definition, err := s.catalog.FindByID(itemDefinitionID)
	if err != nil {
		return PurchaseResult{}, ErrItemNotFound
	}

	if s.isItemInActiveHelper(ctx, itemDefinitionID) {
		return PurchaseResult{}, ErrItemUnavailable
	}

	unitPrice, err := s.CalculateRetailPrice(definition.Price)
	if err != nil {
		return PurchaseResult{}, err
	}
	totalPrice, err := safeMultiply(unitPrice, quantity)
	if err != nil {
		return PurchaseResult{}, err
	}

	var result PurchaseResult
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character (Rank 2)
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return corecharacter.ErrNotFound
		}
		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		// 2. Lock Inventory (Rank 3)
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		kind := itemKindFromSlot(definition.Slot)
		occupied := false
		for _, inst := range inv.Items {
			def, err := s.catalog.FindByID(inst.DefinitionID)
			if err == nil && itemKindFromSlot(def.Slot) == kind {
				occupied = true
				break
			}
		}

		// Slot empty and single quantity -> Add to inventory
		if !occupied && quantity == 1 {
			res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
				CharacterID:       characterID,
				DeductGold:        totalPrice,
				GrantDefinitionID: itemDefinitionID,
				GrantQuantity:     1,
			})
			if err != nil {
				if errors.Is(err, economy.ErrInsufficientGold) {
					return ErrInsufficientFunds
				}
				if errors.Is(err, economy.ErrCharacterNotFound) {
					return corecharacter.ErrNotFound
				}
				return err
			}

			if s.recorder != nil {
				_ = s.recorder.RecordItemDiscovered(txCtx, characterID, definition.ID, definition.Name, categoryForSlot(definition.Slot))
			}

			st := shopTypeFromKind(kind)
			result = PurchaseResult{
				Character:          res.Character,
				Inventory:          res.Inventory,
				ItemInstance:       *res.GrantedItem,
				TotalPrice:         totalPrice,
				TransferredToDepot: false,
				NPCMessage:         SinglePurchaseNPCMessage(st, definition.Name, res.Character.Name, false),
			}
			return nil
		}

		// Slot occupied or quantity > 1: Transfer to Depot (Rank 5)
		if s.depots == nil {
			return ErrDepotNotConfigured
		}

		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			if !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		}
		dep.Capacity = depot.CalculateCapacity(char.JobLevel, dep.ExDepot, char.OverDepot)

		inst, err := item.NewInstance(itemDefinitionID, quantity)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			if errors.Is(err, depot.ErrDepotFull) {
				return ErrDepotFull
			}
			return err
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID: characterID,
			DeductGold:  totalPrice,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientFunds
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return corecharacter.ErrNotFound
			}
			return err
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return err
		}

		st := shopTypeFromKind(kind)
		result = PurchaseResult{
			Character:          res.Character,
			Inventory:          inv,
			Depot:              &dep,
			ItemInstance:       inst,
			TotalPrice:         totalPrice,
			TransferredToDepot: true,
			NPCMessage:         SinglePurchaseNPCMessage(st, definition.Name, res.Character.Name, true),
		}
		return nil
	})
	if err != nil {
		return PurchaseResult{}, err
	}

	return result, nil
}

// BatchPurchase purchases multiple items atomically and delivers ALL of them directly to depot.
func (s *Service) BatchPurchase(ctx context.Context, characterID string, shopType ShopType, items []BatchPurchaseItemRequest) (BatchPurchaseResult, error) {
	if len(items) == 0 {
		return BatchPurchaseResult{}, ErrEmptyPurchaseList
	}
	if characterID == "" {
		return BatchPurchaseResult{}, corecharacter.ErrNotFound
	}
	if !ValidateShopType(shopType) {
		return BatchPurchaseResult{}, ErrInvalidShopType
	}
	if s.depots == nil {
		return BatchPurchaseResult{}, ErrDepotNotConfigured
	}

	totalPrice := 0
	var instances []item.Instance
	var itemNames []string

	for _, it := range items {
		if it.Quantity <= 0 || it.Quantity > MaxTransactionQuantity {
			return BatchPurchaseResult{}, ErrInvalidQuantity
		}
		if s.isItemInActiveHelper(ctx, it.ItemDefinitionID) {
			return BatchPurchaseResult{}, ErrItemUnavailable
		}

		def, err := s.catalog.FindByID(it.ItemDefinitionID)
		if err != nil {
			return BatchPurchaseResult{}, ErrItemNotFound
		}

		unitPrice, err := s.CalculateRetailPrice(def.Price)
		if err != nil {
			return BatchPurchaseResult{}, err
		}
		lineTotal, err := safeMultiply(unitPrice, it.Quantity)
		if err != nil {
			return BatchPurchaseResult{}, err
		}

		// Check overflow in cumulative sum
		if totalPrice > 2_000_000_000-lineTotal {
			return BatchPurchaseResult{}, ErrPriceOverflow
		}
		totalPrice += lineTotal

		inst, err := item.NewInstance(it.ItemDefinitionID, it.Quantity)
		if err != nil {
			return BatchPurchaseResult{}, err
		}
		instances = append(instances, inst)
		itemNames = append(itemNames, def.Name)
	}

	var result BatchPurchaseResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character (Rank 2)
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return corecharacter.ErrNotFound
		}
		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		// 2. Lock Depot (Rank 5)
		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			if !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		}
		dep.Capacity = depot.CalculateCapacity(char.JobLevel, dep.ExDepot, char.OverDepot)

		// Pre-validate & add all items to depot
		for _, inst := range instances {
			if err := dep.AddItem(inst); err != nil {
				if errors.Is(err, depot.ErrDepotFull) {
					return ErrDepotFull
				}
				return err
			}
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID: characterID,
			DeductGold:  totalPrice,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientFunds
			}
			return err
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return err
		}

		result = BatchPurchaseResult{
			Character:  res.Character,
			Depot:      dep,
			Purchased:  instances,
			TotalPrice: totalPrice,
			NPCMessage: BatchPurchaseNPCMessage(shopType, res.Character.Name, itemNames),
		}
		return nil
	})
	if err != nil {
		return BatchPurchaseResult{}, err
	}

	return result, nil
}

func (s *Service) InspectNPC(ctx context.Context, shopType ShopType, characterID string) (NPCInspectResult, error) {
	if !ValidateShopType(shopType) {
		return NPCInspectResult{}, ErrInvalidShopType
	}
	_, npc := GetShopMeta(shopType)
	dialogue, hint := GetInspectDialogue(shopType)
	return NPCInspectResult{
		ShopType:       shopType,
		NPCName:        npc,
		Dialogue:       dialogue,
		SecretShopHint: hint,
	}, nil
}

func (s *Service) TalkNPC(ctx context.Context, shopType ShopType) (string, error) {
	if !ValidateShopType(shopType) {
		return "", ErrInvalidShopType
	}
	words := GetShopWords(shopType)
	if len(words) == 0 {
		return "", nil
	}
	idx := rand.Intn(len(words))
	return words[idx], nil
}

func (s *Service) DiscoverSecretShop(ctx context.Context, characterID string) (bool, string, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return false, "", err
	}
	if char.JobLevel < 7 {
		return false, "転職回数が足りません（7回以上の転職が必要です）", nil
	}
	return true, "秘密の店を見つけました！", nil
}
