# Development Environment

## Purpose

This document describes the reproducible local development environment for
Party2 Re.

The Go toolchain and Go dependencies run inside Docker. A Go installation on
the host machine is not required.

## Services

Docker Compose provides:

- `app` — Go 1.26.7 development container;
- `mariadb` — MariaDB using the `latest` image;
- `valkey` — Valkey (Redis-compatible) used for the ScheduledAction queue
  (added in Issue #106). Configured with both AOF and RDB persistence so
  pending scheduled actions survive container restarts. The `app` service
  connects via `PARTY2_VALKEY_ADDR: valkey:6379`.

## Starting the environment

From the repository root:

```sh
docker compose up -d mariadb valkey
```

Both services must be healthy before the `app` service starts.
The `app` service waits for both health checks automatically.

Run the application database health check with:

```sh
docker compose run --rm app go run ./cmd/party2
```

## Database configuration

The development connection string is supplied to the application through
`PARTY2_DB_DSN` in `compose.yaml`.

The application also accepts `PARTY2_DB_DSN` when run with another environment.
When it is not set, it uses the local-development default for a MariaDB server
at `localhost:3306`.

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
