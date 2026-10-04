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

## Documentation follows changed explanations

The Issue captures the goal, observable requirements and acceptance criteria. Permanent documents explain enduring rules, contracts and decisions; detailed work history belongs in GitHub/Git.

During implementation, assess which explanations the change makes outdated and update their owning documents in the same PR, using the [documentation rule](../../.agents/rules/02-documentation-sync.md#3-update-affected-explanations-within-pr) and [documentation map](../README.md). Game-rule changes affect design documents, responsibility/contract changes affect architecture documents, and procedure changes affect developer documentation. STATUS and ROADMAP change when their major capabilities, gaps or remaining milestones change.

A routine implementation change can leave these explanations accurate. In that case, briefly state that no documentation update is needed. The frozen reconstruction inventory is historical reference, not a completion log to append to each PR. Preserve specification/decision evidence links where they support an explanation, and keep missing coverage and unverified behavior discoverable.

Before committing, re-inspect the active Issue (`gh issue view <issue-number>`) and cross-check each acceptance criterion against the implementation, validation and any affected documentation.

## PR is the unit of review

All code changes go through a PR.

Use the repository's PR template without replacing or bypassing required sections.

The PR template records the change, its verification, and documentation checklist. Mandatory agent rules remain defined by `AGENTS.md`.

A PR should:

- reference its Issue;
- explain the behavior change;
- identify tests;
- assess documentation impact and update affected explanations/contracts, or explain why none needs updating;
- remain within the Issue scope.


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
