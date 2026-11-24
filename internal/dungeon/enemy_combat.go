package dungeon

import (
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// maxEncounterEnemies bounds a stored group: add_monster names members A..H.
const maxEncounterEnemies = 8

// enemyCombat applies legacy defaults before a source template is used.
// Maxima stay unset unless the source gives them; spawn fills the rest.
func enemyCombat(a corebattle.LifetimeActor) *corebattle.LifetimeActor {
	if a.Jobs.JobID == "" {
		a.Jobs.JobID = "starter"
	}
	if a.Jobs.OldJobID == "" {
		a.Jobs.OldJobID = "starter"
	}
	if a.OriginalJobs == (corebattle.BattleJobs{}) {
		a.OriginalJobs = a.Jobs
	}
	if a.Hit == 0 {
		a.Hit = 95
	}
	if a.Tension == 0 {
		a.Tension = 1
	}
	if load := a.NPCLoadout; load != nil {
		load.CustomCost = corebattle.CustomGemCost(load.Gems, load.GemCosts)
	}
	a.NPC = true
	return &a
}

// spawn applies the party-size roll shared by add_monster and add_boss: hp,
// mp, at, df, ag and c_mp in that order, each × (0.9 + rand(0.3) + (party −
// 2) × 0.1). A source maximum survives (map17 w boss); otherwise the roll sets it.
func (m DungeonMonster) spawn(party int, rng RandomSource) DungeonMonster {
	a := *enemyCombat(corebattle.LifetimeActor{})
	if m.combat != nil {
		a = m.combat.Clone()
	}
	bonus := float64(party-2) * 0.1
	roll := func(v *int) { *v = int(float64(*v) * (0.9 + 0.3*rng.Float64() + bonus)) }
	for _, v := range []*int{&m.HP, &a.MP, &m.Attack, &m.Defense, &m.Agility, &a.CMP} {
		roll(v)
	}
	for _, mx := range []struct{ max, cur *int }{{&a.MaxHP, &m.HP}, {&a.MaxMP, &a.MP}, {&a.MaxCMP, &a.CMP}} {
		if *mx.max == 0 {
			*mx.max = *mx.cur
		}
	}
	m.combat = &a
	return m
}

// spawnRoster is add_boss: every roster member with the party-size roll.
func spawnRoster(roster []DungeonMonster, party int, rng RandomSource) []DungeonMonster {
	out := make([]DungeonMonster, len(roster))
	for i, m := range roster {
		out[i] = m.spawn(party, rng)
	}
	return out
}

// participants builds the battle side of a spawned group. An ordinary group
// repeats monsters, so a repeated source ID gets #n for a unique participant.
func participants(group []DungeonMonster) []corebattle.Participant {
	out := make([]corebattle.Participant, len(group))
	seen := make(map[string]int, len(group))
	for i, m := range group {
		id := m.ID
		if seen[m.ID]++; seen[m.ID] > 1 {
			id = fmt.Sprintf("%s#%d", m.ID, seen[m.ID])
		}
		p := corebattle.MustNewParticipant(id, m.HP, m.Attack, m.Defense)
		p.Name, p.Agility, p.TeamID = m.Name, m.Agility, "enemies"
		if a := m.combat; a != nil {
			p.MaxHP = a.MaxHP
			p.MP, p.MaxMP = a.MP, a.MaxMP
			p.CMP, p.MaxCMP = a.CMP, a.MaxCMP
			p.Status = a.Status
		}
		out[i] = p
	}
	return out
}
