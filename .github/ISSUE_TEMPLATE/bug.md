---
name: Bug
about: Report incorrect or unexpected behavior
title: "[Bug] <Domain>: "
labels: ["bug"]
---

## Problem

<!-- What is wrong? -->

> [!NOTE]
> **Ponytail Architecture Guidance (Jules-Ready Standard)**:
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

## Architecture considerations

- [ ] No architectural impact
- [ ] Architectural impact — explain below

## Related issues

<!-- Link related Issues or regressions. -->
