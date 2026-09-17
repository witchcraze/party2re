# Status

Last updated: Issue #660 — [Bug] Contest: Propagate character update errors during prize settlement to prevent reward loss

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
- **Transient State Architecture** (`docs/architecture/transient-run-state.md`, `docs/architecture/valkey-keyspace.md`): ✅ Ephemeral Turn & Session Lobby Architecture (Candidate C) across multiplayer domains with 1800s sliding TTL, active ZSet index, distributed room locking (`WithRoomLock`), and Two-Phase Settlement into MariaDB Master. In-Progress Run Buffers (Candidate D) and Shared Boss HP (Candidate E) active.
- **AST Linter Suite** (`make check`, `make arch-lint`): ✅ Automated static analysis enforcing transaction boundaries (`TransactionRunner`), deterministic lock hierarchy (Rank 0→8), file size (≤500 lines), ISP interface size (≤10 methods), dead code, Valkey keyspace, Battle Adapter boundary, crypto policy, and package boundaries.
- **Benchmark Framework** (`make bench`): ✅ Critical-path benchmarks + baseline regression detection.

### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): ✅ Registration, bcrypt (cost 12), Valkey session (7d TTL + ZSET lazy-purge), PAT (`p2_sk_...`), account deletion.
- **Character** (`internal/core/character`, `internal/character`): ✅ Attributes, progression, wallet (999,999G cap), crystal currency (`AddCrystal`/`DeductCrystal`, 999,999 cap), vitality bounds, 1 HP combat survival, fatigue clamping (`AddTired`/`ReduceTired`), profile, customization; AST-enforced encapsulation.
- **Timer & Daily Quotas** (`internal/core/timer`): ✅ Valkey/in-memory native TTL timers and JST-midnight daily quotas.
- **Progression** (`internal/core/progression`): ✅ Cumulative EXP (`level²×10`), OverLevel (Lv150), Happy Seed; AST-enforced encapsulation.
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): ✅ 72-job catalog, Lv20 job change, mastery, future memory snapshots, gem synthesis triggers.
- **Item / Inventory / Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`, `internal/economy`): ✅ 5-category catalog (269 items), stackability invariants (`IsStackable`), slot management, standardized consumption (`Consume`/`ConsumeOne`), capacity boundary enforcement (`ErrInventoryFull`).
- **Battle & Adapter** (`internal/core/battle`, `internal/battle`): ✅ Deterministic turn resolver (1v1, 4vN, 3+ factions up to 8 players); standardized Battle Adapter bridging Character/Party to Participant, equipment stat calculation, passive triggers, and atomic post-battle state application (`ApplyPostBattleResult`).
- **Random Number Generation** (`internal/core/random`): ✅ Concurrency-safe generator (`math/rand/v2`) and deterministic seeded generator for 100% reproducible tests.
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): ✅ Valkey-backed delayed queue + distributed lock worker; package coverage >90%.
- **Database & Transaction Orchestration** (`internal/database`, `internal/economy`, `internal/core/event`): ✅ Ambient `RunInTx`/`ExecutorFromContext`, deterministic lock hierarchy (Rank 0→8), single-character `economy.TransactionRunner`, multi-aggregate `TransactionProvider`, and 2-phase event dispatcher.
- **Common Utilities** (`internal/pagination`, `internal/id`, `internal/validation`): ✅ Keyset cursor pagination (`CursorPage[T]`), cryptographic ID, validation helpers.

### Feature Modules
- **Activity** (`internal/activity`): ✅ Push-based worker training with manual claim fallback.
- **Adventure** (`internal/adventure`): ✅ 10-floor dungeon crawl, Floor 11 treasure room, post-battle settlement, and combat chronicles.
- **Medal & Achievements** (`internal/medal`): ✅ Small Medal depot exchange and lifetime gameplay milestone achievement tracking.
- **Shop** (`internal/shop`): ✅ 3 town shops with job-level gates, 50% sellback, and depot auto-delivery.
- **Depot** (`internal/depot`): ✅ Up to 500 slots storage, tiered expansion, sort, sell, direct-send, item consumption, centralized transactional reward item delivery, and dynamic capacity refresh across all delivery consumers.
- **Blacksmith & Weapon Seals** (`internal/blacksmith`): ✅ 12 crystal weapon seals, equipment naming, 3-slot weapon storage, combat seal effects, and monster crystal drops.
- **Alchemy** (`internal/alchemy`): ✅ 112 crafting recipes, depot-linked overnight synthesis, home sleep completion, and recipe compendium.
- **Bank** (`internal/bank`): ✅ Gold deposits and withdrawals with 999,999G wallet clamp.
- **Guild** (`internal/guild`): ✅ Foundation, dynamic Guild Points, custom roles, hex colors, membership applications, and 20-day inactivity auto-disbandment.
- **Casino** (`internal/casino`): ✅ Multi-player room lobby (2..8 players), Indian Poker, High-Low, Doppelganger, 3-reel slot machine, and 18 depot prizes.
- **Lottery & Raffle** (`internal/lottery`): ✅ Server-wide 20-cap Takarakuji lottery with rollover jackpot; Tavern Fukubiki raffle (Standard & Special).
- **Monster Ranch** (`internal/monster`): ✅ Monster stabling (50–300 cap), Home pet link (8 pets), renaming, P2P gifting, and wild release.
- **Plantation** (`internal/plantation`): ✅ 6 seeds, 14 fertilizer reagents, midnight JST maturation, wither/yield bonuses, and depot harvest delivery.
- **Auction Hall** (`internal/auction`): ✅ Live P2P trade hall (`@おくる`/`@しらべる`).
- **Collection & Monster Book** (`internal/collection`): ✅ Illustrated monster and item encyclopedia with auto-record on obtain.
- **Chapel & Blessings** (`internal/chapel`): ✅ 5 town church blessings with single-active prayer constraint and daily reset worker.
- **Colosseum PvP** (`internal/pvp`): ✅ Real-time 2..8 player room recruitment, Bet & Split prize pools, 9 team colors, and multi-round combat resolution.
- **GvG Combat** (`internal/gvg`): ✅ Real-time 2..8 player guild battle rooms, GP prize pools, target wins, and 7-tier cascading victory medals.
- **Boss Battles** (`internal/boss`): ✅ 4-player cooperative sealing battles, Dejon banishment, HeroCount increments, and victory celebration banquets.
- **Dungeon Exploration** (`internal/dungeon`): ✅ Multi-floor grid dungeon exploration, branching tile events, party traps, map scouting (`@ちず`), and treasure chests.
- **Battle Replays** (`internal/replay`): ✅ Turn-log recorder, step-by-step playback, and match history queries.
- **Endurance Challenge** (`internal/challenge`): ✅ 4-tier survival waves, HP carryover between rounds, party challenge runs, and Hall of Fame records.
- **Custom Skill Gem Synthesis** (`internal/custom_skill`): ✅ Custom skill naming, phrase triggers, 3-gem recipe synthesis, and atomic gem exchange.
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): ✅ Delivery quests (normal/rare/guild), transactional reward delivery with depot fallback, reliable quest rotation error propagation, and emergency state rescue.
- **Town Park & Bulletin Board** (`internal/park`): ✅ Public bulletin board posts, character authorship, rate-limit, and NPC fortune divination.
- **News & Notifications** (`internal/notification`): ✅ Server-wide news announcements and per-player inbox with read/unread tracking.
- **Home, Towns & Resting** (`internal/home`, `internal/town`): ✅ House construction, companion phrases, mailbox, sleep recovery (full HP/MP/tired restore, job memory revert persistence), and consumable usage.
- **Rankings** (`internal/ranking`): ✅ 12 competitive leaderboards with Valkey caching, singleflight protection, and periodic worker.
- **Event Plaza** (`internal/eventplaza`): ✅ Real-time plaza presence tracking (5-min active window), 26-item merchant catalog at 3× markup, and victory celebration banquets.
- **Secret Shop** (`internal/secretshop`): ✅ JobLv 7 access gate, 8 rare items at 3× price, depot auto-delivery, and puff-puff dialogue.
- **Tavern & Food Delivery** (`internal/tavern`): ✅ 14-item culinary menu, restorative meals, fullness tracking, raffle tickets, and standing order food delivery across adventures.
- **Black Market** (`internal/blackmarket`): ✅ Rare item sacrifice recycling for Rare Points and 24 equipment/item rewards delivered to depot.
- **Flea Market** (`internal/fleamarket`): ✅ Fixed-price player listings (up to 5/char, 120-server max), SQL CAS guard, and depot direct-receive.
- **Gem Store** (`internal/gemstore`): ✅ Dedicated gem box storage, 55+ synthesis formulas, and dual-source (inventory & depot) orb appraisal.
- **God Wishes & Limit Breaks** (`internal/god`): ✅ 19 celestial wishes, permanent stat enhancements, Lv150 OverLevel, and storage limit breaks.
- **Photo Contest** (`internal/contest`): ✅ 10-day cycles, photo submissions, community voting, prize delivery with robust error propagation, and Hall of Fame.
- **Party & Co-op Quests** (`internal/party`): ✅ Up to 4 players, Valkey lobby, speed configs (3/18/25), need_join condition checks, 10-floor crawl, and HP-1 survival guarantee.
- **Altar of Rebirth** (`internal/altar`): ✅ 6-orb offering ritual, Ramia awakening, and 4 otherworld travel item wishes.
- **Wishing Well** (`internal/wishingwell`): ✅ SP sacrifice for permanent stat growth (MHP/MMP +2/SP, ATK/DEF/AGI +1/SP).
- **Player Store & Town Boutiques** (`internal/store`): ✅ Player shop construction (50,000G/90d), gold/item listings, and interior customization (26 wallpapers, 15 furniture types).
- **Maintenance Mode** (`internal/maintenance`): ✅ Valkey/in-memory cache, admin toggle, and 503 HTTP middleware.

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
3. **Domain Helper Primitives (Rule of Three)**: #683 (Dual-source storage consumption), #684 (Vitality & fatigue clamping), #685 (Crystal currency encapsulation) — All Completed.
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
