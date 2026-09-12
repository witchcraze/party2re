package plantation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// Harvest collects the matured crop, evaluates wither and yield bonuses, and deposits harvested items into Depot.
func (s *Service) Harvest(ctx context.Context, characterID string) (HarvestResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return HarvestResult{}, ErrInvalidCharacterID
	}

	var res HarvestResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Lock Character
		_, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 5: Lock Depot
		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 8: Lock Plantation Plot
		plot, err := s.plots.GetPlotForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, ErrPlotNotFound) {
				return ErrNoActivePlot
			}
			return err
		}

		now := s.now()
		if now.Before(plot.MaturesAt) {
			return ErrCropNotMatured
		}

		seed, ok := FindSeed(plot.SeedID)
		if !ok {
			return ErrInvalidSeedID
		}

		fert := GetDefaultFertilizer()
		if plot.FertilizerID != nil {
			if f, found := FindFertilizer(*plot.FertilizerID); found {
				fert = f
			}
		}

		// Check Wither failure rate: $ferts[$plant_fert][3] >= rand(100)
		witherRoll := s.rng.Intn(100)
		if witherRoll < fert.WitherRate {
			if err := s.plots.DeletePlot(txCtx, characterID); err != nil {
				return fmt.Errorf("delete plantation plot: %w", err)
			}
			res = HarvestResult{
				Withered: true,
				Message:  fmt.Sprintf("%sは芽が出なかったよ…", seed.Name),
			}
			return nil
		}

		// Determine yield count: 1 + int(rand(fert.YieldBonus + 1))
		yieldCount := 1
		if fert.YieldBonus > 0 {
			yieldCount += s.rng.Intn(fert.YieldBonus + 1)
		}

		// Generate items according to legacy formula
		itemQuantities := make(map[string]int)
		for i := 0; i < yieldCount; i++ {
			highRoll := s.rng.Intn(100)
			highChance := seed.HighRate + fert.ProbBonus
			var itemNo int
			if highRoll < highChance {
				// High quality
				itemNo = seed.HighBase
				if seed.HighRand > 0 {
					itemNo += s.rng.Intn(seed.HighRand)
				}
			} else {
				// Low quality
				itemNo = seed.LowBase
				if seed.LowRand > 0 {
					itemNo += s.rng.Intn(seed.LowRand)
				}
			}
			itemID := fmt.Sprintf("item-%03d", itemNo)
			itemQuantities[itemID]++
		}

		// Deposit items into Depot
		type itemBatch struct {
			id       string
			name     string
			quantity int
		}
		batches := make([]itemBatch, 0, len(itemQuantities))
		for itemID, qty := range itemQuantities {
			def, defErr := s.items.FindByID(itemID)
			itemName := itemID
			if defErr == nil {
				itemName = def.Name
			}
			batches = append(batches, itemBatch{id: itemID, name: itemName, quantity: qty})
		}
		sort.Slice(batches, func(i, j int) bool {
			return batches[i].id < batches[j].id
		})

		for _, b := range batches {
			inst, err := coreitem.NewInstance(b.id, b.quantity)
			if err != nil {
				return fmt.Errorf("create harvested item instance: %w", err)
			}
			if err := dep.AddItem(inst); err != nil {
				return fmt.Errorf("add item to depot: %w", err)
			}
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}

		if err := s.plots.DeletePlot(txCtx, characterID); err != nil {
			return fmt.Errorf("delete plantation plot: %w", err)
		}

		yields := make([]HarvestYield, 0, len(batches))
		var msgBuilder strings.Builder
		msgBuilder.WriteString("収穫したよ！<br>")
		for _, b := range batches {
			yields = append(yields, HarvestYield{
				ItemID:   b.id,
				ItemName: b.name,
				Quantity: b.quantity,
			})
			msgBuilder.WriteString(fmt.Sprintf("%sを%d個<br>", b.name, b.quantity))
		}
		msgBuilder.WriteString("倉庫に送っておいたよ")

		res = HarvestResult{
			Withered: false,
			Yields:   yields,
			Message:  msgBuilder.String(),
		}
		return nil
	})
	if err != nil {
		return HarvestResult{}, err
	}

	return res, nil
}
