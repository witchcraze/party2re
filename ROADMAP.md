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
- [x] **Player Lifecycle & Session Auth** (Issue #21)
- [x] **Character Initial State & Growth** (Issue #24, #10)
- [x] **Item Catalog & 5-Slot Equipment System** (Issue #11, #19, #51)
- [x] **Job System & Progression Rules** (Issue #17, #31, #38, #50)
- [x] **Skill Definitions & Cost/Condition Evaluation** (Issue #18)
- [x] **Battle Engine & Outcome Resolution** (Issue #12, #20, #36)
- [x] **Valkey ScheduledAction Queue & Worker** (Issue #106, #109, #110)
- [x] **Adventure System & Multi-stage Content** (Issue #13, #56, #57 — 28 stages, 286 monsters)
- [x] **Item Shop System** (Issue #55 — purchase & 50% resale)
- [x] **Character Item Depot** (Issue #58 — storage for items & gold)
- [x] **Blacksmith Enhancement** (Issue #59 — +1 to +10 equipment refinement)
- [x] **Alchemy Synthesis** (Issue #60 — 112 recipe crafting)
- [x] **Job Mastery & Character Rebirth** (Issue #61 — Lv99 mastery & +5 stat rebirth)
- [x] **Inn & Resting** (Issue #62 — HP/MP recovery)
- [x] **Banking & Player Remittance** (Issue #71 — gold deposits & player transfers)
- [x] **HTTP JSON Application API Layer** (Issue #87)
- [x] **Unified Local Verification & Pre-push Hook** (Issue #121, #124)

#### In-Progress & Upcoming Subsystems (Version 1.0 Milestones):

1. **API Security & Ownership Hardening**
   - Player-Character ownership linkage (Issue #131)
   - HTTP security headers & CORS middleware (Issue #132, #133)
2. **Language-Agnostic Core Specifications**
   - Core design specifications in `docs/design/` (Battle, Progression, Jobs, Skills, Items) (Issue #136)
3. **Social & Guild Systems**
   - Guild creation, management, membership, and Guild Battles
4. **Economy & Side Mini-Games**
   - Player Auction House & Marketplace
   - Casino games (High & Low, Indian Poker, Slot Machine, Doppel)
   - Lottery & Raffle tickets
   - Farm & Plantation cultivation
   - Collection & Monster Book encyclopedia
   - Chapel prayer & Blessings
5. **Competitive & Meta Systems**
   - Rankings (Level, Job, Weekly, Contest)
6. **Client Presentation & UI**
   - Web application client / UI-independent presentation layer
   - Asset placeholder mapping and production asset pipeline

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
