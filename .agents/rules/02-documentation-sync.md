---
name: Documentation Sync Rules
description: Rules for maintaining the Single Source of Truth (SSOT) and synchronizing documents within Pull Requests.
---

# Documentation Maintenance Principles

## 1. Single Source of Truth (SSOT)
- `README.md` — Human-facing project introduction.
- `STATUS.md` — Major current capabilities, limitations and immediate gaps.
- `ROADMAP.md` — Remaining milestones and planned direction.
- `docs/architecture/` — Enduring software architecture.
- `docs/design/` — Enduring game/domain design.
- `docs/development/` — Enduring development procedures.
- `docs/migration/feature-inventory.md` — Frozen reconstruction snapshot; historical reference only.
- GitHub Issues / PRs and Git history — Task progress and detailed change history.

Do not use `STATUS.md` or `ROADMAP.md` as substitutes for permanent architecture/design documentation.

During reconstruction, game specifications MUST be checked against the original project under `00-migration-constraints.md`. MUST distinguish legacy requirements, current implementation, and planned changes; NEVER rewrite a correct legacy rule to match an incorrect implementation. See [`docs/README.md`](../../docs/README.md) for document ownership.

## 2. Documentation Role and Timing
- **Design Docs are not Implementation Details:** Documentation in `docs/design/` should represent enduring design and domain rules (e.g., game formulas, system boundaries, core behavior). It does not need to perfectly mirror 100% of the implementation details (like private helpers, internal data structures, or SQL queries).
- **Recording Domain Rules:** When discovering important game rules or formulas during the clean-room investigation, actively record these language-agnostic rules in `docs/design/` as valuable project assets.
- **Avoid Pre-emptive Detailed Tech Specs:** While domain rules should be documented early, do not pre-draft overly rigid technical specification files (e.g., defining exact structs and function signatures in markdown) before writing code. Let the detailed technical boundaries settle through TDD and code, then finalize the documentation.

## 3. Update Affected Explanations within PR

- Assess whether the change makes an existing explanation outdated or introduces a contract that needs permanent documentation. Update the owning documents in the same PR.
- Update design documents when game rules/formulas/state transitions change; architecture documents when responsibilities/contracts/dependencies change; development documents when procedures change.
- Update STATUS only when its summary of major capabilities, limitations or immediate gaps changes; update ROADMAP when remaining milestones or direction change.
- Do not require a fixed list of documents to be reviewed or edited for every feature. A change that leaves their explanations accurate requires no document edit; state that briefly in the PR when applicable.
- Do not duplicate live command coverage, task progress or completion histories in architecture/design documents. Use current OpenAPI/source for registered contracts and STATUS/GitHub for current coverage and gaps. Retain evidence links where they support a specification or design decision.
- Do not routinely append or update feature-inventory.md. Preserve its dated snapshot and existing records. Any correction MUST describe an error in the historical snapshot rather than silently updating it to today's state.
- Keep unresolved specifications, missing coverage, unverified behavior and known differences discoverable through their owning documents or linked open Issues. Freezing a historical inventory MUST NOT mark its unfinished work complete or imply full parity.

## 4. Avoiding Documentation Bloat (STATUS.md)
- **Do not treat STATUS.md as an append-only changelog.**
- When an immediate gap is resolved, remove or revise that gap in STATUS. Record detailed completion history in GitHub/Git; do not move completed tasks into ROADMAP or the frozen inventory.
- `STATUS.md` must remain a slim, accurate snapshot of the *current* state and *immediate next* priorities, not an unbounded historical record.

## 5. OpenAPI Specification Synchronization (SSOT & Modular Paths)
- **Source of Truth:**
  - Base metadata, reusable schemas, and security schemes reside in `docs/api/base.json`.
  - Endpoint path operations reside in modular files: `docs/api/paths/{module}.json` (e.g., `character.json`, `shop.json`).
- **NEVER edit compiled artifacts directly:**
  - Do NOT edit `docs/api/openapi.json` or `internal/api/http/openapi.json` directly. These are compiled build artifacts generated deterministically by `scripts/sync_openapi/`.
- **Synchronization Workflow:**
  - Whenever an HTTP route is added or changed in `internal/api/http/handler.go`, update its modular specification; use `make openapi-scaffold` only when missing-route scaffolding is needed.
  - Modify the modular path specification in `docs/api/paths/{module}.json`.
  - Run `make openapi-sync` to recompile both artifacts.
  - CI enforces 100% route coverage and synchronization via `make openapi-check`.
