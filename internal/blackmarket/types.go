package blackmarket

import (
	"errors"
)

const (
	NPCName      = "@闇商人"
	LocationName = "闇市場"
)

var (
	ErrNilDependency           = errors.New("black market dependency is nil")
	ErrCharacterNotFound       = errors.New("character not found")
	ErrAccessDenied            = errors.New("black market access denied")
	ErrUnownedItem             = errors.New("item instance is not owned in inventory or depot")
	ErrNotSacrificeEligible    = errors.New("item is not eligible for rare point sacrifice")
	ErrInsufficientRarePoints  = errors.New("insufficient rare points for prize trade")
	ErrInsufficientURarePoints = errors.New("insufficient u-rare points for prize trade")
	ErrPrizeNotFound           = errors.New("prize item not found in trade catalog")
	ErrDepotFull               = errors.New("depot is full, cannot receive prize item")
	ErrDepotNotConfigured      = errors.New("depot repository is not configured")
)

var DefaultTalkDialogues = []string{
	"よく来たな…。ここは闇市場だ…",
	"表の世界では手に入れられない物を取引している…",
	"物の取引は金では買えないもの…。つまり、魂…ゴホッゴホッ…ではなく、レアアイテムだ…",
	"お前の魂…ではなく、お前が装備しているレアアイテムをささげろ…",
	"レアアイテムをささげることによって…お前のレアポイントが増える…",
	"レアポイントにより取引できるアイテムが違う…",
}

const InspectDialogue = "…お前の魂で取引したいのか？"

type CharacterPoints struct {
	CharacterID string `json:"character_id"`
	RarePoints  int    `json:"rare_points"`
	URarePoints int    `json:"u_rare_points"`
}

// Status describes black market location, NPC, accumulated points, and available trade prizes.
type Status struct {
	CharacterID  string  `json:"character_id"`
	LocationName string  `json:"location_name"`
	NPCName      string  `json:"npc_name"`
	RarePoints   int     `json:"rare_points"`
	URarePoints  int     `json:"u_rare_points"`
	Prizes       []Prize `json:"prizes"`
	UPrizes      []Prize `json:"u_prizes"`
}

// PointsStatus is an alias for Status for backward compatibility.
type PointsStatus = Status

type TalkResult struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Dialogue    string `json:"dialogue"`
}

type SacrificeResult struct {
	CharacterID       string `json:"character_id"`
	ItemInstanceID    string `json:"item_instance_id"`
	ItemDefinitionID  string `json:"item_definition_id"`
	ItemName          string `json:"item_name"`
	RarePointsGained  int    `json:"rare_points_gained"`
	URarePointsGained int    `json:"u_rare_points_gained"`
	TotalRarePoints   int    `json:"total_rare_points"`
	TotalURarePoints  int    `json:"total_u_rare_points"`
	Message           string `json:"message"`
}

type TradeResult struct {
	CharacterID      string `json:"character_id"`
	PrizeID          string `json:"prize_id"`
	ItemDefinitionID string `json:"item_definition_id"`
	ItemName         string `json:"item_name"`
	DepotInstanceID  string `json:"depot_instance_id,omitempty"`
	Cost             int    `json:"cost"`
	IsURare          bool   `json:"is_u_rare"`
	RemainingRare    int    `json:"remaining_rare"`
	RemainingURare   int    `json:"remaining_u_rare"`
	Message          string `json:"message"`
}
