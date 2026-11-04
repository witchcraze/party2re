package adventure

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestJobEncounterRoomReplacementRetainsRoomFacts(t *testing.T) {
	for _, id := range []string{"job-78-entry-2", "job-78-entry-6"} {
		s, r, rng, reset := travelFixture(t)
		r.Floor, r.Mode, r.EncounterFloor, r.EncounterStageID = 8, "normal", 8, r.Stage.ID
		r.Battle.Fields = corebattle.LifetimeFields{Default: "snow", Temporary: "burn", Turns: 5, AntiTurns: 2}
		r.actor("a").Status, r.actor("a").Tension = corebattle.StatusPoison, 5
		r.actor("a").Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 200}
		if id == "job-78-entry-6" {
			r.Stage, _ = s.cfg.Stages.FindByID("stage-19")
			r.Floor, r.EncounterFloor = 11, 11
			r.Boxes = []TreasureBox{{ID: "old", ItemID: "item-001"}}
			s.cfg.Clock = &testClock{now: time.Unix(100, 0)}
		}
		before := r.Recipients["a"]
		fields := r.Battle.Fields
		ctx := context.WithValue(context.Background(), normalMutationContext{}, r.ID)
		c, transition, err := s.PrepareJobEncounters(ctx, r, "a", &reset)
		if err != nil {
			t.Fatal(err)
		}
		var d skill.BattleDefinitions
		d.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{JobEncounters: c}, 2)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"old"}}
		if _, err := d.InvocationAction(&r.Battle, &f, []string{id}, rng); err != nil || rng.calls != 0 {
			t.Fatal(err, rng.calls)
		}
		if err := r.Battle.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, rng); err != nil {
			t.Fatal(err)
		}
		if r.Recipients["a"] != before || r.Battle.Fields != fields || transition.Consume || !transition.Reload {
			t.Fatal(r, transition)
		}
		if id == "job-78-entry-2" {
			if r.Floor != 9 || r.actor("a").Status != "" || r.actor("a").Tension != 1 || len(r.Enemies) == 0 {
				t.Fatal(r)
			}
		} else {
			if r.Floor != 11 || len(r.Boxes) == 0 || r.Boxes[0].ID == "old" || !transition.UnsealDemonKing || r.actor("a").Jobs.SP != 100 || r.actor("a").Status != corebattle.StatusPoison || !transition.ResponseSuppressed {
				t.Fatal(r, transition)
			}
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var loaded NormalRun
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		copy, err := json.Marshal(loaded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(data, copy) {
			t.Fatal("reload changed room state")
		}
	}
}
