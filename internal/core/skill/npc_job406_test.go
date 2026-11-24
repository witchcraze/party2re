package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	battle "github.com/witchcraze/party2re/internal/core/battle"
)

// Enemies sit below the player status-immunity HP so seals and slows can land.
func npc406Fixture(t *testing.T) (battle.LifetimeState, battle.CallbackContext, BattleDefinitions) {
	t.Helper()
	s, op := runNPCFixture(t)
	s.Actors[0].Jobs = battle.BattleJobs{JobID: "job-406", OldJobID: "starter"}
	s.Actors[0].MP = 100
	a := &s.Actors[1]
	a.HP, a.MaxHP, a.Defense, a.Agility = 900, 2000, 80, 40
	a.BaseStats = battle.BattleBaseStats{Attack: 80, Defense: 80, Agility: 40}
	second := a.Clone()
	second.ID, second.MaxHP = "second", 1000
	s.Actors = append(s.Actors, second)
	op.Damage.EnemyIDs = append(slices.Clone(op.Damage.EnemyIDs), second.ID)
	f := battle.CallbackContext{TargetContext: op.Damage.TargetContext, Damage: &op.Damage, Defeat: op.Defeat}
	var d BattleDefinitions
	registerNPCFixture(&d, &s, &f)
	return s, f, d
}

// ff97fc5:party2/lib/_skill.cgi:8247–8283. Every position is SP0; MP alone filters.
func TestNPC406PoolKeepsSourceOrderAndMPFilter(t *testing.T) {
	s, f, d := npc406Fixture(t)
	for _, row := range []struct {
		mp, sp int
		costs  []int
	}{
		{16, 0, nil}, {17, 0, []int{17}}, {22, 0, []int{22, 17}}, {30, 0, []int{22, 17, 30}}, {30, 999, []int{22, 17, 30}},
	} {
		s.Actors[0].MP, s.Actors[0].Jobs.SP = row.mp, row.sp
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		var costs []int
		for _, choice := range choices {
			if choice.ID != "npc-normal-attack" {
				costs = append(costs, choice.MPCost)
			}
		}
		if !slices.Equal(costs, row.costs) {
			t.Fatalf("MP %d SP %d: costs %v, want %v", row.mp, row.sp, costs, row.costs)
		}
	}
	var keys []string
	for _, entry := range d.Invocations.JobTables[406].Variants[0] {
		keys = append(keys, EntrySourceKey(entry.ID, false))
	}
	if want := []string{"7455e17c2f2d1d2a", "9d63295a929d9295", "2fa18e3e9ec996fa"}; !slices.Equal(keys, want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
}

func executeNPC406(t *testing.T, s *battle.LifetimeState, f *battle.CallbackContext, d BattleDefinitions, id string, draws ...float64) {
	t.Helper()
	rng := &damageGemDraws{t: t, draws: draws}
	op := battle.CallbackInvocation{Kind: "entry", EntryID: id}
	if err := s.ApplyInvocations(f, d.Invocations, []battle.CallbackInvocation{op}, rng); err != nil || rng.used != len(draws) {
		t.Fatalf("%s used %d of %d draws, error %v", id, rng.used, len(draws), err)
	}
}

// The 7/10 draw gates the seal, the slow and the one- or two-draw appearance.
// The appearance goes to the first resolved target only: when that pick has
// fallen, the seal and slow re-resolve randomly but no participant changes form.
func TestNPC406SnowfallSealsSlowsAndTransforms(t *testing.T) {
	const snowman = "assets/images/placeholders/battle/snowman.svg#"
	for _, row := range []struct {
		name       string
		fallen     bool
		draws      []float64
		victim     int
		icon       string
		unaffected int
	}{
		{"first form", false, []float64{.69, 0, 0}, 3, snowman + "1", 1},
		{"second form", false, []float64{.69, 0, .5, 0}, 3, snowman + "2", 1},
		{"third form", false, []float64{0, 0, .5, .5}, 3, snowman + "3", 1},
		{"miss", false, []float64{.7}, -1, "", 1},
		{"fallen pick", true, []float64{0, 0, 0, 0, 0}, 1, "", 3},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, f, d := npc406Fixture(t)
			if row.fallen {
				s.Actors[3].HP = 0
			}
			icons := []string{s.Actors[1].Icon, s.Actors[3].Icon}
			f.Arguments = []string{"second"}
			executeNPC406(t, &s, &f, d, "npc-job-406-position-0", row.draws...)
			if row.victim < 0 {
				for _, i := range []int{1, 3} {
					if a := s.Actors[i]; a.Status != "" || a.Agility != 40 || a.Icon != icons[i/3] {
						t.Fatalf("miss changed target %d: %+v", i, a)
					}
				}
				return
			}
			v, u := s.Actors[row.victim], s.Actors[row.unaffected]
			wantIcon := icons[row.victim/3]
			if row.icon != "" {
				wantIcon = row.icon
			}
			if v.Status != battle.StatusDofuu || v.Agility != 32 || v.Icon != wantIcon {
				t.Fatalf("victim status %q, agility %d, icon %q", v.Status, v.Agility, v.Icon)
			}
			if u.Status != "" || u.Agility != 40 || u.Icon != icons[row.unaffected/3] {
				t.Fatalf("other target changed: %+v", u)
			}
		})
	}
}

