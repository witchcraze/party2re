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

The [frozen feature inventory](docs/migration/feature-inventory.md) records the initial reconstruction snapshot. Detailed completion history is available in GitHub/Git; [STATUS](STATUS.md) describes major current capabilities and gaps.

**Summary**: All core feature modules are implemented, including Player/Character lifecycle, Battle Engine, Adventure, Economy (Shop/Bank/Auction/Flea Market/Gem Store/Black Market), Social systems (Guild/GvG/PvP/Dungeons/Casino/Lottery/Home/Tavern/Delivery), and the current HTTP JSON API layer ([OpenAPI 3.1](docs/api/openapi.json)). Implemented modules and merged parity milestones do not prove complete behavioral equivalence; [known differences](docs/migration/documentation-audit.md) remain to be resolved against the original project.

#### Legacy Clean-room Specification Parity Milestones:

The initial Core/System, Economy/Production, Adventure/Combat/Arenas and Community/Entertainment reconstruction milestones are recorded in the [frozen inventory](docs/migration/feature-inventory.md). [Legacy CGI mapping](docs/migration/legacy-cgi-mapping.md) remains the navigation to behavioral evidence and reconstruction targets; [known differences](docs/migration/documentation-audit.md) remain active reconciliation work.

#### Remaining Version 1.0 Milestones:

1. **Client/Agent Gateway & CQRS Architecture (In Progress)**
   - Complete remaining command coverage; the initial stateful gameplay loop is covered by focused HTTP integration tests.
   - Extend selected/activity scene composition with remaining verified facility and full game detail adapters; connect role/phase-specific continuation mutations (#1051/#1052/#1055 and migration children).
   - Retire individual REST routes only after their replacement commands and observations are implemented and verified, retaining deliberate route exceptions (#947–#950).
   - Preserve the enduring [command/recovery contract](docs/architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions); current coverage is summarized in [STATUS](STATUS.md).
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
6. update affected explanations when capabilities, contracts or plans change;
7. finish with a clean repository state.

Avoid spending the weekly token budget on broad refactors unless they are necessary to unblock the next feature.

---

## Document references

- `STATUS.md` — current state.
- `AGENTS.md` — mandatory rules.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
- `docs/migration/feature-inventory.md` — frozen reconstruction snapshot.
