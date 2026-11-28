---
name: Architecture and Component Rules
description: Rules for modular monolith boundaries, layers, shared components, configuration, file sizing, runtime primitives and recovery.
---

# Architecture Rules

Section numbers are cited by `internal/architecture` lint messages; keep them stable.

## 1. Feature Expansion
New features MUST NOT require modifying core or unrelated features. Prefer isolated modules, stable contracts and data-driven content; avoid God objects, feature branches in core and access to another feature's internal state.

## 2. Modular Monolith
One repository, one process. Microservices are banned without a concrete requirement.

## 3. Small Core
Core holds only shared concepts (Player, Character, Stats, Progression, Items, Time). Core MUST NOT import feature packages.

## 4. Feature Boundaries
Features own their rules and state and MAY use public contracts of core and domain components (e.g. Battle). Feature packages MUST NOT import peer features or `internal/database`; communicate through public contracts, `core/event.Dispatcher` or the composition root (`cmd/party2/`), and persist through repository interfaces. Enforced by `package_boundary_lint_test.go`.

## 5. UI and API Layer Boundary
- Handlers and UI MUST NOT contain game logic, math or complex validation; operations go through the UI-independent service layer. Authorization is also enforced at the service boundary (`06-security.md` §2).
- Handlers and middleware MUST propagate `r.Context()`/`ctx`, never `context.Background()`/`TODO()` (`context_lint_test.go`).
- Domain services (`internal/*` except `internal/api/http`) MUST return structured data, never HTML, NPC dialogue or display formatting; HTTP formats with `writeSuccess`/`writeAppError` or transport DTOs (`presentation_lint_test.go`).
- New interactive clients MUST use the Gateway (`get_character_context`, `execute_character_action`) in [`client-agent-api.md`](../../docs/architecture/client-agent-api.md), consult the [OpenAPI spec](../../docs/api/openapi.json) for registered routes, and MUST NOT infer command support from `available_actions` eligibility. REST routes retire only through the tracked migration after verified replacement.

## 6. Architecture Review Triggers
Create an Issue before changing core responsibilities, component boundaries, dependency direction, public contracts, persistence architecture or external API architecture.

## 7. Shared Components
- `util`/`common`/generic `helper` packages are banned; use single-responsibility packages (`internal/id`, `internal/pagination`, `internal/validation`). `internal/helperquest` is the Helper Quest gameplay feature, not a utility.
- Extract general utilities at three duplicating modules; centralize security (`withAuthenticatedCharacter`) and RNG (`internal/core/random`; no `math/rand` in production, `rand_lint_test.go`) immediately.
- Shared core entity writes use `internal/database` persistence helpers; core evaluations (e.g. `item.Slot.Kind()`) live on core types; overflow-prone currency/quantity math uses `economy.SafeMultiply`/`SafeAdd`.

## 8. Configuration
- Packages take a typed `Config` struct or explicit arguments; constructors MUST NOT read environment variables. Parsing lives in a dedicated loader (`ConfigFromEnvironment`, `DefaultConfig`) or `cmd/party2/config.go`. Defaults work for local development without required variables except secrets and connection addresses.
- A PR that adds, renames or removes a production environment variable, or changes its default, alias or invalid-value behavior, MUST update [`environment-variables.md`](../../docs/development/environment-variables.md) (required ones also in the [README Compose example](../../README.md#docker-compose-での起動例)) with default, aliases and invalid-value behavior. Use placeholders for secrets; never real values. Test/tooling-only variables are not listed. Dynamic names need a literal or `//lint:ignore env-docs <reason>` (`env_docs_lint_test.go`).

## 9. File Size and Cohesion
- Production Go files under `internal/` and `cmd/` ≤ 500 lines, except `main.go` under `cmd/` ≤ 150 (`file_size_lint_test.go`); interfaces ≤ 10 direct methods, composed by embedding (`interface_size_lint_test.go`). Oversized legacy entries are ratcheted: never grow them; remove an entry once it is under the limit.
- Split large files into peer files by responsibility within the same package; add packages only for real layer boundaries.

## 10. Runtime Primitives
- Single-character money/medal/inventory mutations MUST use `economy.TransactionRunner` (`ExecuteTransaction`, `economy.Run[T]`).
- P2P and multi-aggregate operations MUST take an injected `TransactionProvider` (`RunInTx(ctx, fn)`) and follow the lock order in `05-database-and-caching.md`.
- Feature packages MUST NOT call raw `database.RunInTx` or hold `*sql.DB` (`tx_runner_lint_test.go`, `lock_hierarchy_lint_test.go`).
- Domain events use `internal/core/event.Dispatcher` two-phase dispatch ([`cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md)).

## 11. Battle Adapter
Combat features MUST build participants through `battle.ParticipantBuilder` (`internal/battle.Service`) or `battle.BuildParticipantFromData`, never `corebattle.NewParticipantFromCharacter*`/`FromCharacter`, which skip equipment, combat items, passives and orbs (`battle_adapter_lint_test.go`).

## 12. Cancellation
Production delays, retries and polling MUST honor `ctx.Done()` (`select` with `time.After`, timers, tickers); `time.Sleep` is banned (`sleep_lint_test.go`).

## 13. Recoverability
- Prefer a simple normal path with safe automatic recovery over speculative edge-case branches, without weakening domain behavior, validation, authorization, transactions or asset conservation.
- When failure can leave partial state, define how it converges: safe re-execution, reconciliation from the authoritative store or lifecycle cleanup, preferably on next access or in existing workers. No new recovery subsystem without concrete need.
- Keep the minimum durable evidence needed to resume; recovery MUST be idempotent, preserve concurrent updates and never duplicate rewards or charges, and MUST NOT create a competing source of truth.
- Repair automatically only what authoritative data determines; never guess, silently reset assets, drop unfinished work or hide persistent failure. Retries are bounded and cancellable.
- Test a representative interrupted state and repeated recovery, not a hypothetical combination matrix.
