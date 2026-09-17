# Components

This document defines the responsibilities and boundaries of the application components.

These are conceptual boundaries. The primary implementation language is Go.

## Component Definition Model

Each component is described by:

- **Responsibility**: what the component owns and what invariants it protects.
- **Dependencies**: direct package or service dependencies.
- **Persistence Requirements**: database tables, keyspace patterns, and row-lock hierarchy tiers.

---

## Core Components

### Player

**Responsibility:** Account-level identity and authentication-related state. Does not own character state.
Player persistence stores a salted, iterated password hash (`bcrypt`, cost 12). Session state is ephemeral, mastered directly in **Valkey Master** (`party2:session:<token>` with 7-day native TTL and `party2:player:sessions:<player_id>` Sorted Set with lazy pruning). Also provides cryptographically secure Personal Access Tokens (`p2_sk_...`) hashed with SHA-256 in MariaDB (`player_api_tokens`) for API and tooling access. Dual authentication transparently handles sessions and PATs.

### Character

**Responsibility:** The player's in-game character identity and fundamental attributes.
Linked to `Player` via `player_id`. Owns character customization (naming hall, gender changes, profile bio/avatar), wallet operations (`Money`, strictly capped at 999,999G), and crystal currency (`Crystal`, capped at 999,999). Direct field mutations on currency and progression fields are mechanically prohibited outside Core and database mappers via Go AST static analysis.

### Progression

**Responsibility:** Level, cumulative experience, stats, and character progression formulas.
Consumes job growth definitions. Provides canonical domain helpers (`ApplyExperience`, `ApplyExperienceWithJob`, `MaxLevelForCharacter`) to calculate cumulative thresholds (`level²×10`), award Skill Points (SP) per level, trigger skill learning, and apply celestial OverLevel limit breaks up to Lv 150. Direct field mutation of `Experience`, `Level`, and `SP` is mechanically prohibited by AST linters.

### Job

**Responsibility:** Job catalog definitions, availability rules, and character job history.
Job definitions are content data loaded and validated at startup from `internal/core/job/data/`. Exposes definitions through a lookup contract. `CharacterJob` encapsulates transitions (`ChangeTo`), mastery (`RecordMastery`, `Master`), prerequisite checks, and retained mastery SP. Coordinates level-20 job change transactions and temporary mastered-job memory snapshots (`よびおこす`).

### Item

**Responsibility:** Item definitions, 5-category catalog resolution, and item instances.
Separates definitions from instances. Enforces domain stackability invariants (`IsStackable() == (Slot == SlotNone)`), ensuring equipment items and enhanced items cannot stack (`Quantity` strictly 1) across storage systems (`internal/depot`, `internal/core/inventory`).

### Inventory

**Responsibility:** Ownership and storage of active character item instances.
Encapsulates item storage mutations via `Add`, `Consume`, `ConsumeItem`, `ConsumeOne`, `ConsumeOneItem`, and `Update` with quantity validation. Direct mutations of `Inventory.Items` outside Core and database mapping are prohibited via AST static analysis.

### Equipment

**Responsibility:** Slot assignments and un-equipping for eligible item instances.
Encapsulates slot assignments via `Equip` and `Unequip` across 5 equipment slots (weapon, armor, shield, accessory 1, accessory 2) with suitability and ownership checks.

### Currency

**Responsibility:** Generic ownership and movement invariants of game currencies.
Provides boundary interfaces for gold, small medals, casino coins, and crystal currencies without hard-coding ad-hoc rules into unrelated modules.

### Game Time / Scheduling

**Responsibility:** Represent and execute delayed actions without owning feature rules.
`ScheduledAction` (`internal/core/scheduling`) records work units with an explicit state machine (`Pending → Processing → Completed | Failed`). Queue storage is managed in Valkey (`internal/scheduling`):
- `party2:scheduled:pending` (Sorted Set scored by `ExecuteAt` timestamp)
- `party2:scheduled:action:{id}` (action payload string)
- `party2:scheduled:lock:{id}` (distributed lock preventing duplicate execution)
The background `Worker` acquires locks, validates payloads, and dispatches to registered `ActionHandler` implementations.

### Domain Events

