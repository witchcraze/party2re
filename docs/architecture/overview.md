# Architecture Overview

## Purpose

This document defines the initial architecture for the new Party2 OSS implementation.

The project is a clean-room reconstruction: the existing Party2 implementation is a behavioral and functional reference only. Its source code and assets are not reused.

## Core architectural goals

1. Make continued game-feature expansion a first-class design goal.
2. Keep the Core small and stable.
3. Isolate feature-specific rules and state.
4. Define component responsibilities and contracts independently of implementation language.
5. Start with a simple modular monolith rather than microservices.
6. Use Go for the initial implementation.
7. Keep future replacement of individual components by another language possible without making the initial implementation unnecessarily complex.

## Initial structure

```text
Client
  |
Application / API
  |
  +-- Core
  |
  +-- Shared Components
  |     +-- Battle
  |     +-- Scheduling / Game Time
  |
  +-- Feature Modules
        +-- Adventure
        +-- Guild
        +-- Casino
        +-- Alchemy
        +-- Auction
        +-- Farming
        +-- Collection
        +-- Ranking
        +-- Events
        +-- ...
  |
  +-- Application Logging
  |
Persistence
```

These are logical boundaries. They do not imply separate processes.

## Implementation language

Go is the initial implementation language.

A component may eventually be rewritten in another language when there is a concrete reason, such as performance, safety, ecosystem support, or a substantially better implementation fit.

Do not introduce multiple languages, network protocols, or microservices merely to demonstrate language independence.

The important boundary is the component contract, not the programming language.

## Modular monolith

The initial system should normally run as one application.

Keep component boundaries explicit in code, but avoid operational complexity until it is justified by an actual requirement.

A future component extraction should be possible because the component does not depend on another component's private implementation.

## Core

The Core should contain only concepts that are genuinely shared by many parts of the game.

Initial candidates:

- Player
- Character
- Stats
- Progression
- Item definitions and instances
- Inventory
- Equipment
- Currency
- Game time
- Scheduled actions
- Domain events

This list is intentionally provisional.

## Shared components

Some systems are used by many features but should not automatically become part of Core.

Examples:

- Battle
- Adventure / Quest
- Scheduling

These should expose explicit contracts and keep feature-specific assumptions outside themselves.

## Features

Game systems that can be added, removed, or evolved independently should normally be implemented as Feature Modules.

The desired dependency direction is:

```text
Feature -> Core / public component contract
Feature -> Injected persistence / transaction contracts
```

Avoid:

```text
Feature A -> Feature B private implementation
Feature A -> Feature B persistence schema
```

## Rationale

Party2 historically accumulated many distinct game systems. This is not merely an implementation accident: the ability to continuously add systems is part of the game's character.

Therefore, the new architecture optimizes for the following development loop:

```text
Implement feature
  -> review boundary
  -> test
  -> merge
  -> add another feature
```

rather than repeatedly modifying a central game object.

Keeping Core small and features independently owned localizes future additions.
Battle is shared because multiple modes need combat; Scheduling shares time
processing while features own settlement. Domain events suit optional reactions,
but mandatory cross-feature work retains an explicit contract. A single Go
process keeps initial operation simple; extraction or another language requires
a concrete need rather than speculative infrastructure.

## Related documents

- [`AGENTS.md`](../../AGENTS.md) — mandatory architectural and development rules.
- [`components.md`](components.md) — component responsibilities and boundaries.
- [`interfaces.md`](interfaces.md) — component contracts.
- [`../design/game-overview.md`](../design/game-overview.md) — game/domain context.
- [`../../STATUS.md`](../../STATUS.md) — current implementation status.


## UI-independent operations and external access

Game behavior lives behind application services. The current HTTP JSON API is
implemented and documented in [OpenAPI](../api/openapi.json). The versioned context/action Gateway is registered at
`/api/v1/characters/{id}/context` and `/api/v1/characters/{id}/actions`. Browser
presentation (#140) and further facility migration remain separate work.
See [client-agent-api.md](client-agent-api.md) for migration sequencing; API
availability must not be confused with a completed browser client.

## Application logging

Application services may receive the injectable logging contract described in
[interfaces.md](interfaces.md#application-logging-contract). Logging is structured JSON infrastructure, not a
Core or Feature Module responsibility. It carries operation and optional
correlation fields while filtering authentication and credential data.
