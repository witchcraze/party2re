# Status

Capability snapshot reviewed on 2026-10-09. Detailed task history is available in GitHub/Git.

## Current Phase

**Version 1.0 Reconstruction / Refactoring — In Progress (Phase 5+)**

The Go project contains the foundational systems, core combat, the main game feature modules and the HTTP JSON API, implemented in clean-room Go with no legacy code reuse. Current routes and schemas are described in [OpenAPI](docs/api/openapi.json).

- **Component Architecture & Boundaries**: Authoritative responsibilities, dependencies, and lock hierarchy tiers reside in [`docs/architecture/components.md`](docs/architecture/components.md).
- **Reconstruction Reference**: The [frozen feature inventory](docs/migration/feature-inventory.md) preserves an earlier reconstruction snapshot; it is not a live completion log.
- **Domain Design Specifications**: Language-agnostic rules, formulas, and state transitions reside in [`docs/design/README.md`](docs/design/README.md).

---

## Architecture & System Snapshot (What is True Now)

- **Modular Monolith**: Go stdlib HTTP routing with modular wire composition (`cmd/party2/wire.go`).
- **HTTP Transport & Edge Policy**: RESTful JSON API with structured error responses, HATEOAS actions, trusted proxy rate limiting, and centralized Unicode NFC sanitization (`internal/validation`).
- **Durable Persistence**: MariaDB Master with deterministic row-lock hierarchy and ambient transaction propagation. See [component contracts](docs/architecture/components.md) and the repository migrations for details.
- **Transient State**: Valkey Master for ephemeral lobbies & turns (Candidate C), run buffers (Candidate D), actor-indexed unfinished ScheduledActions (Pending/Processing; read/write error propagation & cleanup on schedule/fetch/cancel). SSOT: [`docs/architecture/valkey-keyspace.md`](docs/architecture/valkey-keyspace.md).
- **Lifecycle Contracts**: Fail-fast startup checks for MariaDB/Valkey with clean resource teardown on error.
- **AST Gates & CI**: Automated AST linters (locks, tx runners, ISP, file size ≤500 lines, Valkey keyspace, presentation decoupling), action catalog OpenAPI drift detector (`catalog_lint_test.go`), and non-mutating OpenAPI sync checks (`sync_openapi --check`). SSOT: [`docs/development/ast-linters.md`](docs/development/ast-linters.md).
- **PlayerContext**: Owned `GET /api/v1/characters/{id}/context` and command refresh share typed selected/activity/conflict scenes, unfinished work, recovery and connected eligible choices with strict input schemas/templates. Actual sleep, work, Party/PvP/GvG/Casino membership and Dungeon/Challenge buffers override ordinary selection; conflict preserves all facts and suppresses new entries. Continuation candidates use owned role/phase; room/run mutation commands remain unconnected. Ordinary selection uses one seven-day Valkey record; GET never writes or renews it. Home provides public/owned facts and actor-owned inbox/outbox pages with offset/keyset paging. The four ordinary Shops provide catalog/product facts with explicit purchase IDs; purchase commands remain unconnected. SecretShop provides qualified catalog/product pages, required HelperQuest filtering and explicit purchase templates. Casino provides paged lobby summaries within its 100-room window, selection without admission, owned admitted masked game detail and role/phase choices; mutations remain unconnected. Bank provides owned wallet/savings, deposit limit and NPC facts; other facility and full combat details remain pending.
- **Action Gateway**: Authenticated `POST /api/v1/characters/{id}/actions` and shared outcome/refresh handling are available. Current command coverage includes `adventure_start`, `rescue_request`, `home_sleep`, `home_wake`, `bank_deposit`, `bank_withdraw`, `secretshop_purchase` and the four `scene_*` navigation commands; remaining command coverage is incomplete. SecretShop requires explicit item/quantity; its five REST operations remain pending retirement verification. Home recovery requires explicit Wake after the sleep duration elapses. Catalog availability means entry eligibility; unconnected commands return 501. Known results survive refresh failure with GET-only recovery, with no idempotent replay guarantee. See the [command contract](docs/architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions).

---

## Immediate Priorities (Next Actions)

See [`ROADMAP.md`](ROADMAP.md) for remaining milestones and [open Issues](https://github.com/witchcraze/party2re/issues) for detailed scope/dependencies.

1. **Client/Agent Gateway**: Finish command coverage and retire REST routes after verified replacements exist (#947–#950). The initial Deposit → Sleep → explicit Wake → Adventure loop is covered by stateful HTTP integration tests, including GET-only recovery after context refresh failures; the broader E2E framework remains separate.
2. **Headless E2E Gameplay Simulation**: Deterministic multi-turn gameplay loop verification (Issue #650).
3. **Client Presentation & Web UI**: Browser client and Server-Driven UI (Issue #140).
4. **Production Asset Pipeline & Final Licensing**: Production asset mapping and license attribution catalog (Issues #654, #729; specification pending).

---

## Confirmed & Pending Decisions

- **Confirmed**: Go initial language, modular monolith, small Core, independent Battle engine, MariaDB persistence, Valkey worker queue/ephemeral lobbies, zero legacy code/asset reuse, legacy behavioral parity as the reconstruction target, not a completed certification. Known differences are recorded in [the documentation audit](docs/migration/documentation-audit.md).
- **Pending**: Frontend framework, final software license (MIT/Apache-2.0/AGPLv3), final creative asset licenses (Creative Commons).

---

## Document References

- `AGENTS.md` & `.agents/rules/` — mandatory agent and developer constraints.
- `docs/architecture/` — permanent software architecture.
- `docs/design/` — permanent game/domain design specifications.
- `ROADMAP.md` — phase and future-work planning.
- `docs/migration/feature-inventory.md` — frozen reconstruction snapshot.
