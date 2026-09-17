# Status

Last updated: Issue #676 — [Bug] Economy/Depot: Enforce inventory capacity and reject over-capacity item grants and withdrawals

## Current phase

**Version 1.0 Reconstruction / Refactoring — In Progress**

Phase 0〜4（ゲーム理解・アーキテクチャ・ドメインモデル・骨格・垂直スライス）は完了しています。

現在は **Phase 5+（個別機能の段階的再構築）** にあり、Version 1.0に必要な主要ゲームシステムをクリーンルーム再構築として新規実装しています。
Version 1.0の完成条件は、既存プロジェクトの意味のあるゲーム機能を新規実装として再構築し、必要な画像を新規制作または承認済みプレースホルダーで準備することです。旧ソースコード・旧画像の移植は完成条件に含めません。

---

## Current Component State (What is True Now)

> For detailed implementation notes, see [`docs/architecture/components.md`](docs/architecture/components.md).
> For completed feature issue history, see [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md).

### Architecture & Repository Intelligence
- **Agent Operating Rules** (`AGENTS.md`, `.agents/rules/`): ✅ Prescriptive constraint rules modularized into 9 rule files; rationale in `docs/architecture/`.
- **Guidance Layer** (`.arch/`): ✅ Symbol-anchor module JSON + shared table reverse-index; verified by `arch_test.go`.
- **Transient State Architecture** (`docs/architecture/transient-run-state.md`, `docs/architecture/valkey-keyspace.md`, `internal/casino`, `internal/pvp`, `internal/gvg`): ✅ Ephemeral Turn & Session Lobby Architecture (Candidate C) standardized across multiplayer domains (Party, PvP, GvG, Casino) with authentic 1800s sliding TTL, active ZSet index scored by `UpdatedAt.Unix()`, lazy `ZREMRANGEBYSCORE` pruning, distributed room locking (`WithRoomLock`, `party2:<domain>:lock:room:<room_id>`) with token-safe Lua release, and Two-Phase Settlement into MariaDB Master. In-Progress Run Buffers (Candidate D: Dungeon/Challenge) and Shared Boss HP (Candidate E) verified and active.
- **AST Linter Suite** (`make check`, `make arch-lint`): ✅ Automated static analysis enforcing transaction boundaries (`TransactionRunner`), deterministic lock hierarchy (Rank 0→8), file size (≤500 lines), ISP interface size (≤10 methods), dead code, Valkey keyspace, Battle Adapter boundary enforcement, direct `math/rand` prohibition, raw `time.Sleep` prohibition, HTTP handler detached root context prohibition, cryptographic security policy, and modular monolith package boundaries.
- **Benchmark Framework** (`make bench`): ✅ Critical-path benchmarks + baseline regression detection.

### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): ✅ Registration, bcrypt (cost 12), Valkey session (7d TTL + ZSET lazy-purge), PAT (`p2_sk_...`), account deletion.
- **Character** (`internal/core/character`, `internal/character`): ✅ Stats, progression, SP, wallet (999,999G cap), crystal currency (999,999 cap), profile, customization; AST-enforced field encapsulation.
- **Timer & Daily Quotas** (`internal/core/timer`): ✅ Valkey/in-memory native TTL timers and JST-midnight daily quotas.
- **Progression** (`internal/core/progression`): ✅ Cumulative EXP (`level²×10`), OverLevel (Lv150), Happy Seed; AST-enforced progression helper encapsulation.
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): ✅ 72-job catalog, Lv20 job change, mastery, future memory snapshots, gem synthesis triggers.
- **Item / Inventory / Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`, `internal/economy`): ✅ 5-category catalog (269 items), UsageCategory validation, domain stackability invariants (`IsStackable`), slot management, standardized item consumption interface (`Consume`, `ConsumeItem`, `ConsumeOne`, `ConsumeOneItem`), capacity boundary enforcement (`IsFull()`) across grants and exchanges (`ErrInventoryFull`).
- **Battle & Adapter** (`internal/core/battle`, `internal/battle`): ✅ Deterministic turn resolver; multi-participant party combat (4vN) and multi-faction/team combat (3+ teams/guilds up to 8 players) with independent faction targeting and elimination loops; standardized Battle Adapter (`internal/battle`) bridging Character/Party to Participant, authentic equipment stat calculation across 71 weapons, 55 armors, and accessories, passive trigger binding, and atomic post-battle state application with deterministic row-lock hierarchy (Rank 2 -> Rank 3 -> Rank 5), depot overflow routing, and recipient-targeted drop distribution.
- **Random Number Generation** (`internal/core/random`): ✅ Concurrency-safe generator (`math/rand/v2`) and deterministic seeded generator for 100% reproducible tests.
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): ✅ Valkey-backed delayed queue + distributed lock worker; package coverage >90%.
- **Database & Transaction Orchestration** (`internal/database`, `internal/economy`, `internal/core/event`): ✅ `RunInTx`/`ExecutorFromContext` ambient propagation, deterministic lock hierarchy (Rank 0→8) AST-enforced, single-character `economy.TransactionRunner`, multi-aggregate/P2P `TransactionProvider`, safe `updateCharacter` persistence, and 2-phase event dispatcher.
- **Common Utilities** (`internal/pagination`, `internal/id`, `internal/validation`): ✅ Keyset cursor pagination (`CursorPage[T]`), cryptographic ID, validation helpers.

### Feature Modules
- **Activity** (`internal/activity`): ✅ Training via Valkey Worker push + manual Claim fallback.
- **Adventure** (`internal/adventure`): ✅ Authentic 10-floor dungeon crawl loop across 28 stages (286 monsters), Floor 11 Treasure Room resolution with post-battle settlement delegation via `ApplyPostBattleResult`, inventory persistence, depot overflow fallback, `LostDrops` tracking on full depot, surviving HP/MP persistence, immediate crawl execution, chronicles, and `VictoryHook`.
- **Medal & Achievements** (`internal/medal`): ✅ Small Medal exchange → Depot; milestone achievement observer tracking lifetime gameplay metrics.
- **Shop** (`internal/shop`): ✅ 3 town shops with job-level gates, 50% sellback, MasterCard discount, Depot auto-delivery.
- **Depot** (`internal/depot`): ✅ Dynamic capacity (up to 500 slots), tiered expansion, sort, item sell, gold/item direct-send, standardized item consumption (`Consume`, `ConsumeOne`, `PurgeSlot`), stackability preservation, standardized `RefreshCapacity` helper across commerce modules, centralized transactional reward item delivery engine (`DeliverRewardItem`, `DeliverRewardItems`) enforcing Rank 3 -> Rank 5 lock ordering and configurable overflow policies (`PolicyTreatOverflowAsLost` vs `PolicyAbortOnDepotFull`), unified dual-source item resolution & consumption helpers (`ResolveItem`, `ConsumeItem`, `ConsumeDualSource`, `SaveConsumptionResult`) across `plantation`, `blackmarket`, and `gemstore`, and inventory capacity enforcement on withdrawals (`ErrInventoryFull`).
- **Blacksmith & Weapon Seals** (`internal/blacksmith`, `internal/battle`, `internal/core/battle`): ✅ Authentic 12 weapon seals consuming crystals (`character.crystal`, 999,999 cap), equipment naming for weapons and armors, dedicated 3-slot weapon storage (`blacksmith_deposits`), combat seal scaling and skills wired into Battle Adapter, and crystal drop rolls upon monster defeat.
- **Alchemy** (`internal/alchemy`): ✅ 112 recipes; zero fee, depot-linked overnight synthesis, home sleep completion, depot-direct delivery, recipe compendium & `comp_alc` title.
- **Bank** (`internal/bank`): ✅ Character gold deposit/withdrawal; 999,999G wallet clamp.
- **Guild** (`internal/guild`, `internal/api/http`): ✅ Foundation (5,000G), dynamic Guild Points (`gpoint`) across social/combat hooks, custom member role titles, server-unique hex colors, membership application & approval gating, broadcast callouts, visual personalization (mark, wallpapers), daily scheduled 20-day inactivity automatic disbandment worker, and full HTTP REST API endpoints with OpenAPI 3.1 specs.
- **Casino** (`internal/casino`): ✅ Multi-Player Room Lobby (2..8 players, Indian Poker, High-Low, Doppelganger, speed/rate/password/spectators) hosted in Valkey Master (`ValkeyRoomRepository`, Candidate C) with 1800s sliding TTL, active ZSet index, distributed room locking (`party2:casino:lock:room:<room_id>`), and Two-Phase Settlement; authentic 13-card Indian Poker, multi-player High-Low, multi-player Doppelganger, 18 authentic prizes with Depot auto-routing, deterministic `economy.TransactionRunner` exchange, and slot machine with fatigue mechanics.
- **Lottery & Raffle** (`internal/lottery`): ✅ Server-wide 20-cap Takarakuji lottery with pessimistic row locking (`takarakuji_rounds` Rank 0 `FOR UPDATE`), atomic drawings, Rank 5 depot delivery with capacity rollback protection; Fukubiki raffle with 3-coupon Standard and 300-coupon Special draws with depot overflow routing.
- **Monster Ranch** (`internal/monster`): ✅ Monster Grandpa stabling (50–300 cap), Home pet link (8 pets), renaming (8 chars), P2P gift, wild release.
- **Plantation** (`internal/plantation`): ✅ 6 seeds, 14 fertilizer reagents (Gold or Depot/Inventory items), next-midnight JST maturation, wither/yield bonuses, Depot-direct delivery.
- **Auction Hall** (`internal/auction`): ✅ Live P2P trade (`@おくる`/`@しらべる`).
- **Collection & Monster Book** (`internal/collection`): ✅ Monster + item encyclopedia; auto-record on obtain.
- **Chapel & Blessings** (`internal/chapel`): ✅ 5 prayers, single-active constraint, daily reset Worker.
- **Colosseum PvP (闘技場)** (`internal/pvp`): ✅ Real-time 2..8 player room recruitment, Bet & Split prize pool mechanics, 9 team colors, multi-round party battle resolution, durable `pvp_wins` tracking, draw refund on 10 rounds; Valkey room repository (Candidate C), distributed room locking, and transactional settlement (`RunInTx`) with Rank 2 row locking in ascending ID order.
- **GvG Combat (ギルド戦)** (`internal/gvg`): ✅ Real-time 2..8 player guild battle rooms, room GP prize pool seeding, automatic guild color adoption, friendly guild battle prohibition, multi-round party battle resolution, round winner GP rewards, target wins match victory awards, 10-round draw limit, and 7-tier victory medals & championship cups cascading promotion; Valkey room repository (Candidate C) with distributed room locking.
- **Boss Battles (封印戦)** (`internal/boss`): ✅ 4-player Party Sealing Battles, Dejon banishment (+30% Tired), `@ふういん` resealing, HeroCount increment, celebration banquets, news broadcast, and reward item delivery with depot overflow routing.
- **Dungeon Exploration** (`internal/dungeon`): ✅ Grid-map exploration; Valkey Master run buffer (Lua CAS, 2h TTL, two-phase settle); multi-player party exploration (up to 4 players), trap damage distribution, Treasure Hunter bonus chests, map scouting (`@ちず`) with stacking vision expansion, and reward item delivery with depot overflow routing.
- **Battle Replays** (`internal/replay`): ✅ Turn-log recorder + history viewer (keyset cursor).
- **Endurance Challenge** (`internal/challenge`): ✅ 4-tier survival; Valkey Master session buffer; multi-player party challenge runs (up to 4 players, legacy HP carryover), Hall of Fame records with gravestones, and cashout reward finalization with depot overflow routing.
- **Custom Skill Gem Synthesis** (`internal/custom_skill`): ✅ 3-gem recipe synthesis, CMP/slot constraints, atomic gem swap.
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): ✅ Delivery quests (normal/rare/guild), emergency state reset.
- **Town Park & Bulletin Board** (`internal/park`): ✅ Posts with color/recipient, rate-limit, NPC divination.
- **News & Notifications** (`internal/notification`): ✅ Server-wide announcements + per-player inbox with read/unread management.
- **Home, Towns & Resting** (`internal/home`, `internal/town`): ✅ House construction/expiry cycles, pet phrases (30 max), mailbox (independent delete), sleep recovery (full HP/MP/tired recovery, fullness and chapel resets), and atomic home consumable item usage.
- **Rankings** (`internal/ranking`): ✅ 12 categories; Valkey snapshot cache, singleflight stampede guard, periodic Worker; repository decomposed into ISP-compliant sub-interfaces.
- **Event Plaza** (`internal/eventplaza`): ✅ Real-time plaza concurrency presence tracking (5-minute active window via Valkey Sorted Set + MariaDB), 26-item authentic merchant catalog at 3× markup across Tiers 1–3, active helper quest item exclusion, hand occupancy depot fallback delivery, and King Boss victory celebration banquets directly linked to Valkey presence.
- **Secret Shop** (`internal/secretshop`): ✅ `job_lv >= 7` gate, 8 items at 3× price, Depot auto-delivery, puff-puff dialogue.
- **Tavern & Food Delivery** (`internal/tavern`): ✅ 14-item menu, HP/MP restore, fullness on counter meal, raffle tickets on counter meal, recurring standing order food delivery across solo & party adventures, and automatic post-adventure fullness reset (`is_eat = 0`).
- **Black Market** (`internal/blackmarket`): ✅ Rare-point barter, Depot sacrifice/prize.
- **Flea Market** (`internal/fleamarket`): ✅ Depot-linked listings (120-server max), SQL CAS + RowsAffected guard, Depot direct-receive.
- **Gem Store** (`internal/gemstore`): ✅ Dedicated gem box (`job_lv`-scaled capacity), 55+ synthesis recipes, dual-source (inventory & depot) weighted orb appraisal.
- **God Wishes & Limit Breaks** (`internal/god`): ✅ 19 heaven wishes, OverLevel (Lv150), underworld limit-breaks (5 stages each).
- **Photo Contest** (`internal/contest`): ✅ 10-day cycle, voting, prize distribution, Hall of Fame; ISP-split sub-interfaces.
- **Party & Co-op Quests** (`internal/party`): ✅ Up to 4 players, Valkey lobby (30min idle TTL), speed configs (3/18/25), `need_join` condition checks, stage job level access gates, 10-floor dungeon crawl + Floor 11 treasure room, post-battle settlement delegation via `ApplyPostBattleResult` with automatic depot fallback on full inventory, `LostDrops` tracking, Rank 0 distributed party adventure lock, and HP-1 survival guarantee.
- **Altar of Rebirth** (`internal/altar`): ✅ 6-orb offering, Ramia awakening (30min record), 4 otherworld-item wishes, Depot fallback.
- **Wishing Well** (`internal/wishingwell`): ✅ SP → permanent stat growth (MHP/MMP +2/SP, ATK/DEF/AGI +1/SP).
- **Player Store & Town Boutiques** (`internal/store`): ✅ Shop construction (50,000G/90d), gold/item listings, 26 wallpapers, 15 furniture types.
- **Maintenance Mode** (`internal/maintenance`): ✅ Valkey/in-memory cache, admin API key, 503 middleware.

### API & Transport
- **Server Entrypoint** (`cmd/party2`): ✅ Typed config injection, modular service wiring (`wire.go`), Graceful Shutdown.
- **HTTP JSON API** (`internal/api/http`): ✅ 251 paths / 271 operations (OpenAPI 3.1); dual auth (session + PAT), IDOR defense, rate-limit, CORS, maintenance middleware, detached root context prohibition AST linter.

### Infrastructure & Operations
- **Database** (MariaDB): ✅ Migrations `001`–`085`; `make db-migrate` / `make db-reset`; connection pool env-configurable.
- **Valkey**: ✅ Delayed-action queue, distributed lock, rate-limit, ranking cache (AOF+RDB). Keyspace SSOT: `docs/architecture/valkey-keyspace.md`. Offline mock harness: `internal/testutil/valkeytest`.
- **Logging**: ✅ `log/slog` JSON structured logging with credential masking.
- **Verification**: ✅ `make check` (fmt, vet, AST linters, tests, smoke build); `make openapi-sync`; `make bench`.
- **Deployment**: ✅ Distroless minimal image, GHCR auto-publish.

---

## Immediate Priorities (Next Actions)

See [`ROADMAP.md`](ROADMAP.md) for full milestone details.

1. **Parity Milestone 3 — Adventure, Combat & Dungeons**: Completed.
2. **Parity Milestone 4 — Community, Events & Entertainment**: Completed.
3. **Domain Helper Primitives (Rule of Three)**: #683 (Dual-source storage consumption), #684 (Vitality & fatigue clamping), #685 (Crystal currency encapsulation).
4. **Client Presentation & Web UI**: Issue #140.
5. **Production Asset Pipeline & Final Licensing**: Issue #143, #202.

---

## Confirmed decisions

- Existing Party2 source code will not be reused.
- Existing Party2 assets/images will not be reused.
- Existing Party2 is a behavioral/design reference.
- `Created by Merino` may be acknowledged on the project page as the origin of the game.
- Initial implementation language is Go (Go 1.26.7).
- Components are conceptually language-independent.
- Future replacement of individual components by another language is allowed.
- Start as a modular monolith.
- Do not introduce microservices or remote protocols without a concrete requirement.
- Core should remain small.
- Feature Modules are first-class components.
- Battle is a reusable independent component.
- Scheduled actions use Valkey-backed Worker queue with push-processing and fallback.
- Durable persistence uses MariaDB.
- API layer uses Go standard library `net/http` JSON handlers.
- Production container uses Distroless minimal image.
- Domain events are available for meaningful decoupling, but should be used selectively.
- Architecture review is required for substantial feature additions.

---

## Pending Decisions / Open Questions

- frontend technology / web client framework;
- final software license (candidates: MIT, Apache-2.0, AGPLv3);
- final creative asset licenses (candidates: Creative Commons);
- final asset production and management pipeline.

Do not make these decisions merely for completeness. Decide them when the implementation requires them.

---

## Document references

- `AGENTS.md` — rules that apply to current and future development.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
- `ROADMAP.md` — phase and future-work planning.
- `docs/migration/feature-inventory.md` — Version 1.0 feature inventory.
