package shop

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
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

// Purchase buys an item at retail price.
// If the target slot is empty in inventory and quantity is 1, it is delivered to inventory.
// If the slot is already occupied, quantity > 1, or depot transfer occurs, it is sent to depot.
func (s *Service) Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (PurchaseResult, error) {
	definition, err := s.catalog.FindByID(itemDefinitionID)
	if err != nil {
		return PurchaseResult{}, ErrItemNotFound
	}
	return s.PurchaseInShop(ctx, characterID, shopTypeFromKind(definition.Kind()), itemDefinitionID, quantity)
}

// PurchaseInShop buys an item from a specific shop type, applying shop-specific pricing and messages.
func (s *Service) PurchaseInShop(ctx context.Context, characterID string, shopType ShopType, itemDefinitionID string, quantity int) (PurchaseResult, error) {
	if quantity <= 0 || quantity > MaxTransactionQuantity {
		return PurchaseResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return PurchaseResult{}, corecharacter.ErrNotFound
	}
	if !ValidateShopType(shopType) {
		return PurchaseResult{}, ErrInvalidShopType
	}

	definition, err := s.catalog.FindByID(itemDefinitionID)
	if err != nil {
		return PurchaseResult{}, ErrItemNotFound
	}

	if s.isItemInActiveHelper(ctx, itemDefinitionID) {
		return PurchaseResult{}, ErrItemUnavailable
	}

	unitPrice, err := s.CalculateRetailPriceForShop(shopType, itemDefinitionID, definition.Price)
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

		if shopType == ShopTypeAccessory {
			sales, err := GetSalesItemIDs(shopType, char.JobLevel)
			if err == nil {
				found := false
				for _, sid := range sales {
					if sid == itemDefinitionID {
						found = true
						break
					}
				}
				if !found {
					return ErrItemNotFound
				}
			}
		}

		// 2. Lock Inventory (Rank 3)
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		kind := definition.Kind()
		occupied := false
		for _, inst := range inv.Items {
			def, err := s.catalog.FindByID(inst.DefinitionID)
			if err == nil && def.Kind() == kind {
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
				//lint:ignore error-swallow best-effort collection discovery
				_ = s.recorder.RecordItemDiscovered(txCtx, characterID, definition.ID, definition.Name, definition.Category())
			}

			result = PurchaseResult{
				Character:          res.Character,
				Inventory:          res.Inventory,
				ItemInstance:       *res.GrantedItem,
				TotalPrice:         totalPrice,
				TransferredToDepot: false,
			}
			return nil
		}

		// Slot occupied or quantity > 1: Transfer to Depot (Rank 5)
		if s.depots == nil {
			return ErrDepotNotConfigured
		}

		dep, err := depot.FindOrCreate(txCtx, s.depots, char)
		if err != nil {
			return err
		}

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

		result = PurchaseResult{
			Character:          res.Character,
			Inventory:          inv,
			Depot:              &dep,
			ItemInstance:       inst,
			TotalPrice:         totalPrice,
			TransferredToDepot: true,
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
		newTotal, err := economy.SafeAdd(totalPrice, lineTotal)
		if err != nil {
			return BatchPurchaseResult{}, ErrPriceOverflow
		}
		totalPrice = newTotal

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
		dep, err := depot.FindOrCreate(txCtx, s.depots, char)
		if err != nil {
			return err
		}

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
	idx := random.Intn(len(words))
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