**Responsibility:** Publish meaningful domain facts (`BattleFinished`, `CharacterLeveledUp`, `ItemObtained`, etc.) to decouple optional consumers via a two-phase in-memory dispatcher (`internal/core/event`).

---

## Shared Components

### Battle

**Responsibility:** Deterministic combat resolution and post-battle state application.
Battle engine operates independently of callers (adventures, arena, GvG, bosses, dungeons, challenges):
- **Turn Resolver (`internal/core/battle`)**: Deterministic 1v1 and party combat (up to 4 allies vs 1–N foes) as well as multi-team combat (3+ factions up to 8 participants) with agility-ordered actions, MP/CMP skill costs, elemental field states, anti-field cancellation, and pre-death survival triggers.
- **Battle Adapter (`internal/battle`)**: Standardized integration service bridging Character/Party entities to combat `Participant` models. Calculates authentic equipment stats across 71 weapons, 55 armors, and accessories, binds passive triggers, and provides atomic `ApplyPostBattleResult` enforcing deterministic row-lock hierarchy (Rank 2: Characters ascending -> Rank 3: Inventories/Equipment ascending -> Rank 5: Depots ascending), item consumption persistence, recipient-targeted drop distribution, progression error propagation, and depot overflow fallback.
- **AST Guard**: Prohibits direct construction of un-adapted participants in feature modules (`battle_adapter_lint_test.go`).

### Adventure / Quest

**Responsibility:** Define and execute adventure-oriented game flows, dungeon crawl loops, encounter progressions, and stage reward distributions.

### Common Foundation Packages

- **ID Generation (`internal/id`)**: Centralized 16-byte (32 hex characters) cryptographically secure identifiers (`id.New()`, `id.Sort2(a, b)`).
- **Pagination (`internal/pagination`)**: Reusable offset and keyset cursor containers (`Page[T]`, `CursorPage[T]`) with token encoding/decoding (`EncodeCursor`, `DecodeCursor`).
- **Validation (`internal/validation`)**: Standardized validators (HEX colors, text bounds, string sanitization).
- **Random Number Generation (`internal/core/random`)**: Centralized thread-safe pseudo-random generator backed by `math/rand/v2` and deterministic seeded generator for reproducible tests. Direct imports of `math/rand` in production packages are prohibited by AST linter.
- **Concurrency & Cancellation**: Cooperative context cancellation across services; raw `time.Sleep` is prohibited by AST linter.
- **Database Infrastructure (`internal/database`)**: Ambient transaction propagation (`RunInTx`, `ExecutorFromContext`) and connection pool lifecycle management.

---

## Feature Modules

Each feature owns its specific rules and state. Cross-feature imports and direct `internal/database` imports from feature packages are mechanically prohibited by AST static analysis.

- **Activity** (`internal/activity`): Delayed training actions and experience awards.
  - *Dependencies:* Character repository, Core Progression, Scheduling Service.
  - *Persistence:* `activities` table with atomic `ClaimAndApply` concurrency locking.
- **Adventure** (`internal/adventure`): 10-floor dungeon crawl loop across 28 stages (286 monsters), Floor 11 Treasure Room resolution, post-battle settlement delegation, inventory persistence, depot overflow fallback, and combat chronicles.
  - *Dependencies:* Stage/Monster catalogs, Battle Resolver, Battle Adapter, Character & Inventory repositories.
  - *Persistence:* `adventures` table; transactional lock hierarchy (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots`).
- **Shop** (`internal/shop`): Town equipment and item shops with 2× retail pricing, 50% markdown, job-level catalog gates, and depot auto-delivery.
  - *Dependencies:* Item Catalog, Character, Inventory, Depot, Helper, Collection, Economy.
  - *Persistence:* Atomic updates via global lock hierarchy (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots`).
