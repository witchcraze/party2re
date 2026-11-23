package adventure

import (
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// SummonPool preserves the current (including seasonal) stage table order.
// The normal-floor filter is inappropriate here: the caller supplies separate
// source appearance positions, including repeats, to RosterContext. A dynamic
// stage15 table starts from NormalRun.EncounterPool and rebuilds from the
// summoner on the first request-scoped stage reload.
func (s Stage) SummonPool(payloads map[string]corebattle.LifetimeActor) ([]corebattle.LifetimeActor, error) {
	ids := s.MonsterIDs
	if s.Ordinary != nil {
		ids = s.Ordinary.MonsterIDs
	}
	if len(ids) == 0 {
		return nil, corebattle.ErrUnsupportedEffect
	}
	pool := make([]corebattle.LifetimeActor, 0, len(ids))
	for _, id := range ids {
		a, ok := payloads[id]
		if !ok {
			return nil, corebattle.ErrUnsupportedEffect
		}
		if a.ID != id {
			return nil, ErrInvalidMonster
		}
		pool = append(pool, a)
	}
	return pool, nil
}

// stage15GeneratedMonsters builds the ten stage15 generated entries from the
// current summoner, consuming the appearance and job draws per entry.
func stage15GeneratedMonsters(summoner corebattle.LifetimeActor, rng random.Generator) []Monster {
	pool := make([]Monster, 0, 10)
	for i := range 10 {
		// The source's appearance draw precedes its job draw, even with native art.
		rng.Float64()
		job := int(rng.Float64() * 88)
		jobID := fmt.Sprintf("job-%02d", job)
		if job == 0 {
			jobID = "starter"
		}
		jobs := corebattle.BattleJobs{JobID: jobID, SP: summoner.Jobs.SP, OldJobID: "job-36", OldSP: 999}
		pool = append(pool, Monster{
			ID:                fmt.Sprintf("generated-%d", i),
			EncounterIdentity: generatedEncounterIdentity(jobID),
			Name:              fmt.Sprintf("Generated combatant %d", i+1),
			HP:                summoner.MaxHP,
			MP:                summoner.MaxMP,
			Attack:            summoner.BaseStats.Attack,
			Defense:           summoner.BaseStats.Defense,
			Agility:           summoner.BaseStats.Agility,
			ExperienceReward:  summoner.ExperienceReward,
			GoldReward:        summoner.GoldReward,
			Combat:            &corebattle.LifetimeActor{Jobs: jobs, OriginalJobs: jobs, NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:stage/15.cgi:47"}},
		})
	}
	return pool
}

func stage15SummonPool(summoner corebattle.LifetimeActor, rng random.Generator) []corebattle.LifetimeActor {
	monsters := stage15GeneratedMonsters(summoner, rng)
	pool := make([]corebattle.LifetimeActor, 0, len(monsters))
	for _, m := range monsters {
		pool = append(pool, m.NPCPayload())
	}
	return pool
}
