# Version 1.0 Feature Inventory

> Temporary Version 1 reconstruction document.
>
> This document describes the behavior and content areas to be reconstructed
> from the `party2-main/` reference codebase. It is not a source-code migration plan. The old
> implementation and its assets remain reference material only.

## Scope decision

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

## Reference inventory

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

## Feature groups & Reconstruction Progress

All groups below are Version 1.0 reconstruction requirements.

### A. Application foundation and account lifecycle
- [x] Character/player registration & password hashing ([#21](https://github.com/witchcraze/party2re/issues/21))
- [x] Login and session authentication lifecycle ([#21](https://github.com/witchcraze/party2re/issues/21))
- [x] Character profile and status display ([#87](https://github.com/witchcraze/party2re/issues/87))
- [x] Player-character ownership verification linkage ([#131](https://github.com/witchcraze/party2re/issues/131))
- [ ] Personal Access Token (API Key) generation and authentication ([#163](https://github.com/witchcraze/party2re/issues/163))
- [ ] Player deletion and maintenance behavior
- [ ] Name changes and profile customization
- [x] Notifications, news, and player notification inbox ([#67](https://github.com/witchcraze/party2re/issues/67))
- [ ] Administrator operations

### B. Character, progression, jobs, and skills
- [x] Level and cumulative experience progression ([#10](https://github.com/witchcraze/party2re/issues/10))
- [x] Fundamental stats and initial character bounds ([#24](https://github.com/witchcraze/party2re/issues/24))
- [x] Job definitions, data catalog, and job change history ([#17](https://github.com/witchcraze/party2re/issues/17), [#38](https://github.com/witchcraze/party2re/issues/38), [#50](https://github.com/witchcraze/party2re/issues/50))
- [x] Job-based stat growth formulas ([#31](https://github.com/witchcraze/party2re/issues/31))
- [x] Skill definitions, costs, and availability conditions ([#18](https://github.com/witchcraze/party2re/issues/18))
- [x] Job mastery (Lv99) and Character Rebirth progression (+5 stat bonuses) ([#61](https://github.com/witchcraze/party2re/issues/61))
- [x] Inn resting and HP/MP recovery ([#62](https://github.com/witchcraze/party2re/issues/62))
- [x] Custom skill assignment ([#69](https://github.com/witchcraze/party2re/issues/69))

### C. Items, equipment, storage, and currency
- [x] Item definitions, 5-category data catalog, and instance ownership ([#11](https://github.com/witchcraze/party2re/issues/11), [#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Weapons, armor, shields, accessories, and consumables ([#51](https://github.com/witchcraze/party2re/issues/51))
- [x] Inventory slot management and 5-slot equipment rules ([#19](https://github.com/witchcraze/party2re/issues/19))
- [x] Character Item Depot storage for items and gold ([#58](https://github.com/witchcraze/party2re/issues/58))
- [x] Gold currency wallet & transactions ([#24](https://github.com/witchcraze/party2re/issues/24))
- [x] Item Shop purchase & 50% resale transactions ([#55](https://github.com/witchcraze/party2re/issues/55))
- [x] Bank accounts, gold deposits, withdrawals, and player-to-player transfers ([#71](https://github.com/witchcraze/party2re/issues/71))
- [x] Blacksmith equipment enhancement (+1 to +10) with material/gold costs ([#59](https://github.com/witchcraze/party2re/issues/59))
- [ ] Gem store currency and transactions ([#72](https://github.com/witchcraze/party2re/issues/72))
- [x] Black Market underground trade ([#142](https://github.com/witchcraze/party2re/issues/142))
- [x] Small Medal collection and rare reward exchange ([#160](https://github.com/witchcraze/party2re/issues/160))

### D. Adventure, maps, stages, and battle
- [x] Reusable deterministic Battle component & turn resolver ([#12](https://github.com/witchcraze/party2re/issues/12), [#20](https://github.com/witchcraze/party2re/issues/20), [#36](https://github.com/witchcraze/party2re/issues/36))
- [x] Data-driven Stage Catalog (28 stages) and Monster Catalog (286 clean-room monsters) ([#56](https://github.com/witchcraze/party2re/issues/56))
- [x] Multi-stage adventure and dungeon exploration with branching maps, traps, and boss encounters ([#57](https://github.com/witchcraze/party2re/issues/57), [#74](https://github.com/witchcraze/party2re/issues/74))
- [x] Adventure history logs, stage clear stats, and milestone progression unlocks ([#199](https://github.com/witchcraze/party2re/issues/199))
- [x] Push-based background ScheduledAction completion via Valkey Worker ([#106](https://github.com/witchcraze/party2re/issues/106), [#109](https://github.com/witchcraze/party2re/issues/109), [#110](https://github.com/witchcraze/party2re/issues/110))
- [x] Challenge content & special battle modes: Player versus Player arena combat ([#75](https://github.com/witchcraze/party2re/issues/75))
- [x] Challenge content & special battle modes: King & World Boss challenge battles ([#73](https://github.com/witchcraze/party2re/issues/73))
- [x] Challenge content & special battle modes: Continuous Challenge ([#162](https://github.com/witchcraze/party2re/issues/162))
- [x] Battle replay records and match history viewer ([#66](https://github.com/witchcraze/party2re/issues/66))

### E. Social and competitive systems
- [x] Guild creation, membership, and administration ([#76](https://github.com/witchcraze/party2re/issues/76))
- [x] Guild battles and territory competition: Guild versus Guild combat ([#77](https://github.com/witchcraze/party2re/issues/77))
- [x] Player communication, park, and public interactions ([#78](https://github.com/witchcraze/party2re/issues/78))
- [x] Player private home, mailbox, and letter correspondence ([#159](https://github.com/witchcraze/party2re/issues/159))
- [x] Helper and player rescue assistance ([#79](https://github.com/witchcraze/party2re/issues/79))
- [x] Rankings (level, job, wealth, battle victories, helper, rebirth, medals) ([#63](https://github.com/witchcraze/party2re/issues/63))

### F. Economy and side systems
- [x] Alchemy synthesis with 112 recipes & material requirements ([#60](https://github.com/witchcraze/party2re/issues/60))
- [x] Player Auction house and free-market operations ([#80](https://github.com/witchcraze/party2re/issues/80))
- [x] Casino mini-games: Slot Machine ([#81](https://github.com/witchcraze/party2re/issues/81)), Indian Poker ([#82](https://github.com/witchcraze/party2re/issues/82)), Doppel ([#141](https://github.com/witchcraze/party2re/issues/141)), High & Low ([#64](https://github.com/witchcraze/party2re/issues/64))
- [x] Lottery and raffle ticket systems ([#83](https://github.com/witchcraze/party2re/issues/83))
- [x] Farm and plantation cultivation ([#84](https://github.com/witchcraze/party2re/issues/84))
- [x] Collection and Monster Book encyclopedia ([#85](https://github.com/witchcraze/party2re/issues/85))
- [x] Chapel prayers and blessings ([#86](https://github.com/witchcraze/party2re/issues/86))
- [x] Event Plaza, traveling merchant bazaar, and victory celebrations ([#161](https://github.com/witchcraze/party2re/issues/161))
- [x] Secret Underground Shop and NPC @ヒミツジ ([#192](https://github.com/witchcraze/party2re/issues/192))
- [x] Adventurer's Tavern, menu orders, delivery reservations, and NPC @エレナ ([#185](https://github.com/witchcraze/party2re/issues/185))
- [x] Town item delivery quests and player-to-player parcel courier service ([#195](https://github.com/witchcraze/party2re/issues/195))
- [x] Flea Market player-to-player item stalls and direct fixed-price exchange ([#194](https://github.com/witchcraze/party2re/issues/194))

### G. Presentation, assets, and operations
- [x] UI-independent HTTP JSON Application API layer ([#87](https://github.com/witchcraze/party2re/issues/87))
- [x] MariaDB durable persistence & migration automation ([#124](https://github.com/witchcraze/party2re/issues/124))
- [x] Valkey worker queue with AOF+RDB persistence ([#106](https://github.com/witchcraze/party2re/issues/106))
- [x] Structured JSON logging with credential masking ([#49](https://github.com/witchcraze/party2re/issues/49))
- [x] Unified local verification script and pre-push hook ([#121](https://github.com/witchcraze/party2re/issues/121))
- [x] Minimal production container image published via GHCR ([#88](https://github.com/witchcraze/party2re/issues/88), [#128](https://github.com/witchcraze/party2re/issues/128))
- [x] Initial SVG placeholder assets ([#39](https://github.com/witchcraze/party2re/issues/39))
- [ ] Web presentation UI / client implementation
- [ ] Production asset production and license attribution

---

## Reconstructed Implementation Issues Log

| Issue | Scope / Milestone | Status |
| --- | --- | --- |
| [#4](https://github.com/witchcraze/party2re/issues/4) | Character persistence in MariaDB | Merged |
| [#5](https://github.com/witchcraze/party2re/issues/5) | Activity progression foundation | Merged |
| [#10](https://github.com/witchcraze/party2re/issues/10) | Level progression and cumulative exp thresholds | Merged |
| [#11](https://github.com/witchcraze/party2re/issues/11) | Items and inventory model | Merged |
| [#12](https://github.com/witchcraze/party2re/issues/12) | Reusable deterministic Battle component contract | Merged |
| [#13](https://github.com/witchcraze/party2re/issues/13) | Delayed Adventure flow with Battle integration | Merged |
| [#17](https://github.com/witchcraze/party2re/issues/17) | Job definitions and CharacterJob history | Merged |
| [#18](https://github.com/witchcraze/party2re/issues/18) | Skills definitions, costs, and availability | Merged |
| [#19](https://github.com/witchcraze/party2re/issues/19) | 5-Slot equipment system and item validation | Merged |
| [#20](https://github.com/witchcraze/party2re/issues/20) | Turn-based battle resolution engine | Merged |
| [#21](https://github.com/witchcraze/party2re/issues/21) | Player account creation and session lifecycle | Merged |
| [#24](https://github.com/witchcraze/party2re/issues/24) | Character initial identity, stats, and starting gold | Merged |
| [#31](https://github.com/witchcraze/party2re/issues/31) | Job-based stat growth on level-up | Merged |
| [#35](https://github.com/witchcraze/party2re/issues/35) | Atomic activity and adventure reward claims | Merged |
| [#36](https://github.com/witchcraze/party2re/issues/36) | Battle rewards mapping and application | Merged |
| [#38](https://github.com/witchcraze/party2re/issues/38) | Job catalog JSON loader and data validation | Merged |
| [#39](https://github.com/witchcraze/party2re/issues/39) | Initial placeholder assets for Character, Battle, Adventure, Job | Merged |
| [#49](https://github.com/witchcraze/party2re/issues/49) | Structured JSON logging with credential masking | Merged |
| [#50](https://github.com/witchcraze/party2re/issues/50) | Exhaustive catalog test suite for Jobs | Merged |
| [#51](https://github.com/witchcraze/party2re/issues/51) | Exhaustive 5-category Item catalog and test suite | Merged |
| [#55](https://github.com/witchcraze/party2re/issues/55) | Item Shop purchase and resale operations | Merged |
| [#56](https://github.com/witchcraze/party2re/issues/56) | Stage Catalog (28 stages) and Monster Catalog (286 monsters) | Merged |
| [#57](https://github.com/witchcraze/party2re/issues/57) | Multi-stage Adventure progression and drop rewards | Merged |
| [#58](https://github.com/witchcraze/party2re/issues/58) | Character Item Depot storage management | Merged |
| [#59](https://github.com/witchcraze/party2re/issues/59) | Blacksmith equipment enhancement (+1 to +10) | Merged |
| [#60](https://github.com/witchcraze/party2re/issues/60) | Alchemy synthesis and 112 crafting recipes | Merged |
| [#61](https://github.com/witchcraze/party2re/issues/61) | Job mastery tracking and Character Rebirth | Merged |
| [#62](https://github.com/witchcraze/party2re/issues/62) | Character resting and Inn recovery | Merged |
| [#71](https://github.com/witchcraze/party2re/issues/71) | Bank account management and player transfers | Merged |
| [#87](https://github.com/witchcraze/party2re/issues/87) | HTTP JSON Application API transport layer | Merged |
| [#88](https://github.com/witchcraze/party2re/issues/88) | Multi-stage production container build & GHCR publish | Merged |
| [#106](https://github.com/witchcraze/party2re/issues/106) | Reusable ScheduledAction queue & Valkey Worker | Merged |
| [#109](https://github.com/witchcraze/party2re/issues/109) | ScheduledAction push processing for Activity training | Merged |
| [#110](https://github.com/witchcraze/party2re/issues/110) | ScheduledAction push processing for Adventure completion | Merged |
| [#121](https://github.com/witchcraze/party2re/issues/121) | Unified local verification (`make check`) and Git pre-push hook | Merged |
| [#124](https://github.com/witchcraze/party2re/issues/124) | Safe database migration (`make db-migrate`, `make db-reset`) | Merged |
| [#134](https://github.com/witchcraze/party2re/issues/134) | Documentation maintenance structure and workflow rules | Merged |
| [#76](https://github.com/witchcraze/party2re/issues/76) | Guild creation, management, and membership lifecycle | Merged |
| [#82](https://github.com/witchcraze/party2re/issues/82) | Casino Indian Poker mini-game and coin exchange | Merged |
| [#81](https://github.com/witchcraze/party2re/issues/81) | Casino Slot Machine mini-game and paytable | Merged |
| [#141](https://github.com/witchcraze/party2re/issues/141) | Casino Doppelganger transformation and odds mini-game | Merged |
| [#83](https://github.com/witchcraze/party2re/issues/83) | Lottery and raffle ticket drawings | Merged |
| [#84](https://github.com/witchcraze/party2re/issues/84) | Farm and plantation crop cultivation | Merged |
| [#80](https://github.com/witchcraze/party2re/issues/80) | Player Auction House and marketplace trading | Merged |
| [#64](https://github.com/witchcraze/party2re/issues/64) | Casino mini-game foundation and High & Low | Merged |
| [#85](https://github.com/witchcraze/party2re/issues/85) | Monster Book encyclopedia and item collection catalog | Merged |
| [#86](https://github.com/witchcraze/party2re/issues/86) | Chapel prayer, blessings, and god worship system | Merged |
| [#75](https://github.com/witchcraze/party2re/issues/75) | Player versus Player arena combat | Merged |
| [#77](https://github.com/witchcraze/party2re/issues/77) | Guild versus Guild combat and territory competition | Merged |
| [#73](https://github.com/witchcraze/party2re/issues/73) | King and World Boss challenge battles | Merged |
| [#74](https://github.com/witchcraze/party2re/issues/74) | Dungeon exploration and branching stage encounters | Merged |
| [#66](https://github.com/witchcraze/party2re/issues/66) | Battle replay records and match history viewer | Merged |
| [#162](https://github.com/witchcraze/party2re/issues/162) | Continuous Endurance Challenge combat survival mode | Merged |
| [#69](https://github.com/witchcraze/party2re/issues/69) | Custom skill loadout and skill slot management | Merged |
| [#79](https://github.com/witchcraze/party2re/issues/79) | Player rescue and companion helper assistance | Merged |
| [#78](https://github.com/witchcraze/party2re/issues/78) | Town Park and public bulletin board | Merged |
| [#213](https://github.com/witchcraze/party2re/issues/213) | HTTP API endpoints for helper and rescue | Merged |
| [#67](https://github.com/witchcraze/party2re/issues/67) | News, announcements, and player notification system | Merged |
| [#159](https://github.com/witchcraze/party2re/issues/159) | Player private home, mailbox, and letter correspondence | Merged |
| [#199](https://github.com/witchcraze/party2re/issues/199) | Adventure history and gameplay record access (adventure_record.cgi) | Merged |
| [#254](https://github.com/witchcraze/party2re/issues/254) | Complete HTTP API endpoints for remaining domain features | Merged |
| [#266](https://github.com/witchcraze/party2re/issues/266) | Integrate HTTP API server startup, routing orchestration, and graceful shutdown in cmd/party2 | Merged |
| [#161](https://github.com/witchcraze/party2re/issues/161) | Event Plaza, traveling merchant bazaar, and victory celebrations | Merged |
| [#192](https://github.com/witchcraze/party2re/issues/192) | Secret Underground Shop and NPC @ヒミツジ | Merged |
| [#185](https://github.com/witchcraze/party2re/issues/185) | Adventurer's Tavern, menu orders, delivery reservations, and NPC @エレナ | Merged |
| [#142](https://github.com/witchcraze/party2re/issues/142) | Town Black Market, contraband trading, dynamic pricing, and NPC @ヤミジ | Merged |
| [#195](https://github.com/witchcraze/party2re/issues/195) | Town item delivery quests and player-to-player parcel courier service | Merged |
| [#194](https://github.com/witchcraze/party2re/issues/194) | Flea Market player-to-player item stalls and direct exchange | Merged |
| [#72](https://github.com/witchcraze/party2re/issues/72) | Gem Store, jewel synthesis, gem transfer, and orb appraisal | Merged |

---

## Version 1 completion checklist

- [ ] Every feature group has an implementation Issue and acceptance criteria.
- [x] Every Version 1 feature has domain or component tests.
- [x] Durable state is stored in MariaDB where required by the feature.
- [x] Valkey usage is documented per concrete cache, transient-state, queue, or coordination requirement.
- [ ] Required images are listed in `docs/assets/required-images.md`.
- [ ] Every final image has known provenance and an approved license.
- [x] No old source code or old image is included in the new implementation.
- [x] The full application can be built and operated using the documented development workflow.

---

## Related documents

- [`../../STATUS.md`](../../STATUS.md)
- [`../../ROADMAP.md`](../../ROADMAP.md)
- [`../design/game-overview.md`](../design/game-overview.md)
- [`../assets/required-images.md`](../assets/required-images.md)
- [`../../AGENTS.md`](../../AGENTS.md)
