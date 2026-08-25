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
supplied password. Session state is a separate record with explicit expiry and
revocation. It is currently stored in MariaDB because that is the configured
persistence system; this is an implementation choice, not a claim that
sessions are durable game data. The repository contract leaves room to move
session storage to Valkey when a concrete transient-state requirement
justifies introducing it.

### Character

**Responsibility:** the player's in-game character and its fundamental state.

Linked to a owning `Player` via `player_id` (enforced via foreign key constraint).
Owns invariants about its own state but should not become a God object containing every game system.
Access to character operations is authorized against the authenticated session's player identity.

### Progression

**Responsibility:** level, experience, stats, and other fundamental character progression.

Progression consumes job growth values through the public Job definition
contract. It does not contain a built-in catalog of job-specific data.

### Job

**Responsibility:** job definitions, job availability rules, and character job
history.

Job definitions are content data, not executable progression logic. The initial
catalog is stored outside Go source as validated data under
`internal/core/job/data/`. The Job component loads and validates that data at
startup or construction time, then exposes definitions through a small lookup
contract. Consumers do not access the data file or catalog map directly.

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

Does not contain unrelated item behavior.

### Equipment

**Responsibility:** which eligible item instances are equipped and where.

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
  - Participants
  - State
  - Actions
  - Effects
  - Result
```

Battle must not know whether it was initiated by a quest, guild, arena, or another feature.

### Adventure / Quest

**Responsibility:** define and execute adventure-oriented game flows, including destinations, encounters, durations, requirements, and rewards.

Adventure may use Battle but should not own Battle's implementation.

## Feature modules

Each feature owns its feature-specific rules and state. A feature may consume public contracts from Core or shared components, but must not access another feature's private implementation or database schema.

### Implemented Feature Modules

- **Activity** (`internal/activity`):
  - **Responsibility:** Delayed training actions and experience awards.
  - **Dependencies:** Character repository, Core Progression, Scheduling Service.
  - **Persistence:** `activities` table with atomic `ClaimAndApply` concurrency locking.
- **Adventure** (`internal/adventure`):
  - **Responsibility:** Multi-stage exploration (28 stages, 286 monsters), stage eligibility checks, Battle invocation, and drop rewards.
  - **Dependencies:** Stage/Monster catalogs, Battle Resolver, Character & Inventory repositories, Scheduling Service.
  - **Persistence:** `adventures` table with atomic battle outcome persistence.
- **Shop** (`internal/shop`):
  - **Responsibility:** Item purchases (gold deduction + inventory addition) and resale (inventory removal + 50% gold refund).
  - **Dependencies:** Item Catalog, Character (wallet), Inventory.
  - **Persistence:** Atomic single-transaction `ShopRepository`.
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
  - **Responsibility:** Recording and faithful playback of step-by-step turn logs (actions, damage/healing numbers, critical hits, logs, remaining HP snapshots) across all combat modes (PvP, GvG, Boss, Dungeon, Adventure, Challenge), character match history queries, and replay retention pruning.
  - **Dependencies:** Core Battle Engine.
  - **Persistence:** `battle_replays` table in `internal/database/replay_repository.go`.

### Future Feature Modules

- Rankings (Level, Job, Weekly, Contest)


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
