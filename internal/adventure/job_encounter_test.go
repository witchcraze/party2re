package adventure

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/runhost"
)

func TestJobEncounterRoomReplacementRetainsRoomFacts(t *testing.T) {
	for _, variant := range []string{"job-78-entry-2", "job-78-entry-6", "job-78-entry-6@13"} {
		id, ex := strings.CutSuffix(variant, "@13")
		s, r, rng, reset := travelFixture(t)
		r.Round, r.Mode, r.EncounterFloor, r.EncounterStageID = 8, "normal", 8, r.Stage.ID
		r.Battle.Fields = corebattle.LifetimeFields{Default: "snow", Temporary: "burn", Turns: 5, AntiTurns: 2}
		r.Actor("a").Status, r.Actor("a").Tension = corebattle.StatusPoison, 5
		r.Actor("a").Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 200}
		if id == "job-78-entry-6" {
			r.Stage, _ = s.cfg.Stages.FindByID("stage-19")
			r.Round, r.EncounterFloor = 11, 11
			r.Boxes = []runhost.TreasureBox{{ID: "old", ItemID: "item-001"}}
			s.cfg.Clock = &testClock{now: time.Unix(100, 0)}
		}
		if ex {
			// set_monster at 13 recreates EX and fixed chests (_skill.cgi:6201–6208).
			r.Stage, _ = s.cfg.Stages.FindByID("stage-24")
			r.Round, r.EncounterFloor, r.EXUnlocked = EXTreasureFloor, EXTreasureFloor, true
			r.Boxes = []runhost.TreasureBox{{ID: "old", ItemID: "item-267", Fixed: true}}
		}
		before := r.Recipients["a"]
		fields := r.Battle.Fields
		room, transition, err := s.prepareJobEncounters(context.Background(), r, "a", &reset)
		if err != nil {
			t.Fatal(err)
		}
		c := jobEncounterContext(r, room, &reset)
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
			if r.Round != 9 || r.Actor("a").Status != "" || r.Actor("a").Tension != 1 || len(r.Enemies) == 0 {
				t.Fatal(r)
			}
		} else {
			if r.Round != r.EncounterFloor || len(r.Boxes) == 0 || r.Boxes[0].ID == "old" || transition.UnsealDemonKing == ex || r.Actor("a").Jobs.SP != 100 || r.Actor("a").Status != corebattle.StatusPoison || !transition.ResponseSuppressed {
				t.Fatal(r, transition)
			}
			if ex && (r.Round != EXTreasureFloor || r.Boxes[0].Fixed || !r.Boxes[len(r.Boxes)-1].Fixed) {
				t.Fatal("EX recreation", r.Boxes)
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

// jobEncounterContext binds room as the host does for a player step that
// executes on r's own combat state.
func jobEncounterContext(r *NormalRun, room runhost.JobRoom, reset *corebattle.EquipmentResetContext) *corebattle.JobEncounterContext {
	frame := reset.TargetContext
	jobRoom := &corebattle.JobRoomContext{ActorID: frame.ActorID, NormalStage: true, Floor: r.Round, Cursor: reset.Cursor, Validate: room.Validate}
	for _, box := range r.Boxes {
		if box.IsUnopened() {
			jobRoom.TreasureIDs = append(jobRoom.TreasureIDs, box.ID)
		}
	}
	jobRoom.Apply = func(id string) (corebattle.TargetContext, error) {
		if err := room.Apply(id); err != nil {
			return frame, err
		}
		jobRoom.Floor, jobRoom.Cursor = r.Round, reset.Cursor
		frame.EnemyIDs, frame.MemberOnlyIDs = slices.Clone(r.Enemies), nil
		return frame, nil
	}
	return &corebattle.JobEncounterContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame}}, Room: jobRoom}
}
