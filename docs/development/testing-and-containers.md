# Testing and Container Workflow

## Container roles

`Dockerfile.dev` and `compose.yaml` together form the **development and integration-test environment**. They start the Go test container alongside MariaDB, wait for the database health check, and provide a reproducible database connection via bind-mounted source and cached Go module/build caches.

`Dockerfile` is the **production distribution image**. It uses a multi-stage build:

- **builder stage** (`golang:1.26.7-trixie`) — compiles a statically linked, stripped binary with `CGO_ENABLED=0`.
- **runtime stage** (`gcr.io/distroless/static-debian13:nonroot`) — contains the compiled binary and CA certificates only. No shell, no package manager, no Go toolchain, no source code.

MariaDB remains a separate service in both development and production environments.

The production image is published to GitHub Container Registry (`ghcr.io/witchcraze/party2re`) automatically on every push to `main` and on version tags via `.github/workflows/publish.yml`.

## Test commands and verification

The repository provides a unified `Makefile` and verification script (`scripts/verify.sh`) ensuring local verification matches CI exactly:

```bash
# Auto-format Go code
make fmt

# Run all local checks (formatting check, go vet, db-migrate, docker tests, smoke image build)
make check

# Run clean verification with full database reset (DROP & recreate party2 DB)
make check-clean

# Apply pending database migrations safely without data loss
make db-migrate

# Reset database and reapply all migrations from scratch
make db-reset

# Optional: configure Git pre-push hook to automatically prevent broken pushes
make setup-hooks
```

Direct docker-compose commands remain available if needed:

```bash
docker compose run --rm app go vet ./...
docker compose run --rm app go test -count=1 ./...
```

Integration tests that require MariaDB use `PARTY2_DB_DSN` and are skipped when
that environment variable is absent. Compose supplies it automatically.

Integration tests that require a live Valkey instance follow the same pattern:
they check `PARTY2_VALKEY_ADDR` and call `t.Skip(...)` when it is absent.
Compose supplies `PARTY2_VALKEY_ADDR: valkey:6379` automatically.

> **Known gap**: `internal/scheduling/valkey_repository.go` (ScheduledAction
> queue, lock acquisition, and retention logic) does not yet have integration
> tests. Adding them is tracked as a follow-up issue. Use the
> `PARTY2_VALKEY_ADDR` skip pattern described above when implementing them.

Coverage is reported for inspection, not enforced as a pass/fail threshold.
The CI workflow uploads both `coverage.out` (machine-readable) and
`coverage.txt` (human-readable) as an artifact.

The report is retained as a CI artifact so changes in coverage can be
reviewed over time without making coverage percentage a release gate.

## Content test expectations

Every Job and Item definition must be covered by catalog-wide validation:

- required fields and ranges;
- unique and resolvable identifiers;
- loading and lookup;
- rejection of invalid definitions.

Rules shared by multiple definitions are covered by table-driven boundary
tests. Definitions that select a special rule must also have a corresponding
special-rule test. Adding content must not silently remove an existing test
case.

The repeatable content-only validation command is:

```text
docker compose run --rm app go test ./internal/core/job ./internal/core/item
```

Both Job and Item definitions are covered by catalog-wide loading, validation,
and boundary tests.

## Test Setup and Fixtures

As the number of features grows, setting up preconditions (e.g., creating a player, character, and inventory) can become repetitive. 

**Rules for Test Helpers:**
1. Do not proactively build complex test frameworks or universal `internal/testutil` builders. Wait until test setup duplication *actually occurs* and becomes a tangible burden.
2. When creating test helpers or fixtures, do not use excessive abstraction. If a helper hides the intent of the test (e.g., hiding which specific item was added to the inventory when the test is explicitly about consuming that item), it is an anti-pattern. 
3. Tests must remain readable as documentation of behavior. Explicit setup is preferred over implicit magic.

## Logging direction

Application logs are intended for container stdout/stderr and should use the
standard-library `log/slog` package when runtime logging is introduced.
Production output should be structured JSON. Passwords, session values, and
database credentials must never be logged.
