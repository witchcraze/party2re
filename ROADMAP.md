# Roadmap

## Strategy

Development proceeds in small, complete units so that weekly free-token limits can be used efficiently.

The priority is a working vertical slice and sound boundaries, not broad unfinished implementation.

## Two kinds of project policy

This project deliberately separates **enduring development principles** from **temporary Version 1.0 reconstruction work**.

### Enduring principles

These remain valid after Version 1.0:

- feature-oriented extensibility;
- clear component boundaries;
- small Core;
- language-independent component contracts;
- TDD;
- Issue / PR driven development;
- architecture review;
- dependency and license discipline.

These principles describe how the project should be developed.

### Temporary reconstruction work

These exist because the project is currently rebuilding Party2 toward Version 1.0:

- investigating the existing Party2 implementation;
- reconstructing its important behavior and game rules;
- replacing the implementation rather than migrating it;
- recreating visual assets;
- validating important behavior against the reference;
- completing the initial Version 1.0 feature baseline.

These tasks should not be mistaken for permanent project architecture.

Once Version 1.0 is established, the project should transition from **reconstruction mode** to ordinary feature development. Historical implementation details should then become increasingly irrelevant to normal development.

## Completed

### Phase 0 — Game understanding

Status: Completed in the initial design session.

Outputs:

- identified the core player loop;
- identified major domain areas;
- identified the importance of time-based actions;
- identified Battle as a reusable component;
- identified Feature expansion as a primary product and architecture characteristic;
- decided that existing source code is reference material only.

### Phase 1 — Architecture

Status: Completed in the initial design session.

Decisions:

- initial implementation language: Go;
- modular monolith;
- small Core;
- first-class Feature Modules;
- explicit component contracts;
- future language replacement is allowed;
- no premature microservices or remote protocols;
- architecture review is required for substantial feature additions.

### Phase 2 — Domain model

Status: Completed as an initial design.

Initial concepts:

- Player
- Character
- Progression
- Job
- Skill
- Item
- Inventory
- Equipment
- Currency
- Battle
- Adventure / Quest
- ScheduledAction
- Guild
- DomainEvent
- Feature Module

These remain subject to refinement during implementation.

## Current phase

### Version 1.0 Reconstruction / Refactoring

Status: **In progress**

The project is currently in the transitional phase where the new architecture, domain model, development workflow, and initial implementation are being established while reconstructing the important behavior of Party2.

This phase is temporary.

Version 1.0 completion means that the meaningful functions present in the reference project have been newly reconstructed and that the images required by those functions have been newly produced or replaced with approved placeholders. It does not mean copying or mechanically translating the old implementation.

The objective is not to create a permanent "refactoring project". The objective is to arrive at a maintainable Version 1.0 implementation, after which normal feature development becomes the primary activity.

### Phase 3 — Project skeleton

Create the minimum Go project structure and development tooling.

Goals:

- repository structure;
- build/test commands;
- initial application entry point;
- initial Core package boundaries;
- documentation structure;
- CI;
- minimal health check or equivalent executable behavior.
- MariaDB development persistence and migration workflow.
- Valkey connection/development workflow for concrete transient-state, caching, queue, or coordination requirements.

Do not implement historical game features yet.

### Phase 4 — First vertical slice

Choose the smallest useful end-to-end game loop.

The exact scope should be determined after the Phase 3 skeleton exists.

A likely target is:

```text
Character
  -> activity
  -> battle / result
  -> reward
  -> progression
```

The first slice should demonstrate that the architecture works rather than maximize feature count.
The initial implementation slices are tracked in [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md).

### Phase 5+ — Incremental features

Add features one at a time.

Each feature should:

- have a clear scope;
- own its feature-specific state;
- include focused tests;
- pass architecture review;
- avoid unnecessary Core changes.

Potential feature order is intentionally not fixed yet. Choose based on dependencies and the value of each vertical slice.

## Weekly execution model

Each week:

1. select one small objective;
2. inspect current architecture and status;
3. implement only the selected scope;
4. run focused tests;
5. perform architecture review;
6. update status/roadmap;
7. finish with a clean repository state.

Avoid spending the weekly token budget on broad refactors unless they are necessary to unblock the next feature.

## Document references

- `STATUS.md` — current state.
- `AGENTS.md` — mandatory rules.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
