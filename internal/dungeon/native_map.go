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

// NativeMap0ID is the first native map (legacy map/0, six initial variants).
const NativeMap0ID = "native-map-0"

// nativeMap0 uses the layouts and round limit from map/0/1.cgi–6.cgi.
// Legacy I and 2 are written H and C.
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
	variants := variantFloors(Floor{FloorNumber: 1, Monsters: monsters, Boss: boss,
		events: map[byte]cellEvent{'H': {kind: eventHidden}, 'C': {kind: eventTreasure}}}, grids)
	return Dungeon{
		ID:          NativeMap0ID,
		Name:        "地下迷路",
		Description: "小さな地下迷路。隠し通路と宝箱、奥に潜む主が待つ。",
		RoundLimit:  20,
		Variants:    variants,
	}
}

// gridFloor completes tmpl with one transcribed grid, deriving its size and
// start; floors without S keep the arriving position. A ragged grid is a
// transcription error and fails catalog construction.
func gridFloor(tmpl Floor, rows []string) Floor {
	if len(rows) == 0 {
		panic("dungeon: empty grid")
	}
	tmpl.Grid, tmpl.Width, tmpl.Height = rows, len(rows[0]), len(rows)
	for y, row := range rows {
		if len(row) != tmpl.Width {
			panic(fmt.Sprintf("dungeon: ragged grid row %d %q", y, row))
		}
		if x := strings.IndexByte(row, cellStart); x >= 0 {
			tmpl.StartX, tmpl.StartY = x, y
		}
	}
	return tmpl
}

