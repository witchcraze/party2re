# Development Environment

## Purpose

This document describes the reproducible local development environment for
Party2 Re.

Local `make fmt`, `make test`, and the normal `make check` path use the host Go toolchain. Docker provides MariaDB, Valkey and a Go development container; `make test-docker` runs tests in that container. See [`testing.md`](testing.md) for verification stages.

## Services

Docker Compose provides:

- `app` — Go 1.26.7 development container;
- `mariadb` — MariaDB using the `latest` image;
- `valkey` — Valkey (Redis-compatible) used for the ScheduledAction queue
  (added in Issue #106). Configured with both AOF and RDB persistence so
  pending scheduled actions survive container restarts. The `app` service
  connects via `PARTY2_VALKEY_ADDR: valkey:6379`.

### Runtime Dependency Contract

Both MariaDB and Valkey are mandatory runtime infrastructure dependencies for the application daemon (`cmd/party2`). During startup, `runWithConfig` performs timeout-bounded connectivity verification (`database.PingContext`, `valkey.Ping`) and aborts bootstrap immediately if either backing service is unreachable or misconfigured. In-memory adapters in domain packages are strictly test-only fixtures and are never used as production runtime fallbacks.

## Starting the environment

From the repository root:

```sh
docker compose up -d mariadb valkey
```

Both services must be healthy before the `app` service starts.
The `app` service waits for both health checks automatically.

Start the application, including dependency startup checks, with:

```sh
docker compose run --rm app go run ./cmd/party2
```

## Database configuration

The development connection string is supplied to the application through
`PARTY2_DB_DSN` in `compose.yaml`.

Running outside Compose also requires `PARTY2_DB_DSN`; startup fails when it is unset. For host execution, supply a DSN pointing to the local MariaDB service at `127.0.0.1:3306`.

## Database initialization and migration

SQL files in `migrations/` define the relational schema.

To apply pending migrations safely without dropping data:

```bash
make db-migrate
```

To recreate the local database completely from all migration files:

```bash
make db-reset
```

The database volume can also be wiped manually if needed:

```sh
docker compose down -v
docker compose up -d mariadb valkey
```


## Stopping the environment

```sh
docker compose down
```

The named MariaDB volume remains after this command.

## Related documents

- [`testing.md`](testing.md) — canonical formatting, test, and analysis commands.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license rules.
- [`ci-cd.md`](ci-cd.md) — continuous verification principles.
