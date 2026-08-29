package farm

import (
	"context"
	"errors"
	"time"
)

const (
	MaxPlotsPerCharacter = 4
	DefaultGraceDuration = 24 * time.Hour
)

type PlotStatus string

const (
	PlotStatusEmpty    PlotStatus = "EMPTY"
	PlotStatusGrowing  PlotStatus = "GROWING"
	PlotStatusMature   PlotStatus = "MATURE"
	PlotStatusWithered PlotStatus = "WITHERED"
)

type SeedType string

const (
	SeedHerb      SeedType = "seed_herb"
	SeedMandrake  SeedType = "seed_mandrake"
	SeedMoonlight SeedType = "seed_moonlight"
	SeedGolden    SeedType = "seed_golden"
)

type SeedDefinition struct {
	Type           SeedType      `json:"type"`
	Name           string        `json:"name"`
	GrowthDuration time.Duration `json:"growth_duration"`
	GraceDuration  time.Duration `json:"grace_duration"`
	BaseYield      int           `json:"base_yield"`
	RewardItemID   string        `json:"reward_item_id"`
	RewardGold     int           `json:"reward_gold"`
}

var SeedCatalog = map[SeedType]SeedDefinition{
	SeedHerb: {
		Type:           SeedHerb,
		Name:           "Medicinal Herb Seed",
		GrowthDuration: 5 * time.Minute,
		GraceDuration:  DefaultGraceDuration,
		BaseYield:      2,
		RewardItemID:   "item_medicinal_herb",
		RewardGold:     50,
	},
	SeedMandrake: {
		Type:           SeedMandrake,
		Name:           "Mandrake Root Seed",
		GrowthDuration: 15 * time.Minute,
		GraceDuration:  DefaultGraceDuration,
		BaseYield:      1,
		RewardItemID:   "item_mandrake_root",
		RewardGold:     200,
	},
	SeedMoonlight: {
		Type:           SeedMoonlight,
		Name:           "Moonlight Flower Seed",
		GrowthDuration: 30 * time.Minute,
		GraceDuration:  DefaultGraceDuration,
		BaseYield:      1,
		RewardItemID:   "item_moonlight_flower",
		RewardGold:     500,
	},
	SeedGolden: {
		Type:           SeedGolden,
		Name:           "Golden Apple Seed",
		GrowthDuration: 60 * time.Minute,
		GraceDuration:  DefaultGraceDuration,
		BaseYield:      1,
		RewardItemID:   "item_golden_fruit",
		RewardGold:     2000,
	},
}

const FertilizerItemID = "item_fertilizer"

var (
	ErrInvalidPlotIndex       = errors.New("invalid plot index (0 to 3)")
	ErrInvalidSeedType        = errors.New("invalid seed type")
	ErrPlotNotEmpty           = errors.New("plot is not empty")
	ErrPlotNotGrowing         = errors.New("plot has no growing crop")
	ErrPlotNotMature          = errors.New("crop is not yet mature")
	ErrPlotWithered           = errors.New("crop has withered")
	ErrAlreadyWatered         = errors.New("crop is already watered")
	ErrAlreadyFertilized      = errors.New("crop is already fertilized")
	ErrPlotNotFound           = errors.New("farm plot not found")
	ErrInsufficientSeed       = errors.New("insufficient seed in inventory")
	ErrInsufficientFertilizer = errors.New("insufficient fertilizer in inventory")
)

type FarmPlot struct {
	ID          string     `json:"id"`
	CharacterID string     `json:"character_id"`
	PlotIndex   int        `json:"plot_index"`
	SeedType    SeedType   `json:"seed_type"`
	Status      PlotStatus `json:"status"`
	PlantedAt   *time.Time `json:"planted_at,omitempty"`
	MaturesAt   *time.Time `json:"matures_at,omitempty"`
	WitherAt    *time.Time `json:"wither_at,omitempty"`
	Watered     bool       `json:"watered"`
	Fertilized  bool       `json:"fertilized"`
	Yield       int        `json:"yield"`
}

// ComputeCurrentStatus updates the in-memory status of a plot based on the reference timestamp.
func (p *FarmPlot) ComputeCurrentStatus(now time.Time) PlotStatus {
	if p.Status == PlotStatusEmpty || p.PlantedAt == nil || p.MaturesAt == nil {
		return PlotStatusEmpty
	}
	if p.WitherAt != nil && !now.Before(*p.WitherAt) {
		return PlotStatusWithered
	}
	if !now.Before(*p.MaturesAt) {
		return PlotStatusMature
	}
	return PlotStatusGrowing
}

