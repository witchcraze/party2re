package adventure

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func buildFallbackParticipant(c corecharacter.Character) corebattle.Participant {
	hp := c.Stats.HP
	if hp <= 0 && c.Stats.MaxHP > 0 {
		hp = c.Stats.MaxHP
	}
	p := corebattle.MustNewParticipant(c.ID, hp, c.Stats.Attack, c.Stats.Defense)
	p.Name = c.Name
	p.MaxHP = c.Stats.MaxHP
	p.MP = c.Stats.MP
	p.MaxMP = c.Stats.MaxMP
	p.Agility = c.Stats.Agility
	return p
}

func cloneInspectedBoxes(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mathClamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
