# Components

This document defines the responsibilities and boundaries of the initial components.

These are conceptual boundaries. The initial implementation uses Go.

## Component definition

Each component should be describable by:

- Responsibility
- Inputs
- Outputs
- State
- Dependencies
- Public contract
- Persistence requirements
- Initial implementation

The implementation language is intentionally not part of the component's identity.

## Core

### Player

**Responsibility:** account-level identity and authentication-related state.

Does not own game-specific character state. Owns the relationship to characters created under the account.

Player persistence stores a salted, iterated password hash and never stores the
supplied password. Session state is an ephemeral record with explicit expiry and
revocation. Per RFC #356, session persistence is designated as a **Valkey Master**
entity (`session:<token> -> player_id` with 7-day TTL), eliminating relational
database connection pressure on every authenticated HTTP request while relying on
Valkey's native TTL for automated expiration.

### Character

**Responsibility:** the player's in-game character and its fundamental state.

Linked to an owning `Player` via `player_id` (enforced via foreign key constraint).
Owns invariants about its own state but should not become a God object containing every game system.
Access to character operations is authorized against the authenticated session's player identity.
Owns character renaming at the Naming Hall (`name_change.cgi`, 500,000 G, guild & flea market restrictions, uniqueness validation), gender/appearance changes (10,000 G), and custom profile bio, comment, and avatar image management (`character_profiles`).
Encapsulates wallet currency and medal operations (`AddMoney`, `DeductMoney`, `HasMoney`, `AddSmallMedals`, `DeductSmallMedals`, `HasSmallMedals`) with 0-debt invariants, non-negative bounds checking, and overflow capping. Direct mutations on `.Money` or `.SmallMedals` outside Core character and database mapping are mechanically banned by Go AST static analysis (`internal/core/core_lint_test.go`).

### Progression

**Responsibility:** level, experience, stats, and other fundamental character progression.

Progression consumes job growth values through the public Job definition contract. It does not contain a built-in catalog of job-specific data. It provides canonical domain helpers (`ApplyExperience`, `ApplyExperienceWithJob`, `ApplyExperienceWithProvider`, `MaxLevelForCharacter`) to calculate cumulative thresholds, handle OverLevel limit breaks up to Lv 150, and apply level-ups. Direct field mutation of character progression fields (`Experience`, `Level`) in feature modules is mechanically prohibited by Go AST static analysis (`internal/core/core_lint_test.go`, `internal/core/progression/progression_lint_test.go`).


### Job

**Responsibility:** job definitions, job availability rules, and character job
history.

Job definitions are content data, not executable progression logic. The initial
catalog is stored outside Go source as validated data under
`internal/core/job/data/`. The Job component loads and validates that data at
startup or construction time, then exposes definitions through a small lookup
contract. Consumers do not access the data file or catalog map directly.
Character job state (`CharacterJob`) encapsulates job transitions (`ChangeTo`) and mastery (`Master`, `IsMastered`). Direct mutation of `CurrentJobID` and `MasteredJobs` is prohibited outside Core and database layers and validated via Go AST static analysis.

The data file may contain stable IDs, display names, growth values, and simple
requirements. Dynamic requirements and dynamic growth formulas require explicit
rules and tests; they must not be approximated by silently replacing them with
fixed values.

Visual asset paths, provenance, and licenses are maintained separately from
domain definitions in the asset documentation and asset manifest. Job data
must not embed legacy image paths or legacy asset bytes.

### Item

**Responsibility:** item definitions and concrete item instances.

Keep definitions separate from instances.

### Inventory

**Responsibility:** ownership and storage of item instances.

Does not contain unrelated item behavior. Encapsulates item storage mutations via `Add`, `Consume`, and `Update` (with uniqueness checks and quantity validation). Direct mutations or slicing of `Inventory.Items` outside Core inventory and database mapping are mechanically prohibited by Go AST static analysis (`internal/core/core_lint_test.go`).

### Equipment

**Responsibility:** which eligible item instances are equipped and where.

Encapsulates slot assignments and un-equipping via `Equip` and `Unequip` (with slot suitability and ownership checks). Direct mutations of `Equipment.Slots` outside Core equipment and database mapping are mechanically prohibited by Go AST static analysis (`internal/core/core_lint_test.go`).

### Currency

**Responsibility:** generic ownership and movement of game currencies.

Avoid hard-coding every future currency into Core.


### Game Time / Scheduling

**Responsibility:** represent and process delayed actions without owning
feature-specific rules.

A `ScheduledAction` records what is scheduled and when it becomes executable.
The relevant feature module supplies the processing logic through an
`ActionHandler` contract. The scheduling mechanism itself contains no
game-rule logic.

**Domain model** (`internal/core/scheduling`):

- `ScheduledAction` — the unit of work:
  - `ID`, `ActionType`, `ActorID`, `Params`, `ScheduledAt`, `ExecuteAt`
  - `State` — allow-listed: `pending | processing | completed | failed`
  - `RetainUntil` — auto-expiry time (set by `MarkCompleted`/`MarkFailed`)
- State machine: `Pending → Processing → Completed | Failed`
- `Validate()` — enforces field-size limits and the known-state allow-list;
  must pass before any lock acquisition or handler dispatch.

