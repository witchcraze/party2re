---
name: Bug
about: Report incorrect or unexpected behavior
title: "[Bug] <Domain>: "
labels: ["bug"]
---

## Problem

<!-- What is wrong? -->

> [!NOTE]
> **Ponytail Architecture Guidance**:
> - **Short Core Directive**: `<Directive: e.g. Adopt Shared Primitive depot.FindOrCreate / Zero-Abstraction Guard>`
> - **Do NOT**: `<Anti-patterns to avoid: e.g. Create custom bootstrapping functions or ad-hoc wrappers>`
> - **Do INSTEAD**: `<Shortest-path minimal route: e.g. In internal/...:line, replace manual query with canonical helper>`
> - **Specific Code Targets**: `<file:lines, wire.go>`
> - **Architecture Considerations Linkage**: `- [x] No architectural impact (<Ponytail rationale: e.g. 5-line diff, zero new abstractions>)`

## Affected Components

- **Packages / Modules**: `internal/<module>`
- **Database Tables**: `<table_name>` (or "None")
- **Legacy Reference**: `<file>.cgi` (if applicable)

## Expected behavior

<!-- What should happen? -->

## Actual behavior

<!-- What happens instead? -->

## Reproduction

<!-- Give the smallest reliable reproduction. -->

1. 
2. 
3. 

## Acceptance criteria

- [ ] Expected behavior is restored.

## Tests

<!-- Identify the relevant tests or regression coverage. -->

## Prerequisites & Feasibility

<!-- Are underlying domain models / infrastructure available? If not, detail prerequisite options. -->

- [ ] Feasible with existing codebase capabilities
- [ ] Requires prerequisite infrastructure / specification alignment (detail below)

## Documentation impact

<!-- Identify explanations/contracts this fix may make outdated under
     .agents/rules/02-documentation-sync.md, or state that none apply. -->

- [ ] None
- [ ] Update affected explanations/contracts within implementation PR — list documents below

Documentation content check ([rule 02](https://github.com/witchcraze/party2re/blob/main/.agents/rules/02-documentation-sync.md)):

- [ ] Documents keep only enduring information their owner is responsible for under the AGENTS.md document hierarchy: necessary contracts, decisions and rationale, procedures and unresolved specifications; `docs/design/` keeps only what rule 02 §2's design document content criteria allow
- [ ] Documents contain no implementation detail already expressed by source, tests or generated specifications, no second copy of a rule (link its owner), no reproducible inventories, and no status, progress or completion history owned by GitHub

## Architecture considerations

- [ ] No architectural impact
- [ ] Architectural impact — explain below

## Relationships & Dependencies

<!-- Manage dependencies and parent/sub-issue relationships natively:
     - `gh issue edit <id> --add-blocked-by <id>`
     - `gh issue edit <id> --parent <parent-id>`
     Explain why prerequisites are needed, which requirements/contracts to adopt,
     and how to verify them; reference the prerequisite Issue or owning document.
     Do not repeat relationship-only lists, progress/resolution state, or full shared
     specifications. Describe related Issues only where their relevance needs explanation. -->
