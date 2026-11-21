package adventure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
		{"marker on non-EX destination", func(r *NormalRun) { r.Stage.ID, r.Stage.EX = "stage-15", nil }, ErrInvalidFloorAction},
		{"unopened chest", func(r *NormalRun) { r.Boxes = []TreasureBox{{ID: "box"}} }, ErrInvalidFloorAction},
		{"missing roster", func(r *NormalRun) { cp := *r.Stage.Ordinary; cp.EXRoster = nil; r.Stage.Ordinary = &cp }, corebattle.ErrUnsupportedEffect},
		{"missing EX pool", func(r *NormalRun) { ex := *r.Stage.EX; ex.TreasureItems, r.Floor, r.Stage.EX = nil, EXBossFloor, &ex }, corebattle.ErrUnsupportedEffect},
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
			stages, err := InitialStageCatalog()
			if err != nil {
				t.Fatal(err)
			}
			stage, err := stages.FindByID(tc.stage)
			if err != nil {
				t.Fatal(err)
			}
			r := &NormalRun{Stage: stage, EXUnlocked: tc.marker}
			d := &exDraws{values: slices.Clone(tc.draws)}
			for _, identity := range tc.ids {
				r.EXUnlocked = rollRoyalBeeEX(r.Stage, identity, d) || r.EXUnlocked
			}
			if r.EXUnlocked != tc.want || len(d.calls) != len(tc.draws) || slices.ContainsFunc(d.calls, func(n int) bool { return n != 3 }) {
				t.Fatal(r.EXUnlocked, d.calls)
			}
		})
	}
}

func items(ids ...int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprintf("item-%03d", id)
	}
	return out
}

func span(from, to int) []int {
	var out []int
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// exPools are read independently from ff97fc5 stage/22–26.cgi: the item row of
// @ex_treasures (stage26 aliases each season's @treasures) and @fixed_treasures.
var exPools = map[string]struct {
	pool  []string
	fixed string
}{
	"stage-22": {items(150, 161, 164, 180, 180, 180, 180, 182, 264, 264), ""},
	"stage-23": {items(slices.Concat([]int{13}, span(16, 27), span(30, 40), []int{75, 85, 128, 170, 171, 244, 245, 264})...), ""},
	"stage-24": {items(slices.Concat([]int{104, 107, 126, 185, 206, 210}, span(231, 236))...), "item-267"},
	"stage-25": {items(slices.Concat([]int{104, 107, 126}, span(152, 156), []int{184, 246})...), "item-265"},
	"spring":   {items(slices.Concat(span(43, 56), []int{81, 82, 84, 125, 127}, span(130, 134), []int{215, 263})...), "item-263"},
	"summer":   {items(143, 144, 145, 146, 147, 148, 149, 158, 169, 182, 263), "item-263"},
	"autumn":   {items(slices.Concat(span(1, 3), span(6, 8), []int{10}, span(16, 22), span(16, 22), []int{263})...), "item-263"},
	"winter":   {items(slices.Concat([]int{13}, span(30, 39), []int{75, 85, 184, 263})...), "item-263"},
}

func TestEXTreasureCreationPoolsCountsAndAllowanceReset(t *testing.T) {
	for key, want := range exPools {
		t.Run(key, func(t *testing.T) {
			s, r, _ := exFixture(t, key)
			r.Floor, r.StageCleared = EXBossFloor, false
			r.Recipients = map[string]RunRecipient{"a": {Inspected: 1, Conditional: RunEarnings{Gold: 7}}, "b": {Inspected: 2}}
			if !slices.Equal(r.Stage.exTreasureItems(), want.pool) || r.Stage.EX.FixedTreasure != want.fixed {
				t.Fatalf("pool %v fixed %q", r.Stage.exTreasureItems(), r.Stage.EX.FixedTreasure)
			}
			n := len(want.pool)
			// Roster a (alive) and b (fallen) plus a living helper: $#partys=2, minus one fallen,
			// so two EX chests; fixed chests count real members only, fallen included.
			values, wantCalls := []int{3, n - 1, 0, 1}, []int{7, n, 7, n}
			if want.fixed != "" {
				values, wantCalls = append(values, 1, 0), append(wantCalls, 2, 2)
			}
			d := &exDraws{values: values}
			s.cfg.RNG = d
			if err := s.advance(context.Background(), r, "a"); err != nil {
				t.Fatal(err)
			}
			wantBoxes := []TreasureBox{{ID: "box-1", Name: "黒い宝箱A", ItemID: want.pool[n-1]}, {ID: "box-2", Name: "普通の宝箱B", ItemID: want.pool[1]}}
			if want.fixed != "" {
				wantBoxes = append(wantBoxes, TreasureBox{ID: "box-3", Name: "金の宝箱A", ItemID: want.fixed, Fixed: true}, TreasureBox{ID: "box-4", Name: "煤けた宝箱B", ItemID: want.fixed, Fixed: true})
			}
			if !reflect.DeepEqual(r.Boxes, wantBoxes) || !slices.Equal(d.calls, wantCalls) {
				t.Fatalf("boxes %+v draws %v", r.Boxes, d.calls)
			}
			if r.Floor != EXTreasureFloor || r.EncounterFloor != EXTreasureFloor || r.Recipients["a"].Inspected != 0 || r.Recipients["b"].Inspected != 0 || r.StageCleared || r.Recipients["a"].Conditional.Gold != 7 || r.Recipients["a"].Pending.Gold != 0 {
				t.Fatalf("%+v", r)
			}
			if err := s.advance(context.Background(), r, "a"); !errors.Is(err, ErrInvalidFloorAction) {
				t.Fatal("advance past EX treasure", err)
			}
		})
	}
}

func TestTreasurePartyCountsLivingHelpersOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		helperHP int
		floor    int
		want     int
	}{
		{"ordinary living helper", 2, BossFloor, 2},
		{"ordinary fallen helper", 0, BossFloor, 1},
		{"EX living helper", 2, EXBossFloor, 2},
		{"EX fallen helper", 0, EXBossFloor, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, _ := exFixture(t, "stage-22")
			r.Floor = tc.floor
			r.actor("helper").HP = tc.helperHP
			r.pruneFallenNPCs("a") // the previous command's save boundary
			s.cfg.Clock = &testClock{now: time.Unix(100, 0)}
			if err := s.advance(context.Background(), r, "a"); err != nil {
				t.Fatal(err)
			}
			if len(r.Boxes) != tc.want {
				t.Fatal(len(r.Boxes))
			}
		})
	}
}

