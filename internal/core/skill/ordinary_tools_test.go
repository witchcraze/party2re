package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func ordinaryToolFixture() (corebattle.LifetimeState, *corebattle.OrdinaryToolContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	maxHP, level := 100, 99
	c := &corebattle.OrdinaryToolContext{DamageContext: base.DamageContext,
		EquipmentReset:    &corebattle.EquipmentResetContext{},
		OrdinaryToolInput: corebattle.OrdinaryToolInput{RequesterMaxHP: &maxHP, Inspection: map[string]corebattle.InspectionContent{}}}
	c.RequesterLevel = &level
	for i := range s.Actors {
		a := &s.Actors[i]
		a.MaxMP = 100
		a.BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 40}
		c.Inspection[a.ID] = corebattle.InspectionContent{Empty: true}
	}
	var d BattleDefinitions
	d.RegisterOrdinaryTools(c)
	return s, c, d
}

func executeOrdinaryTool(t *testing.T, s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.OrdinaryToolResult {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) || !reflect.DeepEqual(f.Arguments, args) {
		t.Fatal(id, rng.used, len(draws), f.Arguments, args)
	}
	return c.Results[len(c.Results)-1]
}

func TestOrdinaryToolRegistration(t *testing.T) {
	s, base, _ := dualAgilityFixture()
	c := &corebattle.OrdinaryToolContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterOrdinaryTools(c)
	entries := corebattle.OrdinaryToolEntries()
	if len(entries) != 31 {
		t.Fatal(len(entries))
	}
	for _, entry := range entries {
		if entry.UsageCategory != 1 || entry.ID == "" || entry.DefinitionID == "" {
			t.Fatal(entry)
		}
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&s, &f, []string{"tool-130"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if len(c.Results) != 1 || !c.Results[0].Consume || c.Results[0].ID != "tool-130" || s.Actors[0].MP != 80 {
		t.Fatal(c.Results, s.Actors)
	}
}

// ff97fc5:_data.cgi:623–627: tool-category _heal/_heals, not flat healing
// and not the separate __heal primitive. Every entry has an intrinsic vector.
func TestOrdinaryToolHealing(t *testing.T) {
	for _, row := range []struct{ source, power int }{{1, 40}, {2, 100}, {3, 250}, {4, 200}, {5, 999}} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/npc-%t", row.source, npc), func(t *testing.T) {
				s, c, d := ordinaryToolFixture()
				s.Actors[0].HP, s.Actors[0].Tension = 10, 3
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				draws, calls, healing := []float64{0}, 1, int(float64(row.power)*.9*3)
				if row.source >= 4 {
					// The fallen party position falls back to the actor, using
					// already consumed tension for its second shared heal.
					draws, calls = []float64{0, 0}, 2
					healing += int(float64(row.power) * .9)
				}
				executeOrdinaryTool(t, &s, c, d, fmt.Sprintf("tool-%d", row.source), nil, draws...)
				if s.Actors[0].HP != min(1000, 10+healing) || s.Actors[0].Tension != 1 || s.Actors[2].HP != 0 || c.HealCalls != calls {
					t.Fatal(s.Actors, c)
				}
				if want := row.source >= 4; (c.PartyHealCalls == 1) != want {
					t.Fatal(c.PartyHealCalls)
				}
			})
		}
	}
	for _, row := range []struct {
		name  string
		setup func(*corebattle.LifetimeState, *corebattle.OrdinaryToolContext)
		want  int
	}{
		{"items", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[0].ItemDefinitionIDs = []string{"item-146", "item-256"}
		}, 77},
		{"virulent", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[0].Status = corebattle.StatusVirulentPoison
		}, 3},
		{"double", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[0].Temporary = corebattle.TemporaryDouble
		}, 72},
		{"tool ignores magic field", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { s.Fields.Default = "sanctuary" }, 36},
		{"cap", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { s.Actors[0].HP = 999 }, 1},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := ordinaryToolFixture()
			s.Actors[0].HP = 10
			row.setup(&s, c)
			before := s.Actors[0].HP
			executeOrdinaryTool(t, &s, c, d, "tool-1", nil, 0)
			if s.Actors[0].HP-before != row.want {
				t.Fatal(s.Actors[0], before, row.want)
			}
		})
	}
}

