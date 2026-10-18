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

From the repository root, start MariaDB and Valkey:

```bash
make up
```

For host execution, `PARTY2_DB_DSN` (`root:root@tcp(127.0.0.1:3306)/party2?parseTime=true`) and `PARTY2_VALKEY_ADDR` (`127.0.0.1:6379`) connect directly to these local backing services.

## Database initialization and migration

SQL files in `migrations/` define the relational schema.

- Apply pending migrations: `make db-migrate`
- Recreate database from scratch: `make db-reset`

The local database persists across runs and serves development and shared-DB tests. See [Database Ownership and Isolation](testing.md#51-database-ownership-and-isolation) for test isolation rules.

## Stopping the environment

```bash
docker compose down
```

To wipe volumes completely: `docker compose down -v`.

## Related documents

- [`testing.md`](testing.md) — canonical formatting, test, and verification procedures.
- [`openapi.md`](openapi.md) — OpenAPI specification synchronization.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license rules.