- **Depot** (`internal/depot`): Persistent item storage with dynamic capacity scaling, expansions, sorting, item sales, direct-sending, stackability preservation, standardized item consumption (`Consume`, `ConsumeOne`, `PurgeSlot`), standardized `RefreshCapacity` helper, and centralized reward delivery engine (`DeliverRewardItem`, `DeliverRewardItems`) enforcing Rank 3 -> Rank 5 lock ordering and configurable overflow policies.
  - *Dependencies:* Character, Inventory, Economy, Collection hook.
  - *Persistence:* `character_depots` and `depot_items` tables via `economy.TransactionRunner` and `RunInTx` (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots`).
- **Blacksmith** (`internal/blacksmith`, `internal/battle`): 12 authentic weapon seals consuming crystal currency (`character.crystal`), equipment naming, dedicated 3-slot weapon storage (`blacksmith_deposits`), seal combat effects wired into Battle Adapter, and monster crystal drops.
  - *Dependencies:* Character, Inventory, Equipment, Blacksmith Repository.
  - *Persistence:* `blacksmith_deposits` table and character customization columns (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 8 `blacksmith_deposits`).
- **Alchemy** (`internal/alchemy`): Crafting item synthesis from 112 recipes consuming Depot materials directly without gold fees, Depot-direct output delivery, home rest completion, and Recipe Compendium tracking.
  - *Dependencies:* Recipe Catalog, Item Catalog, Character, Depot, Economy.
  - *Persistence:* `character_alchemy` and `character_alchemy_recipes` tables (Rank 2 `characters` -> Rank 5 `character_depots` -> Rank 8 `character_alchemy`).
- **Plantation** (`internal/plantation`): Seed cultivation facility supporting 6 seeds, 14 fertilizer reagents (Gold or Depot/Inventory items), overnight maturation (`timer.NextMidnightJST`), wither/yield bonuses, and Depot harvest delivery.
  - *Dependencies:* Item Catalog, Character, Inventory, Depot, Timer service, Database (`RunInTx`).
  - *Persistence:* `plantation_plots` table (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots` -> Rank 8 `plantation_plots`).
- **Bank** (`internal/bank`): Gold savings deposits and withdrawals with 999,999G wallet clamp.
  - *Dependencies:* Character repository.
  - *Persistence:* `characters.deposit` column with Tier 2 row locking.
- **Home & Resting** (`internal/home`): Private home profiles, visitor counters, letters/mailbox, companion phrases, sleep recovery (full HP/MP/tired recovery, online-scaled countdown lock, fullness and chapel resets), and atomic home consumable item usage.
  - *Dependencies:* Character, Home repository, Timer service, Economy, Inventory, Depot, Tavern, Chapel.
  - *Persistence:* `character_homes`, `home_letters`, `companion_phrases`, `home_delivery_notices` tables, Valkey timers (`party2:timer:sleep:*`, `party2:timer:asleep:*`).
- **Guild** (`internal/guild`, `internal/api/http`): Guild founding, membership application/approval workflow, dynamic Guild Points (`gpoint`) across social and combat hooks, custom role titles, unique hex colors, broadcast callouts, visual personalization, daily 20-day inactivity disbandment worker, and REST API endpoints.
  - *Dependencies:* Character repository, Letter sender interface.
  - *Persistence:* `guilds` and `guild_members` tables.
- **Casino** (`internal/casino`): Casino currency exchange (1 Coin = 20G), Multi-Player Room Lobby (2..8 players), authentic 13-card Indian Poker, multi-player High & Low, multi-player Doppelganger, 3-reel slot machine, and 18 authentic prizes with Depot auto-routing.
  - *Dependencies:* Character, Depot repository.
  - *Persistence:* `casino_accounts` in MariaDB (Rank 2 -> Rank 5 -> Rank 8). Ephemeral multiplayer rooms and turn state are mastered in Valkey (`party2:casino:*`, `ValkeyRoomRepository`) with 1800s sliding TTL, active ZSet index, distributed room locking (`party2:casino:lock:room:<room_id>`), and Two-Phase Settlement.
- **Lottery & Raffle** (`internal/lottery`): Server-wide 20-cap Takarakuji lottery with pessimistic row locking (`takarakuji_rounds` Rank 0 `FOR UPDATE`), 10-day drawing cycles, and Depot prize delivery. Tavern Fukubiki raffle with 3-coupon Standard and 300-coupon Special draws with depot overflow routing.
  - *Dependencies:* Character, Inventory, Depot, Item Catalog, Collection, TransactionProvider, Scheduling.
  - *Persistence:* `character_lottery`, `takarakuji_rounds`, and `takarakuji_tickets` tables.
