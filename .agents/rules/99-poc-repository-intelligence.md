---
name: Repository Intelligence (PoC) Rules
description: Guidelines for managing the Guidance Layer (.arch/*.json), module selection criteria, agent navigation, and automated verification.
---

# Repository Intelligence & Guidance Layer Principles (PoC)

## 1. Ground Truth vs. Guidance Layer
- **Source Code & Comments are the Ground Truth**: 
  Production source code, tests, and inline comments constitute the absolute, definitive truth of the system.
- **`.arch/` is the Guidance Layer (Navigation Index)**: 
  `.arch/*.json` files are not an authoritative answer key, but an index and navigation layer designed to guide agents to where answers reside without requiring brute-force scans of the entire repository.
- **Always Verify via `source_ref`**:
  Agents must never treat `.arch` metadata as unquestionable fact without verifying the actual code linked by `source_ref` (file path and symbol anchor) before making decisions or changes.

## 2. Module Selection Criteria & Scope Tiers
To prevent documentation rot and maintain zero unnecessary overhead, module-level definitions (`.arch/modules/<module>.json`) are governed by strict selection criteria (see `docs/architecture/guidance-layer.md`):

### A. Selection Criteria (C1 - C4)
A module qualifies for a dedicated `.arch/modules/<module>.json` file only if it meets at least **two** of the following conditions:
1. **C1 (Transaction Depth)**: Spans 2+ distinct entity tables within a single `RunInTx` boundary.
2. **C2 (Lock Hierarchy)**: Implements deterministic `SELECT ... FOR UPDATE` pessimistic locking (multi-table order or sorted ID locks).
3. **C3 (Shared / Escrow State)**: Manages player-to-player transfers, escrow balances, listings, or shared guild states.
4. **C4 (Async Scheduling)**: Integrates with Valkey delayed job queues and background execution workers.

### B. Scope Tiers
- **Tier 1 (High-Leverage Priority - 8 modules)**:
  `tavern` (active), `delivery` (active), `bank`, `auction`, `guild`, `shop`, `blacksmith`, `adventure`.
  *These represent the high-concurrency, high-risk core features.*
- **Tier 2 (On-Demand - authored when modified)**:
  `alchemy`, `dungeon`, `casino`, `medal`, `depot`, `activity`, `farm`, `park`, `home`, `collection`, `chapel`, `secretshop`, `blackmarket`, `eventplaza`, `pvp`, `gvg`, `boss`, `rescue`.
- **Tier 3 (Out-of-Scope - No module-level JSON)**:
  Stateless utilities (`id`, `pagination`, `validation`, `logging`, `ratelimit`, `valkey`) and core domain entities (`core/*`).

## 3. Navigation Anchors: Symbol-First Standard
To ensure definitions remain immutable against everyday code refactorings and line shifts:
- **Module & Shared Table `source_ref` uses Symbol Anchors**:
  Format: `path/to/file.go#SymbolName` or `path/to/file.go#Struct.Method` (e.g., `internal/tavern/tavern.go#Service.OrderMeal` or `internal/tavern/tavern.go#CharacterRepository`).

## 4. Reverse Fan-in Shared Table Index (.arch/shared_tables/)
To assess the blast radius of modifying core database tables and verify global deadlock hierarchies across multiple feature callers without full-codebase grep scans:
- **Scope**: High Fan-in shared database tables (`characters`, `inventory_items`, `bank_accounts`, `guilds`).
- **Structure**: Maps `Table -> Repository Implementation -> Consumer Interfaces -> Caller Feature Methods & Lock Orders`.
- **Zero-Token Blast Radius**: Agents inspect `.arch/shared_tables/<table_name>.json` to locate all mutation points before refactoring shared domain entities.

## 5. Automated Mechanical Verification (Zero Token Overhead)
All architecture definitions and symbol coordinates are mechanically verified during `scripts/verify.sh` and standard `go test ./...`:
1. **Go AST Symbol & Transaction Linter (`internal/architecture/arch_test.go`)**:
   - Executed via `go test ./...` in ~0.05s.
   - Automatically parses all `.arch/modules/*.json` and `.arch/shared_tables/*.json` files and verifies using `go/parser` that every referenced interface, struct, consumer interface, and caller method symbol actually exists in the codebase.
   - Verifies that all symbols declared with `transaction_type: "RunInTx"` or `tx_mode: "RunInTx"` physically contain `RunInTx` calls in their AST bodies.
   - Fails the test immediately if a symbol is misspelled or renamed, preventing broken links with zero token consumption.

## 6. Go Idiom & Implementation Compatibility Guidelines
To maintain idiomatic Go design while maximizing Guidance Layer navigability:
1. **Consumer-Defined Interfaces (`Accept interfaces, return structs`)**:
   - Modules declare external dependencies as Go interfaces in their own package (e.g., `internal/tavern/tavern.go` defines `CharacterRepository`).
   - `.arch` references these interface symbols directly, ensuring decoupled architecture.
2. **Explicit Use-Case Methods & Single Transaction Boundaries**:
   - Each use-case method (e.g., `OrderMeal`, `SendParcel`) acts as a single transaction boundary (`RunInTx`).
   - Locking and mutations execute in explicit linear sequence within the method body.
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
