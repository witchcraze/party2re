package adventure

import (
	"fmt"
	"math/rand"
)

// Legacy item IDs with special adventure interactions.
const (
	//lint:ignore unused legacy item ID for treasure room bonus calculation
	ItemLuckyPendant = "item-191" // ラッキーペンダント (1/3 chance +1 treasure box)
	//lint:ignore unused legacy item ID for master key interaction
	ItemMasterKey = "item-215" // マスターキー (allows opening multiple treasure boxes)
)

var defaultBoxNames = []string{
	"普通の宝箱",
	"大きい宝箱",
	"小さい宝箱",
	"黒い宝箱",
	"青い宝箱",
	"古い宝箱",
	"丸い宝箱",
}

// TreasureCalculationInput specifies the party state when arriving at Floor 11 (Treasure Room).
type TreasureCalculationInput struct {
	AliveMembers      int
	HasMerchant       bool
	HasTreasureHunter bool
	HasLuckyPendant   bool
	Rng               func(n int) int
}

// CalculateTreasureCount calculates the number of treasure boxes spawned on Floor 11
// in 1:1 parity with legacy vs_monster.cgi ($boss_round+1) and _npc_action.cgi (add_treasure).
func CalculateTreasureCount(input TreasureCalculationInput) int {
	if input.AliveMembers <= 0 {
		return 0
	}
	rng := input.Rng
	if rng == nil {
		rng = rand.Intn
	}

	// Base count: 1 box per alive party member
	count := input.AliveMembers

	// Merchant (job 7): +1 box
	if input.HasMerchant {
		count++
	}

	// Treasure Hunter (job 78): +1 to +2 boxes (1 + rand(2))
	if input.HasTreasureHunter {
		count += 1 + rng(2)
	}

	// Lucky Pendant (item-191): 1/3 chance of +1 box (rand(3) < 1)
	if input.HasLuckyPendant && rng(3) == 0 {
		count++
	}

	if count < 0 {
		count = 0
	}
	return count
}

// TreasureBox represents an individual treasure chest discovered on Floor 11.
type TreasureBox struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OpenedBy    string `json:"opened_by,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	ItemName    string `json:"item_name,omitempty"`
	DeliveredTo string `json:"delivered_to,omitempty"` // "inventory" or "depot"
}

// GenerateTreasureBoxes creates the list of treasure chests for Floor 11.
func GenerateTreasureBoxes(count int, dropPool []string, rng func(n int) int) []TreasureBox {
	if count <= 0 {
		return nil
	}
	if rng == nil {
		rng = rand.Intn
	}
	if len(dropPool) == 0 {
		dropPool = []string{"item-001"}
	}

	boxes := make([]TreasureBox, count)
	for i := 0; i < count; i++ {
		baseName := defaultBoxNames[rng(len(defaultBoxNames))]
		suffix := rune('A' + (i % 26))
		if i >= 26 {
			suffix = rune('a' + ((i - 26) % 26))
		}
		itemDefID := dropPool[rng(len(dropPool))]

		boxes[i] = TreasureBox{
			ID:     fmt.Sprintf("box-%d", i+1),
			Name:   fmt.Sprintf("%s%c", baseName, suffix),
			ItemID: itemDefID,
		}
	}
	return boxes
}
