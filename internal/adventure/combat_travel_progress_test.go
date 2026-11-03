package adventure

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type travelAdvanceCombat struct{ ordinaryCombat }

func (*travelAdvanceCombat) PrepareAdvance(_ context.Context, r *NormalRun, id string) error {
	frame := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: id, EnemyIDs: r.Enemies}}
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.actor(id).TeamID {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		}
	}
	if err := battle.ResetAll(&r.Battle, &frame, &ordinaryRNG{}); err != nil {
		return err
	}
	r.Battle.Fields.ResetFloor()
	return nil
}

func TestCombatTravelExplicitAdvanceAndEXHooks(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		floor int
		input CombatTravelInput
		want  int
	}{
		{"item-066", 0, CombatTravelInput{}, 2},
		{"item-083", 8, CombatTravelInput{}, 10},
		{"item-107", 0, CombatTravelInput{}, 11},
		{"item-184", 0, CombatTravelInput{DestinationStageID: "stage-23"}, 2},
	} {
		s, r, _, _ := travelFixture(t)
		s.cfg.Combat, s.cfg.Clock = &travelAdvanceCombat{}, &testClock{now: time.Unix(100, 0)}
		r.Floor = tc.floor
		r.Battle.Fields = corebattle.LifetimeFields{Temporary: "burn", Turns: 4, Default: "snow", AntiTurns: 2}
		if _, err := s.ApplyCombatTravel(context.Background(), r, "a", tc.tool, tc.input, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.advance(context.Background(), r, "a"); err != nil {
			t.Fatal(err)
		}
		if r.Floor != tc.want || r.Battle.Fields.Temporary != "" || r.Battle.Fields.AntiTurns != 0 || r.Battle.Fields.Default != "snow" {
			t.Fatal(r)
		}
		if tc.want == 11 && (len(r.Enemies) != 0 || r.StageCleared || r.Mode != "normal" || r.Recipients["a"].Pending.Gold != 10 || r.Recipients["a"].Conditional.Gold != 0) {
			t.Fatal("107 fabricated boss or lost Try conversion", r)
		}
	}
	for _, stage := range []string{"stage-22", "stage-23", "stage-24", "stage-25", "stage-26"} {
		s, r, rng, _ := travelFixture(t)
		r.Stage, _ = s.cfg.Stages.FindByID(stage)
		r.Floor = 10
		s.cfg.Clock = &testClock{now: time.Unix(100, 0)}
		if err := s.advance(context.Background(), r, "a"); err != nil {
			t.Fatal(err)
		}
		want := stage == "stage-23" || stage == "stage-25"
		if r.EXUnlocked != want || r.HasEX() != want {
			t.Fatal(stage, r.EXUnlocked)
		}
		before, _ := json.Marshal(r)
		calls := rng.calls
		if err := s.advance(context.Background(), r, "a"); err == nil {
			t.Fatal("unsupported EX/terminal advance accepted")
		}
		after, _ := json.Marshal(r)
		if string(before) != string(after) || calls != rng.calls {
			t.Fatal("unsupported EX advance changed state")
		}
	}
}

func TestCombatTravel150BossClearMarkerAndRetainedResponseEffects(t *testing.T) {
	s, r, _, reset := travelFixture(t)
	r.Stage, _ = s.cfg.Stages.FindByID("stage-24")
	if _, err := s.ApplyCombatTravel(context.Background(), r, "a", "item-150", CombatTravelInput{Floor: 10}, &reset); err != nil {
		t.Fatal(err)
	}
	if !r.EXUnlocked || !r.ClearPending || r.StageCleared || !r.Battle.SpecialBossField || r.EncounterFloor != 10 || r.EncounterStageID != "stage-24" {
		t.Fatal("boss creation effects missing", r)
	}
	if r.actor("a").MaxHP != 999 || r.actor("a").HP != 500 {
		t.Fatal("boss cap healed or omitted actor")
	}
	// A same display name cannot impersonate the generated NPC's native identity.
	s, r, _, reset = travelFixture(t)
	old := r.actor("a").Clone()
	old.ID, old.NPC, old.TeamID, old.EncounterIdentity = "old", true, "enemies", "other"
	old.Name = "Stage 00 monster 29"
	r.Battle.Actors = append(r.Battle.Actors, old)
	r.Enemies = []string{old.ID}
	reset.EnemyIDs = r.Enemies
	r.EncounterFloor, r.EncounterStageID = 8, "stage-03"
	transition, err := s.ApplyCombatTravel(context.Background(), r, "a", "item-150", CombatTravelInput{Floor: 1}, &reset)
	if err != nil {
		t.Fatal(err)
	}
	// The enclosing player tail can update the retained old enemy context.
	transition.ResponseEnemies[0].HP = 3
	if transition.BeginNPCResponse(r, time.Unix(100, 0)) != "old" || r.actor("old").HP != 3 || r.EncounterFloor != 8 || r.EncounterStageID != "stage-03" {
		t.Fatal("old response context lost")
	}
}

func TestNativeEncounterCollisionAliases(t *testing.T) {
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	// Source stage1 bosses11/42 share a stem but retain distinct A/B suffixes.
	first, _ := monsters.FindByID("ordinary-01-boss-11")
	second, _ := monsters.FindByID("ordinary-01-boss-42")
	if first.EncounterIdentity == second.EncounterIdentity {
		t.Fatal("boss suffix collapsed")
	}
	for _, pair := range [][2]string{{"job-39", "ordinary-00-monster-38"}, {"job-79", "ordinary-00-monster-65"}} {
		m, err := monsters.FindByID(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if generatedEncounterIdentity(pair[0]) != m.EncounterIdentity {
			t.Fatal("generated job collision alias lost", pair)
		}
	}
}

func TestCombatTravelReloadedContinuationMatchesUninterrupted(t *testing.T) {
	for _, tool := range []string{"item-066", "item-083", "item-107", "item-150"} {
		s, r, _, reset := travelFixture(t)
		s.cfg.Clock = &testClock{now: time.Unix(100, 0)}
		s.cfg.Combat = &travelAdvanceCombat{}
		input := CombatTravelInput{}
		if tool == "item-150" {
			input.Floor = 10
		}
		transition, err := s.ApplyCombatTravel(context.Background(), r, "a", tool, input, &reset)
		if err != nil {
			t.Fatal(err)
		}
		if npc := transition.BeginNPCResponse(r, time.Unix(100, 0)); npc != "" {
			t.Fatal("unexpected immediate NPC", npc)
		}
		r.Version++
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var loaded NormalRun
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		// Defeat is supplied by the separate combat consumer; both continuations
		// carry identical finished encounter facts into explicit advance.
		for i := range r.Battle.Actors {
			if slices.Contains(r.Enemies, r.Battle.Actors[i].ID) {
				r.Battle.Actors[i].HP = 0
			}
		}
		for i := range loaded.Battle.Actors {
			if slices.Contains(loaded.Enemies, loaded.Battle.Actors[i].ID) {
				loaded.Battle.Actors[i].HP = 0
			}
		}
		firstRNG, secondRNG := &ordinaryRNG{}, &ordinaryRNG{}
		s.cfg.RNG = firstRNG
		if err := s.advance(context.Background(), r, "a"); err != nil {
			t.Fatal(err)
		}
		s.cfg.RNG = secondRNG
		if err := s.advance(context.Background(), &loaded, "a"); err != nil {
			t.Fatal(err)
		}
		first, _ := json.Marshal(r)
		second, _ := json.Marshal(loaded)
		if string(first) != string(second) || firstRNG.calls != secondRNG.calls {
			t.Fatal("reload changed continuation", tool)
		}
	}
}
