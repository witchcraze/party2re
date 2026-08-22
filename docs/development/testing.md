# Testing Strategy

## Purpose

Tests specify and protect game behavior.

The most valuable tests are those that remain valid when implementation details change.

## Priority

1. Domain invariants and game rules.
2. Observable component behavior.
3. Important feature integration flows.
4. Regression tests for bugs.

## Domain-first testing

Examples include:

- valid and invalid character state transitions;
- progression calculations;
- job requirements;
- battle outcomes;
- item ownership and equipment rules;
- currency invariants;
- scheduled action state transitions;
- feature-specific rules.

## Avoid implementation-coupled tests

Do not test private implementation structure merely because it is easy to assert.

A refactor that preserves behavior should ideally require few or no test changes.

## TDD

For non-trivial changes:

1. define the behavior in the Issue;
2. write or update the test;
3. confirm the missing behavior is represented;
4. implement the smallest satisfying change;
5. run focused tests;
6. run the broader suite.

## Regression rule

A reproducible bug should normally receive a regression test.

The test should fail before the fix when practical and pass after the fix.

## Architecture and testing

Components should be testable through their public contracts.

If testing a component requires knowledge of another component's private implementation, reconsider the boundary.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory TDD rules.
- [`agent-workflow.md`](agent-workflow.md) — normal agent workflow.
- [`issue-workflow.md`](issue-workflow.md) — how acceptance criteria and PRs are handled.
- [`../architecture/interfaces.md`](../architecture/interfaces.md) — testing through component contracts.


## Canonical test commands

The project uses Go for the initial implementation.

The canonical commands will be defined here when the Go project skeleton is established. They should always reflect commands that actually work in the repository; do not document speculative commands.

At minimum, the project is expected to define commands for:

- formatting;
- unit and integration tests;
- static analysis;
- the complete test suite.

The commands documented here are the authoritative commands for agents and reviewers. When they change, update this document in the same Issue/PR that changes the development workflow.
