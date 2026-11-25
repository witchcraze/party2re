package boss

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// RosterForTest exposes a stage's initial room roster to external tests.
func RosterForTest(stage BossStage, unsealers ...corebattle.LifetimeActor) []corebattle.LifetimeActor {
	return stageRoster(stage, unsealers)
}

// UnsealerForTest exposes a character's battle row as an unsealer.
var UnsealerForTest = unsealerFromCharacter
