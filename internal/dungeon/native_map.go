package dungeon

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Native map cells. Appearance, passability and event dispatch are separate:
// a hidden passage looks like a wall but is traversable.
const (
	cellPassage  = '0'
	cellWall     = '1'
	cellStart    = 'S'
	cellHidden   = 'H'
	cellTreasure = 'C'
	cellBoss     = 'B'
)

// NativeMap0ID is the first native map (legacy map/0, six initial variants).
const NativeMap0ID = "native-map-0"

// nativeMap0 holds independently authored 5×5 layouts with the legacy map0
// event semantics and round limit 20 (map/0/1.cgi–6.cgi). Variants 5 and 6
// have no hidden passage, like their legacy counterparts.
func nativeMap0() Dungeon {
	monsters := []DungeonMonster{
		{ID: "nm0-rat", Name: "洞窟ネズミ", HP: 8, Attack: 7, Defense: 2, Agility: 8, ExpReward: 2, GoldReward: 3},
		{ID: "nm0-bat", Name: "大コウモリ", HP: 10, Attack: 8, Defense: 3, Agility: 12, ExpReward: 3, GoldReward: 4},
		{ID: "nm0-ooze", Name: "緑の粘体", HP: 14, Attack: 9, Defense: 5, Agility: 5, ExpReward: 4, GoldReward: 5},
	}
	boss := &DungeonMonster{ID: "nm0-boss", Name: "洞窟の大粘体", HP: 280, Attack: 32, Defense: 12, Agility: 35, ExpReward: 55, GoldReward: 25}
	grids := [][]string{
		{"S0001", "110H1", "0B000", "01110", "00C10"},
		{"1000S", "1H110", "0000B", "01011", "C0000"},
		{"C0001", "01110", "0H0B0", "01110", "S0000"},
		{"0S010", "01010", "0H0B0", "11010", "C0000"},
		{"00B0C", "01111", "00000", "11110", "S0000"},
		{"S0000", "11110", "C0B00", "01111", "00000"},
	}
	variants := make([]Floor, len(grids))
	for i, grid := range grids {
		f := Floor{FloorNumber: 1, Width: 5, Height: 5, Grid: grid, Monsters: monsters, Boss: boss}
		for y, row := range grid {
			if x := strings.IndexByte(row, cellStart); x >= 0 {
				f.StartX, f.StartY = x, y
			}
		}
		variants[i] = f
	}
	return Dungeon{
		ID:          NativeMap0ID,
		Name:        "地下迷路",
		Description: "小さな地下迷路。隠し通路と宝箱、奥に潜む主が待つ。",
		RoundLimit:  20,
		Variants:    variants,
	}
}

var errInvalidFloor = errors.New("invalid floor index")

// activeFloor resolves the layout saved on a run.
func (s *Service) activeFloor(exp *ActiveExpedition) (Dungeon, Floor, error) {
	d, ok := s.dungeonMap[exp.DungeonID]
	if !ok {
		return Dungeon{}, Floor{}, ErrDungeonNotFound
	}
	idx := exp.MapVariant - 1
	if idx < 0 || idx >= len(d.Variants) {
		return d, Floor{}, errInvalidFloor
	}
	return d, d.Variants[idx], nil
}

// moveNative applies one admitted native step: the round advances and the
// destination's map event is dispatched (legacy vs_dungeon.cgi _susumu).
func (s *Service) moveNative(ctx context.Context, exp *ActiveExpedition, char *corecharacter.Character, floor Floor, x, y int) (ExpeditionStepResult, error) {
	// Only an encounter without living members reaches here; the step drops it.
	clearing := exp.Encounter != nil
	exp.Encounter = nil
	cell := floor.Grid[y][x]
	key := fmt.Sprintf("%c:%d:%d", cell, x, y)
	consumed := slices.Contains(exp.ConsumedEvents, key)
	step := StepParams{
		ExpectedExpeditionID: exp.ID,
		ExpectedRevision:     &exp.Revision,
		Combat:               clearing,
		NewFloor:             exp.CurrentFloor,
		NewX:                 x,
		NewY:                 y,
		TurnsDelta:           -1,
		Now:                  time.Now().UTC(),
	}
	switch {
	case cell == cellPassage:
		// event_0: rand(2) <= 1 spawns.
		if 2*s.rng.Float64() <= 1 && len(floor.Monsters) > 0 {
			return s.resolveMonsterCombat(ctx, exp, char, floor.Monsters[s.rng.Intn(len(floor.Monsters))], EventBattle, x, y, "")
		}
	case cell == cellBoss && !consumed && floor.Boss != nil:
		return s.resolveMonsterCombat(ctx, exp, char, *floor.Boss, EventBoss, x, y, key)
	case cell == cellTreasure && !consumed:
		step.EventKey = key
		step.Encounter = treasureEncounter(exp, char.JobID, s.rng, x, y)
	}
	out, err := s.activeStore.Step(ctx, char.ID, step)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	res := ExpeditionStepResult{Expedition: out.Expedition, EventType: EventMove}
	if out.Expedition.Encounter != nil {
		res.EventType = EventTreasure
	}
	return res, nil
}

// treasureEncounter creates unopened chests like add_treasure: one per living
// member, plus 1–2 when the acting leader is job 78. Contents and inspection
// belong to #1762; the chests only block movement until opened.
func treasureEncounter(exp *ActiveExpedition, leaderJob string, rng RandomSource, x, y int) *ActiveEncounter {
	count := 0
	for _, m := range exp.Members {
		if m.CurrentHP > 0 {
			count++
		}
	}
	if len(exp.Members) == 0 && exp.CurrentHP > 0 {
		count = 1
	}
	if j := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(leaderJob)), "job-"); j == "78" {
		count += 1 + rng.Intn(2)
	}
	if count <= 0 {
		return nil
	}
	return &ActiveEncounter{EventType: EventTreasure, X: x, Y: y, Chests: make([]TreasureChest, count)}
}
