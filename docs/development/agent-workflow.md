# Agent Workflow

This is the session route for CLI agents. [Issue and PR workflow](issue-workflow.md) describes delivery; [`AGENTS.md`](../../AGENTS.md) and its applicable rules own mandatory constraints.

## Start of a session

1. Read `AGENTS.md`, applicable rules and the complete active Issue, including acceptance criteria and dependencies.
2. Use `STATUS.md`, relevant `ROADMAP.md` sections and current Issues to understand the task or select ready work.
3. Inspect relevant code, tests and owning documents through the [documentation map](../README.md).

Do not start by scanning the entire repository unless the Issue requires it.

## Selecting work

Prefer blockers/broken behavior, prerequisites, a small useful vertical slice, features, then cleanup. Use [ticket size and decomposition](issue-workflow.md#ticket-size-and-decomposition) to bound the objective; avoid a broad plan when a smaller ticket suffices.

## During implementation

For behavior changes, use the stages in [`testing.md`](testing.md#2-tiered-verification-strategy):

```text
Issue (acceptance criteria)
  -> write / update tests (TDD)
  -> implement minimum solution
  -> update and consolidate affected explanations under the documentation rule
  -> re-check active issue (verify acceptance criteria & zero omissions)
  -> make fmt
  -> make check (wait for completion)
  -> commit & push (pre-push reuses successful verification)
  -> open PR
```

For documentation-only work, verify sources and changed links/examples; service startup and behavioral tests are unnecessary solely for prose. The [documentation rule](../../.agents/rules/02-documentation-sync.md#4-consolidation-and-addition-criteria) owns addition/removal criteria. Keep maintenance within the active objective.

## When blocked

Resolve routine implementation details locally. For significant architecture, dependency or licensing decisions, use the [architecture review triggers](../../.agents/rules/03-architecture.md#6-architecture-review-triggers) and [dependency policy](dependency-policy.md); record unresolved decisions in an Issue.

## Session end

Report the delivered outcome, verification and remaining limitations. Leave unfinished work in an Issue and document temporary architecture where it affects an enduring contract. Follow [delivery completion](issue-workflow.md#completion) for authorized merge and related-ticket work; reuse successful verification for unchanged content.
