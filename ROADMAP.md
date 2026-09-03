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

#### Completed Feature Modules & Subsystems:
- [x] **Player Lifecycle, Deletion & Session Auth** (Issue #21, #134, #190)
- [x] **Character Initial State, Growth, Rebirth & Customization** (Issue #24, #10, #61, #198)
- [x] **Item Catalog, 5-Slot Equipment System & Item Depot** (Issue #11, #19, #51, #58)
- [x] **Job System, Skills, Mastery & Custom Loadout** (Issue #17, #18, #31, #38, #50, #69)
- [x] **Battle Engine, Deterministic Turn Resolver & Replay Recorder** (Issue #12, #20, #36, #66)
- [x] **Valkey ScheduledAction Queue & Distributed Lock Worker** (Issue #106, #109, #110)
- [x] **Adventure System, Multi-stage Content & Chronicles** (Issue #13, #56, #57, #199 — 28 stages, 286 monsters)
- [x] **Multiplayer Party & Co-op Quests** (Issue #188, #341)
- [x] **Commercial Economy**: Shop, Blacksmith, Alchemy, Gem Store, Black Market, Flea Market, Auctions, Small Medals (Issue #55, #59, #60, #71, #72, #80, #142, #160, #194, #276)
- [x] **Social & Meta Systems**: Guilds, GvG, PvP Arena, Bosses, Dungeons, Endurance Challenge, Park, News/Inbox, Home/Mailbox, Rankings, Photo Contest, Monster Grandpa, Secret Shop, Tavern, Delivery, Event Plaza (Issue #63, #67, #73, #74, #75, #76, #77, #78, #79, #81, #82, #83, #84, #85, #86, #141, #159, #161, #162, #185, #186, #187, #192, #193, #195)
- [x] **HTTP JSON Application API Layer & Complete OpenAPI 3.1 Spec** (Issue #87, #180, #254, #266 — 182 routes)
- [x] **Maintenance Mode & Admin Operations** (Issue #190)
- [x] **Unified Verification Pipeline, Pre-push Hook & Distroless Smoke Build** (Issue #121, #124, #128)

#### Remaining Version 1.0 Milestones:

1. **API Key / Personal Access Token Authentication**
   - Personal Access Token (API Key) generation and authentication (Issue #163)
2. **Client Presentation & Web UI**
   - Web application client / UI-independent presentation layer (Issue #140)
3. **Production Asset Pipeline & Final Licensing**
   - Production asset mapping and license attribution catalog

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
