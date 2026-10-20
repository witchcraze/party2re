---
name: Documentation Sync Rules
description: Rules for maintaining the Single Source of Truth (SSOT) and synchronizing documents within Pull Requests.
---

# Documentation Maintenance Principles

## 1. Single Source of Truth (SSOT)
- MUST use the document ownership defined in [`AGENTS.md`](../../AGENTS.md#document-hierarchy) and indexed in [`docs/README.md`](../../docs/README.md). Keep one owning explanation per contract; other documents and skills MUST link to it rather than copy it.
- During reconstruction, MUST check game specifications against the original project under [`00-migration-constraints.md`](00-migration-constraints.md). MUST distinguish legacy requirements, current implementation and planned changes; NEVER rewrite a correct legacy rule to match an incorrect implementation.

## 2. Documentation Role and Timing
- **Minimum Prose for Implemented Behavior:** For details already expressed in source, tests or generated specifications, MUST retain only the minimum explanation needed to understand responsibilities, public contracts, domain rules and non-obvious rationale. Link to the owning source when needed; do not reproduce private helpers, internal data structures, SQL or execution traces in prose.
- **Recording Domain Rules:** Record newly verified, enduring game rules or formulas in their owning `docs/design/` section with source evidence; merge with existing explanations rather than append investigation reports.
- **Avoid Pre-emptive Detailed Tech Specs:** While domain rules should be documented early, do not pre-draft overly rigid technical specification files (e.g., defining exact structs and function signatures in markdown) before writing code. Let the detailed technical boundaries settle through TDD and code, then finalize the documentation.

## 3. Update Affected Explanations within PR

- Assess whether the change makes an existing explanation outdated or introduces a contract that needs permanent documentation. Update the owning documents in the same PR.
- Update design documents when game rules/formulas/state transitions change; architecture documents when responsibilities/contracts/dependencies change; development documents when procedures change.
- Update STATUS only when its summary of major capabilities, limitations or immediate gaps changes; update ROADMAP when remaining milestones or direction change.
- Do not require a fixed list of documents to be reviewed or edited for every feature. A change that leaves their explanations accurate requires no document edit; state that briefly in the PR when applicable.
- Do not duplicate live command coverage, task progress or completion histories in architecture/design documents. Use current OpenAPI/source for registered contracts and STATUS/GitHub for current coverage and gaps. Maintained documents MUST NOT retain Issue/PR links merely identifying where behavior was implemented or completed; retain references only when needed for specification decisions, legacy evidence or unresolved work.
- Do not routinely append or update feature-inventory.md. Preserve its dated snapshot and existing records. Any correction MUST describe an error in the historical snapshot rather than silently updating it to today's state.
- Keep unresolved specifications, missing coverage, unverified behavior and known differences discoverable through their owning documents or linked open Issues. Freezing a historical inventory MUST NOT mark its unfinished work complete or imply full parity.

## 4. Consolidation and Addition Criteria
- When editing documents, MUST rewrite the owning section as one current explanation, replacing or consolidating superseded/duplicate text rather than appending implementation reports. Keep cleanup within the active objective and its directly affected references.
- When further documentation cleanup opportunities are found outside the active objective, MUST search for an existing open Issue and create a dedicated documentation-cleanup Issue or refine an existing one under `01-development-workflow.md`. Identify concrete target sections, the redundancy or implementation detail to remove, and the contracts/evidence to preserve; do not require a repository-wide sweep to find candidates.
- Add a section or document only for a distinct enduring contract, decision or procedure that existing owners cannot explain clearly. Issue-specific investigation, completion records and temporary cautions belong in Issues/PRs; private implementation details and reproducible inventories belong in source, tests or generated artifacts.
- Add an agent rule only for a cross-cutting constraint or decision criterion needed in future work. A local bug normally needs a regression test, not a repository rule. For mechanically enforced constraints, keep the actionable requirement and link to the check; do not copy its implementation or exception lists into prose.
- Before deleting or consolidating, MUST inspect contents and inbound references. Preserve unique requirements, approved decisions, legacy evidence and unresolved gaps in their owner or a linked Issue; update affected indexes, links and guidance references. Age or line count alone MUST NOT justify deletion.
- MUST remove or revise resolved STATUS gaps; keep it a slim current snapshot. Do not move completion history into ROADMAP or the frozen inventory.
- In the PR's existing documentation assessment, briefly explain meaningful consolidation or necessary growth and its owner. Do not require deletion quotas, a fixed document sweep or a separate maintenance log.

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
