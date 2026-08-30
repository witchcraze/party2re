---
name: Repository Intelligence (PoC) Rules
description: Guidelines for managing the Guidance Layer (.arch/*.json), agent navigation, automated verification, and maintenance scope.
---

# Repository Intelligence & Guidance Layer Principles (PoC)

## 1. Ground Truth vs. Guidance Layer
- **Source Code & Comments are the Ground Truth**: 
  Production source code, tests, and inline comments constitute the absolute, definitive truth of the system.
- **`.arch/` is the Guidance Layer (Navigation Index)**: 
  `.arch/*.json` files are not an authoritative answer key, but an index and navigation layer designed to guide agents to where answers reside without requiring brute-force scans of the entire repository.
- **Always Verify via `source_ref`**:
  Agents must never treat `.arch` metadata as unquestionable fact without verifying the actual code linked by `source_ref` (file path and symbol anchor) before making decisions or changes.

## 2. Maintenance Scope & Hierarchy (Core vs. Optional)
To keep maintenance costs negligible and avoid documentation rot:

### A. Core Maintained Scope (Required Baseline)
- **File**: `.arch/system.architecture.json` (System Topology Map)
- **Purpose**: High-level cross-module topology, storage boundaries, shared core domains, and entrypoint.
- **Maintenance Policy**: Must be updated when cross-module boundaries, new primary domains, or storage systems are introduced. Must satisfy Archify Showcase profile constraints (<= 12 primary nodes).

### B. Module Detailed Guidance (Optional / Experimental Examples)
- **Directory**: `.arch/modules/*.json`
- **Current Reference Samples**: `tavern.json`, `delivery.json`
- **Maintenance Policy**: These detailed files serve as reference implementations for fine-grained multi-resource transaction mapping. They are **not mandatory** for every module. Modules may rely solely on clean Go interfaces, single-boundary methods, and Go docstrings unless high concurrency complexity warrants a dedicated JSON definition.

## 3. Navigation Anchors: Symbol-First Standard
To ensure definitions remain immutable against everyday code refactorings and line shifts:
- **Module Level `source_ref` uses Symbol Anchors**:
  Format: `path/to/file.go#SymbolName` or `path/to/file.go#Struct.Method` (e.g., `internal/tavern/tavern.go#Service.OrderMeal` or `internal/tavern/tavern.go#CharacterRepository`).
- **System Level `sources` uses File-Wide Boundaries**:
  Format: `path: "internal/tavern/tavern.go"` covering the file for Archify repository validator compliance.

## 4. Automated Mechanical Verification (Zero Token Overhead)
All architecture definitions and symbol coordinates are mechanically verified during `scripts/verify.sh` and Git `pre-push` hooks:
1. **Archify CLI Layout & Repository Check**:
   - `node ~/.agents/skills/archify/bin/archify.mjs validate architecture .arch/system.architecture.json --quality showcase --repo-root .`
   - Verifies 9 artifact invariants, layout compactness, and Git line validity.
2. **Go AST Symbol Linter (`internal/architecture/arch_test.go`)**:
   - Executed via `go test ./...` in ~0.02s.
   - Automatically parses existing `.arch/modules/*.json` files and verifies using `go/parser` that every referenced interface, struct, and method symbol actually exists in the codebase.
   - Fails the test immediately if a symbol is misspelled or renamed, preventing broken links with zero token consumption.

## 5. Go Idiom & Implementation Compatibility Guidelines
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

## 6. Definition of Done (DoD) for Architecture Updates
When updating `.arch` definitions:
1. `[ ]` **Symbol Exactness**: Interface and method names are copied directly from Go declarations (verified by `arch_test.go`).
2. `[ ]` **System Map Sync**: Updated components match current Go package boundaries.
3. `[ ]` **Local Verification**: `./scripts/verify.sh` passes all 7 check suites.
