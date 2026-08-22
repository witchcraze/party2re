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
Feature -> Shared infrastructure
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

The architecture should make the next feature easier, not merely make the current feature work.

## Related documents

- [`AGENTS.md`](../../AGENTS.md) — mandatory architectural and development rules.
- [`components.md`](components.md) — component responsibilities and boundaries.
- [`feature-modules.md`](feature-modules.md) — feature ownership and extensibility.
- [`interfaces.md`](interfaces.md) — component contracts.
- [`../design/game-overview.md`](../design/game-overview.md) — game/domain context.
- [`../../STATUS.md`](../../STATUS.md) — current implementation status.

## UI-independent operations and future external access

Game behavior should be implemented independently of any specific UI.

Major player-visible game operations should be executable through an application-level API or command boundary rather than being implemented directly in GUI event handlers. This keeps the game logic testable and makes alternative clients possible.

The architecture should also avoid assumptions that would prevent exposing appropriate game operations through an external API in the future. For example, this may eventually allow an AI Agent or another automated client to play the game through the same public operations available to a human player.

This is an architectural capability to preserve, not a requirement to expose a public network API during the initial implementation.

