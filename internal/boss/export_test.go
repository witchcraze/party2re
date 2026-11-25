package boss

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// RosterForTest exposes a stage's initial room roster to external tests.
func RosterForTest(stage BossStage, unsealers ...corebattle.LifetimeActor) []corebattle.LifetimeActor {
	return stageRoster(stage, unsealers)
}

// UnsealerForTest exposes a character's battle row as an unsealer.
var UnsealerForTest = unsealerFromCharacter

// unsealerFromCharacter builds a character's battle row from its base stats, as the unseal does.
func unsealerFromCharacter(c corecharacter.Character) corebattle.LifetimeActor {
	exp, gold := c.BattleRowRewards()
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID: c.ID, Name: c.Name, MaxHP: c.Stats.MaxHP, MaxMP: c.Stats.MaxMP,
			Attack: c.Stats.Attack, Defense: c.Stats.Defense, Agility: c.Stats.Agility, ExperienceReward: exp, GoldReward: gold,
		},
		Jobs: corebattle.BattleJobs{JobID: c.JobID, SP: c.SP, OldJobID: c.OldJobID, OldSP: c.OldSP},
	}
}
