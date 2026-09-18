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
- [x] Standardize vitality & fatigue state clamping and combat recovery helpers ([#684](https://github.com/witchcraze/party2re/issues/684))
- [x] Endgame wishes, stat boosts, and Lv99+ / storage limit breaks ([#187](https://github.com/witchcraze/party2re/issues/187))

### C. Items, Equipment, Storage, and Currency
- [x] Item definitions, 5-category data catalog, and instance ownership ([#11](https://github.com/witchcraze/party2re/issues/11), [#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Weapons, armor, shields, accessories, and consumables ([#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Inventory slot management and 5-slot equipment rules ([#19](https://github.com/witchcraze/party2re/issues/19))
- [x] Character Item Depot storage for items with dynamic capacity ([#58](https://github.com/witchcraze/party2re/issues/58), [#558](https://github.com/witchcraze/party2re/issues/558), [#559](https://github.com/witchcraze/party2re/issues/559), [#678](https://github.com/witchcraze/party2re/issues/678))
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
- [x] Crystal currency mutation encapsulation and AST linter protection ([#685](https://github.com/witchcraze/party2re/issues/685))
- [x] Centralized depot FindOrCreate helper and dynamic capacity refresh across item delivery and trade modules ([#712](https://github.com/witchcraze/party2re/issues/712))

### D. Adventure, Maps, Stages, and Battle
- [x] Reusable deterministic Battle component & turn resolver ([#12](https://github.com/witchcraze/party2re/issues/12), [#20](https://github.com/witchcraze/party2re/issues/20), [#36](https://github.com/witchcraze/party2re/issues/36))
- [x] Data-driven Stage Catalog (28 stages) and Monster Catalog (286 clean-room monsters) ([#56](https://github.com/witchcraze/party2re/issues/56))
- [x] Multi-stage adventure and dungeon exploration: Authentic 10-floor dungeon crawl loop, Floor 11 Treasure Room, depot overflow fallback, and immediate crawl execution ([#57](https://github.com/witchcraze/party2re/issues/57), [#74](https://github.com/witchcraze/party2re/issues/74), [#478](https://github.com/witchcraze/party2re/issues/478), [#655](https://github.com/witchcraze/party2re/issues/655))
- [x] Multiplayer party formation, co-op adventures, synergy bonuses, speed configs (3/18/25), need_join condition checks, distributed party adventure locking, and group quests ([#188](https://github.com/witchcraze/party2re/issues/188), [#478](https://github.com/witchcraze/party2re/issues/478), [#653](https://github.com/witchcraze/party2re/issues/653), [#656](https://github.com/witchcraze/party2re/issues/656))
- [x] Adventure history logs, stage clear stats, and milestone progression unlocks ([#199](https://github.com/witchcraze/party2re/issues/199))
- [x] Push-based background ScheduledAction completion via Valkey Worker ([#106](https://github.com/witchcraze/party2re/issues/106), [#109](https://github.com/witchcraze/party2re/issues/109), [#110](https://github.com/witchcraze/party2re/issues/110))
- [x] Colosseum PvP: Real-time 8-player Bet & Split combat, 9 team colors, multi-round party battle resolution, and distributed room locking ([#75](https://github.com/witchcraze/party2re/issues/75), [#481](https://github.com/witchcraze/party2re/issues/481), [#594](https://github.com/witchcraze/party2re/issues/594), [#644](https://github.com/witchcraze/party2re/issues/644), [#652](https://github.com/witchcraze/party2re/issues/652), [#661](https://github.com/witchcraze/party2re/issues/661))
- [x] Guild versus Guild (GvG) Combat: Live multi-round matches, GP prize pools, target wins, and 7-tier victory medals & championship cups ([#77](https://github.com/witchcraze/party2re/issues/77), [#482](https://github.com/witchcraze/party2re/issues/482), [#594](https://github.com/witchcraze/party2re/issues/594), [#644](https://github.com/witchcraze/party2re/issues/644), [#652](https://github.com/witchcraze/party2re/issues/652), [#675](https://github.com/witchcraze/party2re/issues/675))
- [x] 4-player Party Sealing Boss Battles, Dejon banishment, Hero Count increments, and victory celebration banquets ([#73](https://github.com/witchcraze/party2re/issues/73), [#479](https://github.com/witchcraze/party2re/issues/479), [#657](https://github.com/witchcraze/party2re/issues/657))
- [x] Dungeon Exploration & Continuous Endurance Challenge multi-player runs, map scouting (@ちず), Hall of Fame records, and Valkey Master run buffers ([#162](https://github.com/witchcraze/party2re/issues/162), [#404](https://github.com/witchcraze/party2re/issues/404), [#405](https://github.com/witchcraze/party2re/issues/405), [#483](https://github.com/witchcraze/party2re/issues/483), [#597](https://github.com/witchcraze/party2re/issues/597), [#600](https://github.com/witchcraze/party2re/issues/600), [#657](https://github.com/witchcraze/party2re/issues/657))
- [x] Battle replay records and match history viewer ([#66](https://github.com/witchcraze/party2re/issues/66))
- [x] Standardized Battle Adapter: Character/Party to Battle Participant Mapping, equipment stat scaling, recipient-targeted item drop routing, and Post-Battle State Application ([#496](https://github.com/witchcraze/party2re/issues/496), [#593](https://github.com/witchcraze/party2re/issues/593), [#596](https://github.com/witchcraze/party2re/issues/596), [#599](https://github.com/witchcraze/party2re/issues/599), [#605](https://github.com/witchcraze/party2re/issues/605), [#643](https://github.com/witchcraze/party2re/issues/643), [#663](https://github.com/witchcraze/party2re/issues/663))

### E. Social and Competitive Systems
- [x] Guild creation, membership application/approval workflow, dynamic Guild Points, hex colors, custom titles, broadcast callouts, and inactivity disbandment worker ([#76](https://github.com/witchcraze/party2re/issues/76), [#490](https://github.com/witchcraze/party2re/issues/490), [#591](https://github.com/witchcraze/party2re/issues/591), [#633](https://github.com/witchcraze/party2re/issues/633))
- [x] Player communication, park, and public interactions ([#78](https://github.com/witchcraze/party2re/issues/78))
- [x] Player private home, mailbox, and letter correspondence ([#159](https://github.com/witchcraze/party2re/issues/159), [#680](https://github.com/witchcraze/party2re/issues/680))
- [x] Helper and player rescue assistance ([#79](https://github.com/witchcraze/party2re/issues/79), [#213](https://github.com/witchcraze/party2re/issues/213), [#659](https://github.com/witchcraze/party2re/issues/659), [#680](https://github.com/witchcraze/party2re/issues/680))
- [x] Rankings (level, job, wealth, battle victories, helper, medals) with ISP-compliant repository interfaces ([#63](https://github.com/witchcraze/party2re/issues/63), [#280](https://github.com/witchcraze/party2re/issues/280), [#454](https://github.com/witchcraze/party2re/issues/454), [#470](https://github.com/witchcraze/party2re/issues/470))
- [x] Photo Contest, screenshots, seasonal voting, prize settlement error propagation, and Hall of Fame ([#186](https://github.com/witchcraze/party2re/issues/186), [#660](https://github.com/witchcraze/party2re/issues/660))

### F. Economy and Side Systems
- [x] Alchemy: Free Overnight Depot-linked Synthesis & Compendium ([#60](https://github.com/witchcraze/party2re/issues/60), [#487](https://github.com/witchcraze/party2re/issues/487))
- [x] Player Auction house and free-market operations ([#80](https://github.com/witchcraze/party2re/issues/80))
- [x] Casino mini-games: Multi-Player Room Lobby (Candidate C), Indian Poker, High & Low, Doppelganger, Slot Machine, Prize Exchange, and distributed room locking ([#81](https://github.com/witchcraze/party2re/issues/81), [#82](https://github.com/witchcraze/party2re/issues/82), [#141](https://github.com/witchcraze/party2re/issues/141), [#397](https://github.com/witchcraze/party2re/issues/397), [#408](https://github.com/witchcraze/party2re/issues/408), [#453](https://github.com/witchcraze/party2re/issues/453), [#486](https://github.com/witchcraze/party2re/issues/486), [#590](https://github.com/witchcraze/party2re/issues/590), [#630](https://github.com/witchcraze/party2re/issues/630), [#635](https://github.com/witchcraze/party2re/issues/635), [#642](https://github.com/witchcraze/party2re/issues/642), [#708](https://github.com/witchcraze/party2re/issues/708))
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
- [x] UI-independent HTTP JSON Application API layer (OpenAPI 3.1) ([#87](https://github.com/witchcraze/party2re/issues/87), [#254](https://github.com/witchcraze/party2re/issues/254), [#266](https://github.com/witchcraze/party2re/issues/266), [#690](https://github.com/witchcraze/party2re/issues/690))
- [x] MariaDB durable persistence & migration automation (`001`–`085`) ([#124](https://github.com/witchcraze/party2re/issues/124), [#399](https://github.com/witchcraze/party2re/issues/399))
- [x] Valkey worker queue with AOF+RDB persistence ([#106](https://github.com/witchcraze/party2re/issues/106))
- [x] Structured JSON logging with credential masking ([#49](https://github.com/witchcraze/party2re/issues/49))
- [x] Unified local verification script and pre-push hook ([#121](https://github.com/witchcraze/party2re/issues/121))
- [x] Minimal production container image published via GHCR ([#88](https://github.com/witchcraze/party2re/issues/88), [#128](https://github.com/witchcraze/party2re/issues/128))
- [x] Initial SVG placeholder assets ([#39](https://github.com/witchcraze/party2re/issues/39))
- [x] Documentation streamlining and SSOT consolidation ([#697](https://github.com/witchcraze/party2re/issues/697))
- [ ] Web presentation UI / client implementation ([#140](https://github.com/witchcraze/party2re/issues/140))
- [ ] Production asset production and license attribution ([#143](https://github.com/witchcraze/party2re/issues/143), [#202](https://github.com/witchcraze/party2re/issues/202))

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
