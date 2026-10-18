# Testing and Verification Strategy

## 1. Purpose & Priorities

Tests specify and protect game behavior. The most valuable tests are those that remain valid when implementation details change.

1. **Domain invariants and game rules**: Progression calculations, job requirements, battle outcomes, item ownership/equipment rules, currency invariants, scheduled actions.
2. **Observable component behavior**: Public service contracts and state transitions.
3. **Integration flows**: Cross-package transactions and API contracts.
4. **Regression tests**: A reproducible bug must receive a failing regression test before being fixed.

Avoid tests tightly coupled to private data structures; a refactor preserving behavior should require minimal test churn.

---

## 2. Tiered Verification Strategy

| Stage | Verification |
| --- | --- |
| TDD iteration | `go test -count=1 ./<package> -run '<test>'` |
| Change ready | `go test -count=1 ./<package>` for affected packages |
| Before commit | `make check`; wait for completion |
| Push | Let the pre-push hook reuse the successful check or verify changed content |
| Clean database verification needed | `make check-clean` resets the database and forces full verification |

For DB/Valkey changes, run affected-package tests with live services and connection settings (`PARTY2_DB_DSN`, `PARTY2_VALKEY_ADDR`). `make test-integration` supplies local connection settings and tests all packages; start services via `make up` and apply migrations via `make db-migrate`. `make test` also tests all packages; integration coverage depends on connection settings.

`make check` runs formatting, static analysis, OpenAPI/architecture checks, migrations, the full test suite and a smoke image build. It and the pre-push hook call the same `scripts/verify.sh`, which reuses a successful result for an unchanged working tree. Do not start duplicate checks concurrently or repeat a successful check solely for push or session end. If the verification environment changes, use `FORCE_VERIFY=1 make check`.

---

## 3. Canonical Commands

All canonical commands run through the repository `Makefile`:

```bash
# Start local DB and Cache for fast Inner Loop and MCP access
make up

# Auto-format Go code and synchronize OpenAPI 3.1 specifications
make fmt

# All-package host tests (integration coverage depends on connection settings)
make test

# Host-based integration tests (requires make up)
make test-integration

# Run all local checks (formatting, vet, openapi-check, db-migrate, full host tests, smoke build)
make check

# Clean verification with full DB reset
make check-clean

# Apply pending database migrations / reset from scratch
make db-migrate
make db-reset
```

---

## 4. Container Roles

- **`Dockerfile.dev` & `compose.yaml`**: Development and integration test environment. Spawns MariaDB and Valkey alongside Go test runner with bind-mounted source and BuildKit cache mounts.
- **`Dockerfile`**: Production distribution image. Multi-stage build compiling a statically linked, stripped binary (`CGO_ENABLED=0`) on `golang:1.26.7-trixie` running on distroless `gcr.io/distroless/static-debian13:nonroot` with CA certificates only.

---

## 5. Test Setup and Fixtures

- **Avoid Premature Test Frameworks**: Do not proactively build complex test frameworks; introduce helpers only when duplication becomes a concrete burden.
- **Explicit Setup over Magic**: Test helpers must not obscure what is being tested. Explicit setup is preferred over implicit abstraction.
- **Reproducible Outcomes**: For exact outcome assertions, inject controlled RNG and clock inputs. Randomized tests should report their seed for replay.
- **Shared Factories**: Use centralized test factories (`CreateTestPlayer`, `CreateTestCharacterWithFunds`, `CreateTestGuildWithLeader`) for integration tests to ensure unique IDs and valid name lengths.

### 5.1 Database Ownership and Isolation

