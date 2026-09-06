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

## 5. UI and API Layer Boundary
- **No Domain Logic in HTTP Handlers:** Do not put game logic, math, or complex validations directly in HTTP handlers or GUI components.
- **Service Layer Abstraction:** Major game operations should pass through the UI-independent application API / Service layer boundary so they can be tested and alternative clients can be added later.
- **Authorization Depth:** While the HTTP layer parses JSON and extracts session tokens, **authorization logic** (ensuring `PlayerID` owns the character) should be enforced deeply at the Service/Domain boundary to prevent bypasses when called from other contexts.

## 6. Architecture Review Triggers
Do not silently make substantial architectural decisions. Create an Issue if the work would change: Core responsibilities, component boundaries, dependency direction, public contracts, persistence architecture, or external API architecture.

## 7. Common Components and DRY Guidelines
- **No Monolithic `util`/`common` Packages**: Do not create generic "junk-drawer" packages (`util` / `common`). Instead, place shared logic in single-responsibility, focused packages (e.g. `internal/id`, `internal/pagination`, `internal/validation`, `internal/api/http/middleware`).
- **Rule of Three for General Utilities**: For general logic, formatting, and mathematical operations, prefer local implementation until duplication occurs across 3+ modules. Then extract to a dedicated shared package to avoid premature abstraction.
- **Immediate Centralization for Security & Concurrency**: Security enforcement (session authentication, character ownership validation wrappers like `withAuthenticatedCharacter`) and concurrency-critical utilities (thread-safe RNG) MUST be centralized and reused immediately across all endpoints. Never duplicate auth or random state logic locally.
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
- **Ratcheting Whitelist**: Pre-existing oversized files are locked at their historical line count. Any growth beyond the baseline fails CI. When a file is decomposed below 500 lines, it must be removed from the whitelist to lock in the improvement.
- **Decomposition by Responsibility**: When a domain file approaches 500 lines, decompose it into focused peer files within the same package (e.g., `service.go`, `session.go`, `step.go`, `repository.go`).
- **Maintain Package Cohesion**: Keep related sub-responsibilities within the same Go package unless clear layer boundaries justify a new package. Splitting across peer files retains package-private visibility while improving navigability.

## 10. Cross-Domain Application Runtime Primitives
To prevent lock inversions, deadlocks, and boundary condition bugs across feature domains:
- **Universal Transaction Runner (`internal/economy.TransactionRunner`)**: Feature operations requiring currency deductions (Gold, Small Medals), inventory item consumption, or multi-resource atomic state mutations must route through `ExecuteTransaction` or `economy.Run[T]`.
- **Mechanical Lock Hierarchy Guarantee**: The runner automatically enforces the global deterministic lock order (`characters` Rank 2 -> `inventory_items` Rank 3 -> secondary domain tables Rank 8). Handlers and feature services must not manually acquire disparate row locks outside this sequence.
- **Strict Pre-Condition Boundary Enforcement**: Currency balances (`char.Money >= cost.Gold`, `char.SmallMedals >= cost.SmallMedals`) and item inventory quantities are verified and deducted atomically before invoking domain business logic.
- **Two-Phase Domain Event Dispatching (`internal/core/event.Dispatcher`)**: Domain events emitted within transaction context (`tc.EmitEvent`) execute in two distinct phases:
  - **Phase 1 (In-Tx Synchronous)**: Dispatched prior to SQL commit; any handler error aborts and rolls back the transaction (ACID consistency).
  - **Phase 2 (Post-Commit Asynchronous)**: Dispatched in separate goroutines after successful commit with at-most-once delivery (resilient side effects like activity logs or announcements).

