package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// ff97fc5:_skill.cgi:1306–1323,1471–1499,1818–1824,6018–6114,
// 6480–6499 and _data.cgi:1641–1675. Content is frozen native definition data.
func TestInspectionEntriesContentAndFlavor(t *testing.T) {
	entries := corebattle.InspectionEntries()
	if len(entries) != 6 {
		t.Fatal(entries)
	}
	for i, row := range []struct {
		id     string
		sp, mp int
	}{
		{"job-7-entry-4", 20, 0}, {"job-9-entry-4", 20, 0}, {"job-9-entry-6", 50, 1},
		{"job-16-entry-1", 10, 2}, {"job-78-entry-1", 15, 0}, {"job-81-entry-1", 30, 0},
	} {
		if e := entries[i]; e.ID != row.id || e.RequiredSP != row.sp || e.MPCost != row.mp || e.Extended {
			t.Fatal(e, row)
		}
	}
	for _, id := range []string{"job-7-entry-4", "job-9-entry-4", "job-81-entry-1", "tool-151"} {
		for _, kind := range []int{0, 1, 2, 3} {
			for _, empty := range []bool{false, true} {
				s, c, d := ordinaryToolFixture()
				content := corebattle.InspectionContent{Treasure: true, Empty: empty, RewardKind: kind, DefinitionID: "item-200", Price: 5000}
				c.Inspection["enemy"] = content
				result := executeOrdinaryTool(t, &s, c, d, id, []string{"enemy", "tail"})
				wantID, wantKind := content.DefinitionID, kind
				if empty {
					wantID, wantKind = "", 0
				}
				if len(result.Facts) != 1 || result.Facts[0].Kind != "content" || result.Facts[0].TargetID != "enemy" || result.Facts[0].DefinitionID != wantID || result.Facts[0].RewardKind != wantKind || result.Facts[0].Empty != empty || result.Consume != (id == "tool-151") {
					t.Fatal(id, result)
				}
				if s.Actors[1].HP != 1000 || len(s.Actors[0].ConsumedItems) != 0 {
					t.Fatal(s.Actors)
				}
			}
		}
		for flavor := range 8 {
			s, c, d := ordinaryToolFixture()
			result := executeOrdinaryTool(t, &s, c, d, id, []string{"enemy"}, float64(flavor)/8)
			if fact := result.Facts[0]; fact.Kind != "scent" || fact.Flavor != flavor || fact.DefinitionID != "" {
				t.Fatal(id, fact)
			}
		}
	}
}

func TestInspectionFallbackIncludesFallenAndStatsUseBaselines(t *testing.T) {
	for _, id := range []string{"job-7-entry-4", "job-9-entry-4", "job-81-entry-1", "job-9-entry-6", "job-16-entry-1"} {
		for _, target := range []string{"", "invalid", "fallen"} {
			s, c, d := ordinaryToolFixture()
			s.Actors[2].Attack = 999
			s.Actors[2].MaxHP = 200
			draws := []float64{}
			if target != "fallen" {
				draws = append(draws, .99)
			}
			stats := id == "job-9-entry-6" || id == "job-16-entry-1"
			if !stats {
				draws = append(draws, .99)
			}
			result := executeOrdinaryTool(t, &s, c, d, id, []string{target, "tail"}, draws...)
			fact := result.Facts[0]
			if fact.TargetID != "fallen" || result.Consume {
				t.Fatal(id, target, result)
			}
			if stats && (fact.Kind != "stats" || fact.HP != 0 || fact.MaxHP != 200 || fact.MaxMP != 100 || fact.BaseStats.Attack != 100) {
				t.Fatal(fact)
			}
			if !stats && (fact.Kind != "scent" || fact.Flavor != 7) {
				t.Fatal(fact)
			}
		}
	}
	// Existing enemies are inspected directly, without wrong-side fallback.
	for _, id := range []string{"job-9-entry-6", "job-16-entry-1"} {
		s, c, d := ordinaryToolFixture()
		result := executeOrdinaryTool(t, &s, c, d, id, []string{"enemy"})
		if result.Facts[0].TargetID != "enemy" {
			t.Fatal(result)
		}
	}
}

