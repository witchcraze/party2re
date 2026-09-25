# Status

Last updated: Issue #832 — Collection: Implement Weapon and Armor Catalogs and wire comp_wea/comp_arm Hall of Fame induction

## Current Phase

**Version 1.0 Reconstruction / Refactoring — In Progress (Phase 5+)**

All Version 1.0 foundational systems, core combat, 39 feature modules, and the HTTP JSON API (289 paths / 311 operations, OpenAPI 3.1) are implemented in clean-room Go (1.26.7) with 0 legacy code reuse.

- **Component Architecture & Boundaries**: Authoritative responsibilities, dependencies, and lock hierarchy tiers reside in [`docs/architecture/components.md`](docs/architecture/components.md).
- **Completed Feature History**: Comprehensive issue-level traceability resides in [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md).
- **Domain Design Specifications**: Language-agnostic rules, formulas, and state transitions reside in [`docs/design/README.md`](docs/design/README.md).

---

## Architecture & System Snapshot (What is True Now)

- **Modular Monolith**: Go standard library HTTP routing with modular wire composition (`cmd/party2/wire.go`).
- **HTTP Transport & Edge Policy**: Standardized success and error response envelopes (`SuccessResponse[T]`, `StructuredErrorResponse`), HATEOAS action resolution (`ActionURLResolver`), trusted proxy CIDR allowlist (`PARTY2_TRUSTED_PROXIES`) with right-to-left forwarding header traversal for spoof-proof rate limiting, safe direct exposure default via `RemoteAddr`, and aligned CORS preflight methods (`GET, POST, PUT, DELETE, OPTIONS`) and headers (`Content-Type, Authorization, X-Admin-Key`).
- **Durable Persistence**: MariaDB Master (Migrations `001`–`091`) with deterministic row-lock hierarchy (Rank 0→8) and ambient transaction propagation (`database.RunInTx`).
- **Transient State Architecture**: Ephemeral Turn & Session Lobby Architecture (Candidate C) across multiplayer domains (Casino, PvP, GvG, Party) in Valkey Master, In-Progress Run Buffers (Candidate D), and Shared Boss HP (Candidate E). SSOT: [`docs/architecture/valkey-keyspace.md`](docs/architecture/valkey-keyspace.md).
- **Lifecycle & Infrastructure Contracts**: Fail-fast startup validation with timeout-bounded connectivity checks for MariaDB and Valkey (`cmd/party2`), zero silent in-memory production fallbacks, and deterministic teardown of allocated resources upon boot failure.
- **AST Static Verification & CI Gates**: Automated linters enforce lock ordering, transaction runners, interface segregation (ISP), file size (≤500 lines), Valkey keyspace, error-swallow prohibition, and presentation markup decoupling (`presentation_lint_test.go`); non-mutating CI gates enforce modular OpenAPI source synchronization (`sync_openapi --check`) and route coverage.

---

## Immediate Priorities (Next Actions)

See [`ROADMAP.md`](ROADMAP.md) for full milestone details.

1. **Client Presentation & Web UI**: Browser client and presentation layer (Issue #140).
2. **Production Asset Pipeline & Final Licensing**: Production asset mapping and license attribution catalog (Issues #143, #202).

---

## Confirmed & Pending Decisions

- **Confirmed**: Go initial language, modular monolith, small Core, independent Battle engine, MariaDB persistence, Valkey worker queue/ephemeral lobbies, zero legacy code/asset reuse, clean-room 1:1 legacy behavioral parity.
- **Pending**: Frontend framework, final software license (MIT/Apache-2.0/AGPLv3), final creative asset licenses (Creative Commons).

---

## Document References

- `AGENTS.md` & `.agents/rules/` — mandatory agent and developer constraints.
- `docs/architecture/` — permanent software architecture.
- `docs/design/` — permanent game/domain design specifications.
- `ROADMAP.md` — phase and future-work planning.
- `docs/migration/feature-inventory.md` — Version 1.0 feature completion inventory.