func TestOrdinaryToolCuresAndTemporaries(t *testing.T) {
	for _, row := range []struct{ id, status string }{
		{"tool-7", corebattle.StatusDeadlyPoison}, {"tool-7", corebattle.StatusPoison}, {"tool-7", corebattle.StatusVirulentPoison},
		{"tool-8", corebattle.StatusParalyze}, {"tool-9", corebattle.StatusConfusion}, {"tool-10", corebattle.StatusMagicSeal},
	} {
		s, c, d := ordinaryToolFixture()
		s.Actors[0].Status = row.status
		// A second living ally keeps confusion's two successive cure targets stable.
		ally := s.Actors[0]
		ally.ID = "ally"
		s.Actors = append(s.Actors, ally)
		c.PartyIDs = append(c.PartyIDs, "ally")
		draws := []float64{}
		if row.status == corebattle.StatusConfusion {
			draws = []float64{.99}
		}
		executeOrdinaryTool(t, &s, c, d, row.id, []string{"ally"}, draws...)
		if s.Actors[3].Status != "" || c.CureCalls != 1 {
			t.Fatal(row, s.Actors, c.CureCalls)
		}
	}
	for _, row := range []struct {
		id  string
		tmp corebattle.TemporaryEffect
	}{
		{"tool-14", corebattle.TemporaryReduceAttack}, {"tool-15", corebattle.TemporaryReflectMagic},
		{"tool-80", corebattle.TemporaryReduceMagic}, {"tool-81", corebattle.TemporaryAbsorbMagic},
		{"tool-82", corebattle.TemporaryDouble}, {"tool-102", corebattle.TemporaryRegeneration}, {"tool-103", corebattle.TemporaryRevival},
	} {
		for _, dead := range []bool{false, true} {
			s, c, d := ordinaryToolFixture()
			target := "self"
			idx := 0
			if row.id == "tool-82" {
				target, idx = "enemy", 1
			}
			draws := []float64{}
			if dead {
				s.Actors[idx].HP = 0
				// Confusion allows fallen targets through the target helper.
				s.Actors[0].Status = corebattle.StatusConfusion
				if idx == 0 {
					draws = []float64{0}
				} else {
					draws = []float64{.4, .4}
				}
			}
			executeOrdinaryTool(t, &s, c, d, row.id, []string{target}, draws...)
			want := row.tmp
			if dead && (row.id == "tool-80" || row.id == "tool-81") {
				want = ""
			}
			if s.Actors[idx].Temporary != want || s.Actors[idx].Defense != 0 || s.Actors[idx].HP != map[bool]int{false: 1000, true: 0}[dead] {
				t.Fatal(row, dead, s.Actors)
			}
		}
	}
}

func TestOrdinaryToolFixedDamageAndStatTails(t *testing.T) {
	for _, row := range []struct {
		id     string
		damage int
	}{{"tool-41", 72}, {"tool-42", 54}} {
		s, c, d := ordinaryToolFixture()
		s.Actors[0].Attack = 900
		executeOrdinaryTool(t, &s, c, d, row.id, []string{"enemy"}, 0)
		if s.Actors[1].HP != 1000-row.damage || c.DamageCalls != 1 {
			t.Fatal(s.Actors, c)
		}
	}
	s, c, d := ordinaryToolFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeOrdinaryTool(t, &s, c, d, "tool-42", nil, 0, 0)
	if s.Actors[1].HP != 946 || s.Actors[3].HP != 955 || c.AreaDamageCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	for _, row := range []struct {
		defense int
		binding bool
		want    int
	}{{0, false, 24}, {200, false, 200}, {0, true, 0}} {
		s, c, d := ordinaryToolFixture()
		s.Actors[0].Defense = row.defense
		if row.binding {
			s.Actors[0].Status = corebattle.StatusSabaku
		}
		executeOrdinaryTool(t, &s, c, d, "tool-79", nil)
		if s.Actors[0].Defense != row.want || s.Actors[0].Temporary != corebattle.TemporaryReduceBreath {
			t.Fatal(s.Actors[0])
		}
	}
	s, c, d = ordinaryToolFixture()
	executeOrdinaryTool(t, &s, c, d, "tool-76", nil)
	if s.Actors[0].Attack != 200 {
		t.Fatal(s.Actors[0])
	}
	// The DF helper's item205 tension/temporary runs before the breath overwrite.
	s, c, d = ordinaryToolFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
	executeOrdinaryTool(t, &s, c, d, "tool-79", nil)
	if s.Actors[0].Attack != 124 || s.Actors[0].Tension != 1.7 || s.Actors[0].Temporary != corebattle.TemporaryReduceBreath {
		t.Fatal(s.Actors[0])
	}
}