**Queue storage** — Valkey (`internal/scheduling`):

| Valkey key | Type | Purpose |
|---|---|---|
| `party2:scheduled:pending` | sorted set | pending IDs scored by `ExecuteAt` unix timestamp |
| `party2:scheduled:action:{id}` | string (JSON) | full action data; TTL set on completion/failure |
| `party2:scheduled:lock:{id}` | string with TTL | distributed lock preventing duplicate execution |

**Worker** (`internal/scheduling.Worker`):

- polls `party2:scheduled:pending` on a configurable interval;
- acquires a per-action `SET NX EX` lock before processing;
- calls `Validate()` as defense-in-depth before lock acquisition;
- dispatches to the registered `ActionHandler` for the action's type;
- marks the action `completed` or `failed` and sets `RetainUntil` TTL;
- runs inside the main process; the same `Run(ctx)` loop can be moved to
  an independent Worker binary without interface changes.

**ActionHandler contract** (`internal/scheduling.ActionHandler`):

```go
type ActionHandler interface {
    Handle(ctx context.Context, action core_scheduling.ScheduledAction) error
}
```

Feature modules implement this interface and register with the Worker:

```go
worker.RegisterHandler("training_complete", myFeature)
```

**Safety guarantees:**

- `Validate()` rejects empty IDs, unknown states, oversized strings, and
  excess parameters before any lock or dispatch occurs.
- Malformed JSON in Valkey is detected on `FetchDue` and immediately
  removed from the queue so it cannot block processing.
- Stale queue entries (key missing) are cleaned up automatically.
- The per-action lock TTL prevents duplicate execution across concurrent
  Workers or restarts.

**Adding a new action type:**

1. Define the action type constant in the feature package.
2. Implement `ActionHandler.Handle`.
3. Call `scheduling.Service.Schedule` with the action type and params.
4. Register the handler on `Worker` at startup.

No changes to the scheduling mechanism itself are required.

### Domain Events

**Responsibility:** publish meaningful domain-level facts to decouple optional consumers.

Examples:

- BattleFinished
- QuestCompleted
- CharacterLeveledUp
- ItemObtained
- JobChanged
- GuildJoined

Events are not required for every operation.

## Shared components

### Battle

**Responsibility:** execute and represent battles between participants.

Conceptual model:

```text
Battle
  - Participants (Standardized Participant builder / character adapter)
  - State
  - Actions
  - Effects
  - Result (Structured TurnLogs for Replay)
```

Battle must not know whether it was initiated by a quest, guild, arena, boss, dungeon, or challenge feature. All combat modes build `Participant` inputs using the shared `corebattle.NewParticipantFromCharacter`, `corebattle.NewParticipantFromCharacterWithHP`, or `corebattle.ParticipantBuilder` adapters, identifying participants strictly by unique entity ID while retaining display names for combat turn logs. Direct assignment of `.Name` to `Participant.ID` is mechanically prohibited by Go AST static analysis (`internal/core/core_lint_test.go`).

### Adventure / Quest

**Responsibility:** define and execute adventure-oriented game flows, including destinations, encounters, durations, requirements, and rewards.

Adventure may use Battle but should not own Battle's implementation.

### Common Foundation & Cross-Cutting Packages

To avoid duplicate boilerplate while preventing monolithic "junk-drawer" packages, shared cross-cutting logic is organized into single-responsibility packages:

- **ID Generation (`internal/id`)**: Centralized cryptographically secure ID generation (`id.New()`, `id.Generate()`, `id.NewLength(n)`) ensuring thread-safe, consistent 16-byte (32 hex characters) identifiers across all domain models without per-package duplicate helper functions.
- **Pagination (`internal/pagination`)**: Reusable generic list container `Page[T]` (`items`, `total`, `limit`, `offset`), parameter structures (`Params`), keyset / cursor pagination container `CursorPage[T]` (`items`, `next_cursor`, `prev_cursor`, `limit`, `has_more`), parameter structures (`CursorParams`), cursor token encoder/decoder utilities (`EncodeCursor`, `DecodeCursor`, `EncodeIDCursor`, `DecodeIDCursor`), and request query parsers (`ParseRequest`, `ParseCursorRequest`, `Normalize`) ensuring standard limit/offset and keyset normalization and unified JSON envelopes across all HTTP list endpoints.
- **Validation (`internal/validation`)**: Standardized format validators (e.g. HEX color codes `#RRGGBB`, text length bounds, HTML tag sanitization).
- **HTTP Transport Middleware (`internal/api/http/middleware`)**: Reusable transport helpers, including session authentication, character ownership verification wrappers (`withAuthenticatedCharacter`), admin role guards, CORS policies, and rate limiters.
- **Shared Entity Persistence Helpers (`internal/database`)**: Standardized transactional update functions for shared Core entities (e.g. updating character progression, stats, and gold) across repository boundaries.

## Feature modules

Each feature owns its feature-specific rules and state. A feature may consume public contracts from Core or shared components, but must not access another feature's private implementation or database schema.

### Implemented Feature Modules

