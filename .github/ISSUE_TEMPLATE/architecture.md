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

- [ ] Documents keep only: approved decisions and their rationale; unresolved specifications and known gaps linked to open Issues; verified rules not yet implemented; non-obvious findings (source defects, unreachable or broken branches, misleading names or values) with legacy line references
- [ ] Documents contain none of: content already expressed by code, tests or OpenAPI (formulas, parameters, shapes, error codes, counts); hand-maintained lists that have a registry or schema; status, progress or completion history, including links to closed Issues/PRs that only say where something was implemented; a second copy of a rule (link its owner); legacy line references that only locate a routine findable by name

## Relationships & Dependencies

<!-- Manage dependencies and parent/sub-issue relationships natively:
     - `gh issue edit <id> --add-blocked-by <id>`
     - `gh issue edit <id> --parent <parent-id>`
     Explain why prerequisites are needed, which requirements/contracts to adopt,
     and how to verify them; reference the prerequisite Issue or owning document.
     Do not repeat relationship-only lists, progress/resolution state, or full shared
     specifications. Describe related Issues only where their relevance needs explanation. -->