// _damages(df,'攻'): indirect physical, the second enemy at 0.85 power.
func TestNPC406RollUsesActorDefense(t *testing.T) {
	s, f, d := npc406Fixture(t)
	executeNPC406(t, &s, &f, d, "npc-job-406-position-1", 0, 0)
	if s.Actors[1].HP != 900-14 || s.Actors[3].HP != 900-9 || f.Damage.AreaDamageCalls != 1 {
		t.Fatalf("HP %d/%d, areas %d", s.Actors[1].HP, s.Actors[3].HP, f.Damage.AreaDamageCalls)
	}
}

// Each hit draws its target, then 5+rand(target MaxHP×0.2) of direct physical
// damage, then a 15% seal on the drawn target. A fallen draw leaves the target
// unspecified: rand(0) is rand(1), and damage and seal each draw again.
func TestNPC406BarrageRetargetsEachHit(t *testing.T) {
	s, f, d := npc406Fixture(t)
	executeNPC406(t, &s, &f, d, "npc-job-406-position-2",
		0,             // three hits
		0, .5, 0, .15, // enemy: 205 → 184, seal misses
		.5, 0, 0, .14, // second: 5 → 4, sealed
		.99, .25, 0, .5) // second: 55 → 49
	if s.Actors[1].HP != 716 || s.Actors[1].Status != "" || s.Actors[3].HP != 847 || s.Actors[3].Status != battle.StatusDofuu {
		t.Fatalf("enemy %d/%q, second %d/%q", s.Actors[1].HP, s.Actors[1].Status, s.Actors[3].HP, s.Actors[3].Status)
	}

	s, f, d = npc406Fixture(t)
	s.Actors[3].HP = 0
	executeNPC406(t, &s, &f, d, "npc-job-406-position-2",
		0,
		.5, .5, 0, 0, .5, // fallen pick: 5.5 → enemy 4, seal draws the fallen
		0, 0, 0, .5,
		0, 0, 0, .5)
	if s.Actors[1].HP != 900-12 || s.Actors[3].HP != 0 || s.Actors[1].Status != "" {
		t.Fatalf("enemy %d/%q, second %d", s.Actors[1].HP, s.Actors[1].Status, s.Actors[3].HP)
	}

	s, f, d = npc406Fixture(t)
	draws := []float64{.99} // six hits
	for range 6 {
		draws = append(draws, 0, 0, 0, .5)
	}
	executeNPC406(t, &s, &f, d, "npc-job-406-position-2", draws...)
	if s.Actors[1].HP != 900-6*4 {
		t.Fatalf("six hits left enemy HP %d", s.Actors[1].HP)
	}
}

// The loop stops once the actor falls; that hit's seal follows the reflection
// to the fallen actor and draws nothing.
func TestNPC406BarrageStopsWhenActorFalls(t *testing.T) {
	s, f, d := npc406Fixture(t)
	s.Actors[0].HP = 1
	s.Actors[1].Temporary, s.Actors[3].Temporary = battle.TemporaryReflectAttack, battle.TemporaryReflectAttack
	executeNPC406(t, &s, &f, d, "npc-job-406-position-2", .99, 0, .5, 0)
	if s.Actors[0].HP > 0 || s.Actors[1].HP != 900 || s.Actors[3].HP != 900 || s.Actors[0].Status != "" {
		t.Fatalf("actor %d/%q, enemies %d/%d", s.Actors[0].HP, s.Actors[0].Status, s.Actors[1].HP, s.Actors[3].HP)
	}
}

// A reachable unsupported defeat rejects the damaging bodies before draws or
// mutation; the snowfall cannot defeat, so the same target does not gate it.
func TestNPC406AdmissionCoversReachableDefeat(t *testing.T) {
	for _, id := range []string{"npc-job-406-position-1", "npc-job-406-position-2"} {
		s, f, d := npc406Fixture(t)
		s.Actors[1].HP, s.Actors[1].Gear.AccessoryID = 10, "item-110"
		before, _ := json.Marshal(s)
		op := battle.CallbackInvocation{Kind: "entry", EntryID: id}
		if err := s.ApplyInvocations(&f, d.Invocations, []battle.CallbackInvocation{op}, noArithmeticDraws{t}); !errors.Is(err, battle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if after, _ := json.Marshal(s); string(after) != string(before) || f.Damage.DamageCalls != 0 || f.Damage.AreaDamageCalls != 0 {
			t.Fatal(id, "rejection changed state or counters")
		}
	}
	s, f, d := npc406Fixture(t)
	s.Actors[1].HP, s.Actors[1].Gear.AccessoryID = 10, "item-110"
	f.Arguments = []string{"enemy"}
	executeNPC406(t, &s, &f, d, "npc-job-406-position-0", .7)
}

// Reload rebuilds the stateless definitions; the same draws give the same
// outcome, including the saved appearance.
func TestNPC406ReloadedActorActsIdentically(t *testing.T) {
	s, f, d := npc406Fixture(t)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded battle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	lf, damage := f, *f.Damage
	lf.Damage = &damage
	var ld BattleDefinitions
	registerNPCFixture(&ld, &loaded, &lf)
	for _, run := range []struct {
		s *battle.LifetimeState
		f *battle.CallbackContext
		d BattleDefinitions
	}{{&s, &f, d}, {&loaded, &lf, ld}} {
		run.f.Arguments = []string{"second"}
		executeNPC406(t, run.s, run.f, run.d, "npc-job-406-position-0", 0, 0, .5, .5)
	}
	data, _ = json.Marshal(loaded)
	var again battle.LifetimeState
	if err := json.Unmarshal(data, &again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, loaded) || !reflect.DeepEqual(again, loaded) || loaded.Actors[3].Icon != "assets/images/placeholders/battle/snowman.svg#3" {
		t.Fatal(s.Actors[3], loaded.Actors[3])
	}
}
