---
name: Architecture and Component Rules
description: Guidelines for system architecture, modular monolith design, layer boundaries, and API connections.
---

# Architecture Principles

## 1. Feature Expansion is a Primary Goal
New features should be possible without unnecessarily modifying the core or unrelated features. Extensibility must come from clear boundaries, not from abstraction for its own sake.
- Prefer: isolated feature modules, explicit responsibilities, stable contracts, data-driven content.
- Avoid: growing a central God object, feature-specific branches scattered through core code, reaching into another feature's internal implementation.

## 2. Start as a Modular Monolith
Use a single repository and initially prefer a single application/process where practical. Logical component boundaries are required, but physical service separation (microservices) is not. Do not introduce microservices unless a concrete requirement justifies them.

## 3. Core Must Remain Small
The Core contains only concepts genuinely shared across the game (e.g., Player, Character, Stats, Progression, Items, Time). Feature-specific concepts belong in their respective feature module. Do not put feature-specific logic into Core merely for convenience.

## 4. Features are First-Class Components
Features (Adventure, Guild, Casino, Alchemy, etc.) should own their feature-specific rules and state. A feature may depend on public contracts of Core or Domain components (like Battle), but **must not access another Feature Module's private implementation or persistence layer.**
- **Prohibition of Cross-Feature Direct Imports**: Feature packages MUST NOT import peer feature packages directly. Inter-feature communication must route through public API contracts, `core/event.Dispatcher`, or composition roots (`cmd/party2/`).
- **Prohibition of Direct Database Imports**: Feature packages MUST NOT import `internal/database` directly. Persistence must be accessed strictly through domain repository interfaces.
- **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/package_boundary_lint_test.go`) during `make check` and CI.


## 5. UI and API Layer Boundary
- **No Domain Logic in HTTP Handlers:** Do not put game logic, math, or complex validations directly in HTTP handlers or GUI components.
- **Service Layer Abstraction:** Major game operations should pass through the UI-independent application API / Service layer boundary so they can be tested and alternative clients can be added later.
- **Authorization Depth:** While the HTTP layer parses JSON and extracts session tokens, **authorization logic** (ensuring `PlayerID` owns the character) should be enforced deeply at the Service/Domain boundary to prevent bypasses when called from other contexts.
- **No Detached Root Contexts in Handlers:** HTTP handlers and middleware functions MUST NOT instantiate detached root contexts (`context.Background()` or `context.TODO()`). Discarding `r.Context()` drops client disconnect signals, timeouts, session identities, and ambient transaction scopes. Handlers MUST always propagate `r.Context()` or the incoming `ctx`.
  - **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/api/http/context_lint_test.go`) during `make check` and CI.

## 6. Architecture Review Triggers
Do not silently make substantial architectural decisions. Create an Issue if the work would change: Core responsibilities, component boundaries, dependency direction, public contracts, persistence architecture, or external API architecture.

## 7. Common Components and DRY Guidelines
- **No Monolithic `util`/`common` Packages**: Do not create generic "junk-drawer" packages (`util` / `common`). Instead, place shared logic in single-responsibility, focused packages (e.g. `internal/id`, `internal/pagination`, `internal/validation`, `internal/api/http/middleware`).
- **Rule of Three for General Utilities**: For general logic, formatting, and mathematical operations, prefer local implementation until duplication occurs across 3+ modules. Then extract to a dedicated shared package to avoid premature abstraction.
- **Immediate Centralization for Security & Concurrency**: Security enforcement (session authentication, character ownership validation wrappers like `withAuthenticatedCharacter`) and concurrency-critical utilities (thread-safe RNG via `internal/core/random`) MUST be centralized and reused immediately across all endpoints. Never duplicate auth or random state logic locally. Direct imports of `math/rand` in production packages are prohibited.
  - **Continuous Mechanical Verification**: Enforced automatically via Go AST static analysis (`internal/architecture/rand_lint_test.go`) during `make check` and CI.
- **Shared Entity Persistence**: Repositories mutating shared Core entities (e.g. character stats, money, level, medals) must use centralized persistence helpers in `internal/database` rather than maintaining scattered raw SQL update queries across multiple repository files.

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
  - **Deterministic Lock Hierarchy Enforcement**: Any P2P or multi-aggregate transaction MUST strictly adhere to the global pessimistic lock hierarchy (Rank 0 → Rank 8) defined in `.agents/rules/05-database-and-caching.md`:
    - Rank 0: Shared peer entity (e.g. `flea_market_items`, `parcels`, `auction_listings`)
    - Rank 2: Character entities — multiple characters MUST be locked in ascending lexicographical order via `id.Sort2(charID1, charID2)`.
    - Rank 3: Inventory items (`inventory_items`).
    - Rank 5: Depot storage (`character_depots`).
    - Rank 8: Secondary domain feature records (`gem_boxes`, `player_stores`, etc.).
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