func TestOrdinaryToolAmbientCursor(t *testing.T) {
	for _, id := range []string{"tool-75", "tool-77", "tool-86"} {
		for _, cursor := range []string{"", "self", "other"} {
			s, c, d := ordinaryToolFixture()
			other := s.Actors[1]
			other.ID = "other"
			if id == "tool-86" {
				other.TeamID = "party"
				c.PartyIDs = append(c.PartyIDs, "other")
			} else {
				c.EnemyIDs = append(c.EnemyIDs, "other")
			}
			s.Actors = append(s.Actors, other)
			s.Actors[0].Tension, s.Actors[1].Tension, s.Actors[3].Tension = 3, 3, 3
			if cursor != "" {
				c.Cursor = corebattle.TargetCursor{Source: "item150", ActorID: cursor}
			}
			draws := []float64{}
			idx := 3
			if id == "tool-86" {
				if cursor == "" {
					draws = []float64{0}
					idx = 0
				} else if cursor == "self" {
					idx = 0
				}
			} else if cursor != "other" {
				draws = []float64{.99}
			}
			executeOrdinaryTool(t, &s, c, d, id, []string{"enemy"}, draws...)
			want := 5.0
			if id == "tool-75" {
				want = 1
			}
			if s.Actors[idx].Tension != want || s.Actors[1].Tension != 3 || !reflect.DeepEqual(c.Arguments, []string{"enemy"}) {
				t.Fatal(id, cursor, s.Actors, c.Arguments)
			}
		}
	}
}

func TestOrdinaryToolResetBeforeRefill(t *testing.T) {
	s, c, d := ordinaryToolFixture()
	s.Actors[0].HP = 10
	s.Actors[0].Gear.AccessoryID, s.Actors[0].ItemDefinitionIDs = "item-112", []string{"item-112"}
	s.Actors[2].MaxHP = 200
	for i := range s.Actors {
		a := &s.Actors[i]
		a.Temporary, a.Tension, a.Status, a.Attack = corebattle.TemporaryDouble, 5, corebattle.StatusPoison, 999
	}
	executeOrdinaryTool(t, &s, c, d, "tool-87", nil, 0)
	for _, a := range s.Actors {
		if a.HP != a.MaxHP || a.MP != a.MaxMP || a.Temporary != "" || a.Status != "" || a.Tension != 1 || a.Attack != a.BaseStats.Attack {
			t.Fatal(a)
		}
	}
	if s.Actors[2].HP != 200 {
		t.Fatal(s.Actors[2])
	}
}

func TestOrdinaryToolCaptureStrictRequesterThresholds(t *testing.T) {
	for _, row := range []struct {
		requester, hp, maxHP int
		capture              bool
	}{{100, 100, 500, false}, {101, 100, 500, true}, {99, 99, 500, true}, {1, 100, 500, false}, {1, 99, 500, true}, {100, 101, 500, false}} {
		s, c, d := ordinaryToolFixture()
		c.RequesterMaxHP = &row.requester
		s.Actors[0].MaxHP = 9999
		s.Actors[1].HP, s.Actors[1].MaxHP = row.hp, row.maxHP
		executeOrdinaryTool(t, &s, c, d, "tool-213", []string{"enemy"})
		if (s.Actors[1].Status == "capture") != row.capture {
			t.Fatal(row, s.Actors)
		}
	}
}