func TestEXFixedChestsBypassOrdinaryAllowance(t *testing.T) {
	s, r, _ := exFixture(t, "stage-24")
	r.Floor = EXBossFloor
	r.Recipients = map[string]RunRecipient{"a": {}, "b": {}}
	if err := s.advance(context.Background(), r, "a"); err != nil {
		t.Fatal(err)
	}
	var fixed, ordinary []string
	for _, b := range r.Boxes {
		if b.Fixed {
			fixed = append(fixed, b.ID)
		} else {
			ordinary = append(ordinary, b.ID)
		}
	}
	// is_get_fixed is not in @battle_datas, so it never survives the request (config.cgi:102–104).
	for _, id := range fixed {
		box, consumed, err := s.claimChest(context.Background(), r, "a", id)
		if err != nil || box.OpenedBy != "a" || len(consumed) != 0 || r.Recipients["a"].Inspected != 0 {
			t.Fatal(box, consumed, err)
		}
	}
	if _, _, err := s.claimChest(context.Background(), r, "a", ordinary[0]); err != nil || r.Recipients["a"].Inspected != 1 {
		t.Fatal("fixed claim consumed the ordinary allowance", err)
	}
	if _, _, err := s.claimChest(context.Background(), r, "a", fixed[0]); !errors.Is(err, ErrNoUnopenedTreasure) {
		t.Fatal("opened fixed chest reclaimed", err)
	}
	if _, _, err := s.claimChest(context.Background(), r, "b", ordinary[1]); !errors.Is(err, ErrUnconsciousCharacter) {
		t.Fatal("fallen member claimed", err)
	}
}

// Stage sets are read from ff97fc5: vs_monster.cgi:92 (triple), 108 (unseal),
// 143–150 (special boss field), quest.cgi:594–600 (once-daily) and the EX producers.
func TestStageTraitsPinLegacyStageSets(t *testing.T) {
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]StageTraits{
		"stage-17": {TripleTreasure: true},
		"stage-19": {UnsealsOnTreasure: true},
		"stage-20": {TripleTreasure: true},
		"stage-21": {TripleTreasure: true},
		"stage-22": {EX: &EXStage{Marker: markerRoyalBeeDefeat}},
		"stage-23": {EX: &EXStage{Marker: markerAdvanceTwoAllies}},
		"stage-24": {SpecialBossFloors: []int{BossFloor, EXBossFloor}, EX: &EXStage{Marker: markerBossEncounter, FixedTreasure: "item-267"}},
		"stage-25": {SpecialBossFloors: []int{EXBossFloor}, EX: &EXStage{Marker: markerAdvance, FixedTreasure: "item-265"}},
		"stage-26": {DailyOnce: true, EX: &EXStage{Marker: markerRoomCreation, FixedTreasure: "item-263"}},
	}
	for _, st := range stages.Stages() {
		got := st.StageTraits
		if got.EX != nil {
			ex := *got.EX
			ex.TreasureItems = nil // pools are pinned by exPools
			got.EX = &ex
		}
		if !reflect.DeepEqual(got, want[st.ID]) {
			t.Errorf("%s traits %+v, want %+v", st.ID, got, want[st.ID])
		}
	}
	// Each season's EX pool aliases its own @treasures (stage/26.cgi:38).
	st, err := stages.FindByID("stage-26")
	if err != nil {
		t.Fatal(err)
	}
	for _, season := range []string{"spring", "summer", "autumn", "winter"} {
		if s := st.ForSeason(season); !slices.Equal(s.exTreasureItems(), s.TreasureItems) || s.EX.Marker != markerRoomCreation {
			t.Errorf("%s EX pool %v", season, s.exTreasureItems())
		}
	}
}
