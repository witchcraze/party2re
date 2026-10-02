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

### Inner Loop (Fast Host Execution)
Day-to-day development relies on the host machine's Go toolchain for instant feedback:
- **`make test`**: Runs unit tests (`go test -count=1 ./...`). Skips DB/Valkey tests if services are absent.
- **`make test-integration`**: Runs tests against local container infrastructure (`127.0.0.1:3306` MariaDB and `127.0.0.1:6379` Valkey; start via `make up`).

### Outer Loop (Strict Container Verification)
Strict verification guarantees reproducibility locally and in CI:
- **`make check`**: Prioritizes fast host-based verification (`gofmt`, `openapi-check`, `go vet`, AST linters, cached host test suite) followed by a fast smoke image build (`make smoke`). Routine turnaround is < 20s.
- **`make check-clean`**: Full database reset (`DROP & recreate`), cache invalidation, and scratch pipeline run.

---

## 3. Canonical Commands

All canonical commands run through the repository `Makefile`:

```bash
# Start local DB and Cache for fast Inner Loop and MCP access
make up

# Auto-format Go code and synchronize OpenAPI 3.1 specifications
make fmt

# Fast host-based unit tests
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
- **Shared Factories**: Use centralized test factories (`CreateTestPlayer`, `CreateTestCharacterWithFunds`, `CreateTestGuildWithLeader`) for integration tests to ensure unique IDs and valid name lengths.

---

## 6. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD and DoD rules.
- [`ast-linters.md`](ast-linters.md) — mechanical AST static analysis suite.
- [`benchmarking.md`](benchmarking.md) — performance benchmarking and regression tracking.
- [`issue-workflow.md`](issue-workflow.md) — acceptance criteria and PR review lifecycle.
