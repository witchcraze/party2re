package challenge

import (
	"context"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// scriptRNG replays draws in call order and fails on an unplanned draw.
type scriptRNG struct {
	t      *testing.T
	ints   []int
	floats []float64
}

func (r *scriptRNG) Intn(int) int {
	if len(r.ints) == 0 {
		r.t.Fatal("unplanned Intn draw")
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	return v
}
func (r *scriptRNG) IntN(n int) int { return r.Intn(n) }
func (r *scriptRNG) Float64() float64 {
	if len(r.floats) == 0 {
		r.t.Fatal("unplanned Float64 draw")
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

type blessingOf string

func (b blessingOf) GetActiveBlessing(context.Context, string) (string, error) { return string(b), nil }

// spawnRun spawns for hero; edit adds members or the taken-treasure flag.
func spawnRun(t *testing.T, stage string, round int, rng *scriptRNG, edit ...func(*Run, *RunConfig)) *Run {
	t.Helper()
	tiers, err := loadTiers()
	if err != nil {
		t.Fatal(err)
	}
	cfg := RunConfig{Tiers: tiers, RNG: rng, Clock: fixedClock(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))}
	r := &Run{State: runhost.State{ID: "run", Round: round, Admission: runhost.Admission{StageID: stage}}}
	r.Battle.Actors = []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "hero", HP: 10, TeamID: "allies"}}}
	for _, e := range edit {
		e(r, &cfg)
	}
	s := &RunService{cfg: cfg}
	if err := s.spawn(context.Background(), r, "hero", "allies"); err != nil {
		t.Fatal(err)
	}
	if len(rng.ints) != 0 || len(rng.floats) != 0 {
		t.Fatal("draws left over", rng.ints, rng.floats)
	}
	return r
}

// add_monster(0, 1 + 0.1 × round) for a solo party: the threshold draw, a
// count draw, then per monster a table draw and six stat draws (hp mp at df ag
// c_mp). Below the threshold no treasure roll is drawn.
func TestSpawnScalesTheLegacyTable(t *testing.T) {
	stats := []float64{0, 0, 0, 0, 0, 0}
	// Stage 0: one job draw for the wave, two スライム at round 5.
	floats := append(append([]float64{0, 0.5, 0}, stats...), append([]float64{0}, stats...)...)
	r := spawnRun(t, "0", 5, &scriptRNG{t: t, ints: []int{6}, floats: floats})
	if len(r.Enemies) != 2 {
		t.Fatal(r.Enemies)
	}
	for i, id := range r.Enemies {
		a := r.Actor(id)
		// int(150 × (0.9 + 0 − 0.1) × 1.5) = 180 HP; 50 MP → 60.
		if a.Jobs.JobID != "job-06" || a.HP != 180 || a.MaxHP != 180 || a.MP != 60 || a.Attack != 120 || a.Agility != 240 || a.ExperienceReward != 20 || !a.NPC || a.TeamID != "enemies" || a.Name != "スライム"+string(rune('A'+i)) {
			t.Fatalf("enemy %d = %+v", i, a)
		}
	}

	// Stage 7 at round 1, table position 13: the ボマー keeps its payload.
	r = spawnRun(t, "7", 1, &scriptRNG{t: t, floats: append([]float64{0, 0, 13.5 / 14}, stats...)})
	b := r.Actor(r.Enemies[0])
	if b.Name != "ボマーA" || b.Status != "大爆発" || b.Temporary != "するぞ" || b.Tension != 3 || b.Jobs.OldJobID != "job-08" || b.Hit != 95 {
		t.Fatalf("bomber = %+v", b)
	}
	// Stage 5's MP maximum stays at mmp.
	r = spawnRun(t, "5", 1, &scriptRNG{t: t, floats: append([]float64{0, 0, 0}, stats...)})
	if rock := r.Actor(r.Enemies[0]); rock.MaxMP != 9999 || rock.Jobs.OldJobID != "job-94" {
		t.Fatalf("rock = %+v", rock)
	}
}

// set_monster's treasure branch (vs_challenge.cgi:64–66) on stage 7, whose
// threshold is int(rand(21) + 30): the roll is drawn only at or above the
// threshold and while the advancing actor holds no taken-treasure flag.
func TestSpawnTreasureRoom(t *testing.T) {
	stats := []float64{0, 0, 0, 0, 0, 0}
	monsters := func(prefix ...float64) []float64 { return append(append(prefix, 0, 0), stats...) }
	for _, tc := range []struct {
		name   string
		round  int
		floats []float64
		edit   func(*Run, *RunConfig)
	}{
		{name: "below threshold", round: 29, floats: monsters(0)},
		{name: "roll misses", round: 30, floats: monsters(0, 0.1)},
		{name: "highest threshold not reached", round: 49, floats: monsters(20.5 / 21)},
		{name: "taken-treasure flag", round: 30, floats: monsters(0), edit: func(r *Run, _ *RunConfig) {
			r.Recipients = map[string]runhost.Recipient{"hero": {Inspected: 1}}
		}},
	} {
		var edits []func(*Run, *RunConfig)
		if tc.edit != nil {
			edits = append(edits, tc.edit)
		}
		if r := spawnRun(t, "7", tc.round, &scriptRNG{t: t, floats: tc.floats}, edits...); len(r.Enemies) != 1 || len(r.Boxes) != 0 {
			t.Errorf("%s: enemies %v boxes %v", tc.name, r.Enemies, r.Boxes)
		}
	}

	// A hit at the highest threshold: one box per member, Monday's orb joins
	// the tool table and the empty weapon table falls back to tools.
	r := spawnRun(t, "7", 50, &scriptRNG{t: t, floats: []float64{20.5 / 21, 0.0999}, ints: []int{1, 0, 0}})
	if len(r.Enemies) != 0 || len(r.Boxes) != 1 || r.Boxes[0].Name != "大きい宝箱A" || r.Boxes[0].ItemID != "item-005" {
		t.Fatalf("treasure = %+v enemies %v", r.Boxes, r.Enemies)
	}

	// $#partys counts the fallen; each fallen member removes a box and the
	// blessing adds one on a 1/5 roll: (3 − 1) + 1 − 1 → boxes 0..2.
	party := func(r *Run, cfg *RunConfig) {
		cfg.Blessings = blessingOf("DROP")
		r.Battle.Actors = append(r.Battle.Actors,
			corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "ally", HP: 1, TeamID: "allies"}},
			corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "fallen", TeamID: "allies"}})
	}
	r = spawnRun(t, "0", 10, &scriptRNG{t: t, floats: []float64{0, 0}, ints: []int{6, 0, 0, 3, 0, 0, 3, 0, 0, 3, 0}}, party)
	if len(r.Boxes) != 3 || r.Boxes[2].ID != "box-3" || r.Boxes[2].Name != "普通の宝箱C" {
		t.Fatalf("party treasure = %+v", r.Boxes)
	}
}
