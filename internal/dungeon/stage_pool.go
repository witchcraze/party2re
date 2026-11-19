package dungeon

import (
	"fmt"
	"slices"
	"sync"

	"github.com/witchcraze/party2re/internal/stagedata"
)

var stageCatalog = sync.OnceValues(func() (map[string]stagedata.Stage, map[string]stagedata.Monster) {
	stages, err := stagedata.Stages()
	if err != nil {
		panic(err)
	}
	list, err := stagedata.Monsters()
	if err != nil {
		panic(err)
	}
	monsters := make(map[string]stagedata.Monster, len(list))
	for _, m := range list {
		monsters[m.ID] = m
	}
	return stages, monsters
})

// stagePool is legacy stage n (party2/stage/<n>.cgi) from the shared catalog;
// its boss is the strongest of the first roster.
func stagePool(n int) EncounterPool {
	stages, monsters := stageCatalog()
	st, ok := stages[fmt.Sprintf("stage-%02d", n)]
	if !ok || len(st.BossRosters) == 0 || len(st.BossRosters[0]) == 0 {
		panic(fmt.Sprintf("dungeon: stage %d missing from stagedata", n))
	}
	resolve := func(id string) DungeonMonster {
		m, ok := monsters[id]
		if !ok {
			panic("dungeon: unknown stage monster " + id)
		}
		return DungeonMonster{ID: m.ID, Name: m.Name, HP: m.HP, Attack: m.Attack, Defense: m.Defense, Agility: m.Agility, ExpReward: m.ExperienceReward, GoldReward: m.GoldReward}
	}
	pool := EncounterPool{Appears: slices.Clone(st.Appears)}
	for _, id := range st.MonsterIDs {
		pool.Monsters = append(pool.Monsters, resolve(id))
	}
	roster := make([]DungeonMonster, len(st.BossRosters[0]))
	for i, id := range st.BossRosters[0] {
		roster[i] = resolve(id)
	}
	boss := strongest(roster)
	pool.Boss = &boss
	return pool
}

// strongest is the roster member with the most HP, the first on a tie.
// shortcut: one enemy stands in for a group until #1954 spawns whole groups.
func strongest(group []DungeonMonster) DungeonMonster {
	best := group[0]
	for _, m := range group[1:] {
		if m.HP > best.HP {
			best = m
		}
	}
	return best
}

// stagePools loads stages from..to, one pool per stage, as int(rand(k) + from) draws them.
func stagePools(from, to int) []EncounterPool {
	pools := make([]EncounterPool, 0, to-from+1)
	for n := from; n <= to; n++ {
		pools = append(pools, stagePool(n))
	}
	return pools
}