- **Auction & Marketplace** (`internal/auction`): Live P2P trading hall (`@おくる`/`@しらべる`) with gold and equipped item transfers to depots.
  - *Dependencies:* Character, Equipment, Inventory, Depot, Item Catalog.
  - *Persistence:* State updates across `characters`, `inventory_items`, and `character_depot_items` with Rank 2 -> 3 -> 5 locking.
- **Collection & Monster Book** (`internal/collection`): Illustrated monster defeat tracking and item discovery recording.
  - *Dependencies:* Character repository.
  - *Persistence:* `character_monster_book` and `character_item_collection` tables.
- **Medal & Lifetime Achievements** (`internal/medal`): Small Medal exchange shop and Lifetime Milestone Achievement tracking with decoupled producer hooks (`VictoryHook`, `GamePlayedHook`, `SynthesisHook`, etc.).
  - *Dependencies:* Character, Inventory, TransactionProvider, Action producers.
  - *Persistence:* `character_achievements` and `character_medals` tables.
- **Chapel & Blessings** (`internal/chapel`): Town church prayer registration, 5 blessing choices, and single active wish enforcement.
  - *Dependencies:* Character repository.
  - *Persistence:* `character_blessings` table.
- **Colosseum PvP (闘技場)** (`internal/pvp`): Real-time 2..8 player room recruitment, Bet & Split prize pools, 9 team colors, multi-round party battle resolution, durable `pvp_wins` tracking, and 10-round draw refund safety.
  - *Dependencies:* Battle Engine, Character repository, TransactionProvider.
  - *Persistence:* Ephemeral room state in Valkey Master (`party2:pvp:*`) with distributed locking; durable wealth and wins in MariaDB with Rank 2 row locking in ascending ID order.
- **Guild versus Guild (GvG) Combat** (`internal/gvg`): Real-time 2..8 player guild battle rooms, room GP prize pool seeding, multi-round battle resolution, round winner GP, match victory awards, and 7-tier cascading victory medals & championship cups.
  - *Dependencies:* Battle Engine, Guild repository, Character repository.
  - *Persistence:* Ephemeral rooms in Valkey Master (`party2:gvg:*`) with distributed locking; durable standings and trophy tiers in MariaDB `gvg_standings`.
- **Boss Battles (封印戦)** (`internal/boss`): 4-player cooperative sealing battles, entry fatigue (+20% Tired), Dejon banishment (+30% Tired), `@ふういん` resealing, HeroCount increment, celebration banquets, news broadcast, and depot overflow reward delivery.
  - *Dependencies:* Battle Engine, Character, Party, Inventory, Core Progression, News Publisher.
  - *Persistence:* `character_boss_records` and `boss_challenge_history` tables (Rank 3 -> Rank 5 lock ordering).
- **Dungeon Exploration** (`internal/dungeon`): Multi-floor grid dungeon navigation, branching tile events, party exploration, trap damage, Treasure Hunter bonus chests, map scouting (`@ちず`) with stacking vision expansion, and reward finalization with depot overflow routing.
  - *Dependencies:* Battle Engine, Character, Inventory, Core Progression, Valkey Master.
  - *Persistence:* `character_dungeon_records` and `dungeon_expedition_history` in MariaDB; volatile in-progress run buffers in Valkey Master (`party2:dungeon:{char:<char_id>}:*`, Candidate D).
- **Battle Replays & Match History** (`internal/replay`): Recording and playback of step-by-step turn logs across all combat modes, character match history queries, and retention pruning.
  - *Dependencies:* Battle Engine, Character.
  - *Persistence:* `battle_replays` table.
- **Endurance Challenge** (`internal/challenge`): Consecutive survival wave combat, progressive wave scaling, legacy HP carryover between rounds, party challenge runs, Hall of Fame records, and cashout reward finalization with depot overflow routing.
  - *Dependencies:* Battle Engine, Character, Inventory, Valkey Master.
  - *Persistence:* `character_challenge_records`, `challenge_sessions`, `challenge_hall_of_fame` tables; volatile active session buffers in Valkey Master (`party2:challenge:{char:<char_id>}:*`, Candidate D).
