# Issue and PR Workflow

## Issue is the unit of work

All substantial development work begins with an Issue.

Use the repository's Issue template. Do not create ad-hoc tickets.

The template captures the work itself: goal, scope, acceptance criteria, tests, and relevant architectural/dependency considerations. Repository-wide rules belong in `AGENTS.md` and should not be duplicated unnecessarily in every ticket.

The Issue should contain:

- problem or goal;
- scope;
- acceptance criteria;
- test requirements;
- architecture considerations;
- out-of-scope items;
- dependencies.

## Two-stage documentation lifecycle

To avoid pre-emptive documentation rework while ensuring permanent design documents remain 100% synchronized with actual code:

1. **Stage 1 — Issue Creation (Pre-implementation)**:
   - Do **not** create or pre-draft specification files (`docs/design/*.md`) before coding.
   - Describe requirements, calculations, and observable behavior directly in the Issue's **Acceptance Criteria**.
2. **Stage 2 — PR Implementation (Post-implementation synchronization)**:
   - Implement the code and unit/integration tests.
   - Once domain rules, formulas, and state transitions are settled, create or update the language-agnostic design specification (`docs/design/<feature>.md`) **within the same implementation PR**.
   - Synchronize `docs/architecture/components.md` and `STATUS.md` in the same PR.
   - **Pre-commit issue re-check**: Re-inspect the active Issue and verify every acceptance criterion and requirement against code, tests, and documentation before committing and opening the PR.

## Pre-commit verification (Active Issue Re-Check)

Before committing changes and submitting a PR:

1. Re-inspect the active Issue in full (`gh issue view <issue-number>`).
2. Cross-check each item in Acceptance Criteria, Scope, and domain requirements against the implemented solution, tests, and synchronized documentation.
3. Verify that there are zero omissions, unaddressed criteria, or dropped edge cases.
4. Only proceed to commit changes once full coverage of the issue requirements is confirmed.

## PR is the unit of review

All code changes go through a PR.

Use the repository's PR template without replacing or bypassing required sections.

The PR template records the change, its verification, and documentation checklist. Mandatory agent rules remain defined by `AGENTS.md`.

A PR should:

- reference its Issue;
- explain the behavior change;
- identify tests;
- include domain design docs (`docs/design/<feature>.md`) and component updates (`docs/architecture/components.md`);
- update `STATUS.md` current state summary (without appending historical changelogs);
- remain within the Issue scope.


## Ticket size

Prefer:

```text
one behavior
+
its tests
+
minimum implementation
```

Large features should be split into multiple Issues.

## Architecture decision tickets

Create a separate Issue when implementation requires a significant architectural decision.

The decision should record:

- problem;
- alternatives;
- trade-offs;
- chosen direction;
- consequences.

## Completion

A ticket is complete only when its acceptance criteria are satisfied, tests pass, review is complete, and the PR is merged.

Do not mark work complete merely because the implementation exists locally.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory Issue/PR rules.
- [`agent-workflow.md`](agent-workflow.md) — agent execution workflow.
- [`testing.md`](testing.md) — test requirements.
- [`../../.github/ISSUE_TEMPLATE/feature.md`](../../.github/ISSUE_TEMPLATE/feature.md) — feature Issue template.
- [`../../.github/ISSUE_TEMPLATE/bug.md`](../../.github/ISSUE_TEMPLATE/bug.md) — bug Issue template.
- [`../../.github/ISSUE_TEMPLATE/architecture.md`](../../.github/ISSUE_TEMPLATE/architecture.md) — architecture decision template.
- [`../../.github/ISSUE_TEMPLATE/chore.md`](../../.github/ISSUE_TEMPLATE/chore.md) — maintenance/workflow template.
- [`../../.github/PULL_REQUEST_TEMPLATE.md`](../../.github/PULL_REQUEST_TEMPLATE.md) — PR template.
