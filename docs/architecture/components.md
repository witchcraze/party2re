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
Player persistence stores a salted, iterated password hash (`bcrypt`, cost 12), update timestamps, last login IP address, and soft-ban timestamps (`banned_at`). Session state is ephemeral, mastered directly in **Valkey Master** (`party2:session:<token>` with 7-day native TTL and `party2:player:sessions:<player_id>` Sorted Set with lazy pruning). Also provides cryptographically secure Personal Access Tokens (`p2_sk_...`) hashed with SHA-256 in MariaDB (`player_api_tokens`) for API and tooling access. Dual authentication transparently handles sessions and PATs, rejecting banned accounts with 403 Forbidden. Privileged administrative management (`/admin/players`, `/admin/players/{id}/ban`) enables player listing (sorted by IP, name, or last activity) and immediate account soft-banning with session/token revocation.

### Character

**Responsibility:** The player's in-game character identity and fundamental attributes.
Linked to `Player` via `player_id`. Owns character customization (naming hall, gender changes, profile bio/avatar), wallet operations (`Money`, strictly capped at 999,999G), crystal currency operations (`AddCrystal`, `DeductCrystal`, strictly capped at 999,999), vitality clamping (`Stats.ClampVitality`, `RecoverVitality`), 1 HP fallen combat survival (`ApplyCombatSurvival`), and fatigue management (`AddTired` capped at 100%, `ReduceTired` allowing authentic celestial buffer). Direct field mutations on currency (`Money`, `SmallMedals`, `Crystal`), progression, and resource fields are mechanically prohibited outside Core and database mappers via Go AST static analysis.

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

Each feature owns its specific domain logic and state. Cross-feature imports and direct `internal/database` imports from feature packages are mechanically prohibited by AST static analysis.

