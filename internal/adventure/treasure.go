package adventure

import (
	"fmt"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/core/random"
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
	StageID             string
	AliveMembers        int
	HasMerchant         bool
	HasGambler          bool
	HasTreasureHunter   bool
	HasLuckyPendant     bool
	HasTreasureBlessing bool
	Rng                 func(n int) int
}

func isThreeTimesTreasureStage(stageID string) bool {
	id := strings.ToLower(strings.TrimSpace(stageID))
	return id == "stage-17" || id == "17" ||
		id == "stage-20" || id == "20" ||
		id == "stage-21" || id == "21"
}

// CalculateTreasureCount calculates the number of treasure boxes spawned on Floor 11
// in 1:1 parity with legacy vs_monster.cgi ($boss_round+1) and _npc_action.cgi (add_treasure).
func CalculateTreasureCount(input TreasureCalculationInput) int {
	if input.AliveMembers <= 0 {
		return 0
	}
	rng := input.Rng
	if rng == nil {
		rng = random.Intn
	}

	// Base count: 1 box per alive party member
	// Legacy vs_monster.cgi:92: Stages 17, 20, 21 grant 3x treasure multiplier
	count := input.AliveMembers
	if isThreeTimesTreasureStage(input.StageID) {
		count = input.AliveMembers * 3
	}
	baseCount := count

	// Merchant (job 7): +1 box
	if input.HasMerchant {
		count++
	}

	// Gambler (job 81): + rand(3) - rand(2)
	// (party2/lib/vs_monster.cgi:94-97)
	// If Lucky Pendant (item-191) is held and count drops below baseCount, restored to baseCount.
	if input.HasGambler {
		count += rng(3) - rng(2)
		if input.HasLuckyPendant && count < baseCount {
			count = baseCount
		}
	}

	// Treasure Hunter (job 78): +1 to +2 boxes (1 + rand(2))
	if input.HasTreasureHunter {
		count += 1 + rng(2)
	}

	// Lucky Pendant (item-191): 1/3 chance of +1 box (rand(3) < 1)
	if input.HasLuckyPendant && rng(3) == 0 {
		count++
	}

	// Chapel Blessing 4 ("宝箱がほしい"): 20% chance of +1 box (party2/lib/_npc_action.cgi:501, rand(5) < 1)
	if input.HasTreasureBlessing && rng(5) == 0 {
		count++
	}

	if count < 0 {
		count = 0
	}
	return count
}

// StageTreasurePools defines candidate item pools across equipment and consumables for Floor 11.
type StageTreasurePools struct {
	Weapons []string
	Armors  []string
	Items   []string
}

// GetDayOfWeekOrb returns the canonical weekday orb item ID matching legacy Party2 (_npc_action.cgi:541-543).
// Sunday (0): random among item-060..item-065
// Monday (1) .. Saturday (6): item-060 .. item-065
func GetDayOfWeekOrb(t time.Time, rng func(n int) int) string {
	if rng == nil {
		rng = random.Intn
	}
	wday := t.Weekday()
	if wday == time.Sunday {
		return fmt.Sprintf("item-%03d", 60+rng(6))
	}
	return fmt.Sprintf("item-%03d", 60+int(wday)-1)
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

// GenerateTreasureBoxes creates the list of treasure chests for Floor 11 (legacy convenience wrapper).
func GenerateTreasureBoxes(count int, dropPool []string, rng func(n int) int) []TreasureBox {
	return GenerateTreasureBoxesWithPools(count, StageTreasurePools{Items: dropPool}, time.Now().UTC(), rng)
}

// GenerateTreasureBoxesWithPools creates treasure chests using authentic legacy drop distribution (_npc_action.cgi:540-550):
// - Appends current day-of-week orb to items candidate pool
// - 25% weapon, 25% armor, 50% tool/item (v = int(rand(4)) + 1; v = 3 if v > 3)
func GenerateTreasureBoxesWithPools(count int, pools StageTreasurePools, now time.Time, rng func(n int) int) []TreasureBox {
	if count <= 0 {
		return nil
	}
	if rng == nil {
		rng = random.Intn
	}

	items := make([]string, len(pools.Items), len(pools.Items)+1)
	copy(items, pools.Items)
	orb := GetDayOfWeekOrb(now, rng)
	items = append(items, orb)

	weapons := pools.Weapons
	armors := pools.Armors

	boxes := make([]TreasureBox, count)
	for i := 0; i < count; i++ {
		baseName := defaultBoxNames[rng(len(defaultBoxNames))]
		suffix := rune('A' + (i % 26))
		if i >= 26 {
			suffix = rune('a' + ((i - 26) % 26))
		}

		// Roll category: 1=weapon (25%), 2=armor (25%), 3 or 4=item (50%)
		v := rng(4) + 1
		if v > 3 {
			v = 3
		}

		var itemDefID string
		switch v {
		case 1:
			if len(weapons) > 0 {
				itemDefID = weapons[rng(len(weapons))]
			} else if len(items) > 0 {
				itemDefID = items[rng(len(items))]
			}
		case 2:
			if len(armors) > 0 {
				itemDefID = armors[rng(len(armors))]
			} else if len(items) > 0 {
				itemDefID = items[rng(len(items))]
			}
		default: // 3
			if len(items) > 0 {
				itemDefID = items[rng(len(items))]
			} else if len(weapons) > 0 {
				itemDefID = weapons[rng(len(weapons))]
			} else if len(armors) > 0 {
				itemDefID = armors[rng(len(armors))]
			}
		}

		if itemDefID == "" {
			itemDefID = "item-001"
		}

		boxes[i] = TreasureBox{
			ID:     fmt.Sprintf("box-%d", i+1),
			Name:   fmt.Sprintf("%s%c", baseName, suffix),
			ItemID: itemDefID,
		}
	}
	return boxes
}
