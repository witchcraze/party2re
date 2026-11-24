package runhost

import (
	"fmt"
	"slices"
	"time"
)

// OrdinaryBoxNames are add_treasure's chest names (_npc_action.cgi:508–516).
var OrdinaryBoxNames = []string{"普通の宝箱", "大きい宝箱", "小さい宝箱", "黒い宝箱", "青い宝箱", "古い宝箱", "丸い宝箱"}

// IsTreasureBlessing reports Chapel Blessing 4 ("宝箱がほしい") as the chapel stores it.
func IsTreasureBlessing(blessing string) bool { return blessing == "DROP" }

// TreasurePools are a stage's @treasures weapon, armor and tool tables.
type TreasurePools struct {
	Weapons []string
	Armors  []string
	Items   []string
}

// TreasureCount is add_treasure's chest count (_npc_action.cgi:499–505,519):
// a zero room count falls back to $#partys (total−1), the treasure blessing
// adds one on a 1/5 roll, each fallen member removes one, and the loop
// 0..count is inclusive. total and fallen count @partys, fallen included.
func TreasureCount(count, total, fallen int, blessed bool, rng func(int) int) int {
	if count == 0 {
		count = total - 1
	}
	if blessed && rng(5) == 0 {
		count++
	}
	return max(count-fallen+1, 0)
}

// GenerateTreasureBoxes is add_treasure's chest draw: each chest first adds
// the weekday orb to the tool pool (so orbs accumulate), then picks weapon,
// armor or tool as 1/4, 1/4, 1/2, falling back to tools when the chosen pool
// is empty. An empty pool entry ("") yields an empty chest.
func GenerateTreasureBoxes(count int, pools TreasurePools, now time.Time, rng func(int) int) []TreasureBox {
	if count <= 0 {
		return nil
	}
	items := slices.Clone(pools.Items)
	boxes := make([]TreasureBox, count)
	for i := range count {
		name := BoxName(OrdinaryBoxNames[rng(len(OrdinaryBoxNames))], i)
		items = append(items, dayOfWeekOrb(now, rng))
		pool := items
		switch v := rng(4) + 1; {
		case v == 1 && len(pools.Weapons) > 0:
			pool = pools.Weapons
		case v == 2 && len(pools.Armors) > 0:
			pool = pools.Armors
		}
		boxes[i] = TreasureBox{ID: fmt.Sprintf("box-%d", i+1), Name: name, ItemID: pool[rng(len(pool))]}
	}
	return boxes
}

// dayOfWeekOrb is the weekday orb (_npc_action.cgi:541–543) in JST server
// time: Sunday draws among item-060..065, Monday..Saturday are item-060..065.
func dayOfWeekOrb(t time.Time, rng func(int) int) string {
	wday := t.In(time.FixedZone("JST", 9*60*60)).Weekday()
	if wday == time.Sunday {
		return fmt.Sprintf("item-%03d", 60+rng(6))
	}
	return fmt.Sprintf("item-%03d", 60+int(wday)-1)
}

// BoxName suffixes the i-th chest so equal names stay distinct.
func BoxName(base string, i int) string {
	suffix := rune('A' + (i % 26))
	if i >= 26 {
		suffix = rune('a' + ((i - 26) % 26))
	}
	return fmt.Sprintf("%s%c", base, suffix)
}
