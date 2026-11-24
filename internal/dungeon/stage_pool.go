package dungeon

import (
	"fmt"
	"slices"
	"sync"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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
// its bosses are the first roster.
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
		var actor corebattle.LifetimeActor
		if m.Combat != nil {
			actor = m.Combat.Clone()
		}
		actor.MP, actor.CMP = m.MP, m.CMP
		return DungeonMonster{ID: m.ID, Name: m.Name, HP: m.HP, Attack: m.Attack, Defense: m.Defense, Agility: m.Agility, ExpReward: m.ExperienceReward, GoldReward: m.GoldReward, combat: enemyCombat(actor)}
	}
	pool := EncounterPool{Appears: slices.Clone(st.Appears)}
	for _, id := range st.MonsterIDs {
		pool.Monsters = append(pool.Monsters, resolve(id))
	}
	for _, id := range st.BossRosters[0] {
		pool.Bosses = append(pool.Bosses, resolve(id))
	}
	return pool
}

// stagePools loads stages from..to, one pool per stage, as int(rand(k) + from) draws them.
func stagePools(from, to int) []EncounterPool {
	pools := make([]EncounterPool, 0, to-from+1)
	for n := from; n <= to; n++ {
		pools = append(pools, stagePool(n))
	}
	return pools
}
