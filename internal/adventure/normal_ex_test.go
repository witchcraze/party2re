package adventure

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// exSource is read independently from ff97fc5 stage/22–26.cgi @ex_bosses.
type exSource struct {
	id, job      string
	sp           int
	old          string
	oldSP        int
	hp, mp, gold int
	crystal      int
	item         string
}

var exSources = map[string][]exSource{
	"stage-22": {{"ex-22-boss-80", "job-83", 999, "job-57", 999, 1500, 1000, 88, 0, ""}, {"ex-22-boss-104", "job-83", 999, "job-34", 999, 8888, 5000, 888, 10, "item-190"}, {"ex-22-boss-131", "job-83", 999, "job-57", 999, 1500, 1000, 88, 0, ""}},
	"stage-23": {{"ex-23-boss-34", "job-406", 999, "job-10", 20, 5000, 1500, 250, 3, ""}},
	"stage-24": {{"ex-24-boss-97", "job-405", 1000, "starter", 0, 25000, 10000, 30000, 50, "item-159"}},
	"stage-25": {{"ex-25-boss-206", "job-55", 999, "job-27", 999, 1100, 2400, 200, 0, "item-159"}, {"ex-25-boss-224", "job-407", 999, "starter", 0, 12000, 1500, 1500, 12, "item-246"}, {"ex-25-boss-243", "job-55", 999, "job-27", 999, 1100, 2400, 200, 0, "item-159"}},
	"spring":   {{"ex-26-boss-658", "job-55", 999, "starter", 0, 150, 400, 36, 0, "item-158"}, {"ex-26-boss-673", "job-13", 999, "job-57", 999, 5000, 800, 400, 2, "item-148"}, {"ex-26-boss-699", "job-55", 999, "starter", 0, 170, 400, 36, 0, "item-158"}},
	"summer":   {{"ex-26-boss-806", "job-21", 999, "starter", 0, 220, 170, 8, 0, "item-212"}, {"ex-26-boss-820", "job-52", 999, "job-42", 999, 6500, 500, 200, 2, "item-158"}, {"ex-26-boss-846", "job-21", 999, "starter", 0, 220, 170, 8, 0, "item-212"}},
	"autumn":   {{"ex-26-boss-953", "job-92", 999, "starter", 0, 150, 330, 48, 0, ""}, {"ex-26-boss-967", "job-95", 30, "job-12", 999, 5800, 790, 600, 2, ""}, {"ex-26-boss-992", "job-90", 999, "starter", 0, 170, 330, 48, 0, ""}},
	"winter":   {{"ex-26-boss-1100", "job-41", 90, "starter", 0, 100, 570, 26, 0, ""}, {"ex-26-boss-1114", "job-51", 999, "job-58", 999, 4500, 800, 450, 2, "item-158"}, {"ex-26-boss-1140", "job-41", 90, "starter", 0, 100, 570, 26, 0, ""}},
}

func exFixture(t *testing.T, key string) (*NormalRunService, *NormalRun, *ordinaryRNG) {
	t.Helper()
	s, r, rng, _ := travelFixture(t)
	s.cfg.Combat = &travelAdvanceCombat{}
	stageID := key
	if !strings.HasPrefix(key, "stage-") {
		stageID = "stage-26"
	}
	stage, err := s.cfg.Stages.FindByID(stageID)
	if err != nil {
		t.Fatal(err)
	}
	r.Stage, r.Floor, r.Mode, r.EXUnlocked = stage.OrdinarySeason(key), TreasureRoomFloor, CrawlModeNormal, true
	r.Battle.Actors[0].HP, r.Battle.Actors[0].MaxHP, r.Battle.Actors[0].MP, r.Battle.Actors[0].MaxMP = 1, 1200, 0, 1500
	r.Battle.Actors[0].CMP, r.Battle.Actors[0].MaxCMP = 0, 40
	helper := r.Battle.Actors[0].Clone()
	helper.ID, helper.NPC, helper.HP, helper.MaxHP, helper.MaxCMP = "helper", true, 2, 3000, 0
	r.Battle.Actors = append(r.Battle.Actors, helper)
	r.Battle.Fields = corebattle.LifetimeFields{Temporary: "burn", Turns: 3, AntiTurns: 2}
	return s, r, rng
}

