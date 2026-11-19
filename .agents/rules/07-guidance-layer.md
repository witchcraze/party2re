---
name: Guidance Layer Rules
description: Rules for the Guidance Layer (.arch/*.json): authority, module selection, anchors and verification.
---

# Guidance Layer Rules

Background, tiers and the current inventory: [`docs/architecture/guidance-layer.md`](../../docs/architecture/guidance-layer.md).

## 1. Authority
- Production source determines what the implementation executes; tests and comments are evidence, not proof. Game rules follow `00-migration-constraints.md`.
- `.arch/*.json` is a navigation index, not an answer key. MUST verify the code at each `source_ref` before acting on it.

## 2. Module selection
A module gets `.arch/modules/<module>.json` only when it meets at least two of:
1. **C1** spans 2+ entity tables within one `RunInTx` boundary;
2. **C2** acquires deterministic `SELECT ... FOR UPDATE` locks (multi-table order or sorted IDs);
3. **C3** manages P2P transfers, escrow, listings or shared guild state;
4. **C4** integrates with Valkey delayed queues or background workers.

Priority tiers do not waive C1–C4. Stateless utilities and `core/*` get no module index.

## 3. Anchors and shared tables
- `source_ref` MUST use symbol anchors: `path/to/file.go#Symbol` or `#Type.Method`.
- Before refactoring a shared entity, MUST inspect its `.arch/shared_tables/<table>.json` when present, and MUST search source callers beyond it; absence from an index does not imply absence of dependency.

## 4. Updating `.arch`
- Update an index in the same PR as the transaction it describes. Symbol and interface names MUST match the Go declarations.
- MUST state whether a use case owns `RunInTx`, calls `ExecuteTransaction` or delegates settlement, and record its actual lock acquisition order separately from ordinary reads/writes. MUST NOT label a delegating entry point atomic over its whole workflow.
- Exported methods that lock or mutate across tables SHOULD carry `Transaction:` and `Lock Order:` lines in their Go doc comment.
- `internal/architecture/arch_test.go` checks symbols and selected transaction calls only. MUST verify table names, lock sequences, delegation and completeness against source.
- New guidance artifact types or tier changes go through an Architecture Issue.
