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

#### Legacy Clean-room Specification Parity Milestones (4-Phase Roadmap):

Following a comprehensive clean-room specification audit of all 40 legacy CGI modules against the current implementation, 33 reconciliation issues (#459–#491) were identified and scheduled into 4 dependency-ordered milestones within Phase 5:

##### Milestone 1: Core Foundation & Shared Storage
Eliminate divergent mechanics in storage, core progression, and battle engines before downstream dependencies.
- [x] **Depot Capacity & Direct Delivery Routing** (#460)
- [x] **Multi-turn Core Battle Engine Parity** (#480)
- [x] **Abolish Fictional Rebirth System & Restore OverLevel Cap** (#470)
- [x] **Altar of Rebirth: Restore 6-Orb Offering, Ramia Awakening & Otherworld Wishes** (#471)
- [ ] **Custom Skills: Incantations, Resource Scaling & Failure Multipliers** (#469)
- [ ] **Job System Mastery & Job Change Requirements** (#467)
- [ ] **Wishing Well & God Stat Seed Grants** (#468, #473)
- [ ] **Chapel: Status Afflictions, Curses, and Tithe Buffs** (#472)

##### Milestone 2: Economic, Life & Production Loop
Reconcile facilities and production mechanics that depend on Depot storage and overnight cycles.
- [ ] **Home & Estate: Overnight Carpenter Construction & Resting Buffs** (#459, #461)
- [ ] **Standard Commercial Shops: Level Gates, Sellback & MasterCard Discount** (#465)
- [ ] **Secret Shop: Passphrase Unlock & Deterministic Rotations** (#462)
- [ ] **Black Market: Entrance Fees, Rare Catalogs & Bust Mechanics** (#463)
- [ ] **Gem Store: Exact Catalogs & Exchange Formulas** (#464)
- [ ] **Player Stores: Custom Pricing, Log Books & Commission Fees** (#466)
- [ ] **Bank: Character Deposits, Daily Interest & Peer-to-Peer Transfers** (#476)
- [ ] **Auction House: Bid Retention, Expiry Settlement & Depot Routing** (#474)
- [ ] **Flea Market: Direct Depot Withdrawals & Trading Logs** (#477)
- [ ] **Tavern Food Delivery: Scheduled Post-Adventure Buffs** (#475)
- [ ] **Alchemy: Free Overnight Depot-linked Synthesis & Compendium** (#487)
- [ ] **Monster Ranch: Monster Stabling, Home Pet Link, Naming & P2P Gifting** (#488)
- [ ] **Plantation: 6 Seeds, 14 Fertilizer Reagents & Overnight Depot Harvest** (#489)

##### Milestone 3: Adventure, Dungeons & Live Combat
Restore crawl structures, boss challenges, and multiplayer PvP/GvG arenas.
- [ ] **Battle Adapter: Standardize Character/Party Mapping & Post-Battle State Application** (#496)
- [ ] **Adventure: 10-Floor Dungeon Crawl, Flee Penalties & Boss Battles** (#478)
- [ ] **Sealing Boss Arena: Proof of Kingship, Revive Counters & Leaves** (#479)
- [ ] **Colosseum PvP: Real-time Wagering & Spectator Broadcasts** (#481)
- [ ] **GvG Arena: Multi-round Tournament Engine & Defensive Battles** (#482)
- [ ] **Dungeon & Challenge: Co-op Multi-party Lobbies & Turn Engines** (#483)

##### Milestone 4: Community, Events & Entertainment
Restore authentic social structures and mini-games.
- [ ] **Guild: Dynamic Guild Points, Hex Colors, Custom Roles & Approval Workflow** (#490)
- [ ] **Event Plaza: Real-time Concurrency Headcount & 3x Markup Catalog** (#491)
- [ ] **Takarakuji Lottery: 20-Cap Tickets & Server-wide Rollover Jackpot** (#484)
- [ ] **Fukubiki Raffle: Stat Seeds, Divine Orbs & Guaranteed Tiers** (#485)
- [ ] **Casino: 8-Player Shared-room Roulette & Progressive Slots** (#486)

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
