package adventure

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NPCPayload supplies an isolated source template to either ordinary stat rolls
// or the shared summon owner. It does not certify any selected callback.
func (m Monster) NPCPayload() corebattle.LifetimeActor {
	a := corebattle.LifetimeActor{Jobs: corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}}
	if m.Combat != nil {
		a = m.Combat.Clone()
	}
	if a.Jobs.JobID == "" {
		a.Jobs.JobID = "starter"
	}
	if a.Jobs.OldJobID == "" {
		a.Jobs.OldJobID = "starter"
	}
	a.ID, a.Name, a.HP, a.MP, a.CMP = m.ID, m.Name, m.HP, m.MP, m.CMP
	a.Attack, a.Defense, a.Agility = m.Attack, m.Defense, m.Agility
	a.ExperienceReward, a.GoldReward = m.ExperienceReward, m.GoldReward
	a.NPC = true
	if a.Hit == 0 {
		a.Hit = 95
	}
	if a.Tension == 0 {
		a.Tension = 1
	}
	if a.Gear == nil {
		a.Gear = &corebattle.BattleGearStats{}
	}
	if a.Icon == "" {
		a.Icon = "assets/images/placeholders/character/character-default.svg"
	}
	if a.NPCLoadout != nil {
		a.NPCLoadout.CustomCost = corebattle.CustomGemCost(a.NPCLoadout.Gems, a.NPCLoadout.GemCosts)
	}
	a.OriginalJobs = a.Jobs
	return a
}
