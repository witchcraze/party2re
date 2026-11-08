---
name: Architecture decision
about: Propose a significant architectural change or decision
title: "[Architecture] <Domain>: "
labels: ["architecture"]
---

## Problem

<!-- What architectural problem needs to be solved? -->

## Affected Components

- **Packages / Modules**: `internal/<module>`
- **Database Tables**: `<table_name>` (or "None")

## Context

<!-- Explain the relevant current architecture and constraints. -->

## Proposed change

<!-- What should change? -->

## Alternatives considered

<!-- What other approaches were considered? -->

## Trade-offs

<!-- What are the important benefits, costs, and risks? -->

## Impact

<!-- Describe affected components, interfaces, features, dependencies, or implementation boundaries as applicable. -->

## Acceptance criteria

- [ ] 

## Documentation

<!-- Which architecture/design documents need to be updated? -->

Documentation content check ([rule 02](https://github.com/witchcraze/party2re/blob/main/.agents/rules/02-documentation-sync.md)):

- [ ] Documents keep only enduring information their owner is responsible for under the AGENTS.md document hierarchy: necessary contracts, decisions and rationale, procedures and unresolved specifications; `docs/design/` keeps only what rule 02 §2's design document content criteria allow
- [ ] Documents contain no implementation detail already expressed by source, tests or generated specifications, no second copy of a rule (link its owner), no reproducible inventories, and no status, progress or completion history owned by GitHub

## Relationships & Dependencies

<!-- Manage dependencies and parent/sub-issue relationships natively:
     - `gh issue edit <id> --add-blocked-by <id>`
     - `gh issue edit <id> --parent <parent-id>`
     Explain why prerequisites are needed, which requirements/contracts to adopt,
     and how to verify them; reference the prerequisite Issue or owning document.
     Do not repeat relationship-only lists, progress/resolution state, or full shared
     specifications. Describe related Issues only where their relevance needs explanation. -->