| Module | Package Path | Primary Responsibility | Dependencies | Storage & Lock Hierarchy |
| :--- | :--- | :--- | :--- | :--- |
| **Activity** | `internal/activity` | Delayed training actions and experience awards | Character, Progression, Scheduling | MariaDB `activities` |
| **Adventure** | `internal/adventure` | 10-floor dungeon crawl loop, treasure room, combat chronicles | Battle, Catalogs, Character, Inventory | MariaDB `adventures` (Rank 2→3→5) |
| **Alchemy** | `internal/alchemy` | 112-recipe crafting consuming depot materials, recipe compendium | Catalogs, Character, Depot, Economy | MariaDB `character_alchemy` (Rank 2→5→8) |
| **Altar** | `internal/altar` | 6-orb ritual, Ramia awakening, otherworld travel wishes | Character, Inventory, Depot, Economy | MariaDB `altar_records` (Rank 2→3→5) |
| **Auction** | `internal/auction` | Live P2P trade hall (`@おくる`/`@しらべる`) | Character, Equipment, Inventory, Depot | MariaDB `characters`, `inventory_items`, `depot_items` (Rank 2→3→5) |
| **Bank** | `internal/bank` | Gold savings deposits/withdrawals with 999,999G wallet clamp | Character | MariaDB `characters.deposit` (Rank 2) |
| **Black Market** | `internal/blackmarket` | Rare item sacrifice for Rare Points, depot barter rewards | Character, Item, Inventory, Depot | MariaDB `blackmarket_character_points` (Rank 2→3→5) |
| **Blacksmith** | `internal/blacksmith` | 12 crystal weapon seals, equipment naming, 3-slot storage | Character, Inventory, Equipment | MariaDB `blacksmith_deposits` (Rank 2→3→8) |
| **Boss** | `internal/boss` | 4-player sealing battles, Dejon banishment, HeroCount | Battle, Character, Party, Inventory, News | MariaDB `character_boss_records` (Rank 3→5) |
| **Casino** | `internal/casino` | 2..8 player room lobby, Indian Poker, High-Low, Doppelganger, Slots | Character, Depot | Valkey Candidate C (`party2:casino:*`); MariaDB `casino_accounts` (Rank 2→5→8) |
| **Challenge** | `internal/challenge` | 4-tier survival wave combat, HP carryover, Hall of Fame | Battle, Character, Inventory, Valkey | Valkey Candidate D (`party2:challenge:*`); MariaDB `character_challenge_records` |
| **Collection** | `internal/collection` | Illustrated monster defeat and item discovery encyclopedia, 100% completion milestones | Character, Notification | MariaDB `character_monster_book`, `character_item_collection`, `character_collection_completions` |
| **Contest** | `internal/contest` | Photo contest submissions, voting rounds, recurring settlement, Hall of Fame | Character, News, Guild, Scheduling | MariaDB `character_photos`, `contest_rounds`, `contest_entries`, `contest_votes`, `contest_legends` |
| **Costume** | `internal/costume` | Daily rented appearance state (`ActiveCostume`), midnight JST expiration, rest/job-change reset | None | Valkey `party2:daily:costume:*` |
| **Custom Skill** | `internal/customskill` | 3-gem recipe synthesis, activation phrase validation | Character, Inventory, Gem Catalog | MariaDB `character_custom_skills` |
| **Depot** | `internal/depot` | Persistent storage (up to 500 slots), item consumption, delivery engine | Character, Inventory, Economy | MariaDB `character_depots`, `depot_items` (Rank 2→3→5) |
| **Dungeon** | `internal/dungeon` | Grid map exploration, party traps, map scouting (`@ちず`) | Battle, Character, Inventory, Valkey | Valkey Candidate D (`party2:dungeon:*`); MariaDB `character_dungeon_records` |
| **Event Plaza** | `internal/eventplaza` | Real-time presence tracking, 3× markup bazaar, victory banquets | Character, Item, Inventory, Depot, Valkey | Valkey `party2:eventplaza:presence`; MariaDB `celebration_banquets` |
| **Flea Market** | `internal/fleamarket` | Fixed-price player listings (up to 5/char, 120 server max) | Character, Item, Inventory | MariaDB `fleamarket_listings` with SQL CAS guard (Rank 2 asc→3→8) |
| **Gem Store** | `internal/gemstore` | Dedicated gem box, 55+ synthesis formulas, orb appraisal | Character, Item, Inventory, Depot | MariaDB `character_gem_boxes`, `gem_box_items` (Rank 2→3→5→8) |
| **God** | `internal/god` | 19 celestial wishes, Lv150 OverLevel, storage limit breaks | Character, Progression, Depot, Inventory | MariaDB `characters`, `character_depots` |
| **Guild** | `internal/guild` | Founding, dynamic GP, custom roles, hex colors, 20d auto-disband | Character | MariaDB `guilds`, `guild_members` |
| **GvG** | `internal/gvg` | 2..8 player guild battle rooms, GP prize pools, 7-tier medals | Battle, Guild, Character, Valkey | Valkey Candidate C (`party2:gvg:*`); MariaDB `gvg_standings` |
| **Helper Quest** | `internal/helperquest` | Delivery quests, alchemy rewards, guild contribution | Character, Inventory, Depot, Item, Guild | MariaDB `helper_quests` |
| **Home** | `internal/home` | House profiles, letters, companion phrases, sleep recovery | Character, Timer, Economy, Inventory, Depot | Valkey `party2:timer:sleep:*`; MariaDB `character_homes`, `home_letters` |
| **Lottery** | `internal/lottery` | 20-cap Takarakuji lottery, rollover jackpot; Tavern Fukubiki raffle | Character, Inventory, Depot, Item | MariaDB `character_lottery`, `takarakuji_rounds` (Rank 0→2→5) |
| **Maintenance** | `internal/maintenance` | Maintenance state, admin toggle, HTTP 503 middleware | Valkey | Valkey `party2:maintenance:status`; MariaDB `system_maintenance` |
| **Medal** | `internal/medal` | Small Medal depot shop, lifetime milestone achievement tracking | Character, Inventory, Action Hooks | MariaDB `character_medals`, `character_achievements` |
| **Monster Ranch** | `internal/monster` | Grandpa stabling (50–300 cap), Home pet link (8), P2P gift | Character, TransactionProvider | MariaDB `character_monsters` |
| **Notification** | `internal/notification` | System news announcements, player notification inbox | Player | MariaDB `news_articles`, `player_notifications` |
| **Park** | `internal/park` | Public bulletin board posts, NPC divination, rate limiting | Character | MariaDB `park_posts` |
| **Party** | `internal/party` | 1–4 player lobbies, speed configs, Rank 0 distributed lock, HP-1 | Character, Battle, Inventory, Depot, Valkey | Valkey `party2:party:*`; MariaDB `party_adventure_logs` (Rank 0 lock) |
| **Plantation** | `internal/plantation` | 6 seeds, 14 fertilizer reagents, midnight JST maturation, harvest | Catalogs, Character, Inventory, Depot | MariaDB `plantation_plots` (Rank 2→3→5→8) |
| **PvP** | `internal/pvp` | 2..8 player Colosseum rooms, Bet & Split prize pool, 9 colors | Battle, Character, TransactionProvider | Valkey Candidate C (`party2:pvp:*`); MariaDB `pvp_wins` (Rank 2 asc) |
| **Ranking** | `internal/ranking` | 14 leaderboards, Hall of Fame (legend.cgi), weekly job change ranking with Sunday rotation, Valkey caching, singleflight stampede guard | Character, Player, Valkey, Scheduling | Valkey `party2:ranking:snapshot:*`; MariaDB `ranking_snapshots`, `legend_records`, `weekly_job_changes` |
| **Rate Limit** | `internal/ratelimit` | Distributed atomic rate limiting, spam defense, throttling | Valkey | Valkey `party2:ratelimit:*` |
| **Replay** | `internal/replay` | Combat turn log recording, step-by-step playback, retention | Battle, Character | MariaDB `battle_replays` |
| **Rescue** | `internal/rescue` | Emergency player unstuck recovery, action clearing, penalty cooldown | Character, Scheduling | MariaDB `rescue_records` |
| **Secret Shop** | `internal/secretshop` | JobLv 7 access gate, 8 rare items at 3× price, depot delivery | Character, Item, Inventory, Depot | MariaDB `characters`, `inventory_items`, `depot_items` (Rank 2→3→5) |
| **Shop** | `internal/shop` | Town equipment/item shops, 50% markdown, depot auto-delivery | Catalogs, Character, Inventory, Depot | MariaDB `characters`, `inventory_items`, `depot_items` (Rank 2→3→5) |
| **Store** | `internal/store` | Player shop construction (50kG/90d), barter listings, interiors, Oracle Shop costume items purchasing (@kau), home wallpapers (@kabegami) | Character, Inventory, Depot, Item, Collection, Helperquest, Guild, Timer, Costume | MariaDB `character_stores`, `store_sales`, `store_interiors`, `character_homes`, `character_item_collection` (Rank 0→2→3→5) |
| **Tavern** | `internal/tavern` | 14-item culinary menu, restorative meals, food delivery standing orders | Character, Lottery | MariaDB `tavern_deliveries`, `tavern_character_status` |
| **Wishing Well** | `internal/wishingwell` | SP sacrifice for permanent stat growth (HP/MP +2/SP, Stats +1/SP) | Character, Economy | MariaDB `characters` (Rank 2) |

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
   - `main.go`: Process entrypoint, server lifecycle, strict startup connectivity validation (timeout-bounded MariaDB and Valkey pings), clean teardown on failure, signal trapping, and graceful shutdown (≤ 150 lines).
   - `services_core.go`: Player, Character, Inventory, Item/Job catalogs, and transaction orchestration.
   - `services_econ.go`: Shop, Bank, Depot, Blacksmith, Alchemy, Plantation, Auction, Flea Market, and Gem Store.
   - `services_cmbt.go`: Battle engine, Boss, PvP, GvG, Dungeon, Challenge, Party, Replay, and Custom Skill.
   - `services_soc.go`: Guild, Ranking, Park, Home, Notification, Scheduling, and Worker.
   - `services_misc.go`: Town facilities and side systems (Casino, Contest, Medal, Collection, Chapel, Altar, Wishing Well, Activity, etc.).
   - `wire.go`: Cross-domain event hooks (`VictoryHook`, `SynthesisHook`, `GamePlayedHook`, `PostAdventureHook`) and HTTP handler composition.
5. **Fail-Fast Startup Connectivity & Clean Teardown**:
   - Both MariaDB and Valkey are required runtime infrastructure dependencies in production.
   - During bootstrap, `runWithConfig` executes blocking, timeout-bounded connectivity checks (`database.PingContext`, `valkey.Ping`) before binding network listeners or launching background workers.
   - If either store is unreachable or unhealthy, the process aborts startup immediately with a non-zero exit status.
   - In-flight or partially initialized resources (database connection pools, client sockets, listeners) are deterministically released via `defer` handlers to ensure zero orphan connections or goroutines.

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
