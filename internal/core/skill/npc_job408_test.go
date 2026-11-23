package skill

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	battle "github.com/witchcraze/party2re/internal/core/battle"
)

func npc408Fixture(t *testing.T) (battle.LifetimeState, battle.CallbackContext, BattleDefinitions) {
	t.Helper()
	s, op := runNPCFixture(t)
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "job-408", OldJobID: "starter"}
	s.Actors[0].HP, s.Actors[0].MaxHP, s.Actors[0].MP = 500, 1000, 100
	for i := 1; i < len(s.Actors); i++ {
		a := &s.Actors[i]
		a.HP, a.MaxHP = 2000, 2000
		a.BaseStats = battle.BattleBaseStats{Attack: 80, Defense: 60, Agility: 40}
	}
	second := s.Actors[1].Clone()
	second.ID = "second"
	s.Actors = append(s.Actors, second)
	op.Damage.EnemyIDs = append(slices.Clone(op.Damage.EnemyIDs), second.ID)
	f := battle.CallbackContext{TargetContext: op.Damage.TargetContext, Damage: &op.Damage, Defeat: op.Defeat}
	var d BattleDefinitions
	registerNPCFixture(&d, &s, &f)
	return s, f, d
}

// ff97fc5:party2/lib/_skill.cgi:8318–8373. HP changes the bodies but keeps the pool and weights.
func TestNPC408PoolAndAdmission(t *testing.T) {
	s, f, d := npc408Fixture(t)
	for _, row := range []struct {
		hp, sp, mp int
		costs      []int
	}{
		{500, 0, 74, nil},
		{500, 0, 75, []int{75}},
		{499, 0, 87, []int{75, 87}},
		{499, 0, 89, []int{75, 89, 87}},
		{500, 999, 89, []int{75, 89, 87}},
	} {
		s.Actors[0].HP, s.Actors[0].Jobs.SP, s.Actors[0].MP = row.hp, row.sp, row.mp
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		var costs []int
		for _, choice := range choices {
			switch choice.ID {
			case "npc-job-408-position-0", "npc-job-408-position-1", "npc-job-408-position-2":
				costs = append(costs, choice.MPCost)
			}
		}
		if !slices.Equal(costs, row.costs) {
			t.Fatalf("HP/SP/MP %d/%d/%d: costs %v, want %v", row.hp, row.sp, row.mp, costs, row.costs)
		}
	}

	// At MP87, the wave is reachable even if the selected action would be a
	// basic attack. The reset itself is gated even on the no-paralysis outcome.
	s.Actors[0].MP = 87
	s.Actors[1].Gear.AccessoryID = "item-110"
	before, _ := json.Marshal(s)
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), noArithmeticDraws{t}); !errors.Is(err, battle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(after) != string(before) || f.Damage.DamageCalls != 0 || f.Defeat.Cursor != (battle.TargetCursor{}) {
		t.Fatal("admission changed actor or context facts")
	}
	s.Actors[0].MP = 86
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), noArithmeticDraws{t}); err != nil {
		t.Fatal("unreachable reset gated the response", err)
	}
}

func executeNPC408(t *testing.T, s *battle.LifetimeState, f *battle.CallbackContext, d BattleDefinitions, id string, draws ...float64) {
	t.Helper()
	rng := &damageGemDraws{t: t, draws: draws}
	op := battle.CallbackInvocation{Kind: "entry", EntryID: id}
	if err := s.ApplyInvocations(f, d.Invocations, []battle.CallbackInvocation{op}, rng); err != nil || rng.used != len(draws) {
		t.Fatalf("%s used %d of %d draws, error %v", id, rng.used, len(draws), err)
	}
}

func TestNPC408SharesSourceNameWithNPC407Wave(t *testing.T) {
	s, f, d := npc408Fixture(t)
	s.Actors[0].Jobs.OldJobID = "job-407"
	pool, err := d.npcChoicePool(&s, &f, NPCDefaultSeed())
	if err != nil {
		t.Fatal(err)
	}
	var waves []string
	for _, choice := range pool.Choices {
		if choice.Name == "freezing-wave" {
			waves = append(waves, choice.ID)
		}
	}
	if want := []string{"npc-job-407-position-3", "npc-job-407-position-3"}; !slices.Equal(waves, want) {
		t.Fatalf("wave bindings %v, want %v", waves, want)
	}
}

