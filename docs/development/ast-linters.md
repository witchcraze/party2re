# Mechanical AST Linting Architecture

## 1. Overview

Party2 maintains an in-tree suite of automated Go AST (Abstract Syntax Tree) linters implemented directly as standard Go unit tests. These linters run with zero external binary dependencies (no `golangci-lint`, no `errcheck`, no third-party installations) and execute instantaneously on any host via `go test` and `make check` / `make arch-lint`.

The purpose of these linters is to enforce enduring architectural invariants, security rules, and code cleanliness mechanically, eliminating reviewer fatigue and preventing structural regressions during fast-paced agentic and human development.

## 2. In-Tree AST Linter Catalog

### 2.1 Core Architectural Linters (`internal/architecture/`)

| Linter Test File | Enforced Rule & Invariant | Motivation & Scope |
| :--- | :--- | :--- |
| **`error_swallow_lint_test.go`** | Prohibits silent error suppression (`_ =`, `_, _ =`, `val, _ :=`, or unassigned expressions) on repository, store, database gateway, container mutation, and currency mutation calls. | Prevents silent data loss, ledger desynchronization, and transactional corruption across all production packages in `internal/`. Legitimate post-commit cache purges and defer compensations must be explicitly documented with `//lint:ignore error-swallow <reason>`. |
| **`arch_test.go`** | Validates Guidance Layer coordinates (`.arch/*.json`) against actual Go package ASTs. | Guarantees that documented module boundaries, exposed symbols, and shared database tables in `.arch` remain 100% synchronized with real code. |
| **`battle_adapter_lint_test.go`** | Prohibits features outside `internal/battle` from constructing battle participants directly from raw characters. | Enforces battle subsystem encapsulation through the dedicated adapter layer. |
| **`crypto_lint_test.go`** | Enforces `crypto/rand` for cryptographic or security-sensitive token/hash generation. | Prevents pseudo-random predictability in security credentials, session tokens, and passwords. |
| **`deadcode_lint_test.go`** | Detects zero-caller domain methods and repository operations. | Ensures obsolete, refactored, or superseded code is immediately purged rather than accumulating rot. |
| **`file_size_lint_test.go`** | Enforces a strict upper limit of ≤ 500 lines per Go source file. | Prevents monolithic file creep and forces high modularity and cohesion. |
| **`interface_size_lint_test.go`** | Restricts consumer-defined interface sizes (≤ 10 methods). | Prevents fat interfaces and ensures lean, mockable dependencies conforming to interface segregation. |
| **`package_boundary_lint_test.go`** | Enforces layered architectural boundaries (e.g. domain layers cannot import HTTP, database layers cannot import presentation). | Keeps system architecture strictly acyclic and modular. |
| **`rand_lint_test.go`** | Prohibits unseeded or non-deterministic random number generators in battle/progression calculations. | Guarantees reproducible, seedable simulation and combat execution. |
| **`sleep_lint_test.go`** | Prohibits raw `time.Sleep` calls in production packages. | Prevents thread blocking and latency spikes in async services. |
| **`tx_runner_lint_test.go`** | Restricts direct transaction management (`RunInTx`, `BeginTx`) to authorized coordinator packages. | Prevents nested transaction hazards and chaotic commit boundaries. |
| **`unused_definitions_lint_test.go`** | Identifies unreferenced top-level constants, unmarshaled DTOs, and orphaned types. | Enforces continuous codebase cleanliness. |
| **`valkey_lint_test.go`** | Validates Valkey keyspace naming formats against the canonical taxonomy (`party2:<namespace>:<entity>[:<id>]`). | Prevents key namespace collisions and untracked cache usage. |

### 2.2 Subsystem-Specific AST Linters

- **`internal/database/lock_hierarchy_lint_test.go`**: Validates pessimistic lock hierarchy (Rank 0 -> Rank 8) across multi-table transactions to prevent deadlocks (`make lock-lint`).
- **`internal/database/tx_lint_test.go`**: Prohibits direct `r.db.BeginTx` in repositories, mandating `database.ExecutorFromContext(ctx, r.db)`.
- **`internal/api/http/auth_lint_test.go`**: Validates that mutating HTTP endpoints wrap execution with standard authentication helpers (`withAuthenticatedCharacter`, `withAuthenticatedCharacterAndJSON`).
- **`internal/core/core_lint_test.go`**: Enforces strict mutation encapsulation for core entities (e.g. currency must be mutated via `AddMoney` / `DeductMoney` rather than direct field assignment).

