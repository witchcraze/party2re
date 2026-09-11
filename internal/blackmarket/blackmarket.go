package blackmarket

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type BlackMarketRepository interface {
	GetCharacterPoints(ctx context.Context, characterID string) (CharacterPoints, error)
	GetCharacterPointsForUpdate(ctx context.Context, characterID string) (CharacterPoints, error)
	SaveCharacterPoints(ctx context.Context, points CharacterPoints) error
}

type ItemDefinitionProvider = coreitem.DefinitionProvider

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Service struct {
	characterRepo   CharacterRepository
	inventoryRepo   InventoryRepository
	depotRepo       DepotRepository
	blackMarketRepo BlackMarketRepository
	catalog         *Catalog
	itemDefs        ItemDefinitionProvider
	txProvider      TransactionProvider
}

type ServiceOption func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) ServiceOption {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithItemDefinitionProvider(itemDefs ItemDefinitionProvider) ServiceOption {
	return func(s *Service) {
		s.itemDefs = itemDefs
	}
}

func WithDepotRepository(depotRepo DepotRepository) ServiceOption {
	return func(s *Service) {
		s.depotRepo = depotRepo
	}
}

// CheckEligibility returns whether a character can enter the Black Market.
// In the authentic Party2 specification, any character can enter unconditionally.
func CheckEligibility(_ corecharacter.Character) bool {
	return true
}

func NewService(
	characterRepo CharacterRepository,
	inventoryRepo InventoryRepository,
	blackMarketRepo BlackMarketRepository,
	catalog *Catalog,
	opts ...ServiceOption,
) (*Service, error) {
	if characterRepo == nil || inventoryRepo == nil || blackMarketRepo == nil || catalog == nil {
		return nil, ErrNilDependency
	}

	svc := &Service{
		characterRepo:   characterRepo,
		inventoryRepo:   inventoryRepo,
		blackMarketRepo: blackMarketRepo,
		catalog:         catalog,
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc, nil
}

// GetStatus returns Black Market status including character points and prize offerings.
func (s *Service) GetStatus(ctx context.Context, characterID string) (*Status, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
	if s.blackMarketRepo != nil {
		if pts, err := s.blackMarketRepo.GetCharacterPoints(ctx, characterID); err == nil {
			points = pts
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	return &Status{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		RarePoints:   points.RarePoints,
		URarePoints:  points.URarePoints,
		Prizes:       s.catalog.RegularPrizes(),
		UPrizes:      s.catalog.UPrizes(),
	}, nil
}

// GetPointsStatus is an alias for GetStatus for compatibility.
func (s *Service) GetPointsStatus(ctx context.Context, characterID string) (*Status, error) {
	return s.GetStatus(ctx, characterID)
}

// Talk returns random atmospheric underworld dialogue from NPC @闇商人.
func (s *Service) Talk(ctx context.Context, characterID string) (*TalkResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	dialogues := DefaultTalkDialogues
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(dialogues))))
	if err != nil {
		return nil, err
	}
	dialogue := dialogues[n.Int64()]

	return &TalkResult{
		CharacterID: char.ID,
		NPCName:     NPCName,
		Dialogue:    dialogue,
	}, nil
}

// Inspect returns NPC @闇商人 inspection dialogue matching legacy Party2 CGI.
func (s *Service) Inspect(ctx context.Context, characterID string) (*TalkResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	return &TalkResult{
		CharacterID: char.ID,
		NPCName:     NPCName,
		Dialogue:    InspectDialogue,
	}, nil
}

// SacrificeItem consumes an eligible rare item instance from character inventory or depot and credits points.
func (s *Service) SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*SacrificeResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return nil, ErrUnownedItem
	}

	var result *SacrificeResult

	operation := func(txCtx context.Context) error {
		// 1. Lock character (Tier 2)
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Lock inventory (Tier 3) and search for item
		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, foundInInv := inv.Find(itemInstanceID)
		var (
			targetDefID string
			foundInDep  bool
			dep         depot.Depot
		)

		if foundInInv {
			targetDefID = inst.DefinitionID
		} else if s.depotRepo != nil {
			// 3. Lock depot (Tier 5) and search for item
			var depErr error
			dep, depErr = s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
			if depErr != nil {
				return depErr
			}
			for _, dItem := range dep.Items {
				if dItem.ID == itemInstanceID {
					targetDefID = dItem.DefinitionID
					foundInDep = true
					break
				}
			}
		}

		if !foundInInv && !foundInDep {
			return ErrUnownedItem
		}

		// Check sacrifice eligibility
		yield, eligible := s.catalog.GetSacrificeYield(targetDefID)
		if !eligible {
			return ErrNotSacrificeEligible
		}

		// Consume the item
		if foundInInv {
			if err := inv.Consume(itemInstanceID, 1); err != nil {
				return err
			}
			if err := s.inventoryRepo.Save(txCtx, inv); err != nil {
				return err
			}
		} else if foundInDep {
			if _, err := dep.RemoveItem(itemInstanceID); err != nil {
				return err
			}
			if err := s.depotRepo.Save(txCtx, dep); err != nil {
				return err
			}
		}

		// 4. Lock and update points (Tier 8)
		points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
		if s.blackMarketRepo != nil {
			pts, err := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, characterID)
			if err == nil {
				points = pts
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}

		points.RarePoints += yield.RarePoints
		points.URarePoints += yield.URarePoints

		if s.blackMarketRepo != nil {
			if err := s.blackMarketRepo.SaveCharacterPoints(txCtx, points); err != nil {
				return err
			}
		}

		itemName := targetDefID
		if s.itemDefs != nil {
			if def, err := s.itemDefs.FindByID(targetDefID); err == nil && def.Name != "" {
				itemName = def.Name
			}
		}

		msg := fmt.Sprintf("…%s…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…", itemName)
		if yield.URarePoints > 0 {
			msg = fmt.Sprintf("これは……! ……いいだろう…。お前の特別なレアポイントを%d加算しておこう…", yield.URarePoints)
		}

		result = &SacrificeResult{
			CharacterID:       char.ID,
			ItemInstanceID:    itemInstanceID,
			ItemDefinitionID:  targetDefID,
			ItemName:          itemName,
			RarePointsGained:  yield.RarePoints,
			URarePointsGained: yield.URarePoints,
			TotalRarePoints:   points.RarePoints,
			TotalURarePoints:  points.URarePoints,
			Message:           msg,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return nil, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// TradePrize exchanges accumulated Rare Points or U-Rare Points for an exclusive prize item deposited to character Depot.
func (s *Service) TradePrize(ctx context.Context, characterID string, prizeID string) (*TradeResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(prizeID) == "" {
		return nil, ErrPrizeNotFound
	}

	prize, found := s.catalog.FindPrizeByID(prizeID)
	if !found {
		return nil, ErrPrizeNotFound
	}

	if s.depotRepo == nil {
		return nil, ErrDepotNotConfigured
	}

	var result *TradeResult

	operation := func(txCtx context.Context) error {
		// 1. Lock character (Tier 2)
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Lock depot (Tier 5) and verify capacity
		dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		dep.Capacity = depot.CalculateCapacity(char.JobLevel, dep.ExDepot, char.OverDepot)

		// 3. Lock and check points (Tier 8)
		points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
		if s.blackMarketRepo != nil {
			pts, err := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, characterID)
			if err == nil {
				points = pts
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}

		if prize.IsURare {
			if points.URarePoints < prize.Cost {
				return ErrInsufficientURarePoints
			}
			points.URarePoints -= prize.Cost
		} else {
			if points.RarePoints < prize.Cost {
				return ErrInsufficientRarePoints
			}
			points.RarePoints -= prize.Cost
		}

		// 4. Deliver prize item into depot
		prizeItem, err := coreitem.NewInstance(prize.ItemDefinitionID, 1)
		if err != nil {
			return err
		}
		if err := dep.AddItem(prizeItem); err != nil {
			return ErrDepotFull
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		if s.blackMarketRepo != nil {
			if err := s.blackMarketRepo.SaveCharacterPoints(txCtx, points); err != nil {
				return err
			}
		}

		result = &TradeResult{
			CharacterID:      char.ID,
			PrizeID:          prize.ID,
			ItemDefinitionID: prize.ItemDefinitionID,
			ItemName:         prize.Name,
			DepotInstanceID:  prizeItem.ID,
			Cost:             prize.Cost,
			IsURare:          prize.IsURare,
			RemainingRare:    points.RarePoints,
			RemainingURare:   points.URarePoints,
			Message:          "取引成立だ…。" + prize.Name + " はお前の預かり所に送っておいた…",
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return nil, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}