func TestInspectionToolTenCapAndNoLoot(t *testing.T) {
	s, c, d := ordinaryToolFixture()
	c.EnemyIDs = nil
	s.Actors = s.Actors[:1]
	c.PartyIDs = []string{"self"}
	for i := range 12 {
		a := s.Actors[0]
		a.ID, a.TeamID = fmt.Sprintf("chest-%d", i), "enemy"
		s.Actors = append(s.Actors, a)
		c.EnemyIDs = append(c.EnemyIDs, a.ID)
		c.Inspection[a.ID] = corebattle.InspectionContent{Treasure: true, Empty: i == 0, RewardKind: i % 4, DefinitionID: fmt.Sprintf("item-%d", i), Price: 999}
	}
	before := s
	delete(c.Inspection, "chest-10")
	delete(c.Inspection, "chest-11")
	result := executeOrdinaryTool(t, &s, c, d, "tool-151", nil)
	if len(result.Facts) != 10 || !result.Facts[0].Empty || result.Facts[9].TargetID != "chest-9" || !reflect.DeepEqual(s, before) {
		t.Fatal(result, s)
	}
}

func TestInspectionValuationThresholdsAndBranches(t *testing.T) {
	for _, row := range []struct{ price, band int }{{0, 0}, {999, 0}, {1000, 1}, {4999, 1}, {5000, 2}, {9999, 2}, {10000, 3}, {14999, 3}, {15000, 4}, {19999, 4}, {20000, 5}} {
		for _, kind := range []int{0, 1, 2, 3} {
			s, c, d := ordinaryToolFixture()
			c.Inspection["enemy"] = corebattle.InspectionContent{Treasure: true, Empty: row.price == 0, RewardKind: kind, DefinitionID: "item-200", Price: row.price}
			result := executeOrdinaryTool(t, &s, c, d, "job-78-entry-1", []string{"enemy"})
			if fact := result.Facts[0]; fact.Kind != "valuation" || fact.Band != row.band || fact.DefinitionID != "" {
				t.Fatal(row, kind, result)
			}
			// Omitted selector first draws a member and then values its chest.
			result = executeOrdinaryTool(t, &s, c, d, "job-78-entry-1", nil, .4)
			if fact := result.Facts[0]; fact.Kind != "valuation" || fact.Band != row.band {
				t.Fatal(row, result)
			}
		}
	}
	s, c, d := ordinaryToolFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	// Search includes nonchest enemies and preserves first duplicate match.
	for _, target := range []string{"enemy", "other"} {
		c.Inspection[target] = corebattle.InspectionContent{RewardKind: 1, DefinitionID: "weapon-1", Price: 100}
	}
	for _, row := range []struct {
		selector string
		found    bool
		target   string
	}{{"weapon-1", true, "enemy"}, {"invalid", false, ""}, {"enemy", false, ""}} {
		result := executeOrdinaryTool(t, &s, c, d, "job-78-entry-1", []string{row.selector})
		if f := result.Facts[0]; f.Kind != "reward-search" || f.Found != row.found || f.TargetID != row.target {
			t.Fatal(row, f)
		}
	}
	for _, selector := range []string{"self", "fallen", "", "  "} {
		draws := []float64{.5}
		if selector == "" || selector == "  " {
			draws = []float64{.6, .5}
		}
		result := executeOrdinaryTool(t, &s, c, d, "job-78-entry-1", []string{selector}, draws...)
		if f := result.Facts[0]; f.Kind != "character" || f.Flavor != 4 {
			t.Fatal(selector, f)
		}
	}
	// A treasure selector wins over nonparty item search, without any draw.
	c.Inspection["enemy"] = corebattle.InspectionContent{Treasure: true, Empty: true}
	result := executeOrdinaryTool(t, &s, c, d, "job-78-entry-1", []string{"enemy"})
	if result.Facts[0].Kind != "valuation" || result.Facts[0].Band != 0 {
		t.Fatal(result)
	}
}
