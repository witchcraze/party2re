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
- **`.agents/rules/07-guidance-layer.md`**: Guidelines for managing the Guidance Layer (.arch/*.json), agent navigation, and automated verification.

## Document hierarchy

Use the documents according to these roles:

- `AGENTS.md` & `.agents/rules/*.md` — mandatory rules for agents and development.
- `README.md` — human-facing project introduction.
- `STATUS.md` — major current capabilities, limitations and immediate gaps.
- `ROADMAP.md` — remaining milestones and planned direction.
- `docs/architecture/` — enduring software architecture.
- `docs/design/` — enduring game/domain design.
- `docs/development/` — enduring development procedures.
- `docs/migration/feature-inventory.md` — frozen reconstruction snapshot; historical reference.
- GitHub Issues / PRs and Git history — task progress and detailed change history.
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

Update the owning documents when a change makes their explanations outdated, as defined in `.agents/rules/02-documentation-sync.md`. Do not append completion records for each Issue/PR or routinely update the frozen inventory. Use [`docs/README.md`](docs/README.md) to locate current contracts, gaps and historical evidence.

### Rules vs. Architecture Docs — Placement Criteria

`.agents/rules/` MUST contain **prescriptive constraints only**:
- Prescriptive rules directly governing agent behavior via MUST / NEVER / BANNED
- Decision criteria and checklists required for judgment (e.g. decision trees)

Place the following in `docs/architecture/`, NOT in `.agents/rules/`:
- Design rationale and explanations of "why" constraints exist
- Historical migration context and decision records
- Detailed implementation patterns already enforced mechanically by linters

## Historical Context

The original `AGENTS.md` contained "Reproducible design rationale" explaining the initial Phase 0–2 design work. This information is preserved in Git history but is no longer required for day-to-day feature development.