- **Activity** (`internal/activity`):
  - **Responsibility:** Delayed training actions and experience awards.
  - **Dependencies:** Character repository, Core Progression, Scheduling Service.
  - **Persistence:** `activities` table with atomic `ClaimAndApply` concurrency locking.
- **Adventure** (`internal/adventure`):
  - **Responsibility:** Multi-stage exploration (28 stages, 286 monsters), stage eligibility checks, Battle invocation, drop rewards, paginated past adventure history logs (`GET /characters/{id}/adventures`), aggregate combat chronicle statistics (`GET /characters/{id}/adventure-chronicle`), and milestone progression unlocks (Try Mode, Image Setting, Calm Mode, Hard Mode, Avatar Setting, Extreme Mode).
  - **Dependencies:** Stage/Monster catalogs, Battle Resolver, Character & Inventory repositories, Scheduling Service.
  - **Persistence:** `adventures` table with atomic battle outcome persistence and compound query indexes (`idx_adventures_character_started`, `idx_adventures_character_claimed`).
- **Shop** (`internal/shop`):
  - **Responsibility:** Item purchases (gold deduction + inventory addition) and resale (inventory removal + 50% gold refund).
  - **Dependencies:** Item Catalog, Character (wallet), Inventory, Economy.
  - **Persistence:** Single-transaction atomic updates via character/inventory repositories with deterministic lock hierarchy (`characters` -> `inventory_items`).
- **Depot** (`internal/depot`):
  - **Responsibility:** Long-term storage management for item instances and gold.
  - **Dependencies:** Character (wallet), Inventory.
  - **Persistence:** `character_depots` and `depot_items` tables with single-transaction commits.
- **Blacksmith** (`internal/blacksmith`):
  - **Responsibility:** Equipment enhancement (+1 to +10) with level-scaling gold and material costs and probability curves.
  - **Dependencies:** Character (wallet), Inventory.
  - **Persistence:** Atomic single-transaction `BlacksmithRepository`.
- **Alchemy** (`internal/alchemy`):
  - **Responsibility:** Crafting item synthesis from recipes (`recipes.json`) using inventory ingredients and gold fees.
  - **Dependencies:** Recipe Catalog, Item Catalog, Character (wallet), Inventory.
  - **Persistence:** Atomic single-transaction `AlchemyRepository`.
- **Bank** (`internal/bank`):
  - **Responsibility:** Bank account management, gold deposits, withdrawals, and player-to-player remittances.
  - **Dependencies:** Character (wallet).
  - **Persistence:** `bank_accounts` and `bank_transfers` tables with `FOR UPDATE` concurrency locking.
- **Inn** (`internal/inn`):
  - **Responsibility:** Character resting and full HP/MP recovery.
  - **Dependencies:** Character repository.
  - **Persistence:** Single-transaction character update.
- **Guild** (`internal/guild`):
  - **Responsibility:** Guild creation, membership lifecycle, role management (Leader, Officer, Member), notice board, gold donations, and level/capacity progression.
  - **Dependencies:** Character repository.
  - **Persistence:** `guilds` and `guild_members` tables in `internal/database/guild_repository.go` with single-guild foreign key uniqueness and transactional integrity.
- **Casino** (`internal/casino`):
  - **Responsibility:** Casino currency exchange (1 Coin = 20 G), account management, and mini-games including Indian Poker (52-card deck, blind wagering, dealer AI, showdown resolution), Slot Machine (3-reel, 5-symbol paytable, 100x 777 jackpot), Doppelganger (8-mark secret match, 4x/6x/8x pool multiplier), and High & Low (card rank prediction, 2x payout, multi-round streak doubling).
  - **Dependencies:** Character repository (wallet gold).
  - **Persistence:** `casino_accounts` table in `internal/database/casino_repository.go` with atomic transactional balance adjustments.
- **Lottery & Raffle** (`internal/lottery`):
  - **Responsibility:** Instant raffle drawings (standard/special orb tiers) and periodic 4-digit numbered lottery purchases, drawing settlement, and prize claims.
  - **Dependencies:** Character repository (wallet gold).
  - **Persistence:** `character_lottery`, `lottery_drawings`, and `lottery_tickets` tables in `internal/database/lottery_repository.go` with atomic transactional claiming.
- **Farm & Plantation** (`internal/farm`):
  - **Responsibility:** Multi-plot crop cultivation (planting seeds, watering for bonus yield, fertilizing for growth acceleration, time-based maturation, harvesting, and withering).
  - **Dependencies:** Character repository (wallet gold/items).
  - **Persistence:** `farm_plots` table in `internal/database/farm_repository.go` with unique per-character plot indexes.
- **Auction & Marketplace** (`internal/auction`):
  - **Responsibility:** Player item listing, starting bids, buyout instant purchases, outbid automatic refunds, duration-based expiration settlement, and cancellation.
  - **Dependencies:** Character repository (wallet gold/items).
  - **Persistence:** `auction_listings` table in `internal/database/auction_repository.go` with row-level transactional concurrency.
