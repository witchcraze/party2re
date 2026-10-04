---
name: Architecture and Component Rules
description: Guidelines for system architecture, modular monolith design, layer boundaries, and API connections.
---

# Architecture Principles

## 1. Feature Expansion is a Primary Goal
New features should be possible without modifying core or unrelated features. Extensibility comes from clear boundaries, not premature abstraction.
- Prefer: isolated feature modules, explicit responsibilities, stable contracts, data-driven content.
- Avoid: growing a central God object, feature-specific branches in core code, accessing another feature's internal state.

## 2. Start as a Modular Monolith
Single repository and single application process. Logical component boundaries are required, but physical service separation (microservices) is banned unless justified by concrete requirements.

## 3. Core Must Remain Small
Core contains only genuinely shared concepts (Player, Character, Stats, Progression, Items, Time). Feature-specific concepts belong strictly in their respective feature module.

## 4. Features are First-Class Components
Features (Adventure, Guild, Casino, Alchemy, etc.) own their feature-specific rules and state. A feature may depend on public contracts of Core or Domain components (like Battle), but **must not access another Feature Module's private implementation or persistence layer.**
- **Prohibition of Cross-Feature Direct Imports**: Feature packages MUST NOT import peer feature packages directly. Inter-feature communication must route through public API contracts, `core/event.Dispatcher`, or composition roots (`cmd/party2/`).
- **Prohibition of Direct Database Imports**: Feature packages MUST NOT import `internal/database` directly. Persistence must be accessed strictly through domain repository interfaces.
- **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/package_boundary_lint_test.go`) during `make check` and CI.


## 5. UI and API Layer Boundary
- **No Domain Logic in HTTP Handlers:** Do not put game logic, math, or complex validations directly in HTTP handlers or GUI components.
- **Service Layer Abstraction:** Major game operations should pass through the UI-independent application API / Service layer boundary so they can be tested and alternative clients can be added later.
- **Authorization Depth:** While the HTTP layer parses JSON and extracts session tokens, **authorization logic** (ensuring `PlayerID` owns the character) should be enforced deeply at the Service/Domain boundary to prevent bypasses when called from other contexts.
- **No Detached Root Contexts in Handlers:** HTTP handlers and middleware functions MUST NOT instantiate detached root contexts (`context.Background()` or `context.TODO()`). Discarding `r.Context()` drops client disconnect signals, timeouts, session identities, and ambient transaction scopes. Handlers MUST always propagate `r.Context()` or the incoming `ctx`.
  - **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/api/http/context_lint_test.go`) during `make check` and CI.
- **Presentation Decoupling & UI-Agnostic Domain Services:** Backend domain services (`internal/*` excluding `internal/api/http`) MUST NEVER return HTML markup (`<b>`, `<br>`, etc.), human-facing NPC dialogue, or layout-specific formatting strings. All rendering, dialogue formatting, and presentation styling belong strictly to the presentation/client layer. Domain service results MUST return pure, structured data (models, counts, facts, or boolean flags like `SentToDepot`). HTTP handlers format presentation messages using standardized envelopes (`writeSuccess` and `writeAppError`) or transport response DTOs.
  - **Continuous Mechanical Verification**: Enforced automatically across all domain packages via Go AST static analysis (`internal/architecture/presentation_lint_test.go`) with ratcheting whitelist (`whitelistedLegacyPresentationFields`) during `make check` and CI.
- **Client/Agent Gateway & CQRS Interaction Model:** New interactive clients MUST target the planned two-pillar Gateway (`get_character_context`, `execute_character_action`) described in [`docs/architecture/client-agent-api.md`](../../docs/architecture/client-agent-api.md). Until #939 and #646 implement its HTTP transport, MUST use the current OpenAPI specification when calling the running server; MUST NOT assume the planned `/api/v1/characters/{id}/context` and `/actions` routes are registered. Existing REST routes MUST be retired only through the tracked #947–#950 migration.