- **Custom Skill Gem Synthesis** (`internal/custom_skill`): Custom skill naming, activation phrase validation, gem-box selection, CMP/slot checks, and atomic gem exchange.
  - *Dependencies:* Character, Inventory, Gem catalog, TransactionProvider.
  - *Persistence:* `character_custom_skills` table.
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): Helper quest generation, delivery validation, alchemy material rewards, guild points, emergency rescue recovery, and HTTP API endpoints.
  - *Dependencies:* Character, Inventory, Item, Guild repository.
  - *Persistence:* `helper_quests` and `rescue_records` tables.
- **Town Park & Public Bulletin Board** (`internal/park`): Public bulletin board posts, character authorship, text sanitization, rate limiting, and NPC fortune divination.
  - *Dependencies:* Character repository.
  - *Persistence:* `park_posts` table.
- **News & Player Notifications** (`internal/notification`): System news announcements, personalized notification inbox, read state tracking, and retention pruning.
  - *Dependencies:* Player repository.
  - *Persistence:* `news_articles` and `player_notifications` tables.
- **Player Leaderboards & Character Rankings** (`internal/ranking`): 12 competitive leaderboards with deterministic tie-breaking, pagination, Valkey caching, singleflight stampede protection, and ISP-decomposed repository sub-interfaces.
  - *Dependencies:* Character, Player, Valkey, Scheduling.
  - *Persistence:* `ranking_snapshots` table and Valkey cache keys (`party2:ranking:snapshot:*`).
- **Distributed Rate Limiting & Cooldown Tracking** (`internal/ratelimit`): Atomic distributed rate limiting, endpoint spam defense, bulletin board cooldowns, and home visitor throttling.
  - *Dependencies:* Valkey with in-memory fallback.
  - *Persistence:* Atomic counter keys in Valkey (`party2:ratelimit:*`).
- **Event Plaza & Victory Banquets** (`internal/eventplaza`): Town gathering state, real-time plaza presence tracking (5-minute window via Valkey Sorted Set + MariaDB), 26-item authentic merchant catalog at 3× markup across Tiers 1–3, and world boss victory celebration banquets directly linked to presence.
  - *Dependencies:* Character, Item, Inventory, Depot, Helper Quest filter, Item Collection, Valkey.
  - *Persistence:* `celebration_banquets`, `banquet_toasts`, `eventplaza_presences` tables and Valkey Sorted Set `party2:eventplaza:presence`.
- **Secret Underground Shop** (`internal/secretshop`): Secret underground shop access validation (`job_lv >= 7`), 8-item rare catalog with 3× pricing multiplier, inventory-to-depot overflow routing, and puff-puff dialogue.
  - *Dependencies:* Character, Item, Inventory, Depot.
  - *Persistence:* Direct inventory, depot, and character balance updates.
- **Adventurer's Tavern** (`internal/tavern`): 14-item culinary menu, restorative HP/MP meals, fullness tracking, raffle tickets, automatic fullness reset upon adventure completion (`is_eat = 0`), and standing order food delivery across solo and party adventures.
  - *Dependencies:* Character, Lottery repository.
  - *Persistence:* `tavern_deliveries` and `tavern_character_status` tables.
- **Town Black Market** (`internal/blackmarket`): Rare item sacrifice recycling system awarding Rare Points, prize trade exchange for 24 equipment/item rewards delivered to Depot, and NPC interactions.
  - *Dependencies:* Character, Item, Inventory, Depot.
  - *Persistence:* `blackmarket_character_points` table.
- **Flea Market** (`internal/fleamarket`): Fixed-price item marketplace (max 5 active listings per character, 1–999,999G), SQL CAS status predicate guard, and atomic purchasing transactions.
  - *Dependencies:* Character, Item, Inventory.
  - *Persistence:* `fleamarket_listings` table with SQL CAS status guard and cross-character row lock hierarchy (`characters` ID asc -> `inventory_items` -> `fleamarket_listings`).
- **Gem Store & Jewel Synthesis** (`internal/gemstore`): Dedicated Gem Box storage with job-level dynamic capacity, sorting, 55+ gem synthesis formulas, player transfers, and dual-source (Inventory and Depot) unidentified orb appraisals.
  - *Dependencies:* Character, Item, Inventory, Depot, Gem Box repository.
  - *Persistence:* `character_gem_boxes` and `gem_box_items` tables with lock hierarchy (`characters` -> `inventory_items` -> `character_depots` -> `character_gem_boxes`).
