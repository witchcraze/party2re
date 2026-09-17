# Version 1.0 Feature Inventory

> Temporary Version 1 reconstruction document.
>
> This document describes the behavior and content areas reconstructed
> from the `party2-main/` reference codebase. It is not a source-code migration plan. The old
> implementation and its assets remain reference material only.

## Scope Decision

Version 1.0 is complete when the existing project's meaningful game functions
have been newly implemented and the images required by those functions have
been newly produced or replaced with approved placeholders.

"Implemented" means behavior is available through the new application's
public contracts and is covered by appropriate tests. It does not mean that
the old CGI files, Perl structure, file layout, HTML, or persistence format are
copied or mechanically translated.

The old archive README states that the original source and license are
unknown. Therefore, the old source and images must not be incorporated into
the new implementation.

Foundational game logic, standard values, and calculated values may generally
be reused as behavioral requirements when independently reconstructed in the
new implementation. Distinctive asset names that evoke a specific game are
not reused; those names are reviewed individually and normally replaced with
generic terminology.

## Reference Inventory

The archive currently contains approximately:

| Area | Observed quantity | Interpretation |
| --- | ---: | --- |
| Top-level CGI endpoints | 27 | Entry points, account, ranking, replay, administration, and other UI operations |
| Feature library CGI files | 74 | Feature and shared behavior candidates |
| Map-related CGI files | 112 | Map content and map-specific rules/data |
| Stage-related CGI files | 39 | Adventure/stage content |
| GIF/PNG/ICO images | 890 | Legacy visual references only |
| All archive files | 1,259 | Includes deployment files, HTML, scripts, data, and assets |

The quantities are inventory indicators, not Version 1 API counts.

---

## Feature Groups & Reconstruction Status

All groups below are Version 1.0 reconstruction requirements.