func TestEXEntryRostersRestorationDefaultFieldAndCaps(t *testing.T) {
	catalog, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range exSources {
		t.Run(key, func(t *testing.T) {
			s, r, _ := exFixture(t, key)
			stageID := r.Stage.ID
			if err := s.advance(context.Background(), r, "a"); err != nil {
				t.Fatal(err)
			}
			if r.Floor != EXBossFloor || r.EncounterFloor != EXBossFloor || r.EncounterStageID != stageID || r.ClearPending || r.StageCleared || len(r.EncounterPool) != len(r.Stage.Ordinary.MonsterIDs) {
				t.Fatalf("%+v", r)
			}
			if len(r.Enemies) != len(want) {
				t.Fatalf("roster %v", r.Enemies)
			}
			for i, src := range want {
				m, err := catalog.FindByID(src.id)
				if err != nil {
					t.Fatal(err)
				}
				e := r.actor(r.Enemies[i])
				j := m.Combat.Jobs
				if j.JobID != src.job || j.SP != src.sp || j.OldJobID != src.old || j.OldSP != src.oldSP || m.HP != src.hp || m.MP != src.mp || m.GoldReward != src.gold || m.Combat.CrystalReward != src.crystal || m.Combat.Gear.AccessoryID != src.item {
					t.Fatalf("%s payload %+v", src.id, m)
				}
				if e.Jobs != j || e.TeamID != "enemies" || !e.NPC || e.Name != m.Name {
					t.Fatalf("%s actor %+v", src.id, e)
				}
			}
			a, b, helper := r.actor("a"), r.actor("b"), r.actor("helper")
			capped := stageID == "stage-24" || stageID == "stage-25"
			if r.Battle.SpecialBossField != capped || b.HP != b.MaxHP || a.MP != 1500 || a.CMP != 40 {
				t.Fatalf("restoration/cap: %+v %+v", a, b)
			}
			// Restoration precedes the cap; the shared owner leaves current HP above a lower maximum.
			if a.HP != 1200 || helper.HP != 3000 || (a.MaxHP == 999) != capped || (helper.MaxHP == 999) != capped {
				t.Fatalf("order: a=%d/%d helper=%d/%d", a.HP, a.MaxHP, helper.HP, helper.MaxHP)
			}
			field := map[string]string{"stage-23": "snow", "stage-24": "sanctuary"}[stageID]
			if r.Battle.Fields.Default != field || r.Battle.Fields.Temporary != "" || r.Battle.Fields.AntiTurns != 0 {
				t.Fatalf("field %+v", r.Battle.Fields)
			}
			data, _ := json.Marshal(r)
			var loaded NormalRun
			if err := json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(loaded)
			if string(data) != string(again) {
				t.Fatal("reload changed EX facts")
			}
		})
	}
}

func TestEXEntryGatesPreserveStateAndDraws(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*NormalRun)
		want  error
	}{
		{"ordinary end without marker", func(r *NormalRun) { r.EXUnlocked = false }, ErrInvalidFloorAction},
		{"marker on non-EX destination", func(r *NormalRun) { r.Stage.ID = "stage-15" }, ErrInvalidFloorAction},
		{"unopened chest", func(r *NormalRun) { r.Boxes = []TreasureBox{{ID: "box"}} }, ErrInvalidFloorAction},
		{"missing roster", func(r *NormalRun) { cp := *r.Stage.Ordinary; cp.EXRoster = nil; r.Stage.Ordinary = &cp }, corebattle.ErrUnsupportedEffect},
		{"EX treasure owned by #1808", func(r *NormalRun) { r.Floor = EXBossFloor }, corebattle.ErrUnsupportedEffect},
		{"living EX enemy", func(r *NormalRun) {
			r.Floor = EXBossFloor
			e := r.Battle.Actors[0].Clone()
			e.ID, e.TeamID, e.HP = "boss", "enemies", 9
			r.Battle.Actors, r.Enemies = append(r.Battle.Actors, e), []string{"boss"}
		}, ErrInvalidFloorAction},
		{"terminal13", func(r *NormalRun) { r.Floor = EXBossFloor + 1 }, ErrInvalidFloorAction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, rng := exFixture(t, "stage-23")
			tc.setup(r)
			before, _ := json.Marshal(r)
			if err := s.advance(context.Background(), r, "a"); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) || rng.calls != 0 {
				t.Fatal("rejected advance changed state or drew")
			}
		})
	}
}

func TestEXMarkerSurvivesTravelAgainstDestination(t *testing.T) {
	s, r, _ := exFixture(t, "stage-23")
	r.Floor = 3
	if _, err := s.ApplyCombatTravel(context.Background(), r, "a", "item-066", CombatTravelInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if r.HasEX() || !r.EXUnlocked {
		t.Fatal("non-EX destination used or cleared the marker")
	}
	if _, err := s.ApplyCombatTravel(context.Background(), r, "a", "item-184", CombatTravelInput{DestinationStageID: "stage-25"}, nil); err != nil {
		t.Fatal(err)
	}
	if !r.HasEX() {
		t.Fatal("borrowed marker lost after travel")
	}
}

type exDraws struct{ values, calls []int }

func (d *exDraws) Intn(n int) int {
	d.calls = append(d.calls, n)
	v := d.values[0]
	d.values = d.values[1:]
	return v
}
func (d *exDraws) IntN(n int) int   { return d.Intn(n) }
func (d *exDraws) Float64() float64 { panic("unexpected draw") }

func TestRoyalBeeCreditedDefeatDraws(t *testing.T) {
	bee := royalBeeIdentity
	for _, tc := range []struct {
		name, stage string
		marker      bool
		ids         []string
		draws       []int
		want        bool
	}{
		{"success on second bee", "stage-22", false, []string{bee + "A", "ordinary-22-monster-162", bee}, []int{2, 0}, true},
		{"failed draws", "stage-22", false, []string{bee, bee + "H"}, []int{1, 2}, false},
		{"existing marker still draws", "stage-22", true, []string{bee}, []int{1}, true},
		{"other stage", "stage-21", false, []string{bee}, nil, false},
		{"prefix is not identity", "stage-22", false, []string{bee + "0", "x" + bee}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &NormalRun{Stage: Stage{ID: tc.stage}, EXUnlocked: tc.marker}
			d := &exDraws{values: slices.Clone(tc.draws)}
			r.recordCreditedDefeats(tc.ids, d)
			if r.EXUnlocked != tc.want || len(d.calls) != len(tc.draws) || slices.ContainsFunc(d.calls, func(n int) bool { return n != 3 }) {
				t.Fatal(r.EXUnlocked, d.calls)
			}
		})
	}
}
