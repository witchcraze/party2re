package plantation

import (
	"errors"
	"time"
)

var (
	ErrNoActivePlot             = errors.New("no active plantation plot")
	ErrPlotAlreadySown          = errors.New("plantation plot is already sown")
	ErrFertilizerAlreadyApplied = errors.New("fertilizer is already applied to this plot")
	ErrCropNotMatured           = errors.New("crop has not yet matured")
	ErrInsufficientGold         = errors.New("insufficient gold")
	ErrMissingFertilizerItem    = errors.New("required fertilizer item not found in depot or inventory")
	ErrInvalidSeedID            = errors.New("invalid seed id")
	ErrInvalidFertilizerID      = errors.New("invalid fertilizer id")
	ErrInvalidCharacterID       = errors.New("invalid character id")
	ErrPlotNotFound             = errors.New("plantation plot not found")
)

// PlotStatus represents the current lifecycle status of a plantation plot.
type PlotStatus string

const (
	StatusNone    PlotStatus = "none"
	StatusGrowing PlotStatus = "growing"
	StatusReady   PlotStatus = "ready"
)

// Seed defines the parameters and reward distribution for a seed variety.
type Seed struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Price    int    `json:"price"`
	HighBase int    `json:"high_base"`
	HighRand int    `json:"high_rand"`
	LowBase  int    `json:"low_base"`
	LowRand  int    `json:"low_rand"`
	HighRate int    `json:"high_rate"` // High quality item chance percentage
}

// Fertilizer defines the cost, bonuses, and wither rates of a fertilizer reagent.
type Fertilizer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Price      int    `json:"price"`
	ItemID     string `json:"item_id,omitempty"`
	ProbBonus  int    `json:"prob_bonus"`  // High quality probability bonus percentage
	WitherRate int    `json:"wither_rate"` // Wither failure rate percentage
	YieldBonus int    `json:"yield_bonus"` // Extra items bonus (int(rand(yield_bonus + 1)))
	IsItem     bool   `json:"is_item"`     // If true, consumed from depot/inventory
}

// Plot represents an active crop plot for a character.
type Plot struct {
	CharacterID  string    `json:"character_id"`
	SeedID       string    `json:"seed_id"`
	FertilizerID *string   `json:"fertilizer_id,omitempty"`
	SownAt       time.Time `json:"sown_at"`
	MaturesAt    time.Time `json:"matures_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StatusResponse represents the complete plantation view for a character.
type StatusResponse struct {
	Plot        *Plot        `json:"plot,omitempty"`
	Status      PlotStatus   `json:"status"`
	Seeds       []Seed       `json:"seeds"`
	Fertilizers []Fertilizer `json:"fertilizers"`
	Dialogue    string       `json:"dialogue"`
}

// SowResult represents the outcome of sowing a seed.
type SowResult struct {
	Plot    Plot   `json:"plot"`
	Message string `json:"message"`
}

// FertilizeResult represents the outcome of applying fertilizer.
type FertilizeResult struct {
	Plot    Plot   `json:"plot"`
	Message string `json:"message"`
}

// HarvestYield represents a harvested item batch.
type HarvestYield struct {
	ItemID   string `json:"item_id"`
	ItemName string `json:"item_name"`
	Quantity int    `json:"quantity"`
}

// HarvestResult represents the outcome of harvesting a plot.
type HarvestResult struct {
	Withered bool           `json:"withered"`
	Yields   []HarvestYield `json:"yields,omitempty"`
	Message  string         `json:"message"`
}
