package adventure

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestSummonPoolUsesCurrentSeasonAndCompleteOrderedPayload(t *testing.T) {
	stage := Stage{MonsterIDs: []string{"ordinary", "boss"}, BossIDs: []string{"boss"}, Seasons: []SeasonalStageData{{Season: "winter", MonsterIDs: []string{"winter", "boss"}}}}
	payloads := map[string]corebattle.LifetimeActor{}
	for _, id := range []string{"ordinary", "boss", "winter"} {
		payloads[id] = corebattle.LifetimeActor{Participant: corebattle.Participant{ID: id, CustomSkills: []corebattle.ActionCustomSkill{{CMPCost: 47}}}, Jobs: corebattle.BattleJobs{JobID: "job-2", OldJobID: "job-3", SP: 12, OldSP: 34}}
	}
	pool, err := stage.ForSeason("winter").SummonPool(payloads)
	if err != nil || len(pool) != 2 || pool[0].ID != "winter" || pool[1].ID != "boss" || pool[0].CustomSkills[0].CMPCost != 47 || pool[0].Jobs.OldSP != 34 {
		t.Fatal(pool, err)
	}
	delete(payloads, "boss")
	if _, err := stage.SummonPool(payloads); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	payloads["boss"] = payloads["ordinary"]
	if _, err := stage.SummonPool(payloads); !errors.Is(err, ErrInvalidMonster) {
		t.Fatal(err)
	}
}
