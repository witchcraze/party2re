# Continuous Performance Verification & Benchmarking

This document defines the continuous performance verification framework and test execution time budget policies for the Party2 project.

---

## 1. Context & Motivation

In shared, virtualized, containerized, or CPU-throttled continuous integration (CI) environments (such as GitHub Actions runners or containerized smoke builds), wall-clock execution times can fluctuate substantially due to CPU scheduling and noisy neighbors.

Historically, functional unit tests asserting strict sub-second execution thresholds (e.g., `elapsed > 200ms` or `elapsed > 1s` in AST linters) suffered from intermittent flakiness, failing CI builds even though all architectural, safety, and business logic invariants passed.

To eliminate test flakiness while preventing silent performance degradation as the codebase scales to Version 1.0, Party2 explicitly separates:
1. **Functional Safety Verification (`go test`)**: asserts correctness and uses liberal safety bounds strictly to catch deadlocks or infinite loops.
2. **Continuous Performance Verification (`go test -bench`, `make bench`)**: tracks latency, throughput, and memory allocations using Go standard benchmarks and baseline regression detection.

---

## 2. Test Time Budget Policy

Codified in [`.agents/rules/01-development-workflow.md`](file:///home/witchcraze/dev/party2re/.agents/rules/01-development-workflow.md):

1. **No Sub-Second Assertions in Unit/Integration Tests**:
   - Functional tests (`go test`) MUST NOT enforce strict sub-second wall-clock thresholds.
   - Informational timing measurements should be emitted using `t.Logf` rather than failing tests via `t.Errorf`.
2. **Liberal Dead-Man Bounds**:
   - When a test requires a timeout check to prevent infinite loops, hangs, or deadlocks in AST traversal or network workers, liberal safety bounds (e.g., 2.0s–5.0s) MUST be used.
   - Failures must explicitly indicate a potential deadlock or infinite loop rather than an ordinary performance violation.
3. **Performance Tracking via Benchmarks**:
   - All CPU-sensitive and critical-path operations must be benchmarked using Go standard `testing.B` (`Benchmark*`) functions.

---

## 3. Dedicated Benchmark Suites

The continuous performance verification framework currently covers the following critical domains:

### 3.1. AST Static Analysis Linters
- **`BenchmarkLockHierarchyLinter`** (`internal/database`):
  - Scans all internal Go files to verify pessimistic row-lock acquisition hierarchy order.
- **`BenchmarkLockEvaluationStatement`** (`internal/database`):
  - Benchmarks individual AST statement lock hierarchy order evaluation.
- **`BenchmarkGuidanceLayerLinter`** (`internal/architecture`):
  - Validates `.arch/modules` symbol anchors and `RunInTx` transaction boundary contracts against Go AST.
- **`BenchmarkValkeyLinter`** (`internal/architecture`):
  - Enforces banned Valkey `KEYS *` scanning and taxonomy compliance.
- **`BenchmarkCoreDomainInvariantLinter`** (`internal/core`):
  - Verifies encapsulation of Core domain invariants (Progression, Currency, Jobs, Inventory, Equipment, Battle Participant Identity) across all production Go packages.

### 3.2. Core Battle Simulation Engine
- **`BenchmarkBattleSimulation`** (`internal/core/battle`):
  - Measures execution latency and heap allocations for a standard 2-participant deterministic battle.
- **`BenchmarkBattleSimulationMultiTurn`** (`internal/core/battle`):
  - Evaluates multi-turn endurance/boss battle simulation throughput (20+ turns).

### 3.3. Valkey & Session Repository Operations
- **`BenchmarkValkeySessionSave_InMemory`** & **`BenchmarkValkeySessionFind_InMemory`** (`internal/player`):
  - Measures in-memory session store latency and allocations.
- **`BenchmarkValkeySessionSave_LiveValkey`** & **`BenchmarkValkeySessionFind_LiveValkey`** (`internal/player`):
  - Measures round-trip Valkey Master session persistence, TTL tracking, and Set indexing latency when connected to a live Valkey server.

---

## 4. Tooling & Workflow

### 4.1. Running Benchmarks (`make bench`)

To run the standard benchmark suite:

```bash
make bench
# or via rtk:
rtk ./scripts/benchmark.sh
```

This runs all benchmarks with memory allocations reported (`-benchmem`) and saves the latest results to `.benchmarks/current.txt`. If `.benchmarks/baseline.txt` exists, it automatically executes the regression comparator.

### 4.2. Establishing a Performance Baseline

To record a new baseline (e.g., after an optimization or milestone release):

```bash
./scripts/benchmark.sh --record
```

This writes the benchmark output to `.benchmarks/baseline.txt`. Note that `.benchmarks/` is ignored by Git in `.gitignore` to prevent repository noise across developer environments.

### 4.3. Comparing Benchmark Results & Regression Detection

The Go comparison tool `scripts/benchcompare` parses standard Go benchmark output and compares latency and memory allocations:

```bash
go run ./scripts/benchcompare .benchmarks/baseline.txt .benchmarks/current.txt
```

Example output:

```text
Benchmark                                 Old ns/op    New ns/op    Delta   Old allocs   New allocs
----------------------------------------------------------------------------------------------------
BenchmarkBattleSimulation                    8.36µs       7.40µs   -11.5%           50           50
BenchmarkBattleSimulationMultiTurn         110.62µs     106.02µs    -4.2%          603          603
BenchmarkCoreDomainInvariantLinter          63.85ms      55.17ms   -13.6%       303196       303196
BenchmarkGuidanceLayerLinter                 1.50ms       1.50ms    -0.2%          677          677
BenchmarkLockEvaluationStatement             4.69µs       4.51µs    -3.8%           22           22
BenchmarkLockHierarchyLinter                32.39ms      29.50ms    -8.9%       140435       140434 (-1)
BenchmarkValkeyLinter                        7.63ms       7.22ms    -5.4%         5311         5312 (+1)
BenchmarkValkeySessionFind_InMemory         181.7ns      182.3ns    +0.3%            0            0
BenchmarkValkeySessionFind_LiveValkey      180.96µs     203.02µs   +12.2%           13           13
BenchmarkValkeySessionSave_InMemory        129.12µs     152.24µs   +17.9%            1            1
BenchmarkValkeySessionSave_LiveValkey      405.64µs     378.47µs    -6.7%           18           18
----------------------------------------------------------------------------------------------------
All benchmarks are within acceptable performance thresholds.
```

If a benchmark regresses beyond the threshold (default: +30%), `benchcompare` tags it with `[REGRESSION]` and exits with code 2, enabling automated regression detection in CI workflows.
