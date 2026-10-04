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

---

## Completed Phases

### Phase 0 — Game understanding
- Status: Completed.
- Outputs: core player loop identified, domain areas outlined, Battle isolated, Feature expansion established as primary design goal.

### Phase 1 — Architecture
- Status: Completed.
- Decisions: Go initial language, modular monolith, small Core, first-class Feature Modules, explicit contracts, MariaDB persistence, Valkey queue.

### Phase 2 — Domain model
- Status: Completed.
- Initial concepts: Player, Character, Progression, Job, Skill, Item, Inventory, Equipment, Currency, Battle, Adventure, ScheduledAction.

### Phase 3 — Project skeleton & Infrastructure
- Status: Completed.
- Outputs: Go project layout, MariaDB migrations workflow (`make db-migrate`, `make db-reset`), Valkey integration, structured JSON logging, CI pipelines, unified local verification (`make check`).

### Phase 4 — First vertical slice
- Status: Completed.
- Outputs: Character creation -> Activity (training) / Battle -> Experience & rewards -> Level Progression loop.

---

## Current Phase: Version 1.0 Reconstruction (Phase 5+)

### Phase 5 — Core Features & Economy Modules (In Progress)

#### Completed Feature Modules & Subsystems

Completed features are tracked in [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md) (the authoritative SSOT for Version 1.0 completion records). The full list of merged issues and feature groups is maintained there.

**Summary**: All core feature modules are implemented, including Player/Character lifecycle, Battle Engine, Adventure, Economy (Shop/Bank/Auction/Flea Market/Gem Store/Black Market), Social systems (Guild/GvG/PvP/Dungeons/Casino/Lottery/Home/Tavern/Delivery), and the current HTTP JSON API layer ([OpenAPI 3.1](docs/api/openapi.json)). Implemented modules and merged parity milestones do not prove complete behavioral equivalence; [known differences](docs/migration/documentation-audit.md) remain to be resolved against the original project.

#### Legacy Clean-room Specification Parity Milestones:

All 4 dependency-ordered milestones (Milestone 1 Core/System, Milestone 2 Economy/Production, Milestone 3 Adventure/Combat/Arenas, Milestone 4 Community/Entertainment) are completed. Comprehensive issue-level traceability and feature catalogs reside in [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md) and [`docs/migration/legacy-cgi-mapping.md`](docs/migration/legacy-cgi-mapping.md).

#### Remaining Version 1.0 Milestones:

1. **Client/Agent Gateway & CQRS Architecture (In Progress)**
   - Query Pillar implemented: owned `GET /api/v1/characters/{id}/context`, all timers and shared context DTO (#944, #946, #938, #939)
   - Command contract approved in #646: preserve success/result when refresh fails, re-fetch GET only, and do not automatically replay unknown command outcomes
   - Common authenticated command boundary implemented (#1010), including typed parameters, fail-closed entry/sleep checks and outcome-preserving context refresh; stage Adventure start connected (#1014)
   - Command adapters remain #1011–#1013 (Bank/Home/Rescue), with #1015 covering Deposit → Sleep → controlled expiry → Wake → Adventure integration
   - Phased migration and complete purge of legacy individual REST routes (#947, #948, #949, #950)
   - Establishes a radical, token-efficient 2-tool API surface for AI Agents, Web UI, and Chatbots
2. **Headless E2E Gameplay Simulation Test Framework**
   - Deterministic multi-turn game loop simulation via the Action Gateway (Issue #650)
3. **Client Presentation & Web UI**
   - Web application client / Server-Driven UI powered entirely by the Gateway (Issue #140)
4. **Production Asset Pipeline & Final Licensing**
   - Production asset manifest (#654, specification pending), resolver (#729), and license attribution catalog

---

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

---

## Document references

- `STATUS.md` — current state.
- `AGENTS.md` — mandatory rules.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
- `docs/migration/feature-inventory.md` — Version 1.0 feature inventory.
