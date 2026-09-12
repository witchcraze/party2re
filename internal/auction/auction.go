package auction

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

type Service struct {
	txProvider TransactionProvider
	charRepo   CharacterRepository
	equipRepo  EquipmentRepository
	invRepo    InventoryRepository
	depotRepo  DepotRepository
	catalog    ItemDefinitionProvider
	tabooItems map[string]bool
}

type Option func(*Service)

func WithTabooItems(ids ...string) Option {
	return func(s *Service) {
		for _, id := range ids {
			s.tabooItems[id] = true
		}
	}
}

func NewService(
	txProvider TransactionProvider,
	charRepo CharacterRepository,
	equipRepo EquipmentRepository,
	invRepo InventoryRepository,
	depotRepo DepotRepository,
	catalog ItemDefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if txProvider == nil {
		return nil, errors.New("transaction provider is required")
	}
	if charRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if equipRepo == nil {
		return nil, errors.New("equipment repository is required")
	}
	if invRepo == nil {
		return nil, errors.New("inventory repository is required")
	}
	if depotRepo == nil {
		return nil, errors.New("depot repository is required")
	}

	s := &Service{
		txProvider: txProvider,
		charRepo:   charRepo,
		equipRepo:  equipRepo,
		invRepo:    invRepo,
		depotRepo:  depotRepo,
		catalog:    catalog,
		tabooItems: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) isTaboo(definitionID string) bool {
	return s.tabooItems[definitionID]
}

func normalizeSlot(raw string) (coreitem.Slot, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "weapon", "wea", string(coreitem.SlotMainHand):
		return coreitem.SlotMainHand, nil
	case "armor", "arm", string(coreitem.SlotBody):
		return coreitem.SlotBody, nil
	case "item", "ite", string(coreitem.SlotAccessory):
		return coreitem.SlotAccessory, nil
	case "shield", string(coreitem.SlotOffHand):
		return coreitem.SlotOffHand, nil
	default:
		return "", ErrInvalidSlot
	}
}

func (s *Service) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	senderID := strings.TrimSpace(req.SenderCharacterID)
	if senderID == "" {
		return SendResult{}, errors.New("sender character ID is required")
	}

	targetID := strings.TrimSpace(req.TargetCharacterID)
	targetName := strings.TrimSpace(req.TargetCharacterName)
	if targetID == "" && targetName == "" {
		return SendResult{}, ErrInvalidSendTarget
	}

	if targetID == "" {
		targetChar, err := s.charRepo.FindByName(ctx, targetName)
		if err != nil {
			return SendResult{}, ErrTargetNotFound
		}
		targetID = targetChar.ID
	}

	if senderID == targetID {
		return SendResult{}, ErrCannotSendToSelf
	}

	if req.Gold < 0 {
		return SendResult{}, ErrInvalidSendAmount
	}

	hasGold := req.Gold > 0
	hasItem := strings.TrimSpace(req.Slot) != "" || strings.TrimSpace(req.InstanceID) != ""

	if !hasGold && !hasItem {
		return SendResult{}, ErrNothingToSend
	}

	var result SendResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Characters locked ascending by ID to prevent deadlocks
		firstID, secondID := id.Sort2(senderID, targetID)
		firstChar, err := s.charRepo.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return ErrTargetNotFound
		}
		secondChar, err := s.charRepo.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return ErrTargetNotFound
		}

		var senderChar, receiverChar *corecharacter.Character
		if firstChar.ID == senderID {
			senderChar = &firstChar
			receiverChar = &secondChar
		} else {
			senderChar = &secondChar
			receiverChar = &firstChar
		}

		if hasGold {
			if req.Gold <= 0 {
				return ErrInvalidSendAmount
			}
			if err := senderChar.DeductMoney(req.Gold); err != nil {
				return ErrInsufficientMoney
			}
			if err := receiverChar.AddMoney(req.Gold); err != nil {
				return err
			}

			if err := s.charRepo.Update(txCtx, firstChar); err != nil {
				return err
			}
			if err := s.charRepo.Update(txCtx, secondChar); err != nil {
				return err
			}

			result = SendResult{
				SenderCharacterID:   senderID,
				TargetCharacterID:   targetID,
				TargetCharacterName: receiverChar.Name,
				TransferredGold:     req.Gold,
				Message:             fmt.Sprintf("%d Gを %s に送りました", req.Gold, receiverChar.Name),
			}
			return nil
		}

		// Transfer Item:
		// Rank 3: Sender Inventory lock
		senderInv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, senderID)
		if err != nil {
			return err
		}

		// Rank 5: Receiver Depot lock
		receiverDepot, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, targetID)
		if err != nil {
			return err
		}
		receiverDepot.RefreshCapacity(receiverChar.JobLevel, receiverChar.OverDepot)

		senderEquip, err := s.equipRepo.FindByCharacterID(txCtx, senderID)
		if err != nil {
			return err
		}

		var targetInstanceID string
		var unequipSlot coreitem.Slot

		if req.Slot != "" {
			slot, err := normalizeSlot(req.Slot)
			if err != nil {
				return err
			}
			instID, ok := senderEquip.Equipped(slot)
			if !ok || instID == "" {
				return ErrItemNotEquipped
			}
			targetInstanceID = instID
			unequipSlot = slot
		} else {
			targetInstanceID = strings.TrimSpace(req.InstanceID)
			for sl, instID := range senderEquip.Slots {
				if instID == targetInstanceID {
					unequipSlot = sl
					break
				}
			}
		}

		inst, ok := senderInv.Find(targetInstanceID)
		if !ok {
			return ErrItemNotFound
		}

		if s.isTaboo(inst.DefinitionID) {
			return ErrTabooItem
		}

		hasSlot := false
		for _, existing := range receiverDepot.Items {
			if existing.DefinitionID == inst.DefinitionID {
				hasSlot = true
				break
			}
		}
		if !hasSlot && len(receiverDepot.Items) >= receiverDepot.Capacity {
			return ErrDepotFull
		}

		itemName := inst.DefinitionID
		if s.catalog != nil {
			if def, err := s.catalog.FindByID(inst.DefinitionID); err == nil && def.Name != "" {
				itemName = def.Name
			}
		}

		if unequipSlot != "" {
			_, _ = senderEquip.Unequip(unequipSlot)
			if err := s.equipRepo.Save(txCtx, senderEquip); err != nil {
				return err
			}
		}

		if err := senderInv.Consume(targetInstanceID, 1); err != nil {
			return err
		}
		if err := s.invRepo.Save(txCtx, senderInv); err != nil {
			return err
		}

		transferredInst := coreitem.Instance{
			ID:               id.New(),
			DefinitionID:     inst.DefinitionID,
			Quantity:         1,
			EnhancementLevel: inst.EnhancementLevel,
		}
		if err := receiverDepot.AddItem(transferredInst); err != nil {
			if errors.Is(err, depot.ErrDepotFull) {
				return ErrDepotFull
			}
			return err
		}
		if err := s.depotRepo.Save(txCtx, receiverDepot); err != nil {
			return err
		}

		result = SendResult{
			SenderCharacterID:   senderID,
			TargetCharacterID:   targetID,
			TargetCharacterName: receiverChar.Name,
			TransferredItem: &TransferredItemInfo{
				InstanceID:       transferredInst.ID,
				DefinitionID:     inst.DefinitionID,
				ItemName:         itemName,
				EnhancementLevel: inst.EnhancementLevel,
			},
			Message: fmt.Sprintf("%sを%sに送りました", itemName, receiverChar.Name),
		}
		return nil
	})

	if err != nil {
		return SendResult{}, err
	}
	return result, nil
}

