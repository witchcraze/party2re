package challenge

import (
	"testing"

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

func spawnRun(t *testing.T, stage string, round int, rng *scriptRNG) *Run {
	t.Helper()
	tiers, err := loadTiers()
	if err != nil {
		t.Fatal(err)
	}
	s := &RunService{cfg: RunConfig{Tiers: tiers, RNG: rng}}
	r := &Run{State: runhost.State{ID: "run", Round: round, Admission: runhost.Admission{StageID: stage}}}
	r.Battle.Actors = []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "hero", HP: 10, TeamID: "allies"}}}
	if err := s.spawn(r, "allies"); err != nil {
		t.Fatal(err)
	}
	if len(rng.ints) != 0 || len(rng.floats) != 0 {
		t.Fatal("draws left over", rng.ints, rng.floats)
	}
	return r
}

// add_monster(0, 1 + 0.1 × round) for a solo party: count draw, then per
// monster a table draw and six stat draws (hp mp at df ag c_mp).
func TestSpawnScalesTheLegacyTable(t *testing.T) {
	stats := []float64{0, 0, 0, 0, 0, 0}
	// Stage 0: one job draw for the wave, two スライム at round 5.
	floats := append(append([]float64{0.5, 0}, stats...), append([]float64{0}, stats...)...)
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
	r = spawnRun(t, "7", 1, &scriptRNG{t: t, floats: append([]float64{0, 13.5 / 14}, stats...)})
	b := r.Actor(r.Enemies[0])
	if b.Name != "ボマーA" || b.Status != "大爆発" || b.Temporary != "するぞ" || b.Tension != 3 || b.Jobs.OldJobID != "job-08" || b.Hit != 95 {
		t.Fatalf("bomber = %+v", b)
	}
	// Stage 5's MP maximum stays at mmp.
	r = spawnRun(t, "5", 1, &scriptRNG{t: t, floats: append([]float64{0, 0}, stats...)})
	if rock := r.Actor(r.Enemies[0]); rock.MaxMP != 9999 || rock.Jobs.OldJobID != "job-94" {
		t.Fatalf("rock = %+v", rock)
	}
}