// variantFloors builds one initial variant per grid from a shared template.
func variantFloors(tmpl Floor, grids [][]string) []Floor {
	floors := make([]Floor, len(grids))
	for i, rows := range grids {
		floors[i] = gridFloor(tmpl, rows)
	}
	return floors
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
// char is the leader owning the run; actor is the mover the event acts through.
func (s *Service) moveNative(ctx context.Context, exp *ActiveExpedition, char, actor *corecharacter.Character, d Dungeon, floor Floor, x, y int) (ExpeditionStepResult, error) {
	// Only an encounter without living members reaches here; the step drops it.
	clearing := exp.Encounter != nil
	exp.Encounter = nil
	cell := floor.Grid[y][x]
	ev := floor.event(cell)
	key := ev.stateKey(cell, x, y)
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
	recovery := false
	switch ev.kind {
	case eventSpawn:
		if 2*s.rng.Float64() <= 1 {
			if pool := s.encounterPool(floor); len(pool.Monsters) > 0 {
				return s.resolveMonsterCombat(ctx, exp, char, pool.monster(s.rng), EventBattle, x, y, "", "")
			}
		}
	case eventMist:
		// The status reaches only the ensuing battle; the next step resets it.
		return s.resolveMonsterCombat(ctx, exp, char, s.encounterPool(floor).monster(s.rng), EventBattle, x, y, "", ev.status)
	case eventBoss:
		if consumed {
			break
		}
		if boss, ok := floor.Bosses[cell]; ok {
			return s.resolveMonsterCombat(ctx, exp, char, boss, EventBoss, x, y, key, "")
		}
		if pool := s.encounterPool(floor); pool.Boss != nil {
			return s.resolveMonsterCombat(ctx, exp, char, *pool.Boss, EventBoss, x, y, key, "")
		}
	case eventKey, eventMessage:
		if !consumed {
			step.EventKey = key
		}
	case eventDoor:
		if consumed {
			break
		}
		if slices.Contains(ev.jobs, actor.JobID) || slices.Contains(exp.ConsumedEvents, string(ev.key)) {
			step.EventKey = key
		} else {
			// Locked: the round is spent and the party returns to its only approach.
			step.NewX, step.NewY = exp.PosX, exp.PosY
		}
	case eventTreasure:
		if !consumed {
			step.EventKey = key
			step.Encounter = treasureEncounter(exp, actor.JobID, s.rng, x, y)
		}
	case eventAttrition:
		if loss := attritionLoss(exp, actor); loss > 0 {
			step.MemberHPDeltas = map[string]int{actor.ID: -loss}
			if actor.ID == char.ID {
				step.HPDelta = -loss // the run-level HP mirrors the leader
			}
		}
	case eventRecovery:
		if consumed {
			break
		}
		step.EventKey = key
		hp, mp, err := recoverOne(exp, actor, s.rng)
		if err != nil {
			return ExpeditionStepResult{}, err
		}
		step.MemberHPDeltas, step.MemberMPDeltas = hp, mp
		step.HPDelta = hp[char.ID]
		recovery = true
	case eventTrap:
		partyTrap(&step, exp, char.ID, float64(ev.base), s.rng)
	case eventChoice:
		if consumed {
			break
		}
		o := ev.outcomes[0]
		if ev.odds > 0 && float64(ev.odds)*s.rng.Float64() >= 1 {
			o = ev.outcomes[1]
		}
		base := float64(o.base)
		if o.random {
			base *= s.rng.Float64()
		}
		step.EventKey = key
		switch o.kind {
		case outcomeMonster:
			m := o.monster
			if m == nil {
				pm := s.encounterPool(floor).monster(s.rng)
				m = &pm
			}
			return s.resolveMonsterCombat(ctx, exp, char, *m, EventBattle, x, y, key, "")
		case outcomeBoss:
			m := o.monster
			if m == nil {
				m = s.encounterPool(floor).Boss
			}
			return s.resolveMonsterCombat(ctx, exp, char, *m, EventBoss, x, y, key, "")
		case outcomeTreasure:
			step.Encounter = treasureEncounter(exp, actor.JobID, s.rng, x, y)
		case outcomeTrap:
			partyTrap(&step, exp, char.ID, base, s.rng)
		case outcomeHeal:
			hp, err := partyHeal(exp, actor, base, s.rng)
			if err != nil {
				return ExpeditionStepResult{}, err
			}
			step.MemberHPDeltas, step.HPDelta = hp, hp[char.ID]
			recovery = true
		case outcomeGold:
			// shortcut: the wallet change lands with #2417; map6 stays unlisted until then.
		}
	case eventRemedy:
		if consumed || actor.JobID != jobRemedy {
			break
		}
		step.EventKey = key
		step.MemberHPDeltas, step.MemberMPDeltas = fullRestore(exp)
		step.HPDelta = step.MemberHPDeltas[char.ID]
		recovery = true
	default:
		if to, ok := floor.Warps[cell]; ok {
			step.NewX, step.NewY = to[0], to[1]
		}
		if exit, ok := floor.Exits[cell]; ok {
			// The destination is drawn first, then each member's damage (map/2 event_T).
			step.MapVariant = exit.Variants[s.rng.Intn(len(exit.Variants))]
			step.RoundLimit = d.limitOf(d.Variants[step.MapVariant-1])
			if exit.Trap > 0 {
				partyTrap(&step, exp, char.ID, float64(exit.Trap), s.rng)
			}
		}
	}
	out, err := s.activeStore.Step(ctx, char.ID, step)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	res := ExpeditionStepResult{Expedition: out.Expedition, EventType: EventMove}
	if !recovery {
		res.MemberDamages = negate(step.MemberHPDeltas)
	}
	if out.Expedition.Encounter != nil {
		res.EventType = EventTreasure
	}
	return res, nil
}

// trapRoll is _trap_d's per-member damage: int(base × (0.9 + rand(0.3))).
// The base may be fractional (map6 rand(200)).
func trapRoll(base float64, rng RandomSource) int {
	return int(base * (0.9 + 0.3*rng.Float64()))
}

// partyTrap hits every roster member; fallen members stay at 0. The run-level
// HP mirrors the leader, as combat keeps it.
func partyTrap(step *StepParams, exp *ActiveExpedition, leaderID string, base float64, rng RandomSource) {
	if len(exp.Members) == 0 {
		step.HPDelta = -trapRoll(base, rng)
		return
	}
	step.MemberHPDeltas = make(map[string]int, len(exp.Members))
	for _, m := range exp.Members {
		step.MemberHPDeltas[m.CharacterID] = -trapRoll(base, rng)
	}
	step.HPDelta = step.MemberHPDeltas[leaderID]
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
// member, plus 1–2 when the mover is job 78. Contents and inspection
// belong to #1762; the chests only block movement until opened.
func treasureEncounter(exp *ActiveExpedition, actorJob string, rng RandomSource, x, y int) *ActiveEncounter {
	count := 0
	for _, m := range exp.Members {
		if m.CurrentHP > 0 {
			count++
		}
	}
	if len(exp.Members) == 0 && exp.CurrentHP > 0 {
		count = 1
	}
	if j := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(actorJob)), "job-"); j == "78" {
		count += 1 + rng.Intn(2)
	}
	if count <= 0 {
		return nil
	}
	return &ActiveEncounter{EventType: EventTreasure, X: x, Y: y, Chests: make([]TreasureChest, count)}
}
