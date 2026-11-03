package adventure

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func travelFixture(t *testing.T) (*NormalRunService, *NormalRun, *ordinaryRNG, corebattle.EquipmentResetContext) {
	t.Helper()
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID("stage-00")
	if err != nil {
		t.Fatal(err)
	}
	party := ordinaryParty()
	for i := range party {
		party[i].Gear = &corebattle.BattleGearStats{}
	}
	r := &NormalRun{ID: "travel", Admission: NormalAdmission{Speed: 3}, Stage: stage, Mode: "try", Floor: 0, Roster: []string{"b", "a"}, LeaderID: "b", Battle: corebattle.LifetimeState{Actors: party}, LastActions: map[string]time.Time{}, Recipients: map[string]RunRecipient{"a": {Conditional: RunEarnings{Gold: 7}}}}
	rng := &ordinaryRNG{}
	s := &NormalRunService{cfg: NormalRunConfig{Stages: stages, Monsters: monsters, Combat: &ordinaryCombat{}, RNG: rng}}
	frame := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}, Command: "tool"}}
	return s, r, rng, frame
}

func TestCombatTravelCoordinatesAndNoEffect(t *testing.T) {
	for _, tc := range []struct {
		tool, stage string
		floor, want int
		input       CombatTravelInput
	}{
		{"item-066", "stage-15", 0, 1, CombatTravelInput{}},
		{"item-067", "stage-16", 11, 1, CombatTravelInput{}},
		{"item-068", "stage-17", 3, 1, CombatTravelInput{}},
		{"item-069", "stage-18", 3, 1, CombatTravelInput{}},
		{"item-070", "stage-19", 3, 1, CombatTravelInput{}},
		{"item-071", "stage-20", 3, 1, CombatTravelInput{}},
		{"item-104", "stage-21", 3, 1, CombatTravelInput{}},
		{"item-184", "stage-23", 3, 1, CombatTravelInput{DestinationStageID: "stage-23"}},
		{"item-184", "stage-25", 3, 1, CombatTravelInput{DestinationStageID: "stage-25"}},
		{"item-083", "stage-00", 0, 1, CombatTravelInput{}},
		{"item-083", "stage-00", 8, 9, CombatTravelInput{}},
		{"item-083", "stage-00", 9, 9, CombatTravelInput{}},
		{"item-083", "stage-00", 10, 10, CombatTravelInput{}},
		{"item-083", "stage-00", 11, 11, CombatTravelInput{}},
		{"item-107", "stage-00", 0, 10, CombatTravelInput{}},
		{"item-107", "stage-00", 10, 10, CombatTravelInput{}},
	} {
		t.Run(fmt.Sprintf("%s/%d/%s", tc.tool, tc.floor, tc.stage), func(t *testing.T) {
			for _, enemies := range []bool{false, true} {
				s, r, rng, _ := travelFixture(t)
				r.Floor, r.StageCleared, r.EXUnlocked, r.ClearPending = tc.floor, true, true, true
				r.Battle.Fields = corebattle.LifetimeFields{Temporary: "burn", Turns: 3, AntiTurns: 2}
				if tc.floor == TreasureRoomFloor {
					r.Boxes = []TreasureBox{{ID: "old-box", ItemID: "item-001"}}
				}
				r.actor("a").Status = corebattle.StatusPoison
				r.actor("a").Gear.WeaponID, r.actor("a").Gear.ArmorID = "weapon-35", "armor-37"
				if enemies {
					r.Enemies = []string{"old"}
					r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "old", HP: 4}, NPC: true})
				}
				before, _ := json.Marshal(r)
				if err := s.ValidateCombatTravel(r, "a", tc.tool, tc.input, nil); err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(r)
				if string(before) != string(after) || rng.calls != 0 {
					t.Fatal("preflight changed state or RNG")
				}
				result, err := s.ApplyCombatTravel(context.Background(), r, "a", tc.tool, tc.input, nil)
				if err != nil {
					t.Fatal(err)
				}

				if r.Stage.ID != tc.stage || r.Floor != tc.want || !result.Consume || result.ResponseSuppressed != (tc.floor == 11) {
					t.Fatal(r.Stage.ID, r.Floor, result)
				}
				// Only coordinates change, even with no effect, living enemies or floor0 nonleader use.
				var want NormalRun
				if err := json.Unmarshal(before, &want); err != nil {
					t.Fatal(err)
				}
				want.Stage, want.Floor = r.Stage, tc.want
				got, _ := json.Marshal(r)
				expected, _ := json.Marshal(want)
				if string(got) != string(expected) || rng.calls != 0 {
					t.Fatal("coordinate tool changed other room facts")
				}
			}
		})
	}
}

func TestCombatTravelRejectsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		tool   string
		input  CombatTravelInput
		mutate func(*NormalRun)
	}{
		{"item-071", CombatTravelInput{}, func(*NormalRun) {}},
		{"item-184", CombatTravelInput{}, func(*NormalRun) {}},
		{"item-184", CombatTravelInput{DestinationStageID: "stage-24"}, func(*NormalRun) {}},
		{"item-150", CombatTravelInput{Floor: 0}, func(*NormalRun) {}},
		{"item-150", CombatTravelInput{Floor: 11}, func(*NormalRun) {}},
		{"item-150", CombatTravelInput{Floor: 1}, func(r *NormalRun) { r.actor("b").Gear.AccessoryID = "item-110" }},
		{"item-150", CombatTravelInput{Floor: 1}, func(r *NormalRun) {
			r.actor("b").RosterDetached, r.actor("b").RosterIdentity = true, "dismissed-helper"
		}},
		{"item-066", CombatTravelInput{Floor: 1}, func(*NormalRun) {}},
		{"item-107", CombatTravelInput{}, func(r *NormalRun) { r.actor("a").HP = 0 }},
		{"item-999", CombatTravelInput{}, func(*NormalRun) {}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			s, r, rng, reset := travelFixture(t)
			tc.mutate(r)
			before, _ := json.Marshal(r)
			if _, err := s.ApplyCombatTravel(context.Background(), r, "a", tc.tool, tc.input, &reset); err == nil {
				t.Fatal("invalid travel accepted")
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) || rng.calls != 0 {
				t.Fatal("rejection mutated state/RNG")
			}
		})
	}
	for _, input := range []string{`{"floor":null}`, `{"floor":"1"}`, `{"floor":1.5}`, `{"floor":11}`, `{"floor":0}`, `null`, `{"floor":1,"unknown":1}`} {
		s, r, _, reset := travelFixture(t)
		value, err := ParseCombatTravelInput(input)
		if err == nil {
			err = s.ValidateCombatTravel(r, "a", "item-150", value, &reset)
		}
		if err == nil {
			t.Fatal("invalid typed input accepted", input)
		}
	}
}

func TestCombatTravel150ResponseAndReload(t *testing.T) {
	for _, state := range []string{"absent", "unready", "ready", "fallen", "collision", "treasure"} {
		t.Run(state, func(t *testing.T) {
			s, r, rng, reset := travelFixture(t)
			r.Floor, r.EncounterFloor, r.EncounterStageID = 2, 2, "stage-00"
			if state == "treasure" {
				r.Floor = 11
			}
			r.Battle.Fields = corebattle.LifetimeFields{Temporary: "burn", Turns: 4, AntiTurns: 2, Default: "snow"}
			r.actor("a").Status, r.actor("a").Tension = corebattle.StatusPoison, 5
			if state != "absent" {
				a := r.actor("a").Clone()
				a.ID, a.TeamID, a.NPC, a.HP, a.EncounterIdentity = "old", "enemies", true, 10, "other-native-enemy"
				if state == "fallen" {
					a.HP = 0
				}
				if state == "collision" {
					// The actual catalog identity is used below, independent of display names.
					m, err := s.cfg.Monsters.FindByID("ordinary-00-monster-29")
					if err != nil {
						t.Fatal(err)
					}
					a.EncounterIdentity = m.EncounterIdentity + "A"
				}
				r.Enemies = []string{a.ID}
				r.Battle.Actors = append(r.Battle.Actors, a)
				reset.EnemyIDs = append([]string(nil), r.Enemies...)
				if state == "unready" || state == "collision" {
					r.LastActions[a.ID] = time.Unix(100, 0)
				}
			}
			fields := r.Battle.Fields
			transition, err := s.ApplyCombatTravel(context.Background(), r, "a", "item-150", CombatTravelInput{Floor: 1}, &reset)
			if err != nil {
				t.Fatal(err)
			}
			if !transition.Consume || r.Floor != 1 || r.actor("a").Status != "" || r.actor("a").Tension != 1 || r.actor("a").HP != 500 || r.Battle.Fields != fields || rng.calls == 0 {
				t.Fatal("reset/generation contract failed", transition)
			}
			npc := ""
			npc = transition.BeginNPCResponse(r, time.Unix(100, 0))
			responded := state == "ready" || state == "collision"
			if (npc != "") != responded {
				t.Fatal("wrong response", npc, state)
			}
			if responded {
				if err := r.RecordNPCResponse(npc, time.Unix(100, 0)); err != nil {
					t.Fatal(err)
				}
			}

			if responded && !reflect.DeepEqual(r.Enemies, []string{"old"}) {
				t.Fatal("old roster not restored", r.Enemies)
			}
			if !responded && (len(r.Enemies) == 0 || r.Enemies[0] == "old") {
				t.Fatal("new encounter lost", r.Enemies)
			}
			if state == "collision" && r.actor("old").HP != 4 {
				t.Fatal("generated collision failed to overwrite old payload", r.actor("old"))
			}
			if len(r.Battle.Actors) != len(r.Enemies)+2 {
				t.Fatal("unsaved roster leaked", r.Battle.Actors)
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var loaded NormalRun
			if err := json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(loaded)
			if string(data) != string(again) {
				t.Fatal("reload changed encounter facts")
			}
		})
	}
}
