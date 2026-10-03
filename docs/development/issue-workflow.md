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

Issue bodies describe the work and its acceptance criteria; implementation tools and providers are not ticket requirements. Size labels indicate complexity without selecting an executor.

## Two-stage documentation lifecycle

To avoid pre-emptive documentation rework while ensuring permanent design documents remain 100% synchronized with actual code:

1. **Stage 1 — Issue Creation (Pre-implementation)**:
   - Do **not** create or pre-draft specification files (`docs/design/*.md`) before coding.
   - Describe requirements, calculations, and observable behavior directly in the Issue's **Acceptance Criteria**.
2. **Stage 2 — PR Implementation (Post-implementation synchronization)**:
   - Implement the code and unit/integration tests.
   - Once domain rules, formulas, and state transitions are settled, create or update the language-agnostic design specification (`docs/design/<feature>.md`) **within the same implementation PR**.
   - Synchronize `docs/architecture/components.md` and `STATUS.md` in the same PR.
   - **Pre-commit issue re-check**: Re-inspect the active Issue (`gh issue view <issue-number>`) and cross-check each acceptance criterion against code, tests, and documentation to guarantee zero omissions before committing and opening the PR.

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


## Dependent PRs and merge verification

Prefer merging a prerequisite PR before opening its dependent PR against `main`. If PRs are stacked, record the dependency and keep the prerequisite branch until the dependent PR is rebased and retargeted.

Before merging, confirm the final diff, passing remote CI for the current head, and native Issue linkage (`gh pr view <number> --json closingIssuesReferences`). `Closes #<number>` creates a native link only when the PR targets the default branch; a body reference alone is insufficient ([GitHub documentation](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue)). After retargeting, verify linkage and CI again; see [`ci-cd.md`](ci-cd.md#ci-evidence-and-failure-investigation).

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

After merging, confirm the Issue closed, synchronize local `main`, and clean up the merged branch. Update affected open Issue bodies with the resolved prerequisite and remaining scope; verify dependent Issues have no open blockers before describing them as unblocked.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory Issue/PR rules.
- [`agent-workflow.md`](agent-workflow.md) — agent execution workflow.
- [`testing.md`](testing.md) — test requirements.
- [`../../.github/ISSUE_TEMPLATE/feature.md`](../../.github/ISSUE_TEMPLATE/feature.md) — feature Issue template.
- [`../../.github/ISSUE_TEMPLATE/bug.md`](../../.github/ISSUE_TEMPLATE/bug.md) — bug Issue template.
- [`../../.github/ISSUE_TEMPLATE/architecture.md`](../../.github/ISSUE_TEMPLATE/architecture.md) — architecture decision template.
- [`../../.github/ISSUE_TEMPLATE/chore.md`](../../.github/ISSUE_TEMPLATE/chore.md) — maintenance/workflow template.
- [`../../.github/PULL_REQUEST_TEMPLATE.md`](../../.github/PULL_REQUEST_TEMPLATE.md) — PR template.