### A. Application Foundation and Account Lifecycle
- [x] Character/player registration & password hashing ([#21](https://github.com/witchcraze/party2re/issues/21))
- [x] Login and session authentication lifecycle in Valkey Master ([#21](https://github.com/witchcraze/party2re/issues/21), [#366](https://github.com/witchcraze/party2re/issues/366), [#378](https://github.com/witchcraze/party2re/issues/378))
- [x] Character profile and status display ([#87](https://github.com/witchcraze/party2re/issues/87))
- [x] Player-character ownership verification linkage ([#131](https://github.com/witchcraze/party2re/issues/131))
- [x] Personal Access Token (API Key) generation and dual authentication ([#163](https://github.com/witchcraze/party2re/issues/163))
- [x] Player deletion and maintenance behavior ([#134](https://github.com/witchcraze/party2re/issues/134), [#190](https://github.com/witchcraze/party2re/issues/190), [#367](https://github.com/witchcraze/party2re/issues/367))
- [x] Name changes and profile customization ([#198](https://github.com/witchcraze/party2re/issues/198))
- [x] Notifications, news, and player notification inbox ([#67](https://github.com/witchcraze/party2re/issues/67))
- [x] Administrator operations ([#190](https://github.com/witchcraze/party2re/issues/190))

### B. Character, Progression, Jobs, and Skills
- [x] Level and cumulative experience progression ([#10](https://github.com/witchcraze/party2re/issues/10))
- [x] Fundamental stats and initial character bounds ([#24](https://github.com/witchcraze/party2re/issues/24))
- [x] Job definitions, data catalog, and job change history ([#17](https://github.com/witchcraze/party2re/issues/17), [#38](https://github.com/witchcraze/party2re/issues/38), [#50](https://github.com/witchcraze/party2re/issues/50))
- [x] Job-based stat growth formulas ([#31](https://github.com/witchcraze/party2re/issues/31))
- [x] Skill definitions, costs, and availability conditions ([#18](https://github.com/witchcraze/party2re/issues/18))
- [x] Job mastery, level-20 job changes, item costs, mastered-job memory exchange, future memory snapshots ("よびおこす"), and 72-job completion title/news notification with "すっぴん" job unlock ([#467](https://github.com/witchcraze/party2re/issues/467), [#470](https://github.com/witchcraze/party2re/issues/470))
- [x] Home resting, sleep recovery, and tired reset ([#62](https://github.com/witchcraze/party2re/issues/62), [#459](https://github.com/witchcraze/party2re/issues/459))
- [x] Custom skill assignment ([#69](https://github.com/witchcraze/party2re/issues/69))
- [x] Wishing Well (願いの泉, @女神) SP sacrifice exchange for permanent stat growth ([#468](https://github.com/witchcraze/party2re/issues/468))
- [x] Endgame wishes, stat boosts, and Lv99+ / storage limit breaks ([#187](https://github.com/witchcraze/party2re/issues/187))

### C. Items, Equipment, Storage, and Currency
- [x] Item definitions, 5-category data catalog, and instance ownership ([#11](https://github.com/witchcraze/party2re/issues/11), [#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Weapons, armor, shields, accessories, and consumables ([#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Inventory slot management and 5-slot equipment rules ([#19](https://github.com/witchcraze/party2re/issues/19))
- [x] Character Item Depot storage for items with dynamic capacity ([#58](https://github.com/witchcraze/party2re/issues/58), [#558](https://github.com/witchcraze/party2re/issues/558), [#559](https://github.com/witchcraze/party2re/issues/559))
- [x] Gold currency wallet & transactions (999,999G cap) ([#24](https://github.com/witchcraze/party2re/issues/24))
- [x] Item Shop purchase & 50% resale transactions ([#55](https://github.com/witchcraze/party2re/issues/55))
- [x] Bank accounts, gold deposits, and withdrawals with wallet clamp ([#71](https://github.com/witchcraze/party2re/issues/71), [#476](https://github.com/witchcraze/party2re/issues/476))
- [x] Blacksmith weapon seals (12 authentic seals consuming crystals), equipment naming, and 3-slot weapon storage ([#458](https://github.com/witchcraze/party2re/issues/458), [#632](https://github.com/witchcraze/party2re/issues/632))
- [x] Gem store, synthesis recipes, and dual-source (inventory & depot) weighted orb appraisals ([#72](https://github.com/witchcraze/party2re/issues/72), [#317](https://github.com/witchcraze/party2re/issues/317), [#560](https://github.com/witchcraze/party2re/issues/560))
- [x] Black Market underground trade and rare point barter ([#142](https://github.com/witchcraze/party2re/issues/142))
- [x] Small Medal collection and rare reward exchange ([#160](https://github.com/witchcraze/party2re/issues/160))
- [x] Character lifetime milestone achievements and commemorative medals ([#70](https://github.com/witchcraze/party2re/issues/70), [#358](https://github.com/witchcraze/party2re/issues/358))
- [x] Centralized transactional reward item delivery engine with configurable depot overflow routing ([#679](https://github.com/witchcraze/party2re/issues/679))
- [x] Unified dual-source item resolution and consumption across Inventory and Depot with deterministic Rank 3 -> Rank 5 lock ordering ([#683](https://github.com/witchcraze/party2re/issues/683))

### D. Adventure, Maps, Stages, and Battle
- [x] Reusable deterministic Battle component & turn resolver ([#12](https://github.com/witchcraze/party2re/issues/12), [#20](https://github.com/witchcraze/party2re/issues/20), [#36](https://github.com/witchcraze/party2re/issues/36))
- [x] Data-driven Stage Catalog (28 stages) and Monster Catalog (286 clean-room monsters) ([#56](https://github.com/witchcraze/party2re/issues/56))
- [x] Multi-stage adventure and dungeon exploration: Authentic 10-floor dungeon crawl loop, Floor 11 Treasure Room, depot overflow fallback, and immediate crawl execution ([#57](https://github.com/witchcraze/party2re/issues/57), [#74](https://github.com/witchcraze/party2re/issues/74), [#478](https://github.com/witchcraze/party2re/issues/478), [#655](https://github.com/witchcraze/party2re/issues/655))
- [x] Multiplayer party formation, co-op adventures, synergy bonuses, speed configs (3/18/25), need_join condition checks, distributed party adventure locking, and group quests ([#188](https://github.com/witchcraze/party2re/issues/188), [#478](https://github.com/witchcraze/party2re/issues/478), [#653](https://github.com/witchcraze/party2re/issues/653), [#656](https://github.com/witchcraze/party2re/issues/656))
- [x] Adventure history logs, stage clear stats, and milestone progression unlocks ([#199](https://github.com/witchcraze/party2re/issues/199))
- [x] Push-based background ScheduledAction completion via Valkey Worker ([#106](https://github.com/witchcraze/party2re/issues/106), [#109](https://github.com/witchcraze/party2re/issues/109), [#110](https://github.com/witchcraze/party2re/issues/110))
- [x] Colosseum PvP: Real-time 8-player Bet & Split combat, 9 team colors, multi-round party battle resolution, and distributed room locking ([#75](https://github.com/witchcraze/party2re/issues/75), [#481](https://github.com/witchcraze/party2re/issues/481), [#594](https://github.com/witchcraze/party2re/issues/594), [#644](https://github.com/witchcraze/party2re/issues/644), [#652](https://github.com/witchcraze/party2re/issues/652), [#661](https://github.com/witchcraze/party2re/issues/661))
- [x] Guild versus Guild (GvG) Combat: Live multi-round matches, GP prize pools, target wins, and 7-tier victory medals & championship cups ([#77](https://github.com/witchcraze/party2re/issues/77), [#482](https://github.com/witchcraze/party2re/issues/482), [#594](https://github.com/witchcraze/party2re/issues/594), [#644](https://github.com/witchcraze/party2re/issues/644), [#652](https://github.com/witchcraze/party2re/issues/652))
- [x] 4-player Party Sealing Boss Battles, Dejon banishment, Hero Count increments, and victory celebration banquets ([#73](https://github.com/witchcraze/party2re/issues/73), [#479](https://github.com/witchcraze/party2re/issues/479), [#657](https://github.com/witchcraze/party2re/issues/657))
- [x] Dungeon Exploration & Continuous Endurance Challenge multi-player runs, map scouting (@ちず), Hall of Fame records, and Valkey Master run buffers ([#162](https://github.com/witchcraze/party2re/issues/162), [#404](https://github.com/witchcraze/party2re/issues/404), [#405](https://github.com/witchcraze/party2re/issues/405), [#483](https://github.com/witchcraze/party2re/issues/483), [#597](https://github.com/witchcraze/party2re/issues/597), [#600](https://github.com/witchcraze/party2re/issues/600), [#657](https://github.com/witchcraze/party2re/issues/657))
- [x] Battle replay records and match history viewer ([#66](https://github.com/witchcraze/party2re/issues/66))
- [x] Standardized Battle Adapter: Character/Party to Battle Participant Mapping, equipment stat scaling, recipient-targeted item drop routing, and Post-Battle State Application ([#496](https://github.com/witchcraze/party2re/issues/496), [#593](https://github.com/witchcraze/party2re/issues/593), [#596](https://github.com/witchcraze/party2re/issues/596), [#599](https://github.com/witchcraze/party2re/issues/599), [#605](https://github.com/witchcraze/party2re/issues/605), [#643](https://github.com/witchcraze/party2re/issues/643), [#663](https://github.com/witchcraze/party2re/issues/663))

### E. Social and Competitive Systems
- [x] Guild creation, membership application/approval workflow, dynamic Guild Points, hex colors, custom titles, broadcast callouts, and inactivity disbandment worker ([#76](https://github.com/witchcraze/party2re/issues/76), [#490](https://github.com/witchcraze/party2re/issues/490), [#591](https://github.com/witchcraze/party2re/issues/591), [#633](https://github.com/witchcraze/party2re/issues/633))
- [x] Player communication, park, and public interactions ([#78](https://github.com/witchcraze/party2re/issues/78))
- [x] Player private home, mailbox, and letter correspondence ([#159](https://github.com/witchcraze/party2re/issues/159))
- [x] Helper and player rescue assistance ([#79](https://github.com/witchcraze/party2re/issues/79), [#213](https://github.com/witchcraze/party2re/issues/213))
- [x] Rankings (level, job, wealth, battle victories, helper, medals) with ISP-compliant repository interfaces ([#63](https://github.com/witchcraze/party2re/issues/63), [#280](https://github.com/witchcraze/party2re/issues/280), [#454](https://github.com/witchcraze/party2re/issues/454), [#470](https://github.com/witchcraze/party2re/issues/470))
- [x] Photo Contest, screenshots, seasonal voting, and Hall of Fame ([#186](https://github.com/witchcraze/party2re/issues/186))

### F. Economy and Side Systems
- [x] Alchemy: Free Overnight Depot-linked Synthesis & Compendium ([#60](https://github.com/witchcraze/party2re/issues/60), [#487](https://github.com/witchcraze/party2re/issues/487))
- [x] Player Auction house and free-market operations ([#80](https://github.com/witchcraze/party2re/issues/80))
- [x] Casino mini-games: Multi-Player Room Lobby (Candidate C), Indian Poker, High & Low, Doppelganger, Slot Machine, Prize Exchange, and distributed room locking ([#81](https://github.com/witchcraze/party2re/issues/81), [#82](https://github.com/witchcraze/party2re/issues/82), [#141](https://github.com/witchcraze/party2re/issues/141), [#397](https://github.com/witchcraze/party2re/issues/397), [#408](https://github.com/witchcraze/party2re/issues/408), [#453](https://github.com/witchcraze/party2re/issues/453), [#486](https://github.com/witchcraze/party2re/issues/486), [#590](https://github.com/witchcraze/party2re/issues/590), [#630](https://github.com/witchcraze/party2re/issues/630), [#635](https://github.com/witchcraze/party2re/issues/635), [#642](https://github.com/witchcraze/party2re/issues/642))
- [x] Lottery: Server-wide 20-cap Takarakuji lottery with pessimistic row locking, and Fukubiki raffle ([#83](https://github.com/witchcraze/party2re/issues/83), [#484](https://github.com/witchcraze/party2re/issues/484), [#485](https://github.com/witchcraze/party2re/issues/485), [#631](https://github.com/witchcraze/party2re/issues/631))
- [x] Plantation seed cultivation with 6 seeds, 14 fertilizers, overnight maturation, and Depot delivery ([#84](https://github.com/witchcraze/party2re/issues/84), [#489](https://github.com/witchcraze/party2re/issues/489))
- [x] Collection and Monster Book encyclopedia ([#85](https://github.com/witchcraze/party2re/issues/85))
- [x] Chapel prayers and blessings ([#86](https://github.com/witchcraze/party2re/issues/86), [#472](https://github.com/witchcraze/party2re/issues/472))
- [x] Event Plaza, traveling merchant bazaar, and victory celebration banquets linked to Valkey presence ([#161](https://github.com/witchcraze/party2re/issues/161), [#491](https://github.com/witchcraze/party2re/issues/491), [#634](https://github.com/witchcraze/party2re/issues/634))
- [x] Secret Underground Shop and NPC @ヒミツジ ([#192](https://github.com/witchcraze/party2re/issues/192))
- [x] Adventurer's Tavern, culinary menu, food delivery standing orders, and post-adventure fullness reset ([#185](https://github.com/witchcraze/party2re/issues/185), [#475](https://github.com/witchcraze/party2re/issues/475), [#595](https://github.com/witchcraze/party2re/issues/595), [#634](https://github.com/witchcraze/party2re/issues/634))
- [x] Flea Market player-to-player item stalls and SQL CAS status guard ([#194](https://github.com/witchcraze/party2re/issues/194), [#398](https://github.com/witchcraze/party2re/issues/398))
- [x] Monster Grandpa & Monster Ranch pet companion storage and stabling ([#193](https://github.com/witchcraze/party2re/issues/193), [#488](https://github.com/witchcraze/party2re/issues/488))
- [x] Altar of Rebirth: 6-Orb Offering, Ramia Awakening, and Otherworld Travel Item Wishes ([#471](https://github.com/witchcraze/party2re/issues/471), [#503](https://github.com/witchcraze/party2re/issues/503))
- [x] Player Store & Town Boutiques: store construction, depot listings, and interior styling ([#424](https://github.com/witchcraze/party2re/issues/424))

### G. Presentation, Assets, and Operations
- [x] UI-independent HTTP JSON Application API layer (OpenAPI 3.1) ([#87](https://github.com/witchcraze/party2re/issues/87), [#254](https://github.com/witchcraze/party2re/issues/254), [#266](https://github.com/witchcraze/party2re/issues/266))
- [x] MariaDB durable persistence & migration automation (`001`–`085`) ([#124](https://github.com/witchcraze/party2re/issues/124), [#399](https://github.com/witchcraze/party2re/issues/399))
- [x] Valkey worker queue with AOF+RDB persistence ([#106](https://github.com/witchcraze/party2re/issues/106))
- [x] Structured JSON logging with credential masking ([#49](https://github.com/witchcraze/party2re/issues/49))
- [x] Unified local verification script and pre-push hook ([#121](https://github.com/witchcraze/party2re/issues/121))
- [x] Minimal production container image published via GHCR ([#88](https://github.com/witchcraze/party2re/issues/88), [#128](https://github.com/witchcraze/party2re/issues/128))
- [x] Initial SVG placeholder assets ([#39](https://github.com/witchcraze/party2re/issues/39))
- [ ] Web presentation UI / client implementation ([#140](https://github.com/witchcraze/party2re/issues/140))
- [ ] Production asset production and license attribution ([#143](https://github.com/witchcraze/party2re/issues/143), [#202](https://github.com/witchcraze/party2re/issues/202))

---

## Reconstructed Implementation Issues Log (Grouped by Domain)

### 1. Core, Account & Progression Issues

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#4](https://github.com/witchcraze/party2re/issues/4) | Character persistence in MariaDB | Merged |
| [#5](https://github.com/witchcraze/party2re/issues/5) | Activity progression foundation | Merged |
| [#10](https://github.com/witchcraze/party2re/issues/10) | Level progression and cumulative exp thresholds | Merged |
| [#17](https://github.com/witchcraze/party2re/issues/17) | Job definitions and CharacterJob history | Merged |
| [#18](https://github.com/witchcraze/party2re/issues/18) | Skills definitions, costs, and availability | Merged |
| [#21](https://github.com/witchcraze/party2re/issues/21) | Player account creation and session lifecycle | Merged |
| [#24](https://github.com/witchcraze/party2re/issues/24) | Character initial identity, stats, and starting gold | Merged |
| [#31](https://github.com/witchcraze/party2re/issues/31) | Job-based stat growth on level-up | Merged |
| [#35](https://github.com/witchcraze/party2re/issues/35) | Atomic activity and adventure reward claims | Merged |
| [#38](https://github.com/witchcraze/party2re/issues/38) | Job catalog JSON loader and data validation | Merged |
| [#50](https://github.com/witchcraze/party2re/issues/50) | Exhaustive catalog test suite for Jobs | Merged |
| [#61](https://github.com/witchcraze/party2re/issues/61) | Job mastery tracking and Character Rebirth (superseded by #470) | Merged |
| [#69](https://github.com/witchcraze/party2re/issues/69) | Custom skill loadout and skill slot management | Merged |
| [#87](https://github.com/witchcraze/party2re/issues/87) | HTTP JSON Application API transport layer & status display | Merged |
| [#131](https://github.com/witchcraze/party2re/issues/131) | Player-character ownership verification linkage | Merged |
| [#163](https://github.com/witchcraze/party2re/issues/163) | Player personal access token (API Key) generation and dual authentication | Merged |
| [#187](https://github.com/witchcraze/party2re/issues/187) | Endgame wishes and stat limit break system (god.cgi, u_god.cgi) | Merged |
| [#198](https://github.com/witchcraze/party2re/issues/198) | Character name changes, gender changes, profile, and avatar customization | Merged |
| [#344](https://github.com/witchcraze/party2re/issues/344) | Fix OverMonster limit break description wording | Merged |
| [#467](https://github.com/witchcraze/party2re/issues/467) | Job change parity, SP-based mastery, item costs, and job memory exchange | Merged |
| [#468](https://github.com/witchcraze/party2re/issues/468) | Wishing Well (@女神) SP sacrifice exchange for permanent stat growth | Merged |
| [#470](https://github.com/witchcraze/party2re/issues/470) | Purge fictional Rebirth and align SP threshold progression | Merged |
| [#502](https://github.com/witchcraze/party2re/issues/502) | Stat Orb progression bonuses and Cursed/Mazin revival battle synergies | Merged |
| [#585](https://github.com/witchcraze/party2re/issues/585) | Core/Character: Standardize and consolidate updateCharacter helper across character services | Merged |

### 2. Storage, Commerce & Economy Issues

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#11](https://github.com/witchcraze/party2re/issues/11) | Items and inventory model | Merged |
| [#19](https://github.com/witchcraze/party2re/issues/19) | 5-Slot equipment system and item validation | Merged |
| [#51](https://github.com/witchcraze/party2re/issues/51) | Exhaustive 5-category Item catalog and test suite | Merged |
| [#55](https://github.com/witchcraze/party2re/issues/55) | Item Shop purchase and resale operations | Merged |
| [#58](https://github.com/witchcraze/party2re/issues/58) | Character Item Depot storage management | Merged |
| [#59](https://github.com/witchcraze/party2re/issues/59) | Blacksmith equipment enhancement (+1 to +10, superseded by #458) | Merged |
| [#60](https://github.com/witchcraze/party2re/issues/60) | Alchemy synthesis and 112 crafting recipes | Merged |
| [#71](https://github.com/witchcraze/party2re/issues/71) | Bank account management and player transfers | Merged |
| [#72](https://github.com/witchcraze/party2re/issues/72) | Gem Store, jewel synthesis, gem transfer, and orb appraisal | Merged |
| [#80](https://github.com/witchcraze/party2re/issues/80) | Player Auction House and marketplace trading | Merged |
| [#83](https://github.com/witchcraze/party2re/issues/83) | Lottery and raffle ticket drawings | Merged |
| [#84](https://github.com/witchcraze/party2re/issues/84) | Farm and plantation crop cultivation (superseded by #488, #489) | Merged |
| [#142](https://github.com/witchcraze/party2re/issues/142) | Town Black Market, contraband trading, dynamic pricing, and NPC @ヤミジ | Merged |
| [#160](https://github.com/witchcraze/party2re/issues/160) | Small Medal collection and rare reward exchange | Merged |
| [#194](https://github.com/witchcraze/party2re/issues/194) | Flea Market player-to-player item stalls and direct exchange | Merged |
| [#276](https://github.com/witchcraze/party2re/issues/276) | Reusable transactional wallet and inventory exchange helpers (internal/economy) | Merged |
| [#317](https://github.com/witchcraze/party2re/issues/317) | Gem Store appraisal and synthesis alignment | Merged |
| [#342](https://github.com/witchcraze/party2re/issues/342) | Adopt reusable economy.Service across commerce modules | Merged |
| [#357](https://github.com/witchcraze/party2re/issues/357) | Fix lock hierarchy inversion in shop.Sell and gemstore.SellGem to prevent database deadlocks | Merged |
| [#359](https://github.com/witchcraze/party2re/issues/359) | Clean up unused transaction fields and dead code in shop and medal services | Merged |
| [#398](https://github.com/witchcraze/party2re/issues/398) | Enforce Flea Market SQL CAS guard and add high-concurrency stress test | Merged |
| [#458](https://github.com/witchcraze/party2re/issues/458) | Blacksmith: Reproduce original weapon seals, naming, and blacksmith storage | Merged |
| [#471](https://github.com/witchcraze/party2re/issues/471) | Altar of Rebirth: 6-Orb Offering, Ramia Awakening, and Otherworld Travel Item Wishes | Merged |
| [#476](https://github.com/witchcraze/party2re/issues/476) | Bank: Purge fictional bank_accounts table, restoring direct character deposit column | Merged |
| [#484](https://github.com/witchcraze/party2re/issues/484) | Takarakuji: Purge fictional 4-digit lottery, reproducing 20-cap 30,000G rare equipment lottery | Merged |
| [#485](https://github.com/witchcraze/party2re/issues/485) | Fukubiki Raffle: Eliminate gold purchase and gold prizes, restoring Stat Seeds and Orbs | Merged |
| [#487](https://github.com/witchcraze/party2re/issues/487) | Alchemy: Free Overnight Depot-linked Synthesis & Compendium | Merged |
| [#489](https://github.com/witchcraze/party2re/issues/489) | Plantation: Reproduce Legacy Seed Cultivation, 14 Fertilizer Reagents, Overnight Harvest, and Depot Delivery | Merged |
| [#503](https://github.com/witchcraze/party2re/issues/503) | Altar orb offerings consume matching inventory items and reject absent orb items | Merged |
| [#557](https://github.com/witchcraze/party2re/issues/557) | Depot: Replace RemoveItem with ConsumeOne in Home, Black Market, and Gem Store | Merged |
| [#558](https://github.com/witchcraze/party2re/issues/558) | Depot: Prevent AddItem from collapsing equipment instances with different enhancement levels | Merged |
| [#559](https://github.com/witchcraze/party2re/issues/559) | Commerce: Standardize dynamic depot capacity calculation across Flea Market, Auction, and Player Store | Merged |
| [#560](https://github.com/witchcraze/party2re/issues/560) | Gem Store: Support dual-source appraisal from Depot storage in AppraiseItem | Merged |
| [#562](https://github.com/witchcraze/party2re/issues/562) | Storage: Standardize item consumption interface (Consume, ConsumeItem, ConsumeOne, PurgeSlot) | Merged |
| [#563](https://github.com/witchcraze/party2re/issues/563) | Core/Item: Enforce stackability invariant (IsStackable) across inventory and depot | Merged |
| [#631](https://github.com/witchcraze/party2re/issues/631) | Lottery: Fix 20-ticket purchase concurrency race, wrap draw in transaction, and prevent silent depot drops | Merged |
| [#679](https://github.com/witchcraze/party2re/issues/679) | Storage: Unify transactional reward item delivery and depot overflow routing into reusable domain helper | Merged |

### 3. Adventure, Combat & Dungeon Clusters

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#12](https://github.com/witchcraze/party2re/issues/12) | Reusable deterministic Battle component contract | Merged |
| [#13](https://github.com/witchcraze/party2re/issues/13) | Delayed Adventure flow with Battle integration | Merged |
| [#20](https://github.com/witchcraze/party2re/issues/20) | Turn-based battle resolution engine | Merged |
| [#36](https://github.com/witchcraze/party2re/issues/36) | Battle rewards mapping and application | Merged |
| [#56](https://github.com/witchcraze/party2re/issues/56) | Stage Catalog (28 stages) and Monster Catalog (286 monsters) | Merged |
| [#57](https://github.com/witchcraze/party2re/issues/57) | Multi-stage Adventure progression and drop rewards | Merged |
| [#66](https://github.com/witchcraze/party2re/issues/66) | Battle replay records and match history viewer | Merged |
| [#73](https://github.com/witchcraze/party2re/issues/73) | King and World Boss challenge battles | Merged |
| [#74](https://github.com/witchcraze/party2re/issues/74) | Dungeon exploration and branching stage encounters | Merged |
| [#75](https://github.com/witchcraze/party2re/issues/75) | Player versus Player arena combat | Merged |
| [#77](https://github.com/witchcraze/party2re/issues/77) | Guild versus Guild combat and territory competition | Merged |
| [#110](https://github.com/witchcraze/party2re/issues/110) | ScheduledAction push processing for Adventure completion | Merged |
| [#162](https://github.com/witchcraze/party2re/issues/162) | Continuous Endurance Challenge combat survival mode | Merged |
| [#188](https://github.com/witchcraze/party2re/issues/188) | Multiplayer co-op party system and group quests (quest.cgi, party.cgi) | Merged |
| [#199](https://github.com/witchcraze/party2re/issues/199) | Adventure history and gameplay record access (adventure_record.cgi) | Merged |
| [#341](https://github.com/witchcraze/party2re/issues/341) | Resolve Character ID vs Name collision and persist surviving HP in party battles | Merged |
| [#379](https://github.com/witchcraze/party2re/issues/379) | Connect party adventure victories and rewards to achievement progress tracking | Merged |
| [#478](https://github.com/witchcraze/party2re/issues/478) | Adventure: Eliminate 1-hour expedition timer and reproduce 10-Floor Party Dungeon Crawl with Treasure Rooms | Merged |
| [#479](https://github.com/witchcraze/party2re/issues/479) | Boss Battles: Purge fictional 1-day-3-attempts solo raid, reproducing 4-Player Party Sealing Battles | Merged |
| [#481](https://github.com/witchcraze/party2re/issues/481) | Colosseum PvP: Eliminate Elo Arena & Reproduce Real-time 8-Player Bet & Split Battles | Merged |
| [#482](https://github.com/witchcraze/party2re/issues/482) | Guild Battles: Eliminate Elo Duels & Reproduce Live GvG Multi-Round Matches with Trophy Decorations | Merged |
| [#483](https://github.com/witchcraze/party2re/issues/483) | Dungeon & Challenge Multi-Player: Support Party Exploration, Map Scouting Skills, and Hall of Fame Records | Merged |
| [#496](https://github.com/witchcraze/party2re/issues/496) | Battle Adapter: Standardize Character/Party to Battle Participant Mapping & Post-Battle State Application | Merged |
| [#593](https://github.com/witchcraze/party2re/issues/593) | Combat/Adapter: Wire Battle Adapter to all combat features and eliminate naked combatants | Merged |
| [#594](https://github.com/witchcraze/party2re/issues/594) | PvP+GvG: Fix multi-team survivor determination in Colosseum and Guild Battle rounds | Merged |
| [#596](https://github.com/witchcraze/party2re/issues/596) | Battle: Support Ex Amulet & Awakening Gem weapon attack scaling for Excalibur | Merged |
| [#597](https://github.com/witchcraze/party2re/issues/597) | Dungeon: Support stacking vision expansion for scouting jobs and scope goggles in map scouting (@ちず) | Merged |
| [#600](https://github.com/witchcraze/party2re/issues/600) | Challenge: Remove fictional 20% inter-round HP recovery and restore legacy HP carryover | Merged |
| [#605](https://github.com/witchcraze/party2re/issues/605) | Combat/PvP/GvG: Core battle engine multi-team 3+ faction support, inter-team targeting, and elimination loops | Merged |
| [#632](https://github.com/witchcraze/party2re/issues/632) | Battle/Blacksmith: Wire 12 weapon seal combat effects into Battle Adapter and restore crystal drops | Merged |
| [#643](https://github.com/witchcraze/party2re/issues/643) | Battle: Track depot overflow LostDrops on full depot and eliminate phantom deliveries | Merged |
| [#644](https://github.com/witchcraze/party2re/issues/644) | PvP+GvG: Standardize room repositories to Candidate C keyspace, UpdatedAt scoring, and lazy TTL pruning | Merged |
| [#652](https://github.com/witchcraze/party2re/issues/652) | PvP+GvG: Standardize distributed room locking with token-safe Lua release and reentrant execution | Merged |
| [#653](https://github.com/witchcraze/party2re/issues/653) | Party: Rank 0 distributed party adventure lock with token-safe Lua release | Merged |
| [#655](https://github.com/witchcraze/party2re/issues/655) | Adventure: Persist Floor 11 treasure box drops, depot fallback, and surviving HP/MP | Merged |
| [#656](https://github.com/witchcraze/party2re/issues/656) | Party: Route Floor 11 treasure drops to depot on full inventory, prevent silent drop loss, and persist MP | Merged |
| [#657](https://github.com/witchcraze/party2re/issues/657) | Combat: Replace raw SQL inventory inserts with capacity-validated model and Depot routing in Boss, Challenge, and Dungeon | Merged |
| [#661](https://github.com/witchcraze/party2re/issues/661) | PvP: Transactional settlement with Rank 2 row locking in ascending ID order | Merged |
| [#663](https://github.com/witchcraze/party2re/issues/663) | Battle: Fix party item drop duplication, propagate level-up errors, and support recipient-targeted drops in ApplyPostBattleResult | Merged |

### 4. Social, Town & Community Facilities

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#62](https://github.com/witchcraze/party2re/issues/62) | Character resting and Inn recovery (superseded by #459) | Merged |
| [#63](https://github.com/witchcraze/party2re/issues/63) | Rankings (level, job, wealth, battle victories, helper, medals) | Merged |
| [#67](https://github.com/witchcraze/party2re/issues/67) | News, announcements, and player notification system | Merged |
| [#70](https://github.com/witchcraze/party2re/issues/70) | Character lifetime milestone achievements and commemorative medals | Merged |
| [#76](https://github.com/witchcraze/party2re/issues/76) | Guild creation, management, and membership lifecycle | Merged |
| [#78](https://github.com/witchcraze/party2re/issues/78) | Town Park and public bulletin board | Merged |
| [#79](https://github.com/witchcraze/party2re/issues/79) | Player rescue and companion helper assistance | Merged |
| [#81](https://github.com/witchcraze/party2re/issues/81) | Casino Slot Machine mini-game and paytable | Merged |
| [#82](https://github.com/witchcraze/party2re/issues/82) | Casino Indian Poker mini-game and coin exchange | Merged |
| [#85](https://github.com/witchcraze/party2re/issues/85) | Monster Book encyclopedia and item collection catalog | Merged |
| [#86](https://github.com/witchcraze/party2re/issues/86) | Chapel prayer, blessings, and god worship system | Merged |
| [#141](https://github.com/witchcraze/party2re/issues/141) | Casino Doppelganger transformation and odds mini-game | Merged |
| [#159](https://github.com/witchcraze/party2re/issues/159) | Player private home, mailbox, and letter correspondence | Merged |
| [#161](https://github.com/witchcraze/party2re/issues/161) | Event Plaza, traveling merchant bazaar, and victory celebrations | Merged |
| [#185](https://github.com/witchcraze/party2re/issues/185) | Adventurer's Tavern, menu orders, delivery reservations, and NPC @エレナ | Merged |
| [#186](https://github.com/witchcraze/party2re/issues/186) | Photo contest, screenshots, seasonal voting, and Hall of Fame (photo.cgi / contest.cgi) | Merged |
| [#192](https://github.com/witchcraze/party2re/issues/192) | Secret Underground Shop and NPC @ヒミツジ | Merged |
| [#193](https://github.com/witchcraze/party2re/issues/193) | Monster Grandpa and pet companion storage (farm.cgi / monster.cgi) | Merged |
| [#195](https://github.com/witchcraze/party2re/issues/195) | Town item delivery quests (superseded by #475) | Merged |
| [#213](https://github.com/witchcraze/party2re/issues/213) | HTTP API endpoints for helper and rescue | Merged |
| [#280](https://github.com/witchcraze/party2re/issues/280) | Test/Ranking: Add unit tests for uncovered Get*Ranking service methods | Merged |
| [#358](https://github.com/witchcraze/party2re/issues/358) | Connect gameplay action producers to achievement milestone progress tracking | Merged |
| [#397](https://github.com/witchcraze/party2re/issues/397) | Implement Indian Poker multi-round session persistence and action API | Merged |
| [#408](https://github.com/witchcraze/party2re/issues/408) | Prevent false rejection on exact coins in Indian Poker action | Merged |
| [#453](https://github.com/witchcraze/party2re/issues/453) | Casino: Eliminate dual execution fallback paths and legacy repository exchange methods | Merged |
| [#454](https://github.com/witchcraze/party2re/issues/454) | Architecture/ISP: Decompose whitelisted legacy interfaces for guild and ranking | Merged |
| [#459](https://github.com/witchcraze/party2re/issues/459) | Inn: Decommission fictional paid Inn and consolidate sleep recovery under internal/home | Merged |
| [#472](https://github.com/witchcraze/party2re/issues/472) | Chapel: Purge fictional donations and align single active prayer constraint | Merged |
| [#475](https://github.com/witchcraze/party2re/issues/475) | Food Delivery: Eliminate fictional courier quests and reproduce Tavern Post-Adventure Food Delivery | Merged |
| [#486](https://github.com/witchcraze/party2re/issues/486) | Casino (Part 1): Multi-Player Room Lobby, Indian Poker, and Prize Depot Routing | Merged |
| [#488](https://github.com/witchcraze/party2re/issues/488) | Monster Ranch: Purge fictional farm crop system, reproducing Monster Stabling, Home Pet Link, Naming, and P2P Gift Delivery | Merged |
| [#490](https://github.com/witchcraze/party2re/issues/490) | Guild (Part 1): Purge Donation Leveling, Reproduce Dynamic Guild Points, Hex Colors, and Custom Titles | Merged |
| [#491](https://github.com/witchcraze/party2re/issues/491) | Event Plaza: Purge modern automated world events, reproducing Traveling Merchant Bazaar and King Celebration Banquet | Merged |
| [#590](https://github.com/witchcraze/party2re/issues/590) | Casino (Part 2): Multi-Player High-Low and Doppelganger Games | Merged |
| [#591](https://github.com/witchcraze/party2re/issues/591) | Guild (Part 2): Membership Application & Approval Workflow, Broadcast Callouts, Customization, and Inactivity Disbandment | Merged |
| [#595](https://github.com/witchcraze/party2re/issues/595) | Tavern: Restore food delivery standing order, remove erroneous deletion and fullness, and trigger on party adventure | Merged |
| [#630](https://github.com/witchcraze/party2re/issues/630) | Casino: Remove divergent reverse exchange, fix multi-winner SQL truncation, and restore authentic slot mechanics | Merged |
| [#633](https://github.com/witchcraze/party2re/issues/633) | Guild: Implement HTTP REST API endpoints, wire worker inactivity check, and align GP hooks | Merged |
| [#634](https://github.com/witchcraze/party2re/issues/634) | Event Plaza & Tavern: Link banquet attendees to Valkey presence and reset post-adventure fullness | Merged |
| [#635](https://github.com/witchcraze/party2re/issues/635) | Casino: Valkey Candidate C room repository migration and drop legacy relational tables | Merged |
| [#642](https://github.com/witchcraze/party2re/issues/642) | Casino: Enforce Valkey distributed room lock and token-safe Lua release on turns | Merged |

### 5. Architecture, Quality & Infrastructure

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#39](https://github.com/witchcraze/party2re/issues/39) | Initial placeholder assets for Character, Battle, Adventure, Job | Merged |
| [#49](https://github.com/witchcraze/party2re/issues/49) | Structured JSON logging with credential masking | Merged |
| [#88](https://github.com/witchcraze/party2re/issues/88) | Multi-stage production container build & GHCR publish | Merged |
| [#106](https://github.com/witchcraze/party2re/issues/106) | Reusable ScheduledAction queue & Valkey Worker | Merged |
| [#109](https://github.com/witchcraze/party2re/issues/109) | ScheduledAction push processing for Activity training | Merged |
| [#121](https://github.com/witchcraze/party2re/issues/121) | Unified local verification (`make check`) and Git pre-push hook | Merged |
| [#124](https://github.com/witchcraze/party2re/issues/124) | Safe database migration (`make db-migrate`, `make db-reset`) | Merged |
| [#128](https://github.com/witchcraze/party2re/issues/128) | Production container publish to GitHub Packages (GHCR) | Merged |
| [#134](https://github.com/witchcraze/party2re/issues/134) | Documentation maintenance structure and workflow rules | Merged |
| [#190](https://github.com/witchcraze/party2re/issues/190) | Player deletion, maintenance mode, and admin operations (delete.cgi, maintenance.cgi) | Merged |
| [#254](https://github.com/witchcraze/party2re/issues/254) | Complete HTTP API endpoints for remaining domain features | Merged |
| [#266](https://github.com/witchcraze/party2re/issues/266) | Integrate HTTP API server startup, routing orchestration, and graceful shutdown in cmd/party2 | Merged |
| [#343](https://github.com/witchcraze/party2re/issues/343) | Unify keyset cursor pagination helper into internal/pagination | Merged |
| [#345](https://github.com/witchcraze/party2re/issues/345) | Deprecate redundant progression_lint_test in favor of core_lint_test | Merged |
| [#356](https://github.com/witchcraze/party2re/issues/356) | Establish data persistence boundary guidelines (MariaDB vs Valkey Master) | Merged |
| [#363](https://github.com/witchcraze/party2re/issues/363) | Enforce deterministic row-lock hierarchy via Go AST linter | Merged |
| [#366](https://github.com/witchcraze/party2re/issues/366) | Migrate player sessions from MariaDB to Valkey with native TTL | Merged |
| [#367](https://github.com/witchcraze/party2re/issues/367) | Cache and master system maintenance state in Valkey | Merged |
| [#368](https://github.com/witchcraze/party2re/issues/368) | Ephemeral wait lobbies and ready-check state in Valkey Master | Merged |
| [#369](https://github.com/witchcraze/party2re/issues/369) | Evaluate and codify transient run state persistence boundary | Merged |
| [#370](https://github.com/witchcraze/party2re/issues/370) | Proof-of-concept for valkey-backed real-time shared world boss hp | Merged |
| [#374](https://github.com/witchcraze/party2re/issues/374) | Establish centralized Valkey keyspace specification and AST linting | Merged |
| [#377](https://github.com/witchcraze/party2re/issues/377) | Enforce atomic lobby mutations in Valkey and add concurrency stress tests | Merged |
| [#378](https://github.com/witchcraze/party2re/issues/378) | Prevent in-memory session leak and track sessions via sorted set with TTL purging | Merged |
| [#380](https://github.com/witchcraze/party2re/issues/380) | Resolve persistence boundary discrepancy and clean up dead schema/docs | Merged |
| [#384](https://github.com/witchcraze/party2re/issues/384) | Centralize database test fixtures, generic concurrency stress harness, and codify P2P stress test DoD | Merged |
| [#385](https://github.com/witchcraze/party2re/issues/385) | Standardize wall-clock test budget policies and establish benchmark framework | Merged |
| [#386](https://github.com/witchcraze/party2re/issues/386) | Codify TTL-scored sorted set with lazy purging pattern | Merged |
| [#387](https://github.com/witchcraze/party2re/issues/387) | Codify Lua scripting standards, execution limits, and cluster hash tagging | Merged |
| [#388](https://github.com/witchcraze/party2re/issues/388) | Apply fast-path byte pre-filtering across AST static analysis linters | Merged |
| [#399](https://github.com/witchcraze/party2re/issues/399) | Make connection pool parameters configurable per environment | Merged |
| [#404](https://github.com/witchcraze/party2re/issues/404) | Dungeon: Valkey run buffer with atomic Lua step processing (Candidate D) | Merged |
| [#405](https://github.com/witchcraze/party2re/issues/405) | Challenge: Valkey session buffer with atomic Lua round processing (Candidate D) | Merged |
| [#451](https://github.com/witchcraze/party2re/issues/451) | Test/Valkey: Implement offline unit test suite for Session repository | Merged |
| [#456](https://github.com/witchcraze/party2re/issues/456) | Test/Valkey: Implement offline unit test suites for Challenge and Party Valkey repositories | Merged |
| [#526](https://github.com/witchcraze/party2re/issues/526) | Modular Monolith: Automated Package Boundary and Cross-Feature Import Linter | Merged |
| [#528](https://github.com/witchcraze/party2re/issues/528) | Security: Automated Cryptographic Policy and Password Hashing Linter | Merged |
| [#531](https://github.com/witchcraze/party2re/issues/531) | Core/Random: Centralized Thread-Safe RNG Provider and Direct math/rand Prohibition | Merged |
| [#532](https://github.com/witchcraze/party2re/issues/532) | HTTP: Automated Linter Prohibiting context.Background and context.TODO in Handlers | Merged |
| [#533](https://github.com/witchcraze/party2re/issues/533) | Concurrency: Automated Linter Prohibiting Raw time.Sleep in Production Services | Merged |
| [#561](https://github.com/witchcraze/party2re/issues/561) | Transaction: Formalize multi-aggregate and P2P transaction runner primitives and align tx_runner linter | Merged |
| [#598](https://github.com/witchcraze/party2re/issues/598) | Test/Valkey: Implement offline unit test suites for PvP, GvG, and Dungeon Valkey repositories | Merged |
| [#599](https://github.com/witchcraze/party2re/issues/599) | Combat/Lint: Automated AST Linter Prohibiting Direct corebattle.NewParticipantFromCharacter in Feature Packages | Merged |
| [#682](https://github.com/witchcraze/party2re/issues/682) | Docs: Streamline STATUS.md, components.md, and feature-inventory.md to extract domain helper candidates | In Progress |

### 6. Extracted Domain Helper Primitives & Next Candidates (Rule of Three)

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#683](https://github.com/witchcraze/party2re/issues/683) | Storage: Unify dual-source item consumption and resolution across Inventory and Depot (`plantation`, `blackmarket`, `gemstore`) | Completed |
| [#684](https://github.com/witchcraze/party2re/issues/684) | Core/Character: Standardize vitality & fatigue state clamping and combat recovery helpers (`battle`, `boss`, `home`, `god`) | Completed |
| [#685](https://github.com/witchcraze/party2re/issues/685) | Core/Character: Enforce crystal currency mutation encapsulation and AST linter protection (`battle`, `home`, `blacksmith`, `core_lint_test`) | Planned |

---

## Version 1 Completion Checklist

- [ ] Every feature group has an implementation Issue and acceptance criteria.
- [x] Every Version 1 feature has domain or component tests.
- [x] Durable state is stored in MariaDB where required by the feature.
- [x] Valkey usage is documented per concrete cache, transient-state, queue, or coordination requirement.
- [ ] Required images are listed in `docs/assets/required-images.md`.
- [ ] Every final image has known provenance and an approved license.
- [x] No old source code or old image is included in the new implementation.
- [x] The full application can be built and operated using the documented development workflow.

---

## Related Documents

- [`../../STATUS.md`](../../STATUS.md)
- [`../../ROADMAP.md`](../../ROADMAP.md)
- [`../architecture/components.md`](../architecture/components.md)
- [`../design/game-overview.md`](../design/game-overview.md)
- [`../assets/required-images.md`](../assets/required-images.md)
- [`../../AGENTS.md`](../../AGENTS.md)
