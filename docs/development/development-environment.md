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

## Database initialization

SQL files in `migrations/` are mounted into MariaDB's
`/docker-entrypoint-initdb.d/` directory. MariaDB applies these files in name
order when the database volume is initialized.

The initial schema is defined in `migrations/001_initial.sql`. Existing
volumes are not reinitialized automatically. To recreate the local database
from the migration files, stop the services and remove the development volume:

```sh
docker compose down -v
docker compose up -d mariadb
```

The `-v` option deletes local database data and should not be used when that
data must be preserved.

## Stopping the environment

```sh
docker compose down
```

The named MariaDB volume remains after this command.

## Related documents

- [`testing.md`](testing.md) — canonical formatting, test, and analysis commands.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license rules.
- [`ci-cd.md`](ci-cd.md) — continuous verification principles.
