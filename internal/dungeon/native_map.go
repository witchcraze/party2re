package dungeon

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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
	cellMap3Boss = 'b' // independent of B
	cellMist     = 'M' // map3 event_T: party confusion, then an encounter
	cellKey      = 'K' // map1: picked up once; never consumed by the door
	cellDoor     = 'D' // map1: opens with the key or an acting job 9
)

// symbolEvents share one run-wide state per symbol, like legacy flags: map1
// repeats K and 3, and map2 repeats 7 and keeps its flags across floors. Other
// events key by cell, since map0/map3 spell their independent repeats with
// distinct legacy symbols.
const symbolEvents = "KD23478"

func eventKey(cell byte, x, y int) string {
	if strings.IndexByte(symbolEvents, cell) >= 0 {
		return string(cell)
	}
	return fmt.Sprintf("%c:%d:%d", cell, x, y)
}

// jobLockpick (legacy job 9) opens a locked door for the acting character only.
const jobLockpick = "job-09"

// NativeMap0ID is the first native map (legacy map/0, six initial variants).
const NativeMap0ID = "native-map-0"

// nativeMap0 uses the layouts and round limit from map/0/1.cgi–6.cgi.
// Legacy I and 2 map to the native hidden passage H and treasure C.
func nativeMap0() Dungeon {
	monsters := []DungeonMonster{
		{ID: "nm0-rat", Name: "洞窟ネズミ", HP: 8, Attack: 7, Defense: 2, Agility: 8, ExpReward: 2, GoldReward: 3},
		{ID: "nm0-bat", Name: "大コウモリ", HP: 10, Attack: 8, Defense: 3, Agility: 12, ExpReward: 3, GoldReward: 4},
		{ID: "nm0-ooze", Name: "緑の粘体", HP: 14, Attack: 9, Defense: 5, Agility: 5, ExpReward: 4, GoldReward: 5},
	}
	boss := &DungeonMonster{ID: "nm0-boss", Name: "ビックスライム", HP: 280, Attack: 32, Defense: 12, Agility: 35, ExpReward: 55, GoldReward: 25}
	grids := [][]string{
		{"0000C", "11B11", "00000", "H1110", "00S00"},
		{"0000C", "11B11", "00000", "0111H", "00S00"},
		{"0H00C", "01B11", "0H000", "01110", "00S00"},
		{"0100C", "01B11", "0H000", "01110", "00S00"},
		{"C0000", "11B11", "00000", "11110", "00S00"},
		{"C0000", "11B11", "00000", "01111", "00S00"},
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
func (s *Service) moveNative(ctx context.Context, exp *ActiveExpedition, char *corecharacter.Character, d Dungeon, floor Floor, x, y int) (ExpeditionStepResult, error) {
	// Only an encounter without living members reaches here; the step drops it.
	clearing := exp.Encounter != nil
	exp.Encounter = nil
	cell := floor.Grid[y][x]
	key := eventKey(cell, x, y)
	consumed := slices.Contains(exp.ConsumedEvents, key)
	step := StepParams{
		ExpectedExpeditionID: exp.ID,
		ExpectedRevision:     &exp.Revision,
		Combat:               clearing,
		NewX:                 x,
		NewY:                 y,
		RoundDelta:           1,
		Now:                  time.Now().UTC(),
	}
	switch {
	case cell == cellPassage:
		// event_0: rand(2) <= 1 spawns.
		if 2*s.rng.Float64() <= 1 {
			if pool := s.encounterPool(floor); len(pool.Monsters) > 0 {
				return s.resolveMonsterCombat(ctx, exp, char, pool.monster(s.rng), EventBattle, x, y, "", "")
			}
		}
	case cell == cellMist:
		// Repeats on every entry; confusion reaches only the ensuing battle.
		return s.resolveMonsterCombat(ctx, exp, char, s.encounterPool(floor).monster(s.rng), EventBattle, x, y, "", string(corebattle.StatusConfusion))
	case (cell == cellBoss || cell == cellMap3Boss) && !consumed:
		if pool := s.encounterPool(floor); pool.Boss != nil {
			return s.resolveMonsterCombat(ctx, exp, char, *pool.Boss, EventBoss, x, y, key, "")
		}
	case floor.Bosses[cell].ID != "" && !consumed:
		return s.resolveMonsterCombat(ctx, exp, char, floor.Bosses[cell], EventBoss, x, y, key, "")
	case cell == cellKey && !consumed:
		step.EventKey = key
	case cell == cellDoor && !consumed:
		if char.JobID == jobLockpick || slices.Contains(exp.ConsumedEvents, string(cellKey)) {
			step.EventKey = key
		} else {
			// Locked: the round is spent and the party returns to its only approach.
			step.NewX, step.NewY = exp.PosX, exp.PosY
		}
	case (cell == cellTreasure || strings.IndexByte("234", cell) >= 0) && !consumed:
		step.EventKey = key
		step.Encounter = treasureEncounter(exp, char.JobID, s.rng, x, y)
	default:
		if to, ok := floor.Warps[cell]; ok {
			step.NewX, step.NewY = to[0], to[1]
		}
		if exit, ok := floor.Exits[cell]; ok {
			// The destination is drawn first, then each member's damage (map/2 event_T).
			step.MapVariant = exit.Variants[s.rng.Intn(len(exit.Variants))]
			step.RoundLimit = d.limitOf(d.Variants[step.MapVariant-1])
			if exit.Trap > 0 {
				step.MemberHPDeltas = trapDamage(exp, exit.Trap, s.rng)
				// The run-level HP mirrors the leader, as combat keeps it.
				step.HPDelta = step.MemberHPDeltas[char.ID]
				if len(exp.Members) == 0 {
					step.HPDelta = -trapRoll(exit.Trap, s.rng)
				}
			}
		}
	}
	out, err := s.activeStore.Step(ctx, char.ID, step)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if out.Status == StatusWipedOut {
		res, err := s.handleWipeout(ctx, &out.Expedition, char)
		res.MemberDamages = negate(step.MemberHPDeltas)
		return res, err
	}
	res := ExpeditionStepResult{Expedition: out.Expedition, EventType: EventMove, MemberDamages: negate(step.MemberHPDeltas)}
	if out.Expedition.Encounter != nil {
		res.EventType = EventTreasure
	}
	return res, nil
}

// trapRoll is _trap_d's per-member damage: int(base × (0.9 + rand(0.3))).
func trapRoll(base int, rng RandomSource) int {
	return int(float64(base) * (0.9 + 0.3*rng.Float64()))
}

// trapDamage hits every roster member; fallen members stay at 0.
func trapDamage(exp *ActiveExpedition, base int, rng RandomSource) map[string]int {
	if len(exp.Members) == 0 {
		return nil
	}
	deltas := make(map[string]int, len(exp.Members))
	for _, m := range exp.Members {
		deltas[m.CharacterID] = -trapRoll(base, rng)
	}
	return deltas
}

func negate(deltas map[string]int) map[string]int {
	if len(deltas) == 0 {
		return nil
	}
	out := make(map[string]int, len(deltas))
	for id, v := range deltas {
		out[id] = -v
	}
	return out
}

// encounterPool draws the content pool for one event; a floor with no or one
// pool draws nothing.
func (s *Service) encounterPool(f Floor) EncounterPool {
	switch len(f.Pools) {
	case 0:
		return EncounterPool{Monsters: f.Monsters, Boss: f.Boss}
	case 1:
		return f.Pools[0]
	}
	return f.Pools[s.rng.Intn(len(f.Pools))]
}

// monster draws one enemy through the weighted index list (add_monster).
func (p EncounterPool) monster(rng RandomSource) DungeonMonster {
	if len(p.Appears) > 0 {
		return p.Monsters[p.Appears[rng.Intn(len(p.Appears))]]
	}
	return p.Monsters[rng.Intn(len(p.Monsters))]
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