type HarvestResult struct {
	PlotIndex    int      `json:"plot_index"`
	SeedType     SeedType `json:"seed_type"`
	Yield        int      `json:"yield"`
	RewardItemID string   `json:"reward_item_id"`
	RewardGold   int      `json:"reward_gold"`
}

type Repository interface {
	GetPlots(ctx context.Context, characterID string) ([]FarmPlot, error)
	GetPlot(ctx context.Context, characterID string, plotIndex int) (FarmPlot, error)
	PlantSeed(ctx context.Context, characterID string, plotIndex int, seedType SeedType, seedDef SeedDefinition, now time.Time) (FarmPlot, error)
	WaterPlot(ctx context.Context, characterID string, plotIndex int, now time.Time) (FarmPlot, error)
	FertilizePlot(ctx context.Context, characterID string, plotIndex int, now time.Time) (FarmPlot, error)
	HarvestPlot(ctx context.Context, characterID string, plotIndex int, rewardGold int, rewardItemID string, yield int, now time.Time) (FarmPlot, error)
	ClearPlot(ctx context.Context, characterID string, plotIndex int) (FarmPlot, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) GetPlots(ctx context.Context, characterID string) ([]FarmPlot, error) {
	plots, err := s.repo.GetPlots(ctx, characterID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range plots {
		plots[i].Status = plots[i].ComputeCurrentStatus(now)
	}
	return plots, nil
}

func (s *Service) PlantSeed(ctx context.Context, characterID string, plotIndex int, seedType SeedType) (FarmPlot, error) {
	if plotIndex < 0 || plotIndex >= MaxPlotsPerCharacter {
		return FarmPlot{}, ErrInvalidPlotIndex
	}
	seedDef, exists := SeedCatalog[seedType]
	if !exists {
		return FarmPlot{}, ErrInvalidSeedType
	}

	now := time.Now().UTC()
	return s.repo.PlantSeed(ctx, characterID, plotIndex, seedType, seedDef, now)
}

func (s *Service) WaterPlot(ctx context.Context, characterID string, plotIndex int) (FarmPlot, error) {
	if plotIndex < 0 || plotIndex >= MaxPlotsPerCharacter {
		return FarmPlot{}, ErrInvalidPlotIndex
	}

	now := time.Now().UTC()
	return s.repo.WaterPlot(ctx, characterID, plotIndex, now)
}

func (s *Service) FertilizePlot(ctx context.Context, characterID string, plotIndex int) (FarmPlot, error) {
	if plotIndex < 0 || plotIndex >= MaxPlotsPerCharacter {
		return FarmPlot{}, ErrInvalidPlotIndex
	}

	now := time.Now().UTC()
	return s.repo.FertilizePlot(ctx, characterID, plotIndex, now)
}

func (s *Service) HarvestPlot(ctx context.Context, characterID string, plotIndex int) (HarvestResult, FarmPlot, error) {
	if plotIndex < 0 || plotIndex >= MaxPlotsPerCharacter {
		return HarvestResult{}, FarmPlot{}, ErrInvalidPlotIndex
	}

	plot, err := s.repo.GetPlot(ctx, characterID, plotIndex)
	if err != nil {
		return HarvestResult{}, FarmPlot{}, err
	}

	now := time.Now().UTC()
	status := plot.ComputeCurrentStatus(now)
	if status == PlotStatusWithered {
		return HarvestResult{}, plot, ErrPlotWithered
	}
	if status != PlotStatusMature {
		return HarvestResult{}, plot, ErrPlotNotMature
	}

	seedDef, exists := SeedCatalog[plot.SeedType]
	if !exists {
		return HarvestResult{}, FarmPlot{}, ErrInvalidSeedType
	}

	totalRewardGold := seedDef.RewardGold * plot.Yield
	harvestRes := HarvestResult{
		PlotIndex:    plotIndex,
		SeedType:     plot.SeedType,
		Yield:        plot.Yield,
		RewardItemID: seedDef.RewardItemID,
		RewardGold:   totalRewardGold,
	}

	clearedPlot, err := s.repo.HarvestPlot(ctx, characterID, plotIndex, totalRewardGold, seedDef.RewardItemID, plot.Yield, now)
	if err != nil {
		return HarvestResult{}, FarmPlot{}, err
	}
	return harvestRes, clearedPlot, nil
}

func (s *Service) ClearPlot(ctx context.Context, characterID string, plotIndex int) (FarmPlot, error) {
	if plotIndex < 0 || plotIndex >= MaxPlotsPerCharacter {
		return FarmPlot{}, ErrInvalidPlotIndex
	}
	return s.repo.ClearPlot(ctx, characterID, plotIndex)
}
