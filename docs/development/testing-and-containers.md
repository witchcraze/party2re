# Testing and Container Workflow

## Container roles

The current `compose.yaml` is the development and integration-test
environment. It starts the Go test container and MariaDB together, waits for
the database health check, and provides the repository with a reproducible
database connection. It is not the distribution image.

`Dockerfile.dev` contains the Go toolchain and source-oriented development
environment. The bind mount in Compose allows tests and source changes to be
run without rebuilding the development image.

The future distribution image will be built by a separate production
`Dockerfile`. It will be a multi-stage build containing the compiled
application, runtime content data, and required assets only. It will not
contain the Go toolchain, tests, source mounts, or MariaDB. MariaDB remains a
separate service.

## Test commands

The smallest useful local checks are:

```text
docker compose run --rm app
docker compose run --rm app go vet ./...
docker compose run --rm app sh -c 'go test ./... -coverprofile=coverage.out -covermode=atomic && go tool cover -func=coverage.out'
```

Integration tests that require MariaDB use `PARTY2_DB_DSN` and are skipped when
that environment variable is absent. Compose supplies it automatically.

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

## Logging direction

Application logs are intended for container stdout/stderr and should use the
standard-library `log/slog` package when runtime logging is introduced.
Production output should be structured JSON. Passwords, session values, and
database credentials must never be logged.
