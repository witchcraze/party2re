# Status

Last updated: Issue #591 — [Refactor] Guild (Part 2): Membership Application & Approval Workflow, Broadcast Callouts, Customization, and Inactivity Disbandment

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
- **AST Linter Suite** (`make check`, `make arch-lint`): ✅ TransactionRunner & dual mutation boundary enforcement (#561), lock hierarchy, file size (≤500 lines), ISP interface size (≤10 methods), dead code, Valkey keyspace.
- **Benchmark Framework** (`make bench`): ✅ Critical-path benchmarks + baseline regression detection.

### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): ✅ Registration, bcrypt (cost 12), Valkey session (7d TTL + ZSET lazy-purge), PAT (`p2_sk_...`), account deletion.
- **Character** (`internal/core/character`, `internal/character`): ✅ Stats, progression, SP, wallet (999,999G cap), profile, customization; fictional Rebirth purged.
- **Timer & Daily Quotas** (`internal/core/timer`): ✅ Valkey/in-memory native TTL timers and JST-midnight daily quotas.
- **Progression** (`internal/core/progression`): ✅ Cumulative EXP (`level²×10`), OverLevel (Lv150), Happy Seed; enforced via AST linter.
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): ✅ 72-job catalog, Lv20 job change, mastery, future memory snapshots, gem synthesis triggers.
- **Item / Inventory / Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`): ✅ 5-category catalog (269 items), UsageCategory validation, domain stackability invariants (`IsStackable`), slot management, standardized item consumption interface (`Consume`, `ConsumeItem`, `ConsumeOne`, `ConsumeOneItem`).
- **Battle & Adapter** (`internal/core/battle`, `internal/battle`): ✅ Deterministic turn resolver; party battle engine (4vN), skill/item/gem-effect/field-state/revival — full `_battle.cgi` parity (#480); standardized Battle Adapter (`internal/battle`) bridging Character/Party to Participant, automatic Stat Orb / passive trigger binding, and atomic post-battle state application with deterministic row-lock hierarchy (Rank 2 -> Rank 3 -> Rank 5) (#496).
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): ✅ Valkey-backed delayed queue + distributed lock worker; package coverage 92.7%.
- **Database & Transaction Orchestration** (`internal/database`, `internal/economy`, `internal/core/event`): ✅ `RunInTx`/`ExecutorFromContext` propagation, deterministic lock hierarchy (Rank 0→8) AST-enforced, single-character `economy.TransactionRunner`, multi-aggregate/P2P `TransactionProvider` formalization (#561), consolidated safe `updateCharacter` persistence eliminating false `ErrNotFound` on unchanged updates (#585), 2-phase event dispatcher.
- **Common Utilities** (`internal/pagination`, `internal/id`, `internal/validation`): ✅ Keyset cursor pagination (`CursorPage[T]`), cryptographic ID, validation helpers.

### Feature Modules
- **Activity** (`internal/activity`): ✅ Training via Valkey Worker push + manual Claim fallback.
- **Adventure** (`internal/adventure`): ✅ Authentic 10-floor dungeon crawl loop (`vs_monster.cgi`) across 28 stages (286 monsters), Floor 11 Treasure Room resolution (`_npc_action.cgi` `add_treasure`), immediate crawl execution (1-hour timer purged), chronicles, VictoryHook (#478).
- **Medal & Achievements** (`internal/medal`): ✅ Small Medal exchange → Depot; milestone achievement observer.
- **Shop** (`internal/shop`): ✅ 3 shops with job-level gates, 50% sellback, MasterCard discount, Depot auto-delivery.
- **Depot** (`internal/depot`): ✅ Dynamic capacity (up to 500 slots), tiered expansion, sort, item sell, gold/item direct-send, standardized item consumption (`Consume`, `ConsumeOne`, `PurgeSlot`), enhancement-level & equipment stackability preservation (#558, #563), standardized `RefreshCapacity` helper across commerce modules (#559).
- **Blacksmith** (`internal/blacksmith`): ✅ +1→+10 enhancement via `economy.TransactionRunner`.
- **Alchemy** (`internal/alchemy`): ✅ 112 recipes; zero fee, depot-linked overnight synthesis, home sleep completion, depot-direct delivery, recipe compendium & `comp_alc` title (#487).
- **Bank** (`internal/bank`): ✅ Character gold deposit/withdrawal; 999,999G wallet clamp; fictional `bank_accounts` table purged (#476).
- **Inn** (`internal/inn`): ✅ Fictional paid-inn purged; resting moved to `internal/home` (#459).
- **Guild** (`internal/guild`): ✅ Foundation (5,000G), dynamic Guild Points (`gpoint`) across social/combat hooks and server rankings, custom member role titles (up to 6 full-width characters via `あたえる`), hex color customization (`からー`) with server-wide uniqueness and GvG eligibility validation, formal membership application & approval gating (`参加申請中`, `あたえる` approval, `追放` rejection/expulsion with notification letters), broadcast member callouts (`よびかける`, +1 GP), visual personalization (3,000G guild mark, catalog-priced wallpapers), and 20-day inactivity automatic disbandment; fictional gold-donation leveling and capacity scaling completely purged (#490, #591).
- **Casino** (`internal/casino`): ✅ Coin exchange, Indian Poker (sessions), Slot, Doppel, High-Low; 8-player shared room pending (#486).
- **Lottery & Raffle** (`internal/lottery`): ✅ Raffle (normal/special); 4-digit lottery — 20-cap/rollover parity pending (#484, #485).
- **Monster Ranch** (`internal/monster`): ✅ Monster Grandpa stabling (50–300 cap), Home pet link (8 pets), renaming (8 chars), P2P gift, wild release (#488). Fictional crop farm purged (#488).
- **Plantation** (`internal/plantation`): ✅ 6 seeds, 14 fertilizer reagents (Gold or Depot/Inventory items), next-midnight JST maturation, wither/yield bonuses, Depot-direct delivery (#489).
- **Auction Hall** (`internal/auction`): ✅ Live P2P trade (`@おくる`/`@しらべる`); fictional async auction house purged (#474).
- **Collection & Monster Book** (`internal/collection`): ✅ Monster + item encyclopedia; auto-record on obtain.
- **Chapel & Blessings** (`internal/chapel`): ✅ 5 prayers, single-active constraint, daily reset Worker; fictional donations purged (#472).
- **Colosseum PvP (闘技場)** (`internal/pvp`): ✅ Real-time 2..8 player room recruitment (`quest.cgi:type=4`), Bet & Split prize pool mechanics, 9 team colors (`@ぱーてぃー`), multi-round party battle resolution (`_battle.cgi:486`), durable `pvp_wins` (`$m{kill_p}`) tracking, draw refund on 10 rounds; fictional Elo rating arena purged (#481).
- **GvG Combat (ギルド戦)** (`internal/gvg`): ✅ Real-time 2..8 player guild battle rooms (`quest.cgi:type=5`, `vs_guild.cgi`), room GP prize pool seeding (2 GP initial + 1 GP per joiner), automatic guild color adoption, friendly guild battle prohibition (`color == '#FFFFFF'`), `@かいし` multi-guild validation and HP restoration, multi-round party battle resolution (`corebattle.ResolvePartyBattle`), round winner GP rewards (+3 GP), target wins (1-3 wins) match victory awards (prize pool GP + 1 Bronze Medal), all-participant +4 GP compensation, 10-round draw limit, and 7-tier victory medals & championship cups cascading promotion (5:1 ratios); fictional asynchronous Elo duels and match tables purged (#482).
- **Boss Battles (封印戦)** (`internal/boss`): ✅ 4-player Party Sealing Battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`), Dejon banishment (+30% Tired), `@ふういん` resealing, HeroCount increment, celebration banquets, news broadcast; fictional 1-day-3-attempts solo raid completely purged (#479).
- **Dungeon Exploration** (`internal/dungeon`): ✅ Grid-map exploration; Valkey Master run buffer (Lua CAS, 2h TTL, two-phase settle); multi-player party exploration (up to 4 players), trap damage distribution, Treasure Hunter (+1..+2 bonus chests), and map scouting (@ちず) vision expansion (Thief/Ninja/Geomancer/Ranger/scope_goggles) (#483).
- **Battle Replays** (`internal/replay`): ✅ Turn-log recorder + history viewer (keyset cursor).
- **Endurance Challenge** (`internal/challenge`): ✅ 4-tier survival; Valkey Master session buffer; multi-player party challenge runs (up to 4 players, legacy HP carryover), and Hall of Fame records with gravestones (`chr/099.gif`) for fallen members (#483, #600).
- **Custom Skill Gem Synthesis** (`internal/custom_skill`): ✅ 3-gem recipe synthesis, CMP/slot constraints, atomic gem swap.
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): ✅ Delivery quests (normal/rare/guild), emergency state reset.
- **Town Park & Bulletin Board** (`internal/park`): ✅ Posts with color/recipient, rate-limit, NPC divination.
- **News & Notifications** (`internal/notification`): ✅ Server-wide announcements + per-player inbox with read/unread management.
- **Home, Towns & Resting** (`internal/home`, `internal/town`): ✅ House construction/expiry cycles, pet phrases (30 max), mailbox (independent delete), sleep recovery; overnight carpenter parity (#459, #461).
- **Rankings** (`internal/ranking`): ✅ 12 categories; Valkey snapshot cache, singleflight stampede guard, periodic Worker.
- **Event Plaza** (`internal/eventplaza`): ✅ Headcount-tiered merchant, 3× price catalog, boss-banquet toasts; real-time concurrency parity pending (#491).
- **Secret Shop** (`internal/secretshop`): ✅ `job_lv >= 7` gate, 8 items at 3× price, Depot auto-delivery, puff-puff (dialogue only) (#462).
- **Tavern & Food Delivery** (`internal/tavern`): ✅ 14-item menu, HP/MP restore, fullness, raffle ticket bonus, delivery reservation with automated post-adventure arrival hook (`adventure.PostAdventureHook`); fictional courier quests and parcel courier purged (#475).
- **Black Market** (`internal/blackmarket`): ✅ Rare-point barter, Depot sacrifice/prize; fictional gold trading purged (#463).
- **Flea Market** (`internal/fleamarket`): ✅ Depot-linked listings (120-server max), SQL CAS + RowsAffected guard, Depot direct-receive.
- **Gem Store** (`internal/gemstore`): ✅ Dedicated gem box (`job_lv`-scaled capacity), 55+ synthesis recipes, dual-source (inventory & depot) weighted orb appraisal (#560).
- **God Wishes & Limit Breaks** (`internal/god`): ✅ 19 heaven wishes, OverLevel (Lv150), underworld limit-breaks (5 stages each).
- **Monster Grandpa & Pets** (`internal/monster`): ✅ 50–300 stable capacity, 8 home pets, P2P transfer, naming; Ranch parity pending (#488).
- **Photo Contest** (`internal/contest`): ✅ 10-day cycle, voting, prize distribution, Hall of Fame; ISP-split to 5 sub-interfaces.
- **Party & Co-op Quests** (`internal/party`): ✅ Up to 4 players, Valkey lobby (30min idle TTL), speed configs (3/18/25), `need_join` condition checks (`hp`/`joblv`), stage job level access gates, 10-floor dungeon crawl + Floor 11 treasure room, synergy bonus, HP-1 survival guarantee (#478).
- **Altar of Rebirth** (`internal/altar`): ✅ 6-orb offering, Ramia awakening (30min record), 4 otherworld-item wishes, Depot fallback.
- **Wishing Well** (`internal/wishingwell`): ✅ SP → permanent stat growth (MHP/MMP +2/SP, ATK/DEF/AGI +1/SP).
- **Player Store & Town Boutiques** (`internal/store`): ✅ Shop construction (50,000G/90d), gold/item listings, 26 wallpapers, 15 furniture types.
- **Maintenance Mode** (`internal/maintenance`): ✅ Valkey/in-memory cache, admin API key, 503 middleware.

### API & Transport
- **Server Entrypoint** (`cmd/party2`): ✅ Typed config injection, modular service wiring (`wire.go`), Graceful Shutdown.
- **HTTP JSON API** (`internal/api/http`): ✅ 222 paths / 243 operations (OpenAPI 3.1); dual auth (session + PAT), IDOR defense, rate-limit, CORS, maintenance middleware.

### Infrastructure & Operations
- **Database** (MariaDB): ✅ Migrations `001`–`072`; `make db-migrate` / `make db-reset`; connection pool env-configurable.
- **Valkey**: ✅ Delayed-action queue, distributed lock, rate-limit, ranking cache (AOF+RDB). Keyspace SSOT: `docs/architecture/valkey-keyspace.md`. Offline mock harness: `internal/testutil/valkeytest`.
- **Logging**: ✅ `log/slog` JSON structured logging with credential masking.
- **Verification**: ✅ `make check` (fmt, vet, AST linters, tests, smoke build); `make openapi-sync`; `make bench`.
- **Deployment**: ✅ Distroless minimal image, GHCR auto-publish.

---

## Immediate Priorities (Next Actions)

See [`ROADMAP.md`](ROADMAP.md) for full milestone details.

1. **Parity Milestone 3 — Adventure, Combat & Dungeons**: #478, #479, #481, #482, #483
2. **Parity Milestone 4 — Community, Events & Entertainment**: #490, #491, #484, #485, #486
3. **Client Presentation & Web UI**: Issue #140
4. **Production Asset Pipeline & Final Licensing**: Issue #143, #202

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
