package battle

import "github.com/witchcraze/party2re/internal/core/character"

// ApplyLevelGrowth updates the room's SP and baselines. cappedLevels counts
// the initial gains under the original room ceiling (_battle.cgi:308–342).
func (a *LifetimeActor) ApplyLevelGrowth(growth []character.Stats, spGain, cappedLevels int) {
	a.Jobs.SP += spGain
	for i, gain := range growth {
		stats := character.Stats{
			MaxHP: a.MaxHP + gain.MaxHP, MaxMP: a.MaxMP + gain.MaxMP,
			Attack: a.BaseStats.Attack + gain.Attack, Defense: a.BaseStats.Defense + gain.Defense, Agility: a.BaseStats.Agility + gain.Agility,
		}
		if i < cappedLevels {
			stats.Clamp(false, 0)
		}
		a.MaxHP, a.MaxMP = stats.MaxHP, stats.MaxMP
		a.BaseStats = BattleBaseStats{Attack: stats.Attack, Defense: stats.Defense, Agility: stats.Agility}
	}
}
