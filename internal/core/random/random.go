package random

import (
	"math/rand/v2"
	"sync"
)

// Generator defines random number generation operations.
type Generator interface {
	Intn(n int) int
	IntN(n int) int
	Float64() float64
}

// defaultGenerator delegates directly to concurrency-safe math/rand/v2 functions.
type defaultGenerator struct{}

func (defaultGenerator) Intn(n int) int {
	return rand.IntN(n)
}

func (defaultGenerator) IntN(n int) int {
	return rand.IntN(n)
}

func (defaultGenerator) Float64() float64 {
	return rand.Float64()
}

// Default returns the default global concurrency-safe random generator.
func Default() Generator {
	return defaultGenerator{}
}

// Intn returns a non-negative pseudo-random number in the half-open interval [0,n) using the default generator.
func Intn(n int) int {
	return rand.IntN(n)
}

// IntN returns a non-negative pseudo-random number in the half-open interval [0,n) using the default generator.
func IntN(n int) int {
	return rand.IntN(n)
}

// Float64 returns a pseudo-random number in the half-open interval [0.0,1.0) using the default generator.
func Float64() float64 {
	return rand.Float64()
}

// deterministicGenerator provides reproducible pseudo-random numbers backed by a mutex-guarded PCG source.
type deterministicGenerator struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewDeterministic creates a thread-safe deterministic Generator seeded with the given value.
func NewDeterministic(seed uint64) Generator {
	return &deterministicGenerator{
		rng: rand.New(rand.NewPCG(seed, seed^0x5DEECE66D)),
	}
}

func (g *deterministicGenerator) Intn(n int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rng.IntN(n)
}

func (g *deterministicGenerator) IntN(n int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rng.IntN(n)
}

func (g *deterministicGenerator) Float64() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rng.Float64()
}
