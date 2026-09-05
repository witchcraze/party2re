package testutil

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ConcurrencyStressConfig defines configuration for concurrent stress execution.
type ConcurrencyStressConfig struct {
	Workers      int
	OpsPerWorker int
}

// GetStressConfig returns the default stress configuration based on the PARTY2_STRESS_ENABLED env variable.
// When PARTY2_STRESS_ENABLED == "1", it returns 50 workers and 20 ops per worker.
// Otherwise, it returns 15 workers and 10 ops per worker for fast execution during make check and CI.
func GetStressConfig() ConcurrencyStressConfig {
	if os.Getenv("PARTY2_STRESS_ENABLED") == "1" {
		return ConcurrencyStressConfig{Workers: 50, OpsPerWorker: 20}
	}
	return ConcurrencyStressConfig{Workers: 15, OpsPerWorker: 10}
}

// ConcurrencyStressResult tracks aggregated metrics from a concurrent stress test run.
type ConcurrencyStressResult struct {
	TotalOps  int64
	Successes int64
	Failures  int64
	Deadlocks int64
	Duration  time.Duration
}

// IsDeadlockError returns true if the given error indicates a database deadlock or lock wait timeout.
func IsDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "1213") ||
		strings.Contains(errStr, "deadlock") ||
		strings.Contains(errStr, "1205") ||
		strings.Contains(errStr, "lock wait timeout")
}

// RunConcurrentStressTest runs an operation across multiple concurrent worker goroutines with barrier start.
// It tracks successes, failures, and deadlocks. If any deadlock is encountered, it fails the test via t.Fatalf.
func RunConcurrentStressTest(
	t *testing.T,
	cfg ConcurrencyStressConfig,
	workerFn func(workerID int, op int) error,
) ConcurrencyStressResult {
	t.Helper()

	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.OpsPerWorker <= 0 {
		cfg.OpsPerWorker = 1
	}

	var res ConcurrencyStressResult
	res.TotalOps = int64(cfg.Workers * cfg.OpsPerWorker)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(cfg.Workers)

	for w := 0; w < cfg.Workers; w++ {
		go func(workerID int) {
			defer done.Done()
			start.Wait()

			for op := 0; op < cfg.OpsPerWorker; op++ {
				err := workerFn(workerID, op)
				if err != nil {
					if IsDeadlockError(err) {
						atomic.AddInt64(&res.Deadlocks, 1)
					} else {
						atomic.AddInt64(&res.Failures, 1)
					}
				} else {
					atomic.AddInt64(&res.Successes, 1)
				}
			}
		}(w)
	}

	startTime := time.Now()
	start.Done()
	done.Wait()
	res.Duration = time.Since(startTime)

	if res.Deadlocks > 0 {
		t.Fatalf("Deadlock detected during concurrent stress test: count = %d", res.Deadlocks)
	}

	return res
}

// RunRace runs N operations concurrently with a synchronized starting barrier.
// It releases all goroutines simultaneously to maximize contention.
func RunRace(fns ...func() error) []error {
	n := len(fns)
	if n == 0 {
		return nil
	}
	errs := make([]error, n)
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(n)

	for i, fn := range fns {
		go func(idx int, target func() error) {
			defer done.Done()
			start.Wait()
			if target != nil {
				errs[idx] = target()
			}
		}(i, fn)
	}

	start.Done()
	done.Wait()
	return errs
}

// RunRace2 runs exactly 2 operations concurrently with a synchronized starting barrier.
func RunRace2(fn1 func() error, fn2 func() error) (error, error) {
	errs := RunRace(fn1, fn2)
	return errs[0], errs[1]
}