func (s *Service) Inspect(ctx context.Context, inspectorID, targetID string) (InspectResult, error) {
	targetChar, err := s.charRepo.FindByID(ctx, targetID)
	if err != nil {
		return InspectResult{}, ErrTargetNotFound
	}
	return s.buildInspectResult(ctx, targetChar)
}

func (s *Service) InspectByName(ctx context.Context, inspectorID, targetName string) (InspectResult, error) {
	targetChar, err := s.charRepo.FindByName(ctx, targetName)
	if err != nil {
		return InspectResult{}, ErrTargetNotFound
	}
	return s.buildInspectResult(ctx, targetChar)
}

func (s *Service) buildInspectResult(ctx context.Context, char corecharacter.Character) (InspectResult, error) {
	res := InspectResult{
		CharacterID: char.ID,
		Name:        char.Name,
		Level:       char.Level,
		JobID:       char.JobID,
		JobLevel:    char.JobLevel,
		Money:       char.Money,
		Message:     char.Color,
	}

	equip, err := s.equipRepo.FindByCharacterID(ctx, char.ID)
	if err == nil {
		inv, err := s.invRepo.FindByCharacterID(ctx, char.ID)
		if err == nil {
			if weaponID, ok := equip.Equipped(coreitem.SlotMainHand); ok {
				if itemInst, ok := inv.Find(weaponID); ok {
					name := itemInst.DefinitionID
					if s.catalog != nil {
						if def, err := s.catalog.FindByID(itemInst.DefinitionID); err == nil && def.Name != "" {
							name = def.Name
						}
					}
					res.Weapon = &EquippedItemInfo{
						InstanceID:       itemInst.ID,
						DefinitionID:     itemInst.DefinitionID,
						Name:             name,
						EnhancementLevel: itemInst.EnhancementLevel,
					}
				}
			}
			if armorID, ok := equip.Equipped(coreitem.SlotBody); ok {
				if itemInst, ok := inv.Find(armorID); ok {
					name := itemInst.DefinitionID
					if s.catalog != nil {
						if def, err := s.catalog.FindByID(itemInst.DefinitionID); err == nil && def.Name != "" {
							name = def.Name
						}
					}
					res.Armor = &EquippedItemInfo{
						InstanceID:       itemInst.ID,
						DefinitionID:     itemInst.DefinitionID,
						Name:             name,
						EnhancementLevel: itemInst.EnhancementLevel,
					}
				}
			}
			if accID, ok := equip.Equipped(coreitem.SlotAccessory); ok {
				if itemInst, ok := inv.Find(accID); ok {
					name := itemInst.DefinitionID
					if s.catalog != nil {
						if def, err := s.catalog.FindByID(itemInst.DefinitionID); err == nil && def.Name != "" {
							name = def.Name
						}
					}
					res.Accessory = &EquippedItemInfo{
						InstanceID:       itemInst.ID,
						DefinitionID:     itemInst.DefinitionID,
						Name:             name,
						EnhancementLevel: itemInst.EnhancementLevel,
					}
				}
			}
		}
	}

	return res, nil
}

func (s *Service) GetVenueInfo() VenueInfo {
	return VenueInfo{
		Title:    VenueName,
		NPCName:  NPCName,
		Dialogue: DialogueWords,
	}
}
