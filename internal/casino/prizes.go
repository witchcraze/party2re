package casino

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

var (
	ErrPrizeNotFound = errors.New("casino prize not found")
	ErrInvalidCount  = errors.New("exchange count must be positive")
)

type Prize struct {
	CostCoins int64  `json:"cost_coins"`
	ItemID    string `json:"item_id"`
	ItemName  string `json:"item_name"`
	Category  int    `json:"category"` // 1: weapon, 2: armor, 3: consumable
}

// CasinoPrizes contains the authentic 18 casino prizes from party2/lib/casino.cgi:41-65.
var CasinoPrizes = []Prize{
	{CostCoins: 100, ItemID: "item-004", ItemName: "賢者の石", Category: 3},
	{CostCoins: 300, ItemID: "item-012", ItemName: "祈りの指輪", Category: 3},
	{CostCoins: 700, ItemID: "item-006", ItemName: "霊樹の葉", Category: 3},
	{CostCoins: 2000, ItemID: "item-032", ItemName: "物真似の心", Category: 3},
	{CostCoins: 4000, ItemID: "item-038", ItemName: "幻獣の実", Category: 3},
	{CostCoins: 5000, ItemID: "item-039", ItemName: "ギャンブルハート", Category: 3},
	{CostCoins: 8000, ItemID: "armor-34", ItemName: "危ない水着", Category: 2},
	{CostCoins: 30000, ItemID: "weapon-31", ItemName: "必殺のピアス", Category: 1},
	{CostCoins: 70000, ItemID: "weapon-40", ItemName: "流銀の剣", Category: 1},
	{CostCoins: 80000, ItemID: "weapon-38", ItemName: "茨の霊鞭", Category: 1},
	{CostCoins: 180000, ItemID: "item-106", ItemName: "金の鶏", Category: 3},
	{CostCoins: 200000, ItemID: "item-105", ItemName: "幸せのくつ", Category: 3},
	{CostCoins: 1000000, ItemID: "item-231", ItemName: "宇宙の壁紙", Category: 3},
	{CostCoins: 1000001, ItemID: "item-232", ItemName: "蟻地獄の壁紙", Category: 3},
	{CostCoins: 1000002, ItemID: "item-233", ItemName: "炎の壁紙", Category: 3},
	{CostCoins: 1000003, ItemID: "item-234", ItemName: "墓場の壁紙", Category: 3},
	{CostCoins: 1000004, ItemID: "item-235", ItemName: "図書館の壁紙", Category: 3},
	{CostCoins: 1000005, ItemID: "item-236", ItemName: "要塞の壁紙", Category: 3},
}

// GetPrizes returns a copy of all available casino prizes.
func GetPrizes() []Prize {
	prizes := make([]Prize, len(CasinoPrizes))
	copy(prizes, CasinoPrizes)
	return prizes
}

// GetPrizeByCost returns the prize associated with the given coin cost.
func GetPrizeByCost(cost int64) (Prize, error) {
	for _, p := range CasinoPrizes {
		if p.CostCoins == cost {
			return p, nil
		}
	}
	return Prize{}, fmt.Errorf("%w: cost %d", ErrPrizeNotFound, cost)
}

type PrizeExchangeResult struct {
	Prize              Prize `json:"prize"`
	Count              int   `json:"count"`
	TotalCostCoins     int64 `json:"total_cost_coins"`
	TransferredToDepot bool  `json:"transferred_to_depot"`
	RemainingCoins     int64 `json:"remaining_coins"`
}

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, characterID string) (corecharacter.Character, error)
}

// ExchangePrize exchanges character's casino coins for prize items and sends them directly
// to the character's depot storage (party2/lib/casino.cgi:596-647).
func (s *Service) ExchangePrize(ctx context.Context, characterID string, costCoins int64, count int) (PrizeExchangeResult, error) {
	if characterID == "" {
		return PrizeExchangeResult{}, ErrInvalidCharacterID
	}
	if count <= 0 {
		return PrizeExchangeResult{}, ErrInvalidCount
	}
	prize, err := GetPrizeByCost(costCoins)
	if err != nil {
		return PrizeExchangeResult{}, err
	}
	if s.depotRepo == nil {
		return PrizeExchangeResult{}, errors.New("depot repository is required for prize exchange")
	}

	totalCost := prize.CostCoins * int64(count)
	var remainingCoins int64

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		var char corecharacter.Character
		if s.charRepo != nil {
			c, err := s.charRepo.FindByID(txCtx, characterID)
			if err == nil {
				char = c
			}
		}

		dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if errors.Is(err, depot.ErrNotFound) {
			dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		dep.RefreshCapacity(char.JobLevel, char.OverDepot)

		acc, err := s.repo.GetAccountForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if acc.Coins < totalCost {
			return ErrInsufficientCoins
		}

		itemInst, err := item.NewInstance(prize.ItemID, 1)
		if err != nil {
			return fmt.Errorf("failed to create prize item instance %s: %w", prize.ItemID, err)
		}

		for i := 0; i < count; i++ {
			if err := dep.AddItem(itemInst); err != nil {
				return err
			}
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		updatedAcc, err := s.repo.AdjustCoins(txCtx, characterID, -totalCost)
		if err != nil {
			return err
		}
		remainingCoins = updatedAcc.Coins
		return nil
	})
	if err != nil {
		return PrizeExchangeResult{}, err
	}

	return PrizeExchangeResult{
		Prize:              prize,
		Count:              count,
		TotalCostCoins:     totalCost,
		TransferredToDepot: true,
		RemainingCoins:     remainingCoins,
	}, nil
}
