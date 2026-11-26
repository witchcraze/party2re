package boss

import (
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

const dejonEntry = "npc-dejon"

// npcSkills is vs_king.cgi's @npc_skills (lines 7–10): こうげき and デジョン.
var npcSkills = append(skill.NPCDefaultSeed(), corebattle.CallbackBinding{
	CallbackEntry: corebattle.CallbackEntry{ID: dejonEntry, Name: "デジョン"},
	Invocation:    corebattle.CallbackInvocation{Kind: "entry", EntryID: dejonEntry},
})

// dejon banishes every fallen player (vs_king.cgi:80–98). The host removes
// banished members from the room after the command.
var dejon = corebattle.CallbackDefinition{
	Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
	Project: func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) (bool, error) {
		banishFallen(s)
		return true, nil
	},
	Execute: func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ random.Generator) error {
		banishFallen(s)
		return nil
	},
}

func banishFallen(s *corebattle.LifetimeState) {
	for i := range s.Actors {
		if a := &s.Actors[i]; !a.NPC && a.HP <= 0 {
			a.Banished = true
		}
	}
}

// scaleCrystals is the vs_king clear-time multiplier on a defeat's crystals,
// measured from the unseal clock (_battle.cgi:152–170).
func scaleCrystals(crystals int, elapsed time.Duration) int {
	switch {
	case elapsed <= 600*time.Second:
		return crystals * 3
	case elapsed <= 1800*time.Second:
		return crystals * 2
	case elapsed <= 3600*time.Second:
		return crystals * 3 / 2
	}
	return crystals
}