- **Collection & Monster Book** (`internal/collection`):
  - **Responsibility:** Illustrated monster defeat tracking (`character_monster_book`), item discovery recording (`character_item_collection`), and career completion percentage queries.
  - **Dependencies:** Character repository.
  - **Persistence:** `character_monster_book` and `character_item_collection` tables in `internal/database/collection_repository.go`.
- **Medal & Lifetime Achievements** (`internal/medal`):
  - **Responsibility:** Small Medal (ちいさなメダル) exchange shop (`medal.cgi`, consuming medals for rare equipment/items) and Lifetime Milestone Achievement & Commemorative Medal collection system tracking gameplay metrics (adventure victories, monsters slain, gold accumulated, bosses conquered, arena wins, casino games, alchemy crafts), event-driven progress recording via explicit decoupled producer hooks (`VictoryHook`, `GamePlayedHook`, `SynthesisHook`, `MonsterDefeatedHook`), milestone completion verification, double-claim prevention, and awarding commemorative medals (`character_medals`) and bonus small medals.
  - **Dependencies:** Core Character, Character repository, Core Inventory, Inventory repository, TransactionProvider (`economy.Service`), wired to action producers in `cmd/party2/main.go`.
  - **Persistence:** `character_achievements` and `character_medals` tables in `internal/database/achievement_repository.go` with pessimistic `FOR UPDATE` locking and ambient transaction propagation.
- **Chapel & Blessings** (`internal/chapel`):
  - **Responsibility:** Active town church prayer registration (`character_blessings`), donation management, and reward modifier calculation (+50% EXP/Gold chance, drop bonuses).
  - **Dependencies:** Character repository (wallet gold).
  - **Persistence:** `character_blessings` table in `internal/database/chapel_repository.go`.
- **Player versus Player (PvP) Arena** (`internal/pvp`):
  - **Responsibility:** Asynchronous player-versus-player arena combat against defending character snapshots, standard Elo rating calculation (K=32, base 1000), matchmaking query with account win-trading prevention, match history, and defense logs.
  - **Dependencies:** Core Battle Engine, Character repository, Core Progression.
  - **Persistence:** `arena_ratings` and `arena_matches` tables in `internal/database/pvp_repository.go` with transactional rating adjustments and match recording.
- **Guild versus Guild (GvG) Combat** (`internal/gvg`):
  - **Responsibility:** Asynchronous guild-versus-guild multi-round roster skirmishes, Elo rating adjustments (K=32, base 1000), victory medals and championship cup tiered promotions (5:1 ratios), Guild Points (Victory Points), guild EXP leveling, and match/round history logging.
  - **Dependencies:** Core Battle Engine, Guild repository, Character repository, Core Progression.
  - **Persistence:** `gvg_standings`, `gvg_matches`, and `gvg_match_rounds` tables in `internal/database/gvg_repository.go` with transactional standing, match, and reward persistence.
- **King & World Boss Battles** (`internal/boss`):
  - **Responsibility:** Legendary raid/boss encounters across 10 progressive tiers and ultimate world boss tier, entry gate requirements (level gate, prerequisite tier completion, daily attempt limits), milestone first-clear bonuses, item drop rewards, and boss clear leaderboard.
  - **Dependencies:** Core Battle Engine, Character repository, Inventory repository, Core Progression.
  - **Persistence:** `character_boss_records` and `boss_challenge_history` tables in `internal/database/boss_repository.go` with atomic reward and record updates.
- **Dungeon Exploration** (`internal/dungeon`):
  - **Responsibility:** Multi-floor grid dungeon navigation, branching tile event state machine (monster encounters, hazard traps, treasure chest loots, floor descent stairs, floor bosses, safe escape portals), buffered reward ledger, and atomic finalization on clear/escape vs forfeiture on wipeout.
  - **Dependencies:** Core Battle Engine, Character repository, Inventory repository, Core Progression.
  - **Persistence:** `character_dungeon_records`, `dungeon_active_expeditions`, and `dungeon_expedition_history` tables in `internal/database/dungeon_repository.go`.
- **Battle Replays & Match History** (`internal/replay`):
  - **Responsibility:** Recording and faithful playback of step-by-step turn logs (actions, damage/healing numbers, critical hits, logs, remaining HP snapshots) across all combat modes (PvP, GvG, Boss, Dungeon, Adventure, Challenge), standardized recording adapters (`ReplayRecorder`, `RecordMatchFromResult`, `RecordCharacterVsCharacter`, `RecordCharacterVsMonster`, `RecordParticipantVsParticipant`), character match history queries, and replay retention pruning.
  - **Dependencies:** Core Battle Engine, Core Character.
  - **Persistence:** `battle_replays` table in `internal/database/replay_repository.go`.
- **Continuous Endurance Challenge** (`internal/challenge`):
  - **Responsibility:** Consecutive survival wave combat resolution, progressive wave stat scaling ($1 + \text{scale} \times (\text{round}-1)$), inter-round 20% HP recovery, milestone bonus item drops, buffered reward ledger with safe retreat cashout vs 50% consolation defeat, and all-time highest streak leaderboards.
  - **Data Catalog:** Embedded JSON tier definitions (`internal/challenge/data/challenge_tiers.json`).
  - **Dependencies:** Core Battle Engine, Character repository, Inventory repository.
  - **Persistence:** `character_challenge_records` and `challenge_sessions` tables in `internal/database/challenge_repository.go`.
