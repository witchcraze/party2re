# Status

Last reviewed: Issue #991 — Guild/Roles: Persist the normalized title used during validation

## Current Phase

**Version 1.0 Reconstruction / Refactoring — In Progress (Phase 5+)**

The Go project contains the foundational systems, core combat, 39 feature modules, and the HTTP JSON API (294 paths / 317 operations, OpenAPI 3.1), implemented in clean-room Go (1.26.7) with 0 legacy code reuse.

- **Component Architecture & Boundaries**: Authoritative responsibilities, dependencies, and lock hierarchy tiers reside in [`docs/architecture/components.md`](docs/architecture/components.md).
- **Completed Feature History**: Comprehensive issue-level traceability resides in [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md).
- **Domain Design Specifications**: Language-agnostic rules, formulas, and state transitions reside in [`docs/design/README.md`](docs/design/README.md).

---

## Architecture & System Snapshot (What is True Now)

- **Modular Monolith**: Go stdlib HTTP routing with modular wire composition (`cmd/party2/wire.go`).
- **HTTP Transport & Edge Policy**: RESTful JSON API with structured error responses, HATEOAS actions, trusted proxy rate limiting, and centralized Unicode NFC sanitization (`internal/validation`).
- **Durable Persistence**: MariaDB Master (Migrations `001`–`092`) with deterministic row-lock hierarchy (Rank 0→8) and ambient transaction propagation (`database.RunInTx`).
- **Transient State**: Valkey Master for ephemeral lobbies & turns (Candidate C), run buffers (Candidate D), actor-indexed unfinished ScheduledActions (Pending/Processing). SSOT: [`docs/architecture/valkey-keyspace.md`](docs/architecture/valkey-keyspace.md).
- **Lifecycle Contracts**: Fail-fast startup checks for MariaDB/Valkey with clean resource teardown on error.
- **AST Gates & CI**: Automated AST linters (locks, tx runners, ISP, file size ≤500 lines, Valkey keyspace, presentation decoupling), action catalog OpenAPI drift detector (`catalog_lint_test.go`), and non-mutating OpenAPI sync checks (`sync_openapi --check`). SSOT: [`docs/development/ast-linters.md`](docs/development/ast-linters.md).
- **PlayerContext**: Uncached Character/ScheduledAction/Sleep observations and ordered six-gate ActionID evaluation; initial town scene and positive-gold entry filtering. HTTP `GET /context` remains #939.

---

## Immediate Priorities (Next Actions)

See [`ROADMAP.md`](ROADMAP.md) for full milestone details.

1. **Client/Agent Gateway & CQRS Architecture**: `GET /context` (#939), `POST /actions` Gateway (#646), and phased purge of legacy REST routes (#947–#950). (Action Catalog defined in #944, Drift detection in #946; availability engine and Scheduling reads implemented in #938/#972).
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
- `docs/migration/feature-inventory.md` — Version 1.0 feature completion inventory.
