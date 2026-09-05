package testutil

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetStressConfig(t *testing.T) {
	orig := os.Getenv("PARTY2_STRESS_ENABLED")
	defer os.Setenv("PARTY2_STRESS_ENABLED", orig)

	os.Setenv("PARTY2_STRESS_ENABLED", "")
	cfgDefault := GetStressConfig()
	if cfgDefault.Workers != 15 || cfgDefault.OpsPerWorker != 10 {
		t.Errorf("expected default 15 workers, 10 ops; got %+v", cfgDefault)
	}

	os.Setenv("PARTY2_STRESS_ENABLED", "1")
	cfgStress := GetStressConfig()
	if cfgStress.Workers != 50 || cfgStress.OpsPerWorker != 20 {
		t.Errorf("expected stress 50 workers, 20 ops; got %+v", cfgStress)
	}
}

func TestIsDeadlockError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"standard deadline exceeded", context.DeadlineExceeded, true},
		{"mariadb 1213 deadlock", errors.New("Error 1213 (40001): Deadlock found when trying to get lock"), true},
		{"mariadb 1205 lock wait timeout", errors.New("Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction"), true},
		{"generic deadlock text", errors.New("detected deadlock in transaction"), true},
		{"unrelated error", errors.New("item not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := IsDeadlockError(tt.err)
			if actual != tt.expected {
				t.Errorf("IsDeadlockError(%v) = %v, want %v", tt.err, actual, tt.expected)
			}
		})
	}
}

func TestRunConcurrentStressTest(t *testing.T) {
	cfg := ConcurrencyStressConfig{
		Workers:      5,
		OpsPerWorker: 8,
	}

	var executionCount int64
	var workerSet atomic.Int64

	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		atomic.AddInt64(&executionCount, 1)
		workerSet.Add(1)
		if op == 0 && workerID == 0 {
			// Simulate 1 domain error
			return errors.New("expected domain error")
		}
		return nil
	})

	expectedTotal := int64(40)
	if res.TotalOps != expectedTotal {
		t.Errorf("expected TotalOps %d, got %d", expectedTotal, res.TotalOps)
	}
	if executionCount != expectedTotal {
		t.Errorf("expected executionCount %d, got %d", expectedTotal, executionCount)
	}
	if res.Successes != 39 {
		t.Errorf("expected 39 successes, got %d", res.Successes)
	}
	if res.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", res.Failures)
	}
	if res.Deadlocks != 0 {
		t.Errorf("expected 0 deadlocks, got %d", res.Deadlocks)
	}
	if res.Duration <= 0 {
		t.Errorf("expected non-zero duration, got %v", res.Duration)
	}
}

func TestRunRace2(t *testing.T) {
	var val1, val2 int64

	err1, err2 := RunRace2(
		func() error {
			atomic.StoreInt64(&val1, 42)
			time.Sleep(1 * time.Millisecond)
			return nil
		},
		func() error {
			atomic.StoreInt64(&val2, 99)
			return errors.New("race error")
		},
	)

	if err1 != nil {
		t.Errorf("expected err1 to be nil, got %v", err1)
	}
	if err2 == nil || err2.Error() != "race error" {
		t.Errorf("expected err2 to be 'race error', got %v", err2)
	}
	if atomic.LoadInt64(&val1) != 42 || atomic.LoadInt64(&val2) != 99 {
		t.Errorf("expected both functions to execute: val1=%d, val2=%d", val1, val2)
	}
}

func TestRunRaceEmpty(t *testing.T) {
	errs := RunRace()
	if errs != nil {
		t.Errorf("expected nil slice for empty RunRace, got %v", errs)
	}
}
