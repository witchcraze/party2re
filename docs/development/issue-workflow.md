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

## PR is the unit of review

All code changes go through a PR.

Use the repository's PR template without replacing or bypassing required sections.

The PR template records the change and its verification. Mandatory agent rules remain defined by `AGENTS.md`.

A PR should:

- reference its Issue;
- explain the behavior change;
- identify tests;
- identify architecture impact;
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
