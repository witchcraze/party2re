# Development Environment

## Purpose

This document describes the reproducible local development environment for
Party2 Re, including backing service startup, database schema preparation,
application daemon execution, and host API reachability verification.

Local `make fmt`, `make test`, and the normal `make check` path use the host Go toolchain. Docker provides MariaDB, Valkey and a Go development container; `make test-docker` runs tests in that container. See [`testing.md`](testing.md) for verification stages.

## Services

Docker Compose provides:

- `app` — Go 1.26.7 development container; exposes port `127.0.0.1:8080:8080` for host access when run with published ports (`-p 127.0.0.1:8080:8080` or `--service-ports`);
- `mariadb` — MariaDB using the `latest` image, exposing `127.0.0.1:3306:3306`;
- `valkey` — Valkey (Redis-compatible) used for the ScheduledAction queue, exposing `127.0.0.1:6379:6379`. Configured with both AOF and RDB persistence so pending scheduled actions survive container restarts. The `app` service connects via `PARTY2_VALKEY_ADDR: valkey:6379`.

### Runtime Dependency Contract

Both MariaDB and Valkey are mandatory runtime infrastructure dependencies for the application daemon (`cmd/party2`). During startup, `runWithConfig` performs timeout-bounded connectivity verification (`database.PingContext`, `valkey.Ping`) and aborts bootstrap immediately if either backing service is unreachable or misconfigured. In-memory adapters in domain packages are strictly test-only fixtures and are never used as production runtime fallbacks.

## Starting and Verifying the Local Server (End-to-End Procedure)

Follow these steps from the repository root to start backing services, prepare the database schema, run the application daemon, and verify API reachability from the host:

### 1. Start Backing Services

Start MariaDB and Valkey containers in the background:

```sh
make up
# Or: docker compose up -d mariadb valkey
```

Both services must be healthy before the application daemon starts. The `app` service waits for both health checks automatically.

### 2. Prepare Database Schema

Before running the server, ensure all database migrations are applied:

```sh
make db-migrate
```

> [!NOTE]
> When `compose.yaml` initializes a newly created named volume (`mariadb-data`), MariaDB executes migration scripts from `./migrations` automatically via `/docker-entrypoint-initdb.d`. However, for existing volumes or newly added migration files, `/docker-entrypoint-initdb.d` does not run again. Running `make db-migrate` guarantees that any pending incremental migrations in `migrations/*.sql` are recorded and applied safely without losing existing data.
>
> If you need to drop and recreate the local database completely from all migration files, run `make db-reset`.

### 3. Start the Application Daemon

Start the server process with host port publishing on `127.0.0.1:8080`:

**Using the container runner (primary):**

```sh
make run
# Or: docker compose run --rm -p 127.0.0.1:8080:8080 app go run ./cmd/party2
```

The `-p 127.0.0.1:8080:8080` flag exposes the daemon's listening port to localhost on port 8080. (Alternatively, `docker compose run --rm --service-ports app go run ./cmd/party2` utilizes the port mapping defined in `compose.yaml`.) To bind to a custom host port, specify `PORT`:

```sh
make run PORT=8088
```

**Using host Go (alternative):**

If running directly on the host Go toolchain:

```sh
PARTY2_DB_DSN="party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true" \
PARTY2_VALKEY_ADDR="127.0.0.1:6379" \
go run ./cmd/party2
```

### 4. Verify Host HTTP Reachability

From a separate terminal on the host machine, query the `/health` endpoint:

```sh
curl -i http://127.0.0.1:8080/health
```

Expected response:

```http
HTTP/1.1 200 OK
Content-Type: application/json
...
{"status":"ok"}
```

A `200 OK` response with `{"status":"ok"}` confirms that the daemon is listening, backing store pings succeeded, and the HTTP router is serving traffic.

### 5. Stopping the Environment

1. **Stop the application daemon**: Press `Ctrl+C` in the terminal running the daemon. The process gracefully terminates active HTTP connections and the background scheduled action worker.
2. **Stop backing services**:
   ```sh
   make down
   # Or: docker compose down
   ```

> [!IMPORTANT]
> `make down` (and `docker compose down`) terminates and removes containers while preserving the persistent named volumes (`mariadb-data`, `valkey-data`). Your database data and cache entries remain intact for subsequent sessions.
>
> Do **NOT** use `docker compose down -v` unless you explicitly want to discard and wipe all local database and cache data.

## Application configuration

The application daemon supports the following core environment variables:
- `PARTY2_DB_DSN`: MariaDB connection DSN (required).
- `PARTY2_VALKEY_ADDR`: Valkey endpoint address (required, e.g. `valkey:6379` or `127.0.0.1:6379`).
- `PARTY2_PORT` / `PARTY2_ADDR`: Server listen port or address (default `8080` / `:8080`).
- `PARTY2_MAX_PLAYERS`: Server-wide player account population limit (default `999`; `0` or negative for unlimited). Active player accounts (`WHERE banned_at IS NULL`) are evaluated at registration time; when capacity is reached, `POST /players` rejects with `409 Conflict`.
- `PARTY2_ADMIN_API_KEY`: API key for administrative endpoints.
- `PARTY2_CORS_ORIGINS`: Comma-separated list of allowed CORS origins.
- `PARTY2_TRUSTED_PROXIES`: Comma-separated CIDR blocks for reverse proxy client IP resolution.

## Database configuration

The development connection string is supplied to the application through
`PARTY2_DB_DSN` in `compose.yaml`.

Running outside Compose also requires `PARTY2_DB_DSN`; startup fails when it is unset. For host execution, supply a DSN pointing to the local MariaDB service at `127.0.0.1:3306` (e.g. `party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true`). For host tests requiring dedicated database isolation, also configure `PARTY2_TEST_DB_ADMIN_DSN` with database creation/drop privileges (e.g. `root:root@tcp(127.0.0.1:3306)/?parseTime=true`). See [`testing.md`](testing.md#52-dedicated-database-setup).

## Database initialization and migration

SQL files in `migrations/` define the relational schema.

To apply pending migrations safely without dropping data:

```bash
make db-migrate
```

The local database persists across runs and normally serves both development and DB-backed tests. Temporary development/test data is allowed. See the [test DB ownership and isolation procedure](testing.md#51-database-ownership-and-isolation) before writing fixture setup or cleanup.

To recreate the local database completely from all migration files:

```bash
make db-reset
```

This explicitly drops and recreates the local Compose `party2` database; it does not reset Valkey. `make check-clean` performs the same DB reset before forcing full verification. Stop tests and application processes using the environment before either command. These are environment operations, not individual test setup/cleanup, and are not required before every normal test run. Normal `make check` applies pending migrations without resetting the database.

The database volume can also be wiped manually if needed:

```sh
docker compose down -v
docker compose up -d mariadb valkey
```

`docker compose down -v` removes all Compose-managed named volumes, including MariaDB and Valkey data. Use it only when explicitly discarding that local environment's data.

## Related documents

- [`testing.md`](testing.md) — canonical formatting, test, and analysis commands.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license rules.
