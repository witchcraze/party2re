# Issue and PR Workflow

## Issue is the unit of work

All substantial development work begins with an Issue.

Use the repository's Issue template. Do not create ad-hoc tickets.

The template captures the work itself: goal, scope, acceptance criteria, tests, and relevant architectural/dependency considerations. Repository-wide rules belong in `AGENTS.md` and should not be duplicated unnecessarily in every ticket.

Issue bodies describe the work and its acceptance criteria; implementation tools and providers are not ticket requirements. Size labels indicate complexity without selecting an executor.

GitHub native relationships own dependencies, parent/sub-issue links and their resolution state under the [workflow rule](../../.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow). The body explains why a prerequisite is needed, which requirements/contracts the work adopts and how to verify them, with references to the prerequisite Issue or owning document. Relationship-only lists, progress/resolution state and full shared specifications do not need a second copy in the body. Adding or resolving a dependency calls for a body edit only when those implementation instructions change.

## Documentation follows changed explanations

Use the [documentation rule](../../.agents/rules/02-documentation-sync.md) and [documentation map](../README.md) to update owning explanations and consolidate affected duplicates in the same PR. The PR's documentation assessment records meaningful consolidation, necessary growth or why no update is needed; no separate maintenance report is required.

Before committing, re-inspect the active Issue (`gh issue view <issue-number>`) and cross-check each acceptance criterion against the implementation, validation and any affected documentation.

## PR is the unit of review

All code changes go through a PR.

Use the repository's PR template without replacing or bypassing required sections.

Fill the template with the issue-scoped change, validation and documentation assessment. Mandatory agent rules remain defined by `AGENTS.md`.

## Dependent PRs and merge verification

Prefer merging a prerequisite PR before opening its dependent PR against `main`. If PRs are stacked, record the dependency and keep the prerequisite branch until the dependent PR is rebased and retargeted.

Before merging, confirm the final diff, passing remote CI for the current head, and native Issue linkage (`gh pr view <number> --json closingIssuesReferences`). `Closes #<number>` creates a native link only when the PR targets the default branch; a body reference alone is insufficient ([GitHub documentation](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue)). After retargeting, verify linkage and CI again; see [`ci-cd.md`](ci-cd.md#ci-evidence-and-failure-investigation).

## Ticket size and decomposition

The [workflow rule](../../.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow) owns size estimates and scope decisions. Each Issue has one size label, chosen from the expected complexity of the whole task. Investigation can change that estimate; update the label and briefly explain a material change in the Issue or PR.

Line counts help assess review effort. A large test matrix, generated specification updates or a code move can produce a substantial diff while leaving the implementation straightforward. DDL, package/layer count and edits to capped files also require judgment about their actual impact. They do not automatically determine a size label or require a separate ticket.

An Issue brings together one objective, its implementation, tests and necessary contract/documentation updates. Consider splitting when doing so makes the work easier to understand, verify and merge. Independently useful changes or prerequisite work are often good candidates; coupled changes can be clearer to validate together. A large estimate can still describe one cohesive PR.

When keeping work together despite decomposition concerns, state the reason briefly in the Issue or PR. For example:

- A Gateway adapter, catalog/OpenAPI input metadata and tests share the acceptance criteria for connecting one command; reviewing them together verifies the complete contract.
- A feature and its schema change may remain together when a single review keeps their contract consistent. An independently useful storage migration may be clearer as a prerequisite PR.
- Moving a configuration function out of a capped handler can belong with its new adapter. A broader handler restructuring may be easier to review separately.
- Fixes to unrelated Bank and Rescue behavior have separate objectives and should be tracked independently.

The judgment applies to task boundaries. Existing architecture, security and production-file limits continue to govern the implementation; unresolved significant design choices still follow the architecture-decision workflow below.

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

After merging, confirm the Issue closed, synchronize local `main`, and clean up the merged branch. Assess related-ticket impact under the [workflow rule](../../.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow), starting with direct dependents and known references to the changed contract. Correct bodies whose requirements, code targets, prerequisites or remaining scope would mislead the next implementer. Expand discovery only for concrete impact; completion or dependency closure alone does not require a body edit. Keep dependency state in native relationships and shared specifications in their owning documents. Verify remaining native blockers before describing an Issue as unblocked, and retain independent specification questions.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory Issue/PR rules.
- [`agent-workflow.md`](agent-workflow.md) — agent execution workflow.
- [`testing.md`](testing.md) — test requirements.
- [`../../.github/ISSUE_TEMPLATE/feature.md`](../../.github/ISSUE_TEMPLATE/feature.md) — feature Issue template.
- [`../../.github/ISSUE_TEMPLATE/bug.md`](../../.github/ISSUE_TEMPLATE/bug.md) — bug Issue template.
- [`../../.github/ISSUE_TEMPLATE/architecture.md`](../../.github/ISSUE_TEMPLATE/architecture.md) — architecture decision template.
- [`../../.github/ISSUE_TEMPLATE/chore.md`](../../.github/ISSUE_TEMPLATE/chore.md) — maintenance/workflow template.
- [`../../.github/PULL_REQUEST_TEMPLATE.md`](../../.github/PULL_REQUEST_TEMPLATE.md) — PR template.
