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
| Assertions can identify the test's own records | Shared DB; use unique fixture identifiers and scope assertions/optional cleanup to those records and their dependents |
| SQL/migration assertions fit entirely within a connection-local temporary table or a guaranteed rollback transaction | Keep that existing isolation when it cannot change unrelated data or compromise concurrent tests |
| Global counts, capacity limits, resets or lifecycle operations cannot be checked reliably alongside unrelated data | Dedicated DB; freely create, update, delete or reset data inside it |
| Real commits or independent worker transactions are part of the behavior | Retain those boundaries; use a dedicated DB if interference would otherwise prevent accurate verification |

Cleanup such as `DELETE ... WHERE character_id = ?` is appropriate when the character and affected dependent rows belong to that fixture. A common town ID, a test-like name prefix, absence of `t.Parallel()` or a cooperating test mutex does not establish ownership of all matching data. Go packages can run concurrently even when individual tests are sequential. Deleting shared rows and later restoring them can erase another test's writes.

The existing repository reset assertion in [`ranking_extra_repository_test.go`](../../internal/database/ranking_extra_repository_test.go) keeps its mutation and reads in one transaction and rolls back before checking the fixture again outside it. This does not verify committed cross-connection behavior. If rollback cannot contain every write, or locking interferes with other tests, use a dedicated DB. Connection-local temporary tables in [`job_points_migration_test.go`](../../internal/database/job_points_migration_test.go) and [`replay_migration_test.go`](../../internal/database/replay_migration_test.go) isolate migration fixtures without clearing persistent tables; their connection boundary must be preserved.

Environment-wide initialization/disposal is separate from per-test setup/cleanup. A fresh CI job may initialize and discard its whole environment; a developer may explicitly reset a local environment between uses. These permissions do not authorize an individual shared-DB test to clear unrelated rows. See [local reset procedures](development-environment.md#database-initialization-and-migration) and [CI environment lifetime](ci-cd.md#test-environment-lifetime).

Future development with multiple concurrent sessions should isolate each environment's DB, cache and runtime resources. This policy does not add session provisioning or change the current Compose configuration.

### 5.2 Existing Dedicated Database Setup

[`weeklyRotationDB`](../../internal/ranking/weekly_rotation_integration_test.go), [`contestIntegrationDB`](../../internal/contest/integration_test.go) and [`replayArchiveDB`](../../internal/api/http/replay_authorization_integration_test.go) already create unique databases, copy only required table definitions (including foreign keys) from `PARTY2_DB_DSN`, and drop only their own databases during cleanup. Weekly rotation workers retain separate connections and transactions; shared ranking rows are never cleared or restored by these fixtures.

`PARTY2_TEST_DB_ADMIN_DSN` must allow creating/dropping these databases. It is supplied by `make test-integration`, `make check`, and the development Compose test runner. For direct host runs with the local Compose services, set it to `root:root@tcp(127.0.0.1:3306)/?parseTime=true`. Missing admin configuration fails these DB-enabled tests rather than falling back to the shared database. This administrator connection is for tests only; ordinary shared-DB tests do not require database-creation privileges.

### 5.3 Known Implementation Gaps

Some existing fixtures still violate the ownership policy: the [Lottery tests clear shared ticket/round tables (#1396)](https://github.com/witchcraze/party2re/issues/1396), and the [Home integration test deletes houses by town rather than fixture ownership (#1397)](https://github.com/witchcraze/party2re/issues/1397). These are pending corrections, not approved exceptions. This policy change does not modify tests or DB configuration.

---

## 6. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD and DoD rules.
- [`ast-linters.md`](ast-linters.md) — mechanical AST static analysis suite.
- [`benchmarking.md`](benchmarking.md) — performance benchmarking and regression tracking.
- [`issue-workflow.md`](issue-workflow.md) — acceptance criteria and PR review lifecycle.