## 3. The Error-Swallow Linter Architecture

### 3.1 Why Naive Matching Fails

A common anti-pattern when building AST linters is naive method-name matching (e.g., flagging any call to `.Save()`, `.Update()`, or `.AddMoney()`). In a complex codebase:
1. **False Positives**: In-memory structs or helper types often have methods like `Add()` or `Update()` where returning a value or error is context-dependent, or standard library helpers like `strconv.Atoi()` return multiple values.
2. **Fatal False Negatives**: Critical database calls like `s.repo.DeductBetAndCreditPayout(...)` or `s.repo.GetRecord(...)` have custom method names or return multiple values (`rec, _ := ...`). Naive matching completely misses them.

### 3.2 Receiver-Based Persistence & Mutation Detection

The Party2 error-swallow linter (`internal/architecture/error_swallow_lint_test.go`) utilizes a two-tier evaluation strategy:

```text
                  ┌── Is Receiver Persistent Gateway?
                  │   (repo, store, database, characters, inventories,
                  │    depots, adventures, contests, standings, quests,
Call Expression ──┤    guilds, accounts, parcels, auctions, listings,
                  │    orders, sales, plots, monsters, parties, updater)
                  │
                  ├── Is Container Mutation?
                  │   (inv.Add, depot.AddItem, box.ConsumeItem)
                  │
                  └── Is State Mutation Method?
                      (AddMoney, DeductMoney, SpendMoney, AddSmallMedals,
                       DeductSmallMedals, AddCrystal, DeductCrystal,
                       RecordMatchSettlement)
```

1. **Persistent Gateways**: Any call to a receiver matching database, repository, or storage gateways (`isRepoReceiver`) is inspected. If its error return is swallowed via `_ =`, multi-value blank assignment (`_, _ =`, `val, _ :=`), or an unassigned expression statement (`ast.ExprStmt`), it is flagged immediately.
2. **Container & Asset Mutations**: Container item additions/removals and wallet/currency operations must always validate return errors (e.g. capacity limits, negative amounts, overflow).

### 3.3 The `//lint:ignore` Directive

When a call is legitimately non-fatal or compensatory, developers and agents must annotate the call with an explicit lint ignore comment:

```go
//lint:ignore error-swallow best-effort post-commit cache eviction
_ = s.activeStore.DeleteActiveSession(ctx, characterID)
```

The linter inspects `ast.File.Comments` and allows suppression ONLY when:
- The comment is positioned immediately preceding the statement or inline on the same line.
- The comment explicitly specifies the rule name (`error-swallow`) followed by a non-empty human-readable rationale.

## 4. Developer Guide: Authoring an In-Tree AST Linter

### 4.1 Basic Structure

All AST linters follow a standardized test structure using Go's standard library packages `go/parser`, `go/ast`, and `go/token`:

```go
package architecture_test

import (
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestMyArchitectureRule(t *testing.T) {
    fset := token.NewFileSet()
    repoRoot := "../.."
    internalDir := filepath.Join(repoRoot, "internal")

    err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return err
        }
        if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
            return nil
        }

        src, err := os.ReadFile(path)
        if err != nil {
            return err
        }

        node, err := parser.ParseFile(fset, path, src, parser.ParseComments)
        if err != nil {
            return err
        }

        ast.Inspect(node, func(n ast.Node) bool {
            // Traverse AST nodes (e.g., ast.CallExpr, ast.AssignStmt)
            return true
        })

        return nil
    })

    if err != nil {
        t.Fatalf("walk failed: %v", err)
    }
}
```

### 4.2 Best Practices

1. **Always Provide Synthetic Unit Tests (`Test*Lint_UnitTests`)**:
   Implement a companion test with synthetic code strings testing both positive violations and negative non-violations. This guarantees the linter logic does not regress when modified.
2. **Keep Zero External Dependencies**:
   Never import external static analysis tools or binaries. Rely exclusively on `go/ast`, `go/parser`, `go/token`, and stdlib utilities.
3. **Include Actionable Error Messages**:
   When reporting a violation via `t.Errorf`, always emit:
   - Relative file path and 1-based line number (`file.go:123`).
   - The exact offending call or symbol (`s.repo.Save`).
   - The relevant rule documentation reference and instructions on how to fix or annotate the code.
4. **Wire into `make arch-lint`**:
   Ensure new architectural lint tests are placed under `internal/architecture/` so they are automatically included when running `make arch-lint` and `make check`.
