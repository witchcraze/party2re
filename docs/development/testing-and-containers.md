# Testing and Container Workflow

## Tiered Verification Strategy (Inner and Outer Loops)

This project uses a tiered verification strategy to ensure both rapid local development (Inner Loop) and strict, reproducible integration checks (Outer Loop).

### 1. Inner Loop (Fast Host Execution)
Day-to-day development relies on the host machine's Go toolchain for instant feedback.
- **`make test`**: Runs `go test ./...` directly on your host machine. This executes all unit tests in milliseconds using Go's incremental compiler and test cache.
- **Integration Tests Skip**: Tests that require a database or Valkey check for `PARTY2_DB_DSN` and `PARTY2_VALKEY_ADDR`. If these environment variables are absent, the tests automatically `t.Skip`.
- **`make test-integration`**: Runs the full test suite against the local Docker Compose infrastructure. It automatically sets the environment variables to connect to `127.0.0.1:3306` (MariaDB) and `127.0.0.1:6379` (Valkey).

**Note for Non-Docker Users**: While Docker Compose is highly recommended, you can develop purely on the host by natively installing MariaDB and Valkey. Set `PARTY2_DB_DSN` and `PARTY2_VALKEY_ADDR` manually in your shell to run integration tests against your native services.

### 2. Outer Loop (Strict Container Verification)
Strict verification is performed inside containers to guarantee reproducibility.
- **`make check`**: Prioritizes fast host-based verification (`gofmt`, `go vet`, cached `go test ./...`) followed by a fast incremental BuildKit Docker build (`make smoke`). This reduces routine check turnaround to < 20 seconds.
- **`make check-clean`**: Performs a full database reset, clears caches, and runs the verification pipeline from scratch to guarantee absolute correctness before merging.

## Container roles

`Dockerfile.dev` and `compose.yaml` together form the **development and integration-test environment**. They start the Go test container alongside MariaDB, wait for the database health check, and provide a reproducible database connection via bind-mounted source and cached Go module/build caches.

`Dockerfile` is the **production distribution image**. It uses a multi-stage build:

- **builder stage** (`golang:1.26.7-trixie`) — compiles a statically linked, stripped binary with `CGO_ENABLED=0` leveraging BuildKit cache mounts (`--mount=type=cache,target=/root/.cache/go-build`).
- **runtime stage** (`gcr.io/distroless/static-debian13:nonroot`) — contains the compiled binary and CA certificates only. No shell, no package manager, no Go toolchain, no source code.

MariaDB remains a separate service in both development and production environments.

The production image is published to GitHub Container Registry (`ghcr.io/witchcraze/party2re`) automatically on every push to `main` and on version tags via `.github/workflows/publish.yml`.

## Test commands and verification

The repository provides a unified `Makefile` and verification script (`scripts/verify.sh`) ensuring local verification matches CI exactly:

```bash
# Start local DB and Cache for fast Inner Loop and MCP access
make up

# Auto-format Go code
make fmt

# Fast host-based unit tests
make test

# Host-based integration tests (requires make up)
make test-integration

# Run all local checks (formatting check, go vet, db-migrate, fast host tests, smoke image build)
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
