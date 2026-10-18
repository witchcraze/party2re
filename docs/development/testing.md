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

The [Makefile](../../Makefile) owns command definitions. Use `make up` for local
services, `make test-integration` for host tests with DB/Valkey, `make test-docker`
for the container runner, and `make check` for full verification. `make test`
does not supply connection settings. Environment reset/disposal is described in
[development-environment.md](development-environment.md#database-initialization-and-migration).

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
| Assertions can identify the test's own records | Shared DB; use unique fixture identifiers and scope assertions/optional cleanup to those records and their dependents |
| SQL/migration assertions fit entirely within a connection-local temporary table or a guaranteed rollback transaction | Keep that existing isolation when it cannot change unrelated data or compromise concurrent tests |
| Global counts, capacity limits, resets or lifecycle operations cannot be checked reliably alongside unrelated data | Dedicated DB; freely create, update, delete or reset data inside it |
| Real commits or independent worker transactions are part of the behavior | Retain those boundaries; use a dedicated DB if interference would otherwise prevent accurate verification |

Cleanup such as `DELETE ... WHERE character_id = ?` is appropriate when the character and affected dependent rows belong to that fixture. A common town ID, a test-like name prefix, absence of `t.Parallel()` or a cooperating test mutex does not establish ownership of all matching data. Go packages can run concurrently even when individual tests are sequential. Deleting shared rows and later restoring them can erase another test's writes.

The existing repository reset assertion in [`ranking_extra_repository_test.go`](../../internal/database/ranking_extra_repository_test.go) keeps its mutation and reads in one transaction and rolls back before checking the fixture again outside it. This does not verify committed cross-connection behavior. If rollback cannot contain every write, or locking interferes with other tests, use a dedicated DB. Connection-local temporary tables in [`job_points_migration_test.go`](../../internal/database/job_points_migration_test.go) and [`replay_migration_test.go`](../../internal/database/replay_migration_test.go) isolate migration fixtures without clearing persistent tables; their connection boundary must be preserved.

Environment-wide initialization/disposal is separate from per-test setup/cleanup. A fresh CI job may initialize and discard its whole environment; a developer may explicitly reset a local environment between uses. These permissions do not authorize an individual shared-DB test to clear unrelated rows. See [local reset procedures](development-environment.md#database-initialization-and-migration) and [CI environment lifetime](#8-continuous-integration-and-delivery).

Future development with multiple concurrent sessions should isolate each environment's DB, cache and runtime resources. This policy does not add session provisioning or change the current Compose configuration.

### 5.2 Dedicated Database Setup

Dedicated fixtures create uniquely named databases, copy the required schema
including foreign keys, and drop only their own databases. Preserve real
worker/commit boundaries when those are the subject of the test.
`PARTY2_TEST_DB_ADMIN_DSN` must permit database creation/deletion; Compose,
`make test-integration` and `make check` provide it. For direct host tests use
`root:root@tcp(127.0.0.1:3306)/?parseTime=true`. Missing admin configuration fails
DB-enabled dedicated tests rather than falling back to the shared database.


## 6. Mechanical AST Static Analysis Linters

In-tree Go tests under [internal/architecture](../../internal/architecture/),
[internal/database](../../internal/database/), Core and HTTP enforce architectural
boundaries, transaction/lock usage, authentication wrappers, mutation encapsulation,
error propagation and unused-definition checks. `make arch-lint`, `make lock-lint`
and `make check` run the relevant suites. Their source owns the exact scope and
exceptions; passing AST checks do not certify runtime locks or legacy parity.

Legitimate best-effort/compensatory persistence calls use
`//lint:ignore error-swallow <reason>` immediately before or on the statement.
New checks use standard Go AST packages, synthetic positive/negative cases, and
actionable file/line diagnostics. Avoid duplicating the checks' implementation in prose.

## 7. Continuous Performance Verification & Benchmarking

Functional test timeout constraints are owned by the [workflow rule](../../.agents/rules/01-development-workflow.md#unit--integration-test-time-budget-policy-vs-benchmarking).
Measure performance separately with `testing.B`: `make bench` runs
[scripts/benchmark.sh](../../scripts/benchmark.sh), writes `.benchmarks/current.txt`
and compares an existing baseline. `./scripts/benchmark.sh --record` records
`.benchmarks/baseline.txt`; these local results are ignored by Git.
`go run ./scripts/benchcompare .benchmarks/baseline.txt .benchmarks/current.txt`
compares latency/allocations; the default +30% regression threshold exits with code 2.

## 8. Continuous Integration and Delivery

The [CI workflow](../../.github/workflows/ci.yml) runs formatting, OpenAPI sync
validation, vet and tests with atomic coverage in the Compose development
container, then builds the production image. It does not currently run the race
detector or exercise the built production image. MariaDB initializes from migrations
on first volume creation; the GitHub-hosted runner is disposable per job. Tests
inside a job still follow the database ownership/isolation rules above.

CI runs on pushes to main and PR opening/synchronization/reopening against main.
After retargeting a PR, use a supported trigger and confirm a passing run for the
final head; an absent check is not a passing check. Before merge, verify native
closing-Issue linkage (`gh pr view <number> --json closingIssuesReferences`);
`Closes #...` links natively when the base is the default branch. Merge completion
and related-ticket synchronization follow the [workflow rule](../../.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow).

Investigate a CI failure against the pre-change baseline in isolated checkouts
under matching conditions. Record revisions, commands and reproducible seed/input;
preserve behavioral assertions when fixing an unstable fixture. Container startup
and headless gameplay coverage remain distinct from a successful image build.

## 9. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD and DoD rules.
- [`development-environment.md`](development-environment.md) — local services and reset procedures.
- [`openapi.md`](openapi.md) — transport specification maintenance.
