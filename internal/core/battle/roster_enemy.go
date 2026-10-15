package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *LifetimeState) validateEnemySummon(c *RosterContext, id string) error {
	if len(s.Actors) >= 10 || id == "gem-82" && len(c.EnemyIDs) > 1 {
		return nil
	}
	if len(c.EnemyPool) == 0 || c.EnemyTeamID == "" {
		return ErrUnsupportedEffect
	}
	if s.RosterSequence >= 1e15 {
		return ErrInvalidParticipant
	}
	for _, a := range c.EnemyPool {
		if a.ID == "" || a.Name == "" || a.HP <= 0 || a.MP < 0 || a.MaxHP < 0 || a.MaxMP < 0 || a.MaxCMP < 0 || a.Attack < 0 || a.Defense < 0 || a.Agility < 0 || a.CMP < 0 || a.ExperienceReward < 0 || a.GoldReward < 0 || a.CrystalReward < 0 || a.Gear == nil {
			return ErrInvalidParticipant
		}
		if a.Jobs.SP < 0 || a.Jobs.OldSP < 0 {
			return ErrInvalidParticipant
		}
		for _, value := range []int{a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP, a.Jobs.SP, a.Jobs.OldSP, a.ExperienceReward, a.GoldReward, a.CrystalReward} {
			if value > 1e12 {
				return ErrInvalidParticipant
			}
		}
		for _, custom := range a.CustomSkills {
			if custom.CMPCost < 0 {
				return ErrInvalidParticipant
			}
		}
	}
	return nil
}

// Whole payloads are retained, including explicit maxima, job pairs, status,
// extended effects, custom costs and rewards. Only source-listed fields scale.
func (s *LifetimeState) summonEnemy(c *RosterContext, id string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{}
	if len(s.Actors) >= 10 || id == "gem-82" && len(c.EnemyIDs) > 1 {
		return r, nil
	}
	suffix := int(rng.Float64() * 18)
	position := 0
	if len(c.AppearancePositions) > 0 {
		position = c.AppearancePositions[int(rng.Float64()*float64(len(c.AppearancePositions)))]
	} else {
		position = int(rng.Float64() * float64(len(c.EnemyPool)))
	}
	if position < 0 || position >= len(c.EnemyPool) {
		position = 0
	}
	a := c.EnemyPool[position]
	identity := func(n int) string { return "enemy-" + a.ID + "-" + string(rune('I'+n)) }
	key := identity(suffix)
	if s.rosterIdentity(key) != nil {
		key = identity((suffix + 17) % 18)
	}
	if s.rosterIdentity(key) != nil {
		return r, nil
	}
	hard := func(v int) int {
		if c.Hard {
			return int(float64(v) * 1.5)
		}
		return v
	}
	partyScale := func(v int) int { return int(float64(hard(v)) * (1 + float64(len(c.PartyIDs)-2)*.05)) }
	a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP = partyScale(a.HP), partyScale(a.MP), partyScale(a.Attack), partyScale(a.Defense), partyScale(a.Agility), partyScale(a.CMP)
	if a.MaxHP == 0 {
		a.MaxHP = a.HP
	}
	if a.MaxMP == 0 {
		a.MaxMP = a.MP
	}
	if a.MaxCMP == 0 {
		a.MaxCMP = a.CMP
	}
	if a.BaseStats.Attack == 0 {
		a.BaseStats.Attack = a.Attack
	}
	if a.BaseStats.Defense == 0 {
		a.BaseStats.Defense = a.Defense
	}
	if a.BaseStats.Agility == 0 {
		a.BaseStats.Agility = a.Agility
	}
	a.Jobs.SP, a.Jobs.OldSP = hard(a.Jobs.SP), hard(a.Jobs.OldSP)
	a.OriginalJobs = a.Jobs
	a.ExperienceReward, a.GoldReward, a.CrystalReward = hard(a.ExperienceReward), hard(a.GoldReward), hard(a.CrystalReward)
	a.ID, a.RosterIdentity, a.TeamID = s.nextRosterID(), key, c.EnemyTeamID
	a.Name += " " + key[len(key)-1:]
	a.NPC = true
	if a.Hit == 0 {
		a.Hit = 95
	}
	if a.Tension == 0 {
		a.Tension = 1
	}
	// Payload definitions are immutable; the live incarnation owns its slices.
	a.Abilities = slices.Clone(a.Abilities)
	a.ItemDefinitionIDs = slices.Clone(a.ItemDefinitionIDs)
	a.Skills = slices.Clone(a.Skills)
	a.ActionItems = slices.Clone(a.ActionItems)
	a.CustomSkills = slices.Clone(a.CustomSkills)
	for i := range a.CustomSkills {
		a.CustomSkills[i].Gems = slices.Clone(a.CustomSkills[i].Gems)
	}
	gear := *a.Gear
	a.Gear = &gear
	s.Actors = append(s.Actors, a)
	if a.TeamID == s.actor(c.ActorID).TeamID {
		c.PartyIDs = append(c.PartyIDs, a.ID)
	} else {
		c.EnemyIDs = append(c.EnemyIDs, a.ID)
	}
	return RosterResult{Applied: true, HelperID: a.ID}, nil
}