- **Endgame God Wishes & Limit Breaks** (`internal/god`): Celestial audiences in Heaven and Underworld, permanent attribute enhancements, currency awards, Lv99+ limit breaks (raising cap to 150), and storage capacity limit breaks.
  - *Dependencies:* Character, Core Progression, Depot, Inventory.
  - *Persistence:* `characters` limit break columns and `character_depots` capacity.
- **Monster Ranch & Pet Companions** (`internal/monster`): Monster Grandpa stabling (base 50 up to 300 via `OverMonster`), home pet estate linking (up to 8 pets), nickname customization, P2P gifting with two-party locking, and wild release.
  - *Dependencies:* Character, TransactionProvider.
  - *Persistence:* `character_monsters` table.
- **Photo Contest & Gallery** (`internal/contest`): Character screenshot storage, contest submissions, community voting, automated round conclusion with prize distribution, and Hall of Fame archiving.
  - *Dependencies:* Character, News publisher, Guild service.
  - *Persistence:* `character_photos`, `contest_rounds`, `contest_entries`, `contest_votes`, and `contest_legends` tables.
- **Multiplayer Party & Co-op Quests** (`internal/party`): Party formation (1–4 members), recruitment lobbies, speed configs (3/18/25), readiness synchronization, Rank 0 distributed adventure lock (`party2:party:lock:adventure:*`) with token-safe Lua release, post-battle settlement delegation via `ApplyPostBattleResult`, Floor 11 treasure drops with depot overflow routing, and HP-1 survival guarantee.
  - *Dependencies:* Character, Battle Engine, Inventory, Item, Progression, Depot, Catalogs, News publisher, Valkey.
  - *Persistence:* Ephemeral lobbies and locks in Valkey Master (`party2:party:*`); durable quest logs in MariaDB `party_adventure_logs`.
- **Altar of Rebirth** (`internal/altar`): 6-orb offering ritual, Ramia awakening (30min record), and 4 otherworld travel item wishes with Depot overflow fallback.
  - *Dependencies:* Character, Inventory, Depot, Item Catalog, Economy.
  - *Persistence:* `altar_records` and character updates via `economy.TransactionRunner`.
- **Wishing Well** (`internal/wishingwell`): Wishing Well (@女神) SP sacrifice exchange for permanent stat growth (MHP/MMP +2/SP, ATK/DEF/AGI +1/SP).
  - *Dependencies:* Character, Economy.
  - *Persistence:* `characters` table with Tier 2 row locking.
- **Player Store & Town Boutiques** (`internal/store`): Player store construction in towns 1–4 (50,000G, 90-day duration), gold and barter listings from depot (up to 20 via `OverStore`), atomic purchasing transactions, and interior customization (26 wallpapers, 15 furniture styles).
  - *Dependencies:* Character, Depot, Item, Guild, Timer service, TxProvider.
  - *Persistence:* `character_stores`, `store_sales`, and `store_interiors` tables (Rank 0 `store_sales` -> Rank 2 `characters` asc -> Rank 5 `character_depots` asc).
- **Maintenance Mode** (`internal/maintenance`): Maintenance status management, administrative toggle, and HTTP middleware interception.
  - *Dependencies:* Maintenance repository, Valkey.
  - *Persistence:* Valkey Master / in-memory caching (`party2:maintenance:status`) backed by `system_maintenance` table in MariaDB.

---

## Transaction Orchestration & Persistence Tiers

### Cross-Module Application Orchestrator Pattern

Cross-module workflows spanning multiple repositories use the **Application Orchestrator Pattern**:
- **Ambient Context Boundary**: Transactions are started at the application service level via `database.RunInTx(ctx, db, fn)`.
- **Automatic Participation**: Repositories resolve SQL executors via `database.ExecutorFromContext(ctx, r.db)` without explicit transaction passing across domain boundaries.
- **Deadlock-Free Lock Ordering**: Enforced mechanically by Go AST static analysis (`make lock-lint`), requiring all transactions to acquire row locks strictly in ascending rank order (Rank 0 through 8). Multi-character operations order locks by ascending ID (`id.Sort2`).
- **Cross-Domain Application Primitives (`internal/economy`)**: Single-character currency/inventory operations route through `economy.TransactionRunner`, while multi-aggregate and P2P operations use `TransactionProvider` (`RunInTx`). Raw `database.RunInTx` in feature packages is prohibited by AST static analysis.

