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
Player persistence stores a salted, iterated password hash (`bcrypt`, cost 12), update timestamps, last login IP address, and soft-ban timestamps (`banned_at`). Session state is ephemeral, mastered directly in **Valkey Master** (`party2:session:<token>` with 7-day native TTL and `party2:player:sessions:<player_id>` Sorted Set with lazy pruning). Also provides cryptographically secure Personal Access Tokens (`p2_sk_...`) hashed with SHA-256 in MariaDB (`player_api_tokens`) for API and tooling access. Dual authentication transparently handles sessions and PATs, rejecting banned accounts with 403 Forbidden. Privileged administrative management (`/admin/players`, `/admin/players/{id}/ban`, `/admin/characters/{id}/unstuck`) enables player listing (sorted by IP, name, or last activity), immediate account soft-banning with session/token revocation and guild membership cleanup, and character unstuck.

### Character

**Responsibility:** The player's in-game character identity and fundamental attributes.
Linked to `Player` via `player_id`. Owns character creation invariants requiring valid starter jobs (1..12) and gender (`'m'` or `'f'`), character customization (naming hall, gender changes, profile bio/avatar, aura effect 0..8), wallet operations (`Money`, strictly capped at 999,999G), crystal currency operations (`AddCrystal`, `DeductCrystal`, strictly capped at 999,999), vitality clamping (`Stats.ClampVitality`, `RecoverVitality`), 1 HP fallen combat survival (`ApplyCombatSurvival`), and fatigue management (`AddTired` capped at 100%, `ReduceTired` allowing authentic celestial buffer). Direct field mutations on currency (`Money`, `SmallMedals`, `Crystal`), progression, and resource fields are mechanically prohibited outside Core and database mappers via Go AST static analysis.

### Progression

**Responsibility:** Level, cumulative experience, stats, and character progression formulas.
Consumes job growth definitions. Provides canonical domain helpers (`ApplyExperience`, `ApplyExperienceWithJob`, `MaxLevelForCharacter`) to calculate cumulative thresholds (`level²×10`), award Skill Points (SP) per level, trigger skill learning, and apply celestial OverLevel limit breaks up to Lv 150. Direct field mutation of `Experience`, `Level`, and `SP` is mechanically prohibited by AST linters.

### Job

**Responsibility:** Job catalog definitions, availability rules, and character job history.
Job definitions are content data loaded and validated at startup from `internal/core/job/data/`. Exposes definitions through a lookup contract. `CharacterJob` encapsulates transitions (`ChangeTo`), mastery (`RecordMastery`, `Master`), prerequisite tree checks (`_is_need_job`), milestone counters (`MonsterKills`, `PvPWins`, `HeroCount`, `MaoCount`, `CasinoWins`, `JobLevel`), CMP tier rating (`c_mp_tier`), and retained mastery SP. Coordinates level-20 job change transactions, atomic equipped armor consumption for FireFighter (`job-84`), item possession and consumption validation, temporary mastered-job memory snapshots (`よびおこす`), and character job mastery catalog / completion percentage calculation (`job_master.cgi`).

### Item

**Responsibility:** Item definitions, 5-category catalog resolution, and item instances.
Separates definitions from instances. Enforces domain stackability invariants (`IsStackable() == (Slot == SlotNone)`), ensuring equipment items and enhanced items cannot stack (`Quantity` strictly 1) across storage systems (`internal/depot`, `internal/core/inventory`).

### Inventory

**Responsibility:** Ownership and storage of active character item instances.
Encapsulates item storage mutations via `Add`, `Consume`, `ConsumeItem`, `ConsumeOne`, `ConsumeOneItem`, and `Update` with quantity validation. Direct mutations of `Inventory.Items` outside Core and database mapping are prohibited via AST static analysis.

### Equipment

**Responsibility:** Slot assignments and un-equipping for eligible item instances.
Encapsulates slot assignments via `Equip` and `Unequip` across 4 equipment slots (weapon, armor, shield, accessory) with suitability and ownership checks.

### Currency

**Responsibility:** Generic ownership and movement invariants of game currencies.
Provides boundary interfaces for gold, small medals, casino coins, and crystal currencies without hard-coding ad-hoc rules into unrelated modules.

### PlayerContext

**Responsibility:** Evaluates and projects current action-entry eligibility for active characters.
Maintains the static Action Catalog (`internal/playercontext/catalog.go`) defining top-level player actions, required parameters, and six precondition gates. `playercontext.Service.Query(ctx, charID)` reads Character, unfinished ScheduledActions and existing Sleep/Asleep timers through consumer-defined interfaces, returning the shared domain snapshot and available ActionIDs. The pure pipeline runs Dead → Fatigue → Sleep → Cooldown → Currency → Location with short-circuit exclusion. Rescue is exempt, and wake requires elapsed sleep plus pending recovery. Currency eligibility checks positive wallet gold, with exact request-specific prerequisites validated by execution services. No result cache or HTTP knowledge is included. Automatically verifies static schema drift against `docs/api/openapi.json` (`internal/playercontext/catalog_lint_test.go`). SSOT: [`action-preconditions.md`](../design/action-preconditions.md).

