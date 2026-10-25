package testutil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
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

func TestRunConcurrentStressTest_WorkerFailureSubprocess(t *testing.T) {
	if os.Getenv("TESTUTIL_SUBPROCESS") == "failure" {
		cfg := ConcurrencyStressConfig{Workers: 2, OpsPerWorker: 2}
		res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
			if workerID == 1 && op == 0 {
				return errors.New("sentinel worker failure")
			}
			return nil
		})
		if res.Failures > 0 {
			t.Fatalf("caller observed failure: %d", res.Failures)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunConcurrentStressTest_WorkerFailureSubprocess$")
	cmd.Env = append(os.Environ(), "TESTUTIL_SUBPROCESS=failure")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected subprocess to fail, got success")
	}
	output := string(out)
	if !strings.Contains(output, "concurrent stress worker failure sample (worker=1, op=0): sentinel worker failure") {
		t.Fatalf("expected failure sample log in output, got:\n%s", output)
	}
}

func TestRunConcurrentStressTest_MultipleFailuresBoundedToOneSample(t *testing.T) {
	if os.Getenv("TESTUTIL_SUBPROCESS") == "multi_failure" {
		cfg := ConcurrencyStressConfig{Workers: 4, OpsPerWorker: 4}
		res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
			return errors.New("recurring failure")
		})
		if res.Failures > 0 {
			t.Fatalf("caller observed failure: %d", res.Failures)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunConcurrentStressTest_MultipleFailuresBoundedToOneSample$")
	cmd.Env = append(os.Environ(), "TESTUTIL_SUBPROCESS=multi_failure")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected subprocess to fail, got success")
	}
	count := strings.Count(string(out), "concurrent stress worker failure sample")
	if count != 1 {
		t.Fatalf("expected exactly 1 failure sample logged, got %d; output:\n%s", count, string(out))
	}
}

func TestRunConcurrentStressTest_DeadlockLoggingSubprocess(t *testing.T) {
	if os.Getenv("TESTUTIL_SUBPROCESS") == "deadlock" {
		cfg := ConcurrencyStressConfig{Workers: 4, OpsPerWorker: 4}
		RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
			return errors.New("1213: Deadlock found when trying to get lock")
		})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunConcurrentStressTest_DeadlockLoggingSubprocess$")
	cmd.Env = append(os.Environ(), "TESTUTIL_SUBPROCESS=deadlock")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected subprocess to fail due to deadlock fatal, got success")
	}
	output := string(out)
	if !strings.Contains(output, "concurrent stress deadlock sample") || !strings.Contains(output, "Deadlock found when trying to get lock") {
		t.Fatalf("expected deadlock sample log in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Deadlock detected during concurrent stress test: count = 16") {
		t.Fatalf("expected deadlock fatal summary in output, got:\n%s", output)
	}
	count := strings.Count(output, "concurrent stress deadlock sample")
	if count != 1 {
		t.Fatalf("expected exactly 1 deadlock sample logged, got %d; output:\n%s", count, output)
	}
}

func TestRunConcurrentStressTest_MixedFailureAndDeadlockSamples(t *testing.T) {
	if os.Getenv("TESTUTIL_SUBPROCESS") == "mixed" {
		cfg := ConcurrencyStressConfig{Workers: 2, OpsPerWorker: 2}
		RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
			if workerID == 0 {
				return errors.New("domain validation error")
			}
			return errors.New("1213: Deadlock found")
		})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunConcurrentStressTest_MixedFailureAndDeadlockSamples$")
	cmd.Env = append(os.Environ(), "TESTUTIL_SUBPROCESS=mixed")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected subprocess to fail due to deadlock fatal, got success")
	}
	output := string(out)
	if strings.Count(output, "concurrent stress worker failure sample") != 1 {
		t.Fatalf("expected exactly 1 failure sample, output:\n%s", output)
	}
	if strings.Count(output, "concurrent stress deadlock sample") != 1 {
		t.Fatalf("expected exactly 1 deadlock sample, output:\n%s", output)
	}
}

func TestRunConcurrentStressTest_RaceAndConsistency(t *testing.T) {
	cfg := ConcurrencyStressConfig{Workers: 20, OpsPerWorker: 20}
	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		if (workerID+op)%3 == 0 {
			return errors.New("some regular error")
		}
		return nil
	})
	expectedTotal := int64(400)
	if res.TotalOps != expectedTotal {
		t.Fatalf("expected TotalOps %d, got %d", expectedTotal, res.TotalOps)
	}
	if res.Successes+res.Failures != expectedTotal {
		t.Fatalf("expected successes (%d) + failures (%d) == total (%d)", res.Successes, res.Failures, expectedTotal)
	}
	if res.Deadlocks != 0 {
		t.Fatalf("expected 0 deadlocks, got %d", res.Deadlocks)
	}
}