## 6. Architecture Review Triggers
Do not silently make substantial architectural decisions. Create an Issue if the work would change: Core responsibilities, component boundaries, dependency direction, public contracts, persistence architecture, or external API architecture.

## 7. Common Components and DRY Guidelines
- **No Monolithic `util`/`common` Packages**: Generic "junk-drawer" packages are BANNED. Shared logic belongs in single-responsibility packages (`internal/id`, `internal/pagination`, `internal/validation`).
- **No Generic "Helper" Packages & Domain Identity of `internal/helperquest`**: Generic "helper" packages are strictly forbidden. The package `internal/helperquest` represents the legacy Party2 Helper Quests (`lib/helper.cgi`) and is a gameplay feature domain package, not a utility module.
- **Rule of Three for General Utilities**: Prefer local implementation until duplication occurs across 3+ modules before extracting shared utilities.
- **Immediate Centralization for Security & Concurrency**: Security enforcement (`withAuthenticatedCharacter`) and thread-safe RNG (`internal/core/random`) MUST be centralized immediately. Direct imports of `math/rand` in production packages are prohibited (verified by `rand_lint_test.go`).
- **Shared Entity Persistence**: Repositories mutating shared Core entities must use centralized persistence helpers in `internal/database` rather than scattered raw SQL updates.
- **Encapsulation of Core Domain Attributes**: Core domain evaluations (e.g. `item.Slot.Kind()`) MUST be encapsulated directly on domain types (`internal/core/*`), never reconstructed via ad-hoc switches in feature services.
- **Centralized Safe Arithmetic**: Currency/quantity arithmetic subject to integer overflow MUST use `internal/economy` (`economy.SafeMultiply`, `economy.SafeAdd`). Local duplicate overflow checks are prohibited.

## 8. Configuration & Environment Variable Boundaries
To preserve testability, decouple packages from global runtime state, and prevent hidden configuration dependencies:
- **Config Struct First**: Packages requiring configuration parameters MUST define a pure configuration struct (e.g., `pkg.Config`) containing typed fields with zero dependencies on `os.Getenv` or environment variables.
- **Constructors Accept Config**: Service/repository constructors MUST accept config structs or explicit arguments (e.g., `NewService(cfg Config, ...)`), and MUST NOT call `os.Getenv` or load secrets/paths directly within constructors.
- **Isolated Environment Loaders**: Environment variable parsing MUST be isolated to dedicated loader functions (e.g., `pkg.ConfigFromEnvironment()` or `pkg.DefaultConfig()`), or handled entirely at the composition root (`cmd/party2/config.go`).
- **Test-Friendly Defaults**: Provide sane default values for local development and testing without requiring mandatory environment variables unless strictly necessary for secrets or external connection addresses.

## 9. File Sizing, Package Cohesion, and Mechanical Linting
To keep files readable, maintainable, and within effective token limits for AI pair programming:
- **Mechanical Line Limits**: Enforced automatically via `internal/architecture/file_size_lint_test.go` on `make check` and `make arch-lint`:
  - Production Go files (`internal/**/*.go`): **≤ 500 lines** (excluding tests).
  - Application entry points (`cmd/*/main.go`): **≤ 150 lines**.
- **Interface Sizing & Segregation (ISP)**: Enforced automatically via `internal/architecture/interface_size_lint_test.go` on `make check` and `make arch-lint`:
  - Domain interfaces must declare **≤ 10 direct methods** (`maxDirectInterfaceMethods`).
  - Monolithic interfaces violating ISP must be decomposed into cohesive sub-interfaces (focused on specific sub-aggregates / operations) and assembled via interface embedding (composition).
  - Embedded interfaces are excluded from direct method counts, actively incentivizing composition.
  - Pre-existing oversized interfaces are ratcheted down in `whitelistedLegacyInterfaceLimits` as they are refactored.
