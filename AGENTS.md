# AGENTS.md (Agent Operating Rules Index)

This file serves as the root index for AI coding agents and human developers contributing to the Party2 reconstruction project.

To prevent context window flooding, the monolithic rules previously stored here have been split into modular, context-specific rule files located in the `.agents/rules/` directory. Antigravity and other agentic tools will automatically apply these rules based on the context of the task.

## Available Rule Modules

- **`.agents/rules/00-migration-constraints.md`**: Absolute constraints for transitioning from the original Party2 implementation (Clean-room rules, IP protection).
- **`.agents/rules/01-development-workflow.md`**: Rules for Branching, PRs, Issue Templates, TDD, Token Budgeting, and Definition of Done.
- **`.agents/rules/02-documentation-sync.md`**: Rules for maintaining the Single Source of Truth (SSOT) and synchronizing documents within Pull Requests.
- **`.agents/rules/03-architecture.md`**: Guidelines for system architecture, modular monolith design, layer boundaries, and API connections.
- **`.agents/rules/04-domain-modeling.md`**: Guidelines for modeling game logic, combat, progression, and scheduled actions.
- **`.agents/rules/05-database-and-caching.md`**: Guidelines for database transaction boundaries (Unit of Work), pessimistic locking, and appropriate usage of Valkey (Redis).
- **`.agents/rules/06-security.md`**: Guidelines for security reviews, authorization, input validation, and preventing common vulnerabilities.
- **`.agents/rules/99-poc-repository-intelligence.md`**: *(PoC / Experimental)* Guidelines for managing the Guidance Layer (.arch/*.json), agent navigation, and autonomous improvement.

## Document hierarchy

Use the documents according to these roles:

- `AGENTS.md` & `.agents/rules/*.md` — mandatory rules for agents and development.
- `README.md` — human-facing project introduction.
- `STATUS.md` — current project state; what is true now.
- `ROADMAP.md` — planned work and phase progression.
- `docs/architecture/` — enduring software architecture.
- `docs/design/` — enduring game/domain design.
- `docs/development/` — enduring development procedures.
- `.github/ISSUE_TEMPLATE/` — mandatory ticket/review formats.

The distinction is important:

```text
AGENTS.md (and .agents/rules/)
  rules
    |
    +--> docs/architecture/   how the software is structured
    +--> docs/design/         what the game means
    +--> docs/development/    how work is performed
    |
    +--> STATUS.md            current state
    +--> ROADMAP.md           future work
```

Do not use `STATUS.md` or `ROADMAP.md` as substitutes for permanent architecture/design documentation.

## Historical Context

The original `AGENTS.md` contained "Reproducible design rationale" explaining the initial Phase 0–2 design work. This information is preserved in Git history but is no longer required for day-to-day feature development.