- **Custom Skill Loadout & Slot Management** (`internal/custom_skill`):
  - **Responsibility:** Cross-job ability customization, job mastery verification, slot capacity enforcement, tactical priority configuration, and supplying active equipped loadouts to combat participants across all battle modes.
  - **Data Catalog:** Embedded JSON skill catalog definitions (`internal/custom_skill/data/skills.json`).
  - **Dependencies:** Core Skill, Core Job, Core Character, Character repository, Character Job repository.
  - **Persistence:** `character_custom_skills` table in `internal/database/custom_skill_repository.go`.
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`):
  - **Responsibility:** Helper quest generation, item/monster delivery validation, alchemy material rewards, guild points contribution, emergency state rescue recovery with cooldown penalties, and HTTP JSON API endpoints (`/helpers/quests`, `/helpers/complete`, `/rescues/penalty`, `/rescues/request`).
  - **Dependencies:** Core Character, Core Inventory, Core Item, Guild repository.
  - **Persistence:** `helper_quests` and `rescue_records` tables in `internal/database/helper_repository.go` and `internal/database/rescue_repository.go`.
- **Town Park & Public Bulletin Board** (`internal/park`):
  - **Responsibility:** Public chat/bulletin board messages, character authorship, text sanitization, rate limiting, and NPC interactions (@町娘 talk, inspect, and 20-tier fortune divination).
  - **Dependencies:** Character repository (identity verification).
  - **Persistence:** `park_posts` table in `internal/database/park_repository.go`.
- **News & Player Notifications** (`internal/notification`):
  - **Responsibility:** System-wide news announcement broadcasts (`news.cgi`), categorized server announcements, personalized player notification inbox for asynchronous game alerts, read state tracking, unread count queries, and retention pruning.
  - **Dependencies:** Core Player.
  - **Persistence:** `news_articles` and `player_notifications` tables in `internal/database/notification_repository.go`.
- **Player Private Home & Mailbox** (`internal/home`):
  - **Responsibility:** Character private estate management (`home.cgi`), home wallpaper/theme customization, visitor tracking, player-to-player letter mail correspondence (inbox/outbox), companion greeting phrase customization (`ことばをおしえる`), and delivery notices ledger.
  - **Dependencies:** Core Character, Character repository.
  - **Persistence:** `character_homes`, `character_letters`, `character_companion_phrases`, and `character_delivery_notices` tables in `internal/database/home_repository.go`.
- **Player Leaderboards & Character Rankings** (`internal/ranking`):
  - **Responsibility:** Multi-category competitive leaderboards and character rankings (`ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`), including Level, Player Wealth, Character Wealth, Battle Victories, PvP Victories, World Boss Defeats, Adventure Victories, Job Mastery, Job Popularity, Helper Quests, Rebirth Count, and Small Medals, with deterministic tie-breaking, pagination, in-memory TTL caching, Valkey distributed caching, singleflight cache stampede protection, and background worker refresh action (`party2:ranking:refresh`).
  - **Dependencies:** Core Character, Core Player, Valkey (`github.com/valkey-io/valkey-go`), Scheduling (`internal/scheduling`).
  - **Persistence:** `ranking_snapshots` table and dedicated high-performance query indexes in `internal/database/ranking_repository.go`, and Valkey distributed cache keys (`party2:ranking:snapshot:*`).
- **Distributed Rate Limiting & Cooldown Tracking** (`internal/ratelimit`):
  - **Responsibility:** Atomic distributed rate limiting, burst protection, public endpoint spam defense, park bulletin board posting cooldowns, and private home visitor throttling without SQL lookup overhead.
  - **Dependencies:** Valkey (`github.com/valkey-io/valkey-go`) with in-memory thread-safe fallback.
  - **Persistence:** Transient atomic counter keys in Valkey with TTL (`party2:ratelimit:*`).
- **Server Entrypoint & Lifecycle Orchestration** (`cmd/party2`):
  - **Responsibility:** Process initialization, configuration loading (`PARTY2_DB_DSN`, `PARTY2_VALKEY_ADDR`, `PORT`/`ADDR`), full domain repository and service wiring, background scheduler worker execution, HTTP JSON API route registration, and graceful shutdown signal handling (`SIGINT`/`SIGTERM`) with connection draining.
  - **Dependencies:** All domain services and repositories, `internal/api/http`, `internal/scheduling`, `internal/database`, `internal/valkey`, `internal/ratelimit`, `internal/logging`.
  - **Persistence:** Coordinates connection lifecycles for MariaDB and Valkey.
- **OpenAPI 3.1 Modular Specification, Bundler, AST Scaffolder & CI Guard** (`docs/api/base.json`, `docs/api/paths/*.json`, `scripts/sync_openapi.go`, `internal/api/http`):
  - **Responsibility:** Standardized machine-readable OpenAPI 3.1 REST API specification modularized into domain-specific paths (`docs/api/paths/{module}.json`) and base schemas (`docs/api/base.json`) as Single Source of Truth (SSOT). Pure Go AST toolchain (`scripts/sync_openapi.go`) deterministically bundles paths into compiled artifacts (`docs/api/openapi.json` and embedded `internal/api/http/openapi.json`), automatically scaffolds missing route definitions from `internal/api/http/handler.go`, and enforces 100% route coverage via CI (`make openapi-check`). Automated Go AST auth static analysis (`internal/api/http/auth_lint_test.go`) enforces security wrappers across all endpoints.
  - **Dependencies:** Go standard library `net/http`, `go/ast`, `go/parser`, `//go:embed`.
  - **Persistence:** In-memory embedded artifact and modular version-controlled repository documents.
- **Event Plaza, Traveling Merchant Bazaar & Victory Banquets** (`internal/eventplaza`):
  - **Responsibility:** Town event plaza gathering state, dynamic population tier calculations, traveling merchant rare item bazaar catalog with tier-based unlocks and concurrency-safe purchasing transactions, and world boss victory celebration banquets with celebratory toast rewards. Strict session authentication and character ownership authorization on all mutating endpoints (`/eventplaza/merchant/purchase`, `/eventplaza/banquets/{id}/toast`).
  - **Dependencies:** Core Character, Core Item, Character repository, Inventory repository.
  - **Persistence:** `celebration_banquets` and `banquet_toasts` tables in `internal/database/eventplaza_repository.go`.
- **Secret Underground Shop & NPC @ヒミツジ** (`internal/secretshop`):
  - **Responsibility:** Secret underground shop discovery and access validation (Level >= 15 or Reborn), rare item catalog with 3x pricing multiplier, helper quest exclusion filter, concurrency-safe purchasing transactions, and humorous NPC interactions (sheep dialogues, inspect lore, and restorative `@ぱふぱふ` puff-puff service).
  - **Dependencies:** Core Character, Core Item, Core Inventory, Character repository, Inventory repository, Helper Quest filter.
  - **Persistence:** Direct inventory and character balance persistence via character/inventory repositories.
- **Adventurer's Tavern & Barkeep @エレナ** (`internal/tavern`):
  - **Responsibility:** Adventurer's Tavern culinary menu (14 food, drink, dessert, and full-course items), restorative HP/MP recovery meals with fullness tracking, lottery raffle ticket rewards, post-adventure meal delivery reservation and claim workflow, and barkeep dialogue interactions.
  - **Dependencies:** Core Character, Character repository, Lottery repository.
  - **Persistence:** `tavern_deliveries` and `tavern_character_status` tables in `internal/database/tavern_repository.go`.
- **Town Black Market & Shady Broker @ヤミジ** (`internal/blackmarket`):
  - **Responsibility:** Town Black Market contraband item trade (Level >= 10), dynamic market conditions (`Quiet`, `HotDemand`, `Crackdown`, `Bargain`) with buy price multipliers and sell buyback rates, daily purchase quotas, Rare Point and U-Rare Point sacrifice recycling system (`SacrificeItem`), exclusive prize trade exchange (`TradePrize`), pessimistic inventory and gold transaction handling, and shady broker dialogue and rumor intelligence.
  - **Dependencies:** Core Character, Core Item, Core Inventory, Character repository, Inventory repository.
  - **Persistence:** `blackmarket_character_purchases`, `blackmarket_market_state`, and `blackmarket_character_points` tables in `internal/database/blackmarket_repository.go`.
- **Town Delivery Quests & Player Courier Service** (`internal/delivery`):
  - **Responsibility:** Town item delivery quest generation and lifecycle (max 3 concurrent in-progress quests, atomic item verification & reward settlement), and player-to-player mail/parcel courier service with gold and item attachments, 50 G flat courier fee, and sender cancellation/refund workflow.
  - **Dependencies:** Core Character, Core Item, Core Inventory, Character repository, Inventory repository.
  - **Persistence:** `delivery_quests`, `character_deliveries`, and `delivery_parcels` tables in `internal/database/delivery_repository.go`.
- **Flea Market & Player Item Stalls** (`internal/fleamarket`):
  - **Responsibility:** Player-to-player direct fixed-price item marketplace (`free.cgi`), inventory listing creation (max 5 active listings per character, 1–999,999 G price range), atomic purchasing transactions with cross-character deterministic locking, and seller cancellation and item return workflows.
  - **Dependencies:** Core Character, Core Item, Core Inventory, Character repository, Inventory repository.
  - **Persistence:** `fleamarket_listings` table in `internal/database/fleamarket_repository.go`.
- **Gem Store & Jewel Synthesis** (`internal/gemstore`):
  - **Responsibility:** Gem retail shop, 55+ advanced gem synthesis formulas (`kako`), player gem transfers (`okuru`), and unidentified orb appraisals with weighted randomized loot pools (`kantei`) (`gem_store.cgi`, `_data.cgi` No. 251–255, NPC `@ジェマ`).
  - **Dependencies:** Core Character, Core Item, Core Inventory, Character repository, Inventory repository.
  - **Persistence:** Direct inventory and character balance persistence via character/inventory repositories with deterministic lock hierarchy (`characters` -> `inventory_items`).
- **Endgame God Wishes & Limit Breaks** (`internal/god`):
  - **Responsibility:** Celestial audiences in Heaven (天界, NPC `@神`, `god.cgi`) and Underworld (裏天界, NPC `@神?`, `u_god.cgi`), permanent character attribute enhancements (+40 all stats), currency/resource awards, Level 99+ limit breaks (raising character level cap to 150), and tier-up capacity limit breaks (depot capacity, monster storage, job memory, flea market listings, shop listings).
  - **Dependencies:** Core Character, Core Progression, Character repository, Depot repository, Inventory repository.
  - **Persistence:** `characters` table (`over_level`, `over_depot`, `over_monster`, `over_future`, `over_flea`, `over_store`) and `character_depots` capacity persistence.
- **Monster Grandpa & Pet Companions** (`internal/monster`):
  - **Responsibility:** Monster storage box (base capacity 50 up to 300 via `OverMonster`), home pet companions (up to 8 pets per home estate), taming/capturing, renaming, gifting to other players, and releasing into the wild (`farm.cgi` / `monster.cgi`, NPC `@モンジィ`).
  - **Dependencies:** Core Character, Character repository.
  - **Persistence:** `character_monsters` table in `internal/database/monster_repository.go`.
- **Photo Contest, Screenshots & Gallery** (`internal/contest`):
  - **Responsibility:** Character screenshots and photo gallery storage (up to 20 photos per character), photo contest entry submissions, community voting with comments, automated round conclusion with prize distribution (15,000 / 7,000 / 3,000 Gold, 10 / 6 / 3 Small Medals, 700 / 300 / 100 Guild Points), voter bonus medal distribution, Hall of Fame (殿堂入り / Legends) archiving, and news announcements (`photo.cgi` / `contest.cgi`, NPC `@ワコール`).
  - **Dependencies:** Core Character, Character repository, News publisher, Guild service.
  - **Persistence:** `character_photos`, `contest_rounds`, `contest_entries`, `contest_votes`, and `contest_legends` tables in `internal/database/contest_repository.go`.
- **Multiplayer Party & Co-op Quests** (`internal/party`):
  - **Responsibility:** Multiplayer party formation (1–4 members), recruitment lobbies, password protection, readiness synchronization, leader management (kick, disband), and coordinated multi-participant combat against dungeon/stage encounters with cooperative synergy multipliers (+10% to +30% EXP/Gold bonus) and shared reward distribution (`quest.cgi`, `party.cgi`).
  - **Dependencies:** Core Character, Core Battle, Core Inventory, Core Item, Core Progression, Character repository, Inventory repository, Stage/Monster Catalogs, News publisher.
  - **Persistence:** `parties`, `party_members`, and `party_adventure_logs` tables in `internal/database/party_repository.go`.
- **System Maintenance Mode** (`internal/maintenance`):
  - **Responsibility:** System-wide maintenance mode status management, public status queries, and administrative configuration (enable/disable, message, estimated end time) with HTTP middleware request interception.
  - **Dependencies:** Maintenance repository, Valkey client (`internal/valkey`).
  - **Persistence:** Valkey Master / In-Memory caching via `internal/maintenance/valkey_repository.go` (`party2:maintenance:status`) backed by `system_maintenance` table in `internal/database/maintenance_repository.go`, eliminating MariaDB queries on normal HTTP request routing.

### Cross-Module Transaction Orchestration & Ambient Context Propagation

Cross-module workflows spanning multiple distinct feature and core repositories (such as auction settlement transferring character gold, seller bank deposits, and inventory items) use the **Application Orchestrator Pattern**:
- **Ambient Context Boundary**: Transactions are established at the application service / orchestrator level via `database.RunInTx(ctx, db, fn)`.
- **Automatic Participation**: Repositories resolve their SQL executor via `database.ExecutorFromContext(ctx, r.db)` and automatically participate in the active transaction without explicit transaction object passing across domain layers.
- **Deadlock Prevention**: All repositories and orchestrators observe the deterministic lock acquisition hierarchy defined in [`feature-modules.md`](feature-modules.md) and [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md).
- **Deterministic Lock Hierarchy CI Guard (`make lock-lint`)**: A Go AST static analysis linter (`internal/database/lock_hierarchy_lint_test.go`) enforces the deterministic lock acquisition hierarchy across all production transactions in `internal/` during CI (`< 0.1s`), preventing multi-resource lock inversion cycles and runtime deadlocks.
- **Deterministic Two-Party Locking (`internal/id.Sort2`)**: When acquiring pessimistic row locks across two player/character entities (e.g. P2P transfers, flea market purchases, monster trading, gem sending), IDs are sorted in ascending alphanumeric order via `id.Sort2(idA, idB)` to ensure identical lock acquisition order regardless of initiator.
- **Standardized Transactional Exchange Helpers (`internal/economy`)**: Reusable domain service (`economy.Service`) providing atomic wallet currency deductions/credits (`DeductGold`, `AddGold`, `TransferGold`, `DeductSmallMedals`, `AddSmallMedals`), inventory operations (`GrantItem`, `ConsumeItemInstance`, `ConsumeItemDefinition`), and compound exchanges (`Exchange`) under deterministic lock hierarchy (`characters` -> `inventory_items`) with integer overflow protection (`SafeMultiply`).
- **Standardized Keyset Cursor Pagination (`internal/pagination`)**: Centralized generic pagination orchestration (`BuildCursorPage`, `BuildCursorPageWithMapper`, `DecodeCursorParts`) providing consistent keyset compound token encoding/decoding, limit truncation, and bidirectional cursor links across stream/log subsystems (`park`, `delivery`, `home`, `replay`, `adventure`).
 
### Storage Authority & Data Persistence Tiers (MariaDB vs. Valkey Master)

To maintain uncompromising durability for economic and progression assets while avoiding unnecessary relational database connection pressure for ephemeral state, storage authority is divided into three distinct tiers per [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md):

1. **MariaDB Master (Canonical Relational Persistence)**:
   - **Scope:** Player Accounts, Characters, Inventories, Equipment, Currencies, Jobs, Depots, Bank Accounts, Guilds, Persistent Feature State (e.g. farms, auctions, contests, parties), and Audit/Chronicle Records.
   - **Properties:** ACID transactions, foreign keys, deterministic row-lock hierarchy (Rank 0 -> 8), zero tolerance for uncommitted data loss.
2. **Valkey Master (Primary Authoritative Ephemeral Store)**:
   - **Scope:** Player Sessions (`session:<token>`), Distributed Locks (`party2:scheduled:lock:*`), Rate Limiting Counters (`party2:ratelimit:*`), Scheduled Action Queues (`party2:scheduled:queue`).
   - **Properties:** Pure in-memory/AOF persistence with **no backing SQL tables**. Governed by native TTL expiration. State loss during crash or eviction is limited to non-critical ephemeral records (e.g. requiring a player to re-login, with zero impact on assets or progression).
3. **Valkey Cache (Read Acceleration & Projections)**:
   - **Scope:** Competitive Leaderboards & Standings (`party2:ranking:snapshot:*`), Player Profile Projections.
   - **Properties:** MariaDB is the Single Source of Truth; Valkey holds read-optimized projections (e.g. Sorted Sets) for O(log N) operations. Reconstructible from MariaDB on cache miss.

#### Persistence Decision Tree (Durability & Rebuildability First)

```text
                    ┌─ Yes ─→ MariaDB Master (Wallets, Inventories, Progression)
Durability critical?
                    │
                    No
                    ↓
              Naturally expiring (TTL)?
                    │
             ┌──────┴──────┐
            Yes            No
             ↓              ↓
        Valkey Master   Rebuildable from SQL / audit logs?
        (Sessions)          │
                       ┌────┴────┐
                      Yes       No
                       ↓         ↓
                  Valkey Master MariaDB Master
                  (Queues)      (Audit Records)
```

#### Migration Candidates & Roadmap
- **Candidate A: Player Authentication Sessions (`sessions`)**: Formally approved for migration to Valkey Master (`session:<token> -> player_id, EX 604800`) to eliminate relational database connection bottleneck on every authenticated API request ([Issue #366](https://github.com/witchcraze/party2re/issues/366)).
- **Candidate B: System Maintenance Mode State (`system_maintenance`)**: Implemented via Valkey Master / In-Memory cache (`internal/maintenance/valkey_repository.go`) to eliminate synchronous MariaDB queries from `maintenanceMiddleware` on every incoming HTTP request ([Issue #367](https://github.com/witchcraze/party2re/issues/367)).
- **Candidate C: Party & Matchmaking Wait Lobbies (`party`, `matchmaking`)**: Candidate for Valkey Master (`party:lobby:{id}`) to decouple ephemeral wait states and 60-second ready checks from relational database tables ([Issue #368](https://github.com/witchcraze/party2re/issues/368)).
- **Candidate D: In-Progress Run Buffers (`dungeon_active_expeditions`, `challenge_sessions`)**: Candidate for Valkey Master step-by-step turn buffers, eliminating write amplification on MariaDB until final settlement upon exit/completion ([Issue #369](https://github.com/witchcraze/party2re/issues/369)).
- **Candidate E: World Boss Real-time Shared HP (`boss`)**: High-risk candidate requiring proof-of-concept for dual-write settlement (bridging Valkey atomic `DECRBY` with MariaDB transactional loot settlement) ([Issue #370](https://github.com/witchcraze/party2re/issues/370)).
- **Candidate F: Real-time Leaderboards (`ranking`)**: Remains classified as Valkey Cache (projection), not Valkey Master.

### Future Feature Modules

- Web Presentation UI / Client



## Component review criteria

For every new component ask:

1. What does this component own?
2. What does it deliberately not own?
3. What are its public inputs and outputs?
4. Which dependencies are necessary?
5. Could another implementation replace it without changing consumers?
6. Would adding another component of the same kind require changes here?
7. Is the component boundary justified by an actual responsibility rather than speculative abstraction?

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`feature-modules.md`](feature-modules.md) — feature boundaries.
- [`interfaces.md`](interfaces.md) — public component contracts.
- [`../design/game-overview.md`](../design/game-overview.md) — domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory architectural rules.
