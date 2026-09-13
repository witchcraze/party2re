package random_test

import (
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDefaultGenerator(t *testing.T) {
	gen := random.Default()
	for i := 0; i < 100; i++ {
		val := gen.Intn(10)
		if val < 0 || val >= 10 {
			t.Fatalf("Intn(10) returned out of bounds: %d", val)
		}
		valN := gen.IntN(10)
		if valN < 0 || valN >= 10 {
			t.Fatalf("IntN(10) returned out of bounds: %d", valN)
		}
		f := gen.Float64()
		if f < 0.0 || f >= 1.0 {
			t.Fatalf("Float64() returned out of bounds: %f", f)
		}
	}

	// Package-level helpers
	val := random.Intn(5)
	if val < 0 || val >= 5 {
		t.Fatalf("random.Intn(5) out of bounds: %d", val)
	}
	valN := random.IntN(5)
	if valN < 0 || valN >= 5 {
		t.Fatalf("random.IntN(5) out of bounds: %d", valN)
	}
	f := random.Float64()
	if f < 0.0 || f >= 1.0 {
		t.Fatalf("random.Float64() out of bounds: %f", f)
	}
}

func TestDeterministicGenerator_Reproducibility(t *testing.T) {
	gen1 := random.NewDeterministic(12345)
	gen2 := random.NewDeterministic(12345)

	for i := 0; i < 50; i++ {
		v1 := gen1.Intn(100)
		v2 := gen2.Intn(100)
		if v1 != v2 {
			t.Fatalf("step %d: gen1=%d != gen2=%d for same seed", i, v1, v2)
		}

		f1 := gen1.Float64()
		f2 := gen2.Float64()
		if f1 != f2 {
			t.Fatalf("step %d: gen1 float=%f != gen2 float=%f for same seed", i, f1, f2)
		}
	}
}

func TestDeterministicGenerator_DifferentSeeds(t *testing.T) {
	gen1 := random.NewDeterministic(111)
	gen2 := random.NewDeterministic(999)

	identical := true
	for i := 0; i < 20; i++ {
		if gen1.Intn(1000000) != gen2.Intn(1000000) {
			identical = false
			break
		}
	}
	if identical {
		t.Fatal("expected different seeds to produce different sequences")
	}
}

func TestConcurrency(t *testing.T) {
	gen := random.NewDeterministic(42)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = gen.Intn(100)
				_ = gen.Float64()
				_ = random.Intn(100)
				_ = random.Float64()
			}
		}()
	}
	wg.Wait()
}