func TestNPC408AuraUsesSourceRecipient(t *testing.T) {
	for _, row := range []struct {
		name        string
		hp          int
		temporary   battle.TemporaryEffect
		draws       []float64
		wantHP      [2]int
		wantMaximum [2]int
	}{
		{"half health hits cover", 500, battle.TemporaryCover, []float64{0}, [2]int{2000, 1685}, [2]int{2000, 1600}},
		{"healthy block prevents reduction", 500, battle.TemporaryOneHitDefense, nil, [2]int{2000, 2000}, [2]int{2000, 2000}},
		{"wounded block still reduces", 499, battle.TemporaryOneHitDefense, []float64{0}, [2]int{2000, 1640}, [2]int{1600, 1600}},
		{"wounded hits do not decay", 499, "", []float64{0, 0}, [2]int{1640, 1640}, [2]int{1600, 1600}},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, f, d := npc408Fixture(t)
			s.Actors[0].HP, s.Actors[1].Temporary, s.Actors[3].Temporary = row.hp, row.temporary, battle.TemporaryCovering
			f.Arguments = []string{s.Actors[1].ID}
			executeNPC408(t, &s, &f, d, "npc-job-408-position-0", row.draws...)
			if hp, maximum := [2]int{s.Actors[1].HP, s.Actors[3].HP}, [2]int{s.Actors[1].MaxHP, s.Actors[3].MaxHP}; hp != row.wantHP || maximum != row.wantMaximum {
				t.Fatalf("HP %v and maximum %v, want %v and %v", hp, maximum, row.wantHP, row.wantMaximum)
			}
		})
	}
}

func TestNPC408BreathCountReflectAndHPBoundary(t *testing.T) {
	for _, row := range []struct {
		hp, hits int
		draw     float64
	}{{500, 2, 0}, {500, 5, .99}, {499, 3, 0}, {499, 6, .99}} {
		s, f, d := npc408Fixture(t)
		s.Actors[0].HP = row.hp
		draws := append([]float64{row.draw}, make([]float64, 2*row.hits)...)
		executeNPC408(t, &s, &f, d, "npc-job-408-position-1", draws...)
		if f.Damage.AreaDamageCalls != row.hits || s.Actors[1].HP != 2000-162*row.hits || s.Actors[3].HP != 2000-137*row.hits {
			t.Fatalf("HP %d draw %v: areas %d, target HP %d/%d", row.hp, row.draw, f.Damage.AreaDamageCalls, s.Actors[1].HP, s.Actors[3].HP)
		}
	}
	s, f, d := npc408Fixture(t)
	s.Actors[0].HP, s.Actors[1].Temporary = 1, battle.TemporaryReflectBreath
	executeNPC408(t, &s, &f, d, "npc-job-408-position-1", .99, 0)
	if s.Actors[0].HP != 0 || s.Actors[3].HP != 2000 || f.Damage.AreaDamageCalls != 1 {
		t.Fatal("breath continued after reflection killed the actor")
	}
}

func TestNPC408BreathRejectsLaterDefeatBeforeDraws(t *testing.T) {
	s, f, d := npc408Fixture(t)
	s.Actors[1].HP, s.Actors[1].Gear.AccessoryID = 700, "item-110"
	before, _ := json.Marshal(s)
	op := battle.CallbackInvocation{Kind: "entry", EntryID: "npc-job-408-position-1"}
	if err := s.ApplyInvocations(&f, d.Invocations, []battle.CallbackInvocation{op}, noArithmeticDraws{t}); !errors.Is(err, battle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(after) != string(before) || f.Damage.AreaDamageCalls != 0 {
		t.Fatal("unsupported defeat changed the response's state or counters")
	}
}

func TestNPC408WaveResetsBeforeEachParalysisDraw(t *testing.T) {
	s, f, d := npc408Fixture(t)
	for _, i := range []int{1, 3} {
		a := &s.Actors[i]
		a.HP, a.Attack, a.Defense, a.Agility = 800, 99, 99, 99
		a.Status, a.Temporary, a.Extended, a.ExtendedTurns = battle.StatusPoison, battle.TemporaryDefense, battle.ExtendedInterception, 2
	}
	executeNPC408(t, &s, &f, d, "npc-job-408-position-2", .5, .49)
	for _, row := range []struct {
		i                   int
		status              battle.StatusEffect
		attack, defense, ag int
	}{{1, "", 80, 60, 40}, {3, battle.StatusParalyze, 40, 30, 20}} {
		a := s.Actors[row.i]
		if a.Status != row.status || a.Attack != row.attack || a.Defense != row.defense || a.Agility != row.ag || a.Temporary != "" || a.Extended != "" {
			t.Fatalf("target %d: status %q, stats %d/%d/%d, temporary %q, extended %q", row.i, a.Status, a.Attack, a.Defense, a.Agility, a.Temporary, a.Extended)
		}
	}
}
