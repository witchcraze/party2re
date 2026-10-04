# CI/CD

This document defines the project's durable CI/CD principles. Concrete GitHub Actions workflows are implementation details and may evolve.

## Continuous verification

GitHub Actions is the project's automated verification and delivery mechanism.

The CI pipeline should progressively cover:

1. formatting and static analysis;
2. unit tests;
3. integration tests;
4. Docker image build;
5. tests executed against the built image where applicable.

The exact workflow and commands should reflect the actual repository and must not be documented speculatively.

The current [CI workflow](../../.github/workflows/ci.yml) runs Go checks with atomic coverage inside the development container defined by `Dockerfile.dev` and `compose.yaml`. Docker Compose starts MariaDB and Valkey for integration tests; production-image verification currently checks the build.

## CI evidence and failure investigation

CI runs on pushes to `main` and PRs targeting `main`. The default PR triggers are opening, synchronization and reopening; changing the base alone does not start CI ([GitHub documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request)). After retargeting, use a supported trigger and confirm a successful `verify` run for the final head before merging. An absent check is not a passing check.

Treat a new CI failure as a possible regression. Compare the failing test on the PR and its pre-change baseline in isolated checkouts under matching conditions before attributing it to an existing issue. Record revisions, commands and failure evidence; for random behavior, capture or control the seed. Preserve behavioral assertions when fixing an unstable fixture.

## Docker image verification

A Docker image is not considered valid merely because it builds.

When the application is distributed as a container, CI should verify that the generated image can start and perform its applicable basic operations. Integration tests should exercise the containerized runtime where this provides meaningful coverage.

## API-level verification

Where practical, game behavior should be testable through the application API/command boundary without requiring GUI interaction. This supports automated integration testing and preserves the possibility of alternative clients in the future.

## Current API and planned client verification

The HTTP JSON API already exists. Its registered routes and specification are
verified by the current checks. Context/action Gateway handlers (#939/#646),
headless gameplay simulations (#650), and browser presentation (#140) are
remaining work. Do not describe external API access itself as unimplemented.
