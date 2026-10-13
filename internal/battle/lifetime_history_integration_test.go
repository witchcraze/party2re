package battle_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/database"
)

func TestLifetimeRecordedJobPresenceDatabaseAndStateRoundTrip(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(jobs))
	for _, tc := range []struct {
		name, target                string
		points                      map[string]int
		history                     []corejob.Change
		mastered                    []string
		explicit, recorded, unknown bool
		wantSP                      int
	}{
		{name: "migrated current only", target: "job-82", points: map[string]int{"job-82": 111}},
		{name: "migrated previous only", target: "job-36", points: map[string]int{"job-36": 17}},
		{name: "departed unmastered zero", target: "job-01", points: map[string]int{"job-01": 0}, history: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-82"}}, recorded: true},
		{name: "departed unmastered positive", target: "job-01", points: map[string]int{"job-01": 17}, history: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-82"}}, recorded: true, wantSP: 17},
		{name: "departed unknown SP", target: "job-01", history: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-82"}}, recorded: true, unknown: true},
		{name: "rejoined current", target: "job-82", points: map[string]int{"job-82": 40}, history: []corejob.Change{{FromJobID: "job-82", ToJobID: "job-36"}, {FromJobID: "job-36", ToJobID: "job-82"}}, recorded: true, wantSP: 40},
		{name: "mastery record", target: "job-01", points: map[string]int{"job-01": 80}, mastered: []string{"job-01"}, recorded: true, wantSP: 80},
		{name: "explicit same-job record", target: "job-82", explicit: true, recorded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			char, err := database.CreateTestCharacter(ctx, db, "Job History")
			if err != nil {
				t.Fatal(err)
			}
			char.JobID, char.SP, char.OldJobID, char.OldSP = "job-82", 111, "job-36", 100
			if err := chars.Update(ctx, char); err != nil {
				t.Fatal(err)
			}
			permanent, err := corejob.NewCharacterJob(char.ID, char.JobID)
			if err != nil {
				t.Fatal(err)
			}
			permanent.RecordedJobSP, permanent.History, permanent.MasteredJobs = tc.points, tc.history, tc.mastered
			if tc.explicit {
				if err := permanent.RecordJobSP(tc.target, tc.wantSP); err != nil {
					t.Fatal(err)
				}
			}
			if err := jobs.Save(ctx, permanent); err != nil {
				t.Fatal(err)
			}
			before, err := jobs.FindByCharacterID(ctx, char.ID)
			if err != nil || before.HasRecordedJob(tc.target) != tc.recorded {
				t.Fatal(before, err)
			}
			a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, recorded := a.JobHistory[tc.target]; recorded != tc.recorded {
				t.Fatal(a)
			}
			copy := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "copy", HP: 100, MaxHP: 100}, Jobs: corebattle.BattleJobs{JobID: "job-79", OldJobID: "job-03"}}
			s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, copy}}
			c := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID, "copy"}}}}
			if r, err := s.ApplyTransformation(&c, "book-211-copy", "copy", random.Default()); err != nil || !r.Applied {
				t.Fatal(r, err)
			}
			encoded, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded corebattle.LifetimeState
			if err := json.Unmarshal(encoded, &reloaded); err != nil || !reflect.DeepEqual(s, reloaded) {
				t.Fatal(err, s, reloaded)
			}
			r, err := reloaded.ApplyTransformation(&c, "tool-168-memory", tc.target, random.Default())
			if tc.unknown {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(s, reloaded) {
					t.Fatal(r, err, reloaded)
				}
			} else if err != nil || r.Applied != tc.recorded || tc.recorded && (reloaded.Actors[0].Jobs.OldJobID != tc.target || reloaded.Actors[0].Jobs.OldSP != tc.wantSP) {
				t.Fatal(r, err, reloaded)
			}
			after, err := jobs.FindByCharacterID(ctx, char.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("battle copy changed permanent records", before, after, err)
			}
			if after.RecordedJobSP[tc.target] != tc.points[tc.target] && !tc.explicit {
				t.Fatal("preserved SP changed", after)
			}
		})
	}
}
