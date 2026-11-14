# Development Environment

## Purpose

This document is a **Linux example** of a working development environment for
Party2 Re: required software, a clone-to-`make check` sequence, running the
server, and resetting local data. Other platforms and setups work when they meet
the [environment contract](#environment-contract); they are not documented step
by step.

See [`testing.md`](testing.md) for verification stages and
[`environment-variables.md`](environment-variables.md) for application settings.

## Required software

Install these with your distribution's standard packages or the linked official
guides. Versions are not listed here; the Go version is defined by
[`go.mod`](../../go.mod).

- [Go](https://go.dev/doc/install)
- [Docker Engine](https://docs.docker.com/engine/install/) with the
  [Compose plugin](https://docs.docker.com/compose/install/linux/) (`docker compose`)
- [git](https://git-scm.com/downloads/linux), GNU make and bash

**When Go is used.** Changing code requires host Go: `make fmt`, `make test`,
`make check`, `make check-changed` and the pre-push hook run it (`make check`
formats code, regenerates the OpenAPI bundles and tests against the isolated
test stack). Running the application without changing code (`make up`,
`make db-migrate`, `make run`, `make test-docker`) works in containers without
host Go. Without host Go, `make check` currently stops at its first `gofmt`
call.

## Example: from clone to a passing `make check`

```sh
git clone https://github.com/witchcraze/party2re.git
cd party2re
make setup-hooks   # run make check's verification before every push
make up            # start the development MariaDB and Valkey
make db-migrate    # apply the schema to the development database
make check         # format, analyze and run the full test suite
```

`make check` takes several minutes on its first run and ends with
`==> ALL VERIFICATION CHECKS PASSED!`. It starts its own disposable
[test stack](testing.md#4-container-roles), so it does not change the
development database.

## Services

[`compose.yaml`](../../compose.yaml) defines the development stack; image tags
and published ports are defined there:

- `app` — Go development container built from [`Dockerfile.dev`](../../Dockerfile.dev); used by `make run` and `make test-docker`;
- `mariadb` — MariaDB holding the development database;
- `valkey` — Valkey (Redis-compatible) used for the ScheduledAction queue, configured with AOF and RDB persistence so pending scheduled actions survive container restarts.

### Runtime Dependency Contract

Both MariaDB and Valkey are mandatory runtime infrastructure dependencies for the application daemon (`cmd/party2`). During startup, `runWithConfig` performs timeout-bounded connectivity verification (`database.PingContext`, `valkey.Ping`) and aborts bootstrap immediately if either backing service is unreachable or misconfigured. In-memory adapters in domain packages are strictly test-only fixtures and are never used as production runtime fallbacks.

## Starting and Verifying the Local Server (End-to-End Procedure)

Follow these steps from the repository root to start backing services, prepare the database schema, run the application daemon, and verify API reachability from the host:

### 1. Start Backing Services

Start MariaDB and Valkey containers in the background:

```sh
make up
```

The command returns once both services are healthy and fails if they do not become healthy. The `app` service also waits for both health checks.

### 2. Prepare Database Schema

Before running the server, ensure all database migrations are applied:

```sh
make db-migrate
```

The [character name uniqueness migration](../../migrations/110_character_name_uniqueness.sql)
stops before schema changes if existing names conflict under the target exact
comparison. It reports duplicate names and character IDs and retains their data.
Resolve the listed identities explicitly, then rerun `make db-migrate`; the
migration never deletes, merges or renames characters automatically. For a
disposable local test database, an explicit reset is a separate choice described
below; it is not an automatic migration repair.

> [!NOTE]
> When `compose.yaml` initializes a newly created named volume (`mariadb-data`), MariaDB executes migration scripts from `./migrations` automatically via `/docker-entrypoint-initdb.d`. However, for existing volumes or newly added migration files, `/docker-entrypoint-initdb.d` does not run again. Running `make db-migrate` guarantees that any pending incremental migrations in `migrations/*.sql` are recorded and applied safely without losing existing data.
>
> If you need to drop and recreate the local database completely from all migration files, run `make db-reset`.

### 3. Start the Application Daemon

Start the server process in the `app` container, published on localhost:

```sh
make run
```

The host port is the Makefile `PORT` variable (default `8080`); override it with `make run PORT=8088`.

### 4. Verify Host HTTP Reachability

From a separate terminal on the host machine, query the `/health` endpoint (adjust the port if you overrode it):

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
   ```

> [!IMPORTANT]
> `make down` terminates and removes containers while preserving the persistent named volumes (`mariadb-data`, `valkey-data`). Your database data and cache entries remain intact for subsequent sessions.
>
> `make check` and `make check-changed` leave the separate [test stack](testing.md#4-container-roles) running, using about 540MB of memory. `make down` also stops it and discards its tmpfs data; `docker compose down` does not stop it.
>
> Do **NOT** use `docker compose down -v` unless you explicitly want to discard and wipe all local database and cache data.

## Environment contract

Use this section to build your own setup, such as services installed directly on the host. It names the files that define each value instead of repeating them.

- **Services**: MariaDB (or compatible MySQL) and Valkey (or compatible Redis) are mandatory at startup (see the [runtime dependency contract](#runtime-dependency-contract)).
- **Development stack**: [`compose.yaml`](../../compose.yaml) defines services, credentials and published ports. `make test-integration` in the [`Makefile`](../../Makefile) shows the matching host connection settings.
- **Test stack**: [`compose.test.yaml`](../../compose.test.yaml) defines the disposable stack used by `make check`; [`scripts/test_services.sh`](../../scripts/test_services.sh) defines its port overrides and its fallback to the development stack when it cannot start.
- **Connection settings**: the application and tests read the variables in [`environment-variables.md`](environment-variables.md). DB-backed tests skip when connection variables are absent; tests requiring a dedicated database also need `PARTY2_TEST_DB_ADMIN_DSN` (see [Dedicated Database Setup](testing.md#52-dedicated-database-setup)).
- **Schema**: `migrations/*.sql` are applied in filename order and recorded in the `schema_migrations` table by [`scripts/migrate.sh`](../../scripts/migrate.sh).
- **Tooling**: Make targets that manage services or the schema call `docker compose`. A setup without Docker must apply the schema itself, then run `go test -count=1 ./...` or `go run ./cmd/party2` with the connection variables set.

## Database initialization and migration

SQL files in `migrations/` define the relational schema.

To apply pending migrations safely without dropping data:

```bash
make db-migrate
```

The local database persists across runs and serves development and directly run DB-backed tests. `make check` and `make check-changed` use the disposable [test stack](testing.md#4-container-roles) instead, so they neither change this database nor apply migrations to it unless they warn that the stack is unavailable. The [test DB ownership and isolation procedure](testing.md#51-database-ownership-and-isolation) owns the residual-data guarantee and fixture isolation requirements.

To recreate the local database completely from all migration files:

```bash
make db-reset
```

This explicitly drops and recreates the local Compose `party2` database; it does not reset Valkey. To reclaim accumulated test SQL rows and Valkey keys, use `make check-clean`: it discards the test stack before forcing full verification, whose tests can leave new fixtures. `make down` discards that stack without running verification. Stop tests and application processes using the affected environment before resetting or stopping it. Normal `make check` applies pending migrations without resetting the stores; a cached verification does not run tests or clean data.

The database volume can also be wiped manually if needed:

```sh
docker compose down -v
docker compose up -d --wait mariadb valkey
```

`docker compose down -v` removes all Compose-managed named volumes, including MariaDB and Valkey data. Use it only when explicitly discarding that local environment's data.

## Related documents

- [`testing.md`](testing.md) — canonical formatting, test, and analysis commands.
- [`environment-variables.md`](environment-variables.md) — required and optional application settings.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license rules.