- **Ratcheting Whitelist**: Pre-existing oversized files are locked at their historical line count. Any growth beyond the baseline fails CI. When a file is decomposed below 500 lines, it must be removed from the whitelist to lock in the improvement.
- **Decomposition by Responsibility**: When a domain file approaches 500 lines, decompose it into focused peer files within the same package (e.g., `service.go`, `session.go`, `step.go`, `repository.go`).
- **Maintain Package Cohesion**: Keep related sub-responsibilities within the same Go package unless clear layer boundaries justify a new package. Splitting across peer files retains package-private visibility while improving navigability.

## 10. Cross-Domain Application Runtime Primitives
- **Transactional Mutation Boundaries**:
  - **Single-Character Currency & Inventory Operations**: Operations modifying a single character's wallet, medals, or inventory items MUST route through `economy.TransactionRunner` (`ExecuteTransaction` or `economy.Run[T]`). Feature services MUST NOT roll their own ad-hoc transaction orchestration or row locking for single-character currency/inventory mutations.
  - **Peer-to-Peer (P2P) and Multi-Aggregate Operations**: Operations orchestrating transfers between multiple characters (e.g. sender & receiver, buyer & seller) or mutating multiple domain aggregates across storage/table boundaries (e.g. `character_depots`, `gem_boxes`, `flea_market_items`, `player_stores`, `auction_listings`) MUST inject a transaction boundary provider interface (`TransactionProvider` / `RunInTx(ctx, fn) error`, matching `economy.TransactionProvider`).
  - **Deterministic Lock Hierarchy Enforcement**: Any P2P or multi-aggregate transaction MUST strictly adhere to the global pessimistic lock hierarchy (Rank 0 → Rank 8) defined in [`.agents/rules/05-database-and-caching.md`](05-database-and-caching.md) and [`docs/architecture/cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md).
  - **Prohibition of Direct Infrastructure Coupling**: Feature packages MUST NOT import `internal/database` directly to invoke raw `database.RunInTx(ctx, db, ...)` or hold raw `*sql.DB` / `db` identifiers. All database transaction boundaries must be injected via interfaces.
  - **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/tx_runner_lint_test.go` and `internal/database/lock_hierarchy_lint_test.go`) during `make check` and CI.
- **Event Dispatcher**: Domain events MUST use `internal/core/event.Dispatcher` two-phase dispatch. See [`docs/architecture/cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md) for architecture, lock order enforcement, and migration examples.

## 11. Combat Cluster Boundaries & Battle Adapter Enforcement
- **Prohibition of Direct `corebattle.NewParticipantFromCharacter`**: Feature modules (`adventure`, `boss`, `pvp`, `gvg`, `dungeon`, `challenge`, `party`, etc.) MUST NOT construct combat participants directly from Core character models via `corebattle.NewParticipantFromCharacter`, `corebattle.NewParticipantFromCharacterWithHP`, or `FromCharacter`. Direct generation bypasses equipment stat calculation (weapons, armors, accessories), combat-time consumable items, passive triggers (e.g. `ItemToukiShield`, `ItemDokuroAmulet`, `ItemCursedTalisman`), and status orbs, producing "naked combatants".
- **Mandatory Battle Adapter Routing**: All combat features MUST inject `battle.ParticipantBuilder` (implemented by `internal/battle.Service`) or use `battle.BuildParticipantFromData` to ensure complete, authentic character combatant construction.
- **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/battle_adapter_lint_test.go`) during `make check`, `make arch-lint`, and CI.

## 12. Concurrency and Cooperative Cancellation
- **Context-Aware Delays and Prohibiting Raw `time.Sleep`**: Backend services and workers must always support cooperative cancellation via `context.Context`. Raw `time.Sleep` calls block goroutines unconditionally, ignoring server graceful shutdown signals, container termination deadlines, and request timeout/cancellation events.
- **Mandatory Pattern**: Any delay, retry backoff, or polling interval in production services MUST listen to `ctx.Done()` (e.g. via `select { case <-ctx.Done(): return ctx.Err(); case <-time.After(delay): }` or `time.NewTicker` / `time.NewTimer`). Direct calls to `time.Sleep` in production code are strictly prohibited.
- **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/sleep_lint_test.go`) during `make check` and CI.

