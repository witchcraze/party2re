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

---

## 6. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD and DoD rules.
- [`ast-linters.md`](ast-linters.md) — mechanical AST static analysis suite.
- [`benchmarking.md`](benchmarking.md) — performance benchmarking and regression tracking.
- [`issue-workflow.md`](issue-workflow.md) — acceptance criteria and PR review lifecycle.
