package adventure

import (
	_ "embed"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// OrdinaryStage is the complete source-position table, separate from the batch
// catalog's historical last-monster boss convention. Persist it with the room.
type OrdinaryStage struct {
	BossRosters [][]string `json:"boss_rosters"`
	MonsterIDs  []string   `json:"monster_ids"`
	Appears     []int      `json:"appears"`
	PartyCopy   bool       `json:"party_copy,omitempty"`
	// EXRoster is the floor-12 table; summons keep using MonsterIDs/Appears.
	EXRoster []string `json:"ex_roster,omitempty"`
	EXField  string   `json:"ex_field,omitempty"`
}

// OrdinaryEncounter retains the complete unrolled summon table as well as the
// selected opponents. Summons use their own nonrandom scaling, not boss rolls.
type OrdinaryEncounter struct {
	Actors  []corebattle.LifetimeActor
	Pool    []corebattle.LifetimeActor
	Appears []int
}

//go:embed data/ordinary_monsters.json
var ordinaryMonsterData []byte

//go:embed data/ordinary_stages.json
var ordinaryStageData []byte

// OrdinarySeason follows room-marker priority. An unmarked room is spring;
// calendar time and the batch resolver's random season are not inputs.
func (s Stage) OrdinarySeason(markers ...string) Stage {
	for _, season := range []string{"spring", "summer", "autumn", "winter"} {
		for _, marker := range markers {
			if marker == season {
				return s.ForSeason(season)
			}
		}
	}
	if s.ID == "stage-26" {
		return s.ForSeason("spring")
	}
	return s
}

// BuildOrdinaryEncounter freezes branch selection and stat rolls once. party
// includes helpers and fallen members in semantic order; requester is distinct
// from the acting NPC and supplies stage15's generated monster baselines.
func (s Stage) BuildOrdinaryEncounter(catalog *MonsterCatalog, party []corebattle.LifetimeActor, requester corebattle.LifetimeActor, floor int, mode, encounterID string, rng random.Generator) (OrdinaryEncounter, error) {
	if rng == nil || len(party) == 0 || floor < 1 || floor > EXBossFloor || floor == TreasureRoomFloor || encounterID == "" {
		return OrdinaryEncounter{}, ErrInvalidMonster
	}
	if s.Ordinary == nil || floor == EXBossFloor && len(s.Ordinary.EXRoster) == 0 {
		return OrdinaryEncounter{}, corebattle.ErrUnsupportedEffect
	}
	table := s.Ordinary
	if floor == EXBossFloor {
		// add_ex_boss shares add_boss scaling and draws no branch.
		cp := *table
		cp.BossRosters, cp.PartyCopy = [][]string{table.EXRoster}, false
		table = &cp
	}
	var pool, bosses []Monster
	if table.PartyCopy {
		for i, a := range party {
			m := Monster{ID: fmt.Sprintf("copy-%d", i), EncounterIdentity: "party-echo:" + a.ID, Name: fmt.Sprintf("Party echo %d", i+1), HP: a.MaxHP, MP: a.MaxMP, Attack: a.Attack, Defense: a.Defense, Agility: a.Agility, ExperienceReward: a.ExperienceReward, GoldReward: a.GoldReward,
				Combat: &corebattle.LifetimeActor{Jobs: a.Jobs, OriginalJobs: a.Jobs, Temporary: corebattle.TemporaryReflectMagic, CrystalReward: 5, NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:stage/15.cgi:15"}}}
			bosses = append(bosses, m)
		}
		for i := 0; i < 10; i++ {
			// The source's appearance draw precedes its job draw, even with native art.
			rng.Float64()
			job := int(rng.Float64() * 88)
			jobID := fmt.Sprintf("job-%02d", job)
			if job == 0 {
				jobID = "starter"
			}
			jobs := corebattle.BattleJobs{JobID: jobID, SP: requester.Jobs.SP, OldJobID: "job-36", OldSP: 999}
			pool = append(pool, Monster{ID: fmt.Sprintf("generated-%d", i), EncounterIdentity: generatedEncounterIdentity(jobID), Name: fmt.Sprintf("Generated combatant %d", i+1), HP: requester.MaxHP, MP: requester.MaxMP, Attack: requester.BaseStats.Attack, Defense: requester.BaseStats.Defense, Agility: requester.BaseStats.Agility, ExperienceReward: requester.ExperienceReward, GoldReward: requester.GoldReward,
				Combat: &corebattle.LifetimeActor{Jobs: jobs, OriginalJobs: jobs, NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:stage/15.cgi:47"}}})
		}
	} else {
		if len(table.BossRosters) == 0 || len(table.MonsterIDs) == 0 {
			return OrdinaryEncounter{}, ErrInvalidMonster
		}
		branch := 0
		if len(table.BossRosters) > 1 {
			branch = int(rng.Float64() * float64(len(table.BossRosters)))
		}
		for _, group := range []struct {
			ids []string
			dst *[]Monster
		}{{table.BossRosters[branch], &bosses}, {table.MonsterIDs, &pool}} {
			for _, id := range group.ids {
				m, err := catalog.FindByID(id)
				if err != nil {
					return OrdinaryEncounter{}, err
				}
				*group.dst = append(*group.dst, m)
			}
		}
	}
	chosen := bosses
	if floor < BossFloor {
		count := int(rng.Float64()*(2.5+float64(len(party))*.6)) + 1
		chosen = make([]Monster, count)
		for i := range chosen {
			position := 0
			if len(table.Appears) > 0 {
				position = table.Appears[int(rng.Float64()*float64(len(table.Appears)))]
			} else {
				position = int(rng.Float64() * float64(len(pool)))
			}
			if position < 0 || position >= len(pool) {
				return OrdinaryEncounter{}, ErrInvalidMonster
			}
			chosen[i] = pool[position]
		}
	}
	actors := make([]corebattle.LifetimeActor, 0, len(chosen))
	session := CrawlSession{Mode: mode, Rng: rng.Intn, RngFloat: rng.Float64}
	for i, m := range chosen {
		a := session.buildMonsterActor(fmt.Sprintf("%s-%d", encounterID, i), m, float64(len(party)-2)*.1, floor >= BossFloor)
		if floor < BossFloor && i < 8 {
			a.EncounterIdentity += string(rune('A' + i))
		}
		a.TeamID = "enemies"
		actors = append(actors, a)
	}
	result := OrdinaryEncounter{Actors: actors, Appears: append([]int(nil), table.Appears...)}
	for _, m := range pool {
		result.Pool = append(result.Pool, m.NPCPayload())
	}
	return result, nil
}
