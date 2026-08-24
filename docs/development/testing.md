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
- [`development-environment.md`](development-environment.md) — containerized local environment.
- [`agent-workflow.md`](agent-workflow.md) — normal agent workflow.
- [`issue-workflow.md`](issue-workflow.md) — how acceptance criteria and PRs are handled.
- [`../architecture/interfaces.md`](../architecture/interfaces.md) — testing through component contracts.


## Canonical test commands

The canonical commands run through the repository `Makefile` and Docker Compose, ensuring identical execution locally and in CI:

```bash
# Auto-format Go code
make fmt

# Run all verification checks (formatting check, go vet, db-migrate, full docker test suite, smoke build)
make check

# Run clean verification with full database reset (DROP & recreate database)
make check-clean
```

Direct Docker Compose commands remain available for targeted sub-package testing:

```bash
docker compose run --rm app go test ./internal/core/battle
```

For complete container workflows, database migrations, and CI integration, see [`testing-and-containers.md`](testing-and-containers.md).

