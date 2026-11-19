---
name: Documentation Sync Rules
description: Rules for document ownership, content criteria, same-PR updates, consolidation and OpenAPI sources.
---

# Documentation Rules

## 1. Single Source of Truth
- Use the ownership in [`AGENTS.md`](../../AGENTS.md#document-hierarchy), indexed in [`docs/README.md`](../../docs/README.md). One owning explanation per contract; other documents and skills MUST link to it, not copy it.
- Check game specifications against the original under `00-migration-constraints.md`. Distinguish legacy requirement, current implementation and planned change; NEVER rewrite a correct legacy rule to match the implementation.

## 2. Documentation Role and Timing
- For behavior expressed in source, tests or generated specs, keep only the minimum needed to understand responsibilities, public contracts, domain rules and non-obvious rationale. Do not reproduce private helpers, data structures, SQL or execution traces.
- `docs/design/` MUST keep only what neither the code/tests nor a direct reading of the legacy routine provides:
  - approved decisions and rationale, including intentional departures from legacy;
  - unresolved specifications and known gaps, linked to open Issues;
  - verified rules not yet implemented, with legacy evidence, until code and tests own them;
  - non-obvious findings with legacy line references: source defects, unreachable or broken branches, misleading names or values, values captured or applied at unexpected times.
- MUST NOT restate per-entry formulas, parameters or execution order that code or tests express, or keep legacy line references that only locate a routine findable by name. Merge findings into the owning section.
- Do not pre-draft structs or signatures in markdown before the code settles.

## 3. Same-PR Updates
- Update owning documents in the same PR when a change makes them outdated or adds an enduring contract: design for game rules, architecture for responsibilities and contracts, development for procedures. An unaffected document needs no edit; say so briefly in the PR.
- GitHub Issues and milestones own current gaps, progress and plans. MUST NOT create status/roadmap files, coverage or completion histories, or Issue/PR links that only mark where something was implemented.
- `docs/migration/feature-inventory.md` is a frozen snapshot: do not update it routinely; a correction MUST describe the snapshot's error. Freezing it MUST NOT imply its unfinished work is complete.

## 4. Consolidation and Additions
- Rewrite the owning section as one current explanation; replace superseded or duplicate text instead of appending reports. Keep cleanup within the active objective.
- Cleanup found outside the objective goes to a new or existing documentation Issue naming the target sections, what to remove and what to preserve.
- Add a section or document only for a distinct enduring contract, decision or procedure. Add an agent rule only for a cross-cutting constraint or decision criterion; a local bug needs a regression test. For mechanically enforced rules, state the requirement and link the check.
- Before deleting or consolidating, inspect content and inbound references; preserve unique requirements, decisions, §2 findings and open gaps, and fix links. Age or length alone never justifies deletion.
- Explain meaningful consolidation or growth in the PR's documentation section; no quotas or maintenance logs.

## 5. OpenAPI Specification Synchronization (SSOT & Modular Paths)
- Sources: `docs/api/base.json` (metadata, schemas, security) and `docs/api/paths/{module}.json` (operations).
- NEVER edit the generated `docs/api/openapi.json` or `internal/api/http/openapi.json`.
- When a route in `internal/api/http/handler.go` changes, edit its module file (`make openapi-scaffold` for missing routes) and run `make openapi-sync`. `make openapi-check` enforces coverage and sync.
