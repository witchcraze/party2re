---
name: Guidance Layer Rules
description: Guidelines for managing the Guidance Layer (.arch/*.json), module selection criteria, agent navigation, and automated verification.
---

# Guidance Layer Principles

## 1. Ground Truth vs. Guidance Layer
- **Verify Implementation in Source**:
  Production source code determines what the current implementation executes. Tests and comments are evidence to inspect, not proof that game behavior is correct. During reconstruction, MUST use the original project's behavior as the game specification authority under `00-migration-constraints.md`.
- **`.arch/` is the Guidance Layer (Navigation Index)**: 
  `.arch/*.json` files are not an authoritative answer key, but an index and navigation layer designed to guide agents to where answers reside without requiring brute-force scans of the entire repository.
- **Always Verify via `source_ref`**:
  Agents must never treat `.arch` metadata as unquestionable fact without verifying the actual code linked by `source_ref` (file path and symbol anchor) before making decisions or changes.

## 2. Module Selection Criteria & Scope Tiers
To prevent documentation rot and maintain zero unnecessary overhead, module-level definitions (`.arch/modules/<module>.json`) are governed by strict selection criteria (see [`docs/architecture/guidance-layer.md`](../../docs/architecture/guidance-layer.md)):

### A. Selection Criteria (C1 - C4)
A module qualifies for a dedicated `.arch/modules/<module>.json` file only if it meets at least **two** of the following conditions:
1. **C1 (Transaction Depth)**: Spans 2+ distinct entity tables within a single `RunInTx` boundary.
2. **C2 (Lock Hierarchy)**: Implements deterministic `SELECT ... FOR UPDATE` pessimistic locking (multi-table order or sorted ID locks).
3. **C3 (Shared / Escrow State)**: Manages player-to-player transfers, escrow balances, listings, or shared guild states.
4. **C4 (Async Scheduling)**: Integrates with Valkey delayed job queues and background execution workers.

### B. Scope Tiers
- **Tier 1 (Priority navigation)**:
  MUST prioritize `tavern`, `auction`, `guild`, `shop`, `blacksmith`, `adventure`, and `store` when investigating transactions. A priority designation MUST NOT imply that a module-level JSON exists or waive C1–C4. Depot mailing belongs to `internal/depot`; MUST NOT infer an `internal/delivery` module from the feature name.
- **Tier 2 (On-Demand - authored when modified)**:
  `bank`, `alchemy`, `dungeon`, `casino`, `medal`, `depot`, `activity`, `monster`, `plantation`, `park`, `home`, `collection`, `chapel`, `secretshop`, `blackmarket`, `gemstore`, `fleamarket`, `eventplaza`, `pvp`, `gvg`, `boss`, `rescue`. A single-row bank savings update does not justify duplicating its repository in a module index.
- **Tier 3 (Out-of-Scope - No module-level JSON)**:
  Stateless utilities (`id`, `pagination`, `validation`, `logging`, `ratelimit`, `valkey`) and core domain entities (`core/*`).

## 3. Navigation Anchors: Symbol-First Standard
To ensure definitions remain immutable against everyday code refactorings and line shifts:
- **Module & Shared Table `source_ref` uses Symbol Anchors**:
  Format: `path/to/file.go#SymbolName` or `path/to/file.go#Struct.Method` (e.g., `internal/tavern/tavern.go#Service.OrderMeal` or `internal/tavern/tavern.go#CharacterRepository`).

## 4. Reverse Fan-in Shared Table Index (.arch/shared_tables/)
Agents MUST inspect the available `.arch/shared_tables/<table_name>.json` (`characters`, `inventory_items`, `guilds`) before refactoring shared domain entities. MUST inspect source callers beyond these partial indices; absence from an index does not imply absence of dependency. Bank savings live in `characters.deposit`, not a separate `bank_accounts` table. See [`docs/architecture/guidance-layer.md`](../../docs/architecture/guidance-layer.md).

## 5. Automated Mechanical Verification
Referenced symbols and transaction declarations MUST pass the Go AST checks in `internal/architecture/arch_test.go` on `make check` / `go test ./...`. Agents MUST separately verify table names, lock sequences, delegated calls, and completeness against source; those semantic properties are not proven by the symbol check. See [`docs/architecture/guidance-layer.md`](../../docs/architecture/guidance-layer.md).

## 6. Go Idiom & Implementation Compatibility Guidelines
To maintain idiomatic Go design while maximizing Guidance Layer navigability:
1. **Consumer-Defined Interfaces (`Accept interfaces, return structs`)**:
   - Modules declare external dependencies as Go interfaces in their own package (e.g., `internal/tavern/tavern.go` defines `CharacterRepository`).
   - `.arch` references these interface symbols directly, ensuring decoupled architecture.
2. **Explicit Use-Case Methods & Transaction Boundaries**:
   - MUST identify whether a use case owns `RunInTx`, invokes `ExecuteTransaction`, or delegates settlement to an adapter.
   - MUST document the actual lock acquisition sequence separately from ordinary reads/writes; MUST NOT label a delegating entry point as an atomic transaction over its whole workflow.
3. **Self-Documenting Concurrency Docstrings (Go Doc as Ground Truth)**:
   - For exported methods involving pessimistic locking or cross-table mutations, annotate the Go doc comment with transaction semantics:
     ```go
     // OrderMeal executes meal purchase and immediate fullness/stat replenishment.
     // Transaction: RunInTx
     // Lock Order: characters(2) -> tavern_character_status(8)
     func (s *Service) OrderMeal(ctx context.Context, charID string, mealID string) (*MealResult, error)
     ```

## 7. Definition of Done (DoD) for Architecture Updates
When updating `.arch` definitions:
1. `[ ]` **Criteria Check**: Module qualifies under Tier 1 or Tier 2 criteria.
2. `[ ]` **Symbol Exactness**: Interface and method names are copied directly from Go declarations (verified by `arch_test.go`).
3. `[ ]` **System Map Sync**: Updated components match current Go package boundaries.
4. `[ ]` **Local Verification**: `./scripts/verify.sh` passes all 7 check suites.

## 8. Governance & Continuous Evolution Process
The Guidance Layer is an evolvable system maintained via the GitHub issue lifecycle:
1. **Tier Adjustments**: Tier promotions/demotions are proposed via Architecture Issues when concurrency characteristics change.
2. **New Guidance Types**: Extended artifacts (e.g., shared table reverse indices, worker state graphs) can be introduced by updating schemas and linter tests via Architecture PRs.
