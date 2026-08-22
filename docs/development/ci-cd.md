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

## Docker image verification

A Docker image is not considered valid merely because it builds.

When the application is distributed as a container, CI should verify that the generated image can start and perform its applicable basic operations. Integration tests should exercise the containerized runtime where this provides meaningful coverage.

## API-level verification

Where practical, game behavior should be testable through the application API/command boundary without requiring GUI interaction. This supports automated integration testing and preserves the possibility of alternative clients in the future.

## Future external API capability

The architecture should preserve the possibility of exposing appropriate application operations through an external API in the future.

For example, an AI Agent could eventually play the game by interacting with those operations programmatically. This is an architectural direction, not a requirement to publish a network API during the initial implementation.