### Storage Authority Tiers

Storage authority is divided into three distinct tiers per [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md) and [`valkey-keyspace.md`](valkey-keyspace.md):

1. **MariaDB Master (Canonical Relational Persistence)**:
   - *Scope:* Player Accounts, Characters, Inventories, Equipment, Currencies, Jobs, Depots, Bank Accounts, Guilds, Persistent Feature State, and Audit Records.
   - *Guarantees:* ACID transactions, foreign keys, deterministic row-lock hierarchy (Rank 0→8), zero data loss tolerance.
2. **Valkey Master (Primary Authoritative Ephemeral Store)**:
   - *Scope:* Player Sessions (`party2:session:*`), Session Indices (`party2:player:sessions:*`), System Maintenance State (`party2:maintenance:status`), Party Wait Lobbies (`party2:party:lobby:*`), Ephemeral Multiplayer Card/PvP/GvG Rooms (`party2:<domain>:room:*`), Scheduled Action Queues (`party2:scheduled:*`), Distributed Locks (`party2:<domain>:lock:*`), Rate Limiting Counters (`party2:ratelimit:*`), and In-Progress Run Buffers (`party2:<domain>:{char:<id>}:*`).
   - *Guarantees:* In-memory/AOF persistence governed by native TTL or application lifecycle hooks without backing SQL tables. Ephemeral failure causes session re-login or lobby recreation with zero impact on economic assets.
3. **Valkey Cache (Read Projections)**:
   - *Scope:* Competitive Leaderboards (`party2:ranking:snapshot:*`), Profile Projections.
   - *Guarantees:* Read-optimized projection reconstructible from MariaDB on cache miss.

#### Persistence Decision Tree

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

---

## Component Configuration Lifecycle and Composition Root

To preserve parallel testability and eliminate global state mutations:

1. **Config Struct First**: Every configurable package defines a pure `Config` struct with a `DefaultConfig()` constructor.
2. **Explicit Injection**: Constructors accept typed `Config` structs or functional options; they do not read `os.Getenv` directly.
3. **Isolated Environment Loaders**: Environment parsing functions (`ConfigFromEnvironment()`) validate and clamp variables independently.
4. **Composition Root Organization (`cmd/party2/`)**:
   - `config.go`: Top-level `Config` struct and environment parsing.
   - `main.go`: Process entrypoint, server lifecycle, signal trapping, and graceful shutdown (≤ 150 lines).
   - `services_core.go`: Player, Character, Inventory, Item/Job catalogs, and transaction orchestration.
   - `services_econ.go`: Shop, Bank, Depot, Blacksmith, Alchemy, Plantation, Auction, Flea Market, and Gem Store.
   - `services_cmbt.go`: Battle engine, Boss, PvP, GvG, Dungeon, Challenge, Party, Replay, and Custom Skill.
   - `services_soc.go`: Guild, Ranking, Park, Home, Notification, Scheduling, and Worker.
   - `services_misc.go`: Town facilities and side systems (Casino, Contest, Medal, Collection, Chapel, Altar, Wishing Well, Activity, etc.).
   - `wire.go`: Cross-domain event hooks (`VictoryHook`, `SynthesisHook`, `GamePlayedHook`, `PostAdventureHook`) and HTTP handler composition.

---

## Component Review Criteria

For every new component or refactoring, verify:

1. What does this component own?
2. What does it deliberately not own?
3. What are its public inputs and outputs?
4. Which dependencies are necessary?
5. Could another implementation replace it without changing consumers?
6. Would adding another component of the same kind require changes here?
7. Is the component boundary justified by an actual responsibility rather than speculative abstraction?

---

## Related Documents

- [`overview.md`](overview.md) — overall architecture.
- [`feature-modules.md`](feature-modules.md) — feature boundaries and lock hierarchy.
- [`interfaces.md`](interfaces.md) — public component contracts.
- [`transient-run-state.md`](transient-run-state.md) — ephemeral run state specification.
- [`valkey-keyspace.md`](valkey-keyspace.md) — centralized Valkey keyspace SSOT.
- [`../design/game-overview.md`](../design/game-overview.md) — domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory architectural rules.
