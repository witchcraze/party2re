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

## 9. File Sizing and Package Cohesion
To keep files readable, maintainable, and within effective token limits for AI pair programming:
- **Target File Size**: Production Go source files should target ≤ 500 lines of code, with a soft limit of 800 lines.
- **Decomposition by Responsibility**: When a domain file approaches 800–1,000 lines (e.g., monolithic services handling exploration, encounters, combat resolution, and state updates all in one file), it MUST be decomposed into focused peer files within the same package (e.g., `service.go`, `combat.go`, `exploration.go`, `repository.go`).
- **Maintain Package Cohesion**: Keep related sub-responsibilities within the same Go package unless clear layer or domain boundaries justify a new package. Splitting across peer files retains package-private visibility while improving navigability.