The HTTP adapter (`internal/api/http/context_handler.go`) verifies ownership, maps the query snapshot to the shared `PlayerContextResponse` transport DTO, and enriches profile avatars and job names. Its versioned GET route returns character, scene, all scheduled/sleep recovery observations and eligible action metadata. Presentation stays in HTTP; current scene/asset limitations are recorded in [STATUS](../../STATUS.md).

The common command boundary (`internal/api/http/action_dispatcher.go`) registers versioned `POST /api/v1/characters/{id}/actions`, with strict typed parameter decoding, authenticated actor selection, current query eligibility and the shared Sleep/CanWake guard (explicit Wake/Rescue exceptions). Its package-local registration seam connects catalog IDs to direct service calls and known rejection mappings during Handler construction. HTTP owns result/error formatting and post-command composition of the shared observation DTO, rechecking query/profile ownership. Existing services retain game rules and persistence. SSOT: [Client/Agent command contract](client-agent-api.md#3-command-pillar-post-apiv1charactersidactions); current coverage and gaps are summarized in [STATUS](../../STATUS.md).

### Game Time / Scheduling

**Responsibility:** Represent and execute delayed actions without owning feature rules.
`ScheduledAction` (`internal/core/scheduling`) records work units with an explicit state machine (`Pending → Processing → Completed | Failed`). Queue storage is managed in Valkey (`internal/scheduling`):
- `party2:scheduled:pending` (Sorted Set scored by `ExecuteAt` timestamp)
- `party2:scheduled:action:{id}` (action payload string)
- `party2:scheduled:lock:{id}` (distributed lock preventing duplicate execution)
- `party2:scheduled:actor:{actor_id}` (Set of indexed action IDs, not an exclusive actor lock)
`Schedule` requires actions with an `ActorID` to successfully index into this Set before enqueueing into the pending queue; failure halts registration and cleans up partial writes, guaranteeing that queued work is discoverable.
`FindPendingByActorID` reads the actor Set with SMEMBERS and bulk-loads payloads with MGET, returning Pending/Processing records including overdue work. Terminal and missing records are omitted; invalid records and storage errors propagate. This read contract supports PlayerContext without an additional SQL query or result cache. Lifecycle rules: [scheduled work](valkey-keyspace.md#scheduled-work-lifecycle).
The background `Worker` validates payloads, acquires locks for Pending work, and dispatches to registered `ActionHandler` implementations. Processing work is not replayed after lock expiry because its feature outcome may be uncertain. Queued Completed/Failed records receive metadata-only finalization without handler dispatch. Terminal persistence precedes destructive cleanup; required write failures propagate. The partial-step and retention contract is documented in [Valkey keyspace](valkey-keyspace.md#scheduled-terminal-persistence-and-recovery).

### Domain Events

**Responsibility:** Publish meaningful domain facts (`BattleFinished`, `CharacterLeveledUp`, `ItemObtained`, etc.) to decouple optional consumers via a two-phase in-memory dispatcher (`internal/core/event`). Async publication has no durable delivery guarantee and no ambient outer-commit hook; see [runtime primitives](cross-domain-primitives.md).

---

## Shared Components

### Battle

**Responsibility:** Deterministic combat resolution and post-battle state application.
Battle engine operates independently of callers (adventures, arena, GvG, bosses, dungeons, challenges):
- **Turn Resolver (`internal/core/battle`)**: Deterministic 1v1 and party combat (capacity supplied by the feature, including six-member king encounters) as well as multi-team combat (3+ factions up to 8 participants) with agility-ordered actions, MP/CMP skill costs, elemental field states, anti-field cancellation, and pre-death survival triggers.
- **Battle Adapter (`internal/battle`)**: Standardized integration service bridging Character/Party entities to combat `Participant` models. Calculates authentic equipment stats across 71 weapons, 55 armors, and accessories, binds passive triggers, and provides atomic `ApplyPostBattleResult` enforcing deterministic row-lock hierarchy (Rank 2: Characters ascending -> Rank 3: Inventories/Equipment ascending -> Rank 5: Depots ascending), item consumption persistence, recipient-targeted drop distribution, milestone counter increments (`char.MonsterKills` on strong enemies, `char.MaoCount` on Stage EX unseal), progression error propagation, and depot overflow fallback.
- **AST Guard**: Prohibits direct construction of un-adapted participants in feature modules (`battle_adapter_lint_test.go`).

### Adventure / Quest

**Responsibility:** Define and execute adventure-oriented game flows, dungeon crawl loops, encounter progressions, and stage reward distributions.

Current solo and cooperative starts resolve an entire crawl in one request;
`CrawlSession` has no production persistence between player operations. This
omits the original command/action-interval/individual-exit boundary and is a
reconstruction defect tracked by
[#1323](https://github.com/witchcraze/party2re/issues/1323), not an approved
substitute for interactive combat. The required behavior and actual settlement
limitations are owned by [Adventure](../design/adventure.md#original-interaction-contract-and-reconstruction-target).
The existing battle adapter, SQL settlement and Party lobby are reusable pieces,
not proof of a resumable run or cross-store atomicity.
The selected [durable normal-run boundary](transient-run-state.md#normal-adventure-durable-command-boundary-1325)
assigns enrolled solo/cooperative runs to Adventure and waiting admission to
Party; its decision status is recorded in that owner. Core battle supplies
feature-independent effects, the battle adapter supplies character binding and
asset application, and PlayerContext consumes owned public Adventure facts.

### Common Foundation Packages

- **ID Generation (`internal/id`)**: Centralized 16-byte (32 hex characters) cryptographically secure identifiers (`id.New()`, `id.Sort2(a, b)`).
- **Pagination (`internal/pagination`)**: Reusable offset and keyset cursor containers (`Page[T]`, `CursorPage[T]`) with token encoding/decoding (`EncodeCursor`, `DecodeCursor`).
- **Validation (`internal/validation`)**: Standardized text sanitization and input validators (Unicode NFC normalization, automated whitespace trimming, C0/C1 control character rejection, zero-width space/BOM rejection, bidirectional override rejection, Zalgo diacritical mark capping $\le 2$, character name validation, and starter job/gender checks).
- **Random Number Generation (`internal/core/random`)**: Centralized thread-safe pseudo-random generator backed by `math/rand/v2` and deterministic seeded generator for reproducible tests. Direct imports of `math/rand` in production packages are prohibited by AST linter.
- **Concurrency & Cancellation**: Cooperative context cancellation across services; raw `time.Sleep` is prohibited by AST linter.
- **Database Infrastructure (`internal/database`)**: Ambient transaction propagation (`RunInTx`, `ExecutorFromContext`) and connection pool lifecycle management.

---

## Feature Modules

Each feature owns its specific domain logic and state. Cross-feature imports and direct `internal/database` imports from feature packages are mechanically prohibited by AST static analysis.

A feature owns its rules, state, persistence and public application contract.
Choose only the layers needed by that feature; feature modules do not imply a
runtime plugin framework. Optional reactions can use domain events; mandatory
cross-feature work uses an explicit public contract or application orchestrator.
Delayed feature rewards require one atomic claim-and-apply persistence operation.
Costume owns rented appearance state independently of player-store transactions.

Event Plaza owns ordinary presence and the temporary sealing crowd through its
injected presence tracker. The composition root connects the Boss celebration
hook directly to that public crowd operation; it has no banquet/toast SQL
repository dependency. Historical banquet tables remain untouched. See
[Event Plaza design](../design/eventplaza.md#database-persistence) for storage boundaries.

Game rules and legacy action mappings are indexed in [design/README.md](../design/README.md).
Transport schemas belong to [OpenAPI](../api/openapi.json), SQL definitions to
[migrations](../../migrations/), and complex transaction navigation to [.arch](../../.arch/).
These sources replace a manually maintained package/table inventory here.

---

## Transaction Orchestration & Persistence Tiers

### Cross-Module Application Orchestrator Pattern

Cross-module workflows spanning multiple repositories use the **Application Orchestrator Pattern**:
- **Ambient Context Boundary**: Transactions are started at the application service level via `database.RunInTx(ctx, db, fn)`.
- **Automatic Participation**: Repositories resolve SQL executors via `database.ExecutorFromContext(ctx, r.db)` without explicit transaction passing across domain boundaries.
- **Deadlock-Free Lock Ordering**: Enforced mechanically by Go AST static analysis (`make lock-lint`), requiring all transactions to acquire row locks strictly in ascending rank order (Rank 0 through 8). Multi-character operations order locks by ascending ID (`id.Sort2`).
- **Cross-Domain Application Primitives (`internal/economy`)**: Single-character currency/inventory operations route through `economy.TransactionRunner`, while multi-aggregate and P2P operations use `TransactionProvider` (`RunInTx`). Raw `database.RunInTx` in feature packages is prohibited by AST static analysis.

### Storage authority

[Valkey keyspace and lifetimes](valkey-keyspace.md#storage-authority) owns the
MariaDB / authoritative ephemeral Valkey / reconstructible-cache distinction.
Bank savings belong to Character; maintenance is SQL-backed with a Valkey
projection. The [database rules](../../.agents/rules/05-database-and-caching.md)
own persistence selection and lock requirements. A queue or lease does not
establish cross-store rollback, crash recovery or exactly-once settlement.

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
- [`interfaces.md`](interfaces.md) — public component contracts.
- [`transient-run-state.md`](transient-run-state.md) — ephemeral run state specification.
- [`valkey-keyspace.md`](valkey-keyspace.md) — centralized Valkey keyspace SSOT.
- [`../design/game-overview.md`](../design/game-overview.md) — domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory architectural rules.
