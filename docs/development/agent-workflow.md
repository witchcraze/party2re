# Agent Workflow

## Purpose

This document defines how CLI coding agents should operate in this repository.

`AGENTS.md` contains mandatory rules. This document describes the normal operational workflow.

## Start of a session

1. Read `AGENTS.md`.
2. Read `STATUS.md`.
3. Read `ROADMAP.md`.
4. Inspect the current Issues.
5. Select an existing Issue that is ready for implementation.
6. Read the complete Issue and its acceptance criteria.
7. Inspect only the relevant code, tests, and architecture documents.

Do not start by scanning the entire repository unless the Issue requires it.

## Selecting work

Prefer:

1. blockers and broken behavior;
2. prerequisite architecture or infrastructure;
3. the smallest useful vertical slice;
4. feature implementation;
5. cleanup and refactoring.

Do not create a large implementation plan when a smaller ticket can be completed first.

Assess the Issue's complexity and cohesive objective using [ticket size and decomposition](issue-workflow.md#ticket-size-and-decomposition). Consider splitting when it improves understanding, verification and merging. Keep coupled changes together when that better serves the same acceptance criteria, and briefly record the reason when scope raises decomposition concerns. Revise the size estimate as investigation clarifies the work.

## During implementation

Choose development checks using the stages in [`testing.md`](testing.md#2-tiered-verification-strategy), then follow:

```text
Issue (acceptance criteria)
  -> write / update tests (TDD)
  -> implement minimum solution
  -> write / update docs/design/<feature>.md (language-agnostic spec)
  -> update docs/architecture/components.md & STATUS.md (current state)
  -> re-check active issue (verify acceptance criteria & zero omissions)
  -> make fmt
  -> make check (wait for completion)
  -> commit & push (pre-push reuses successful verification)
  -> open PR
```

Before committing changes upon task completion, agents MUST re-inspect the active Issue (`gh issue view <issue-number>`) and cross-check each acceptance criterion to guarantee zero omissions before committing and opening the PR.

Keep unrelated changes out of the ticket.


## When blocked

If the blocker is a minor implementation detail, resolve it locally.

If the blocker requires a significant architectural decision, create an Issue for that decision rather than silently changing the architecture.

Examples:

- changing Core responsibilities;
- changing component boundaries;
- introducing a new cross-feature dependency;
- changing persistence architecture;
- introducing a new external dependency;
- changing the project licensing strategy.

## Session end

Before ending a session:

- confirm relevant tests and full verification cover the final changes; reuse successful checks as described in [`testing.md`](testing.md#2-tiered-verification-strategy);
- synchronize documentation:
  - update `STATUS.md` current component state and immediate priorities (avoid appending historical logs);
  - update `docs/design/<feature>.md` if game mechanics or formulas were added/modified;
  - update `docs/architecture/components.md` if component responsibilities or boundaries were added/modified;
  - update `docs/migration/feature-inventory.md` and `ROADMAP.md` if milestones progressed;
- leave unfinished work clearly represented by an Issue;
- do not leave undocumented temporary architecture.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory rules; this document explains the normal workflow.
- [`issue-workflow.md`](issue-workflow.md) — Issue and PR lifecycle.
- [`testing.md`](testing.md) — TDD and testing priorities.
- [`dependency-policy.md`](dependency-policy.md) — dependency and license checks.
- [`../../STATUS.md`](../../STATUS.md) — current state.
- [`../../ROADMAP.md`](../../ROADMAP.md) — planned work.
