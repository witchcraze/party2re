---
name: Documentation Sync Rules
description: Rules for maintaining the Single Source of Truth (SSOT) and synchronizing documents within Pull Requests.
---

# Documentation Maintenance Principles

## 1. Single Source of Truth (SSOT)
- `README.md` — Human-facing project introduction.
- `STATUS.md` — Current project state; what is true now.
- `ROADMAP.md` — Planned work and phase progression.
- `docs/architecture/` — Enduring software architecture.
- `docs/design/` — Enduring game/domain design.

Do not use `STATUS.md` or `ROADMAP.md` as substitutes for permanent architecture/design documentation.

## 2. Documentation Role and Timing
- **Design Docs are not Implementation Details:** Documentation in `docs/design/` should represent enduring design and domain rules (e.g., game formulas, system boundaries, core behavior). It does not need to perfectly mirror 100% of the implementation details (like private helpers, internal data structures, or SQL queries).
- **Recording Domain Rules:** When discovering important game rules or formulas during the clean-room investigation, actively record these language-agnostic rules in `docs/design/` as valuable project assets.
- **Avoid Pre-emptive Detailed Tech Specs:** While domain rules should be documented early, do not pre-draft overly rigid technical specification files (e.g., defining exact structs and function signatures in markdown) before writing code. Let the detailed technical boundaries settle through TDD and code, then finalize the documentation.

## 3. Feature Documentation Sync within PR
When opening a feature or domain change PR, **always synchronize in that same PR**:
1. `docs/design/<feature>.md` — language-agnostic rules, formulas, and state transitions.
2. `docs/architecture/components.md` — component responsibilities.
3. `docs/migration/feature-inventory.md` — feature tracking status.
4. `STATUS.md` — update the current state summary and clear completed priorities.

## 4. Avoiding Documentation Bloat (STATUS.md)
- **Do not treat STATUS.md as an append-only changelog.**
- When a task in "Current Priorities" is completed, **remove it** or move it to `ROADMAP.md` / `feature-inventory.md`.
- `STATUS.md` must remain a slim, accurate snapshot of the *current* state and *immediate next* priorities, not an unbounded historical record.