The [workflow rule](../../.agents/rules/01-development-workflow.md#test-database-ownership-and-isolation) defines mandatory constraints. Ordinary DB-backed tests share the database selected by `PARTY2_DB_DSN` within one environment. Unit tests without a database dependency do not need a DB. Shared fixtures can coexist with temporary local development data; neither an empty database nor cleanup of every fixture after every test is a default requirement.

Choose the smallest isolation boundary that preserves the behavior being verified:

| Verification need | Approach |
| --- | --- |
| Assertions can identify the test's own records | Shared DB; use unique fixture identifiers and scope assertions/cleanup to those records and their dependents |
| SQL/migration assertions fit within a temporary table or rollback transaction | Keep existing isolation when it cannot change unrelated data or compromise concurrent tests |
| Global counts, capacity limits, resets or lifecycle operations | Dedicated DB; freely create, update, delete or reset data inside it |
| Real commits or independent worker transactions | Retain those boundaries; use a dedicated DB if interference would prevent accurate verification |

Cleanup such as `DELETE ... WHERE character_id = ?` is appropriate when the character and affected dependent rows belong to that fixture. Go packages run concurrently even when individual tests are sequential. Deleting shared rows and later restoring them can erase another test's writes.

Dedicated test fixtures create unique databases, copy only required table definitions (including foreign keys) from `PARTY2_DB_DSN`, and drop only their own databases during cleanup. `PARTY2_TEST_DB_ADMIN_DSN` must allow creating/dropping these databases (supplied by `make test-integration`, `make check`, and Compose test runners; or `root:root@tcp(127.0.0.1:3306)/?parseTime=true` on host). Missing admin configuration fails dedicated DB tests rather than falling back to the shared DB.

---

## 6. Mechanical AST Static Analysis Linters

Party2 maintains an in-tree suite of automated Go AST (Abstract Syntax Tree) linters implemented as standard Go tests under `internal/architecture/` and subsystem directories. They run with zero external binary dependencies via `go test`, `make arch-lint`, and `make check`.

### Key Enforced Rules

- **Silent Error Suppression Prohibition (`error_swallow_lint_test.go`)**: Discarding errors returned from database repositories, persistent stores, or state mutations via blank identifiers (`_ =`, `_, _ =`, `val, _ :=`) or unassigned expressions is strictly prohibited. If a call is strictly best-effort or compensatory (e.g. post-commit cache eviction), annotate with `//lint:ignore error-swallow <reason>`.
- **Lock Hierarchy Linter (`internal/database/lock_hierarchy_lint_test.go`)**: Enforces pessimistic row-lock acquisition hierarchy order (Rank 0 through Rank 8) across multi-table transactions.
- **Guidance Layer Coordinates (`arch_test.go`)**: Verifies `.arch/*.json` module boundaries, exported symbols, and shared tables against real Go AST.
- **Package & Layer Boundaries (`package_boundary_lint_test.go`)**: Enforces acyclic layered architecture (e.g. domain layers cannot import HTTP transport).
- **Core Mutation Encapsulation (`core_lint_test.go`)**: Enforces that currency, inventory, and stats are mutated through encapsulated domain methods rather than direct field assignment.
- **Auth Wrapper Compliance (`auth_lint_test.go`)**: Verifies mutating HTTP endpoints wrap execution with standard authentication helpers.
- **Dead Code & Unused Definitions (`deadcode_lint_test.go`, `unused_definitions_lint_test.go`)**: Flags unreferenced top-level constants, DTOs, and orphaned domain methods.

---

## 7. Continuous Performance Verification & Benchmarking

Party2 explicitly separates functional correctness from performance verification:

1. **Functional Safety Verification (`go test`)**: Functional tests MUST NOT enforce sub-second wall-clock thresholds (fragile in CI/virtualized environments). Use liberal dead-man bounds (2.0s–5.0s) strictly to catch deadlocks and hangs. Emit informational timing via `t.Logf`.
2. **Continuous Performance Verification (`make bench`)**: CPU-sensitive operations (AST linters, battle simulation, Valkey session persistence) use Go standard `testing.B` benchmarks.
   - Run benchmarks: `make bench` (runs `./scripts/benchmark.sh`).
   - Baseline recording: `./scripts/benchmark.sh --record` writes `.benchmarks/baseline.txt`.
   - Regression comparator: `go run ./scripts/benchcompare .benchmarks/baseline.txt .benchmarks/current.txt` detects regressions beyond +30% threshold.

---

## 8. Continuous Integration & Environment Lifetime

- **Disposable Per-Job Environment**: CI (`.github/workflows/ci.yml`) runs on GitHub-hosted runners using Docker Compose (`Dockerfile.dev` and `compose.yaml`). Compose initializes MariaDB from `migrations/` on first volume creation; runner disposal releases the environment afterward.
- **Isolation in CI**: CI's disposable environment does not waive test database ownership rules for individual tests within a run.
- **Pipeline Stages**: Formatting & static analysis -> AST linters -> migrations -> test suite with race detector -> production Docker smoke build.

---

## 9. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD, DoD, and workflow rules.
- [`development-environment.md`](development-environment.md) — local service configuration.
- [`openapi.md`](openapi.md) — OpenAPI specification synchronization.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license governance.
