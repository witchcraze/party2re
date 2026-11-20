package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// enemyCombat applies legacy defaults before a source template is used.
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
	if a.MaxMP == 0 {
		a.MaxMP = a.MP
	}
	if a.MaxCMP == 0 {
		a.MaxCMP = a.CMP
	}
	if load := a.NPCLoadout; load != nil {
		load.CustomCost = corebattle.CustomGemCost(load.Gems, load.GemCosts)
	}
	a.NPC = true
	return &a
}

func (m DungeonMonster) participant() corebattle.Participant {
	p := corebattle.MustNewParticipant(m.ID, m.HP, m.Attack, m.Defense)
	p.Name, p.Agility, p.TeamID = m.Name, m.Agility, "enemies"
	if a := m.combat; a != nil {
		p.MaxHP = max(m.HP, a.MaxHP) // a preset maximum survives scaling (map17 w boss)
		p.MP, p.MaxMP = a.MP, a.MaxMP
		p.CMP, p.MaxCMP = a.CMP, a.MaxCMP
		p.Status = a.Status
	}
	return p
}
