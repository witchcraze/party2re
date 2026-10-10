# Action Catalog & Preconditions Matrix Specification

## 1. Overview & CQRS Integration

In the Party2 Re CQRS Client/Agent Gateway architecture ([`docs/architecture/client-agent-api.md`](../architecture/client-agent-api.md)), clients use two unified operations. HTTP observation and initial commands exist; the approved progressive observation/navigation contract describes their remaining facility replacement:
- **Observation**: `GET /api/v1/characters/{id}/context` (returns character facts, unfinished work, typed selected scenes and connected eligible choices; verified facility detail remains adapter-owned)
- **Execution**: `POST /api/v1/characters/{id}/actions` (dispatches state transition commands)

To prevent LLM context window bloating (Token Bloat), the Action Catalog registers **strictly top-level facilities, services, and commands**, including verified activity continuations. Sub-selections (such as specific item IDs, skill recipes, or shop item choices) are handled within individual action parameter payloads.

Each action is statically linked to an OpenAPI 3.1 contract and explicit
`required_params`. Names alone do not describe parameter types or target choices.
The shared composer supplies strict schemas and selected-value templates;
retirement must not leave command validation linked to a removed REST operation.
Migrated entries use `executeCharacterAction` and their ActionID-specific params
condition; unmigrated entries remain linked to their REST operation. The
[command input contract](../architecture/client-agent-api.md#input-and-execution-boundary)
owns schema selection and drift validation. Gates and required parameter names
are independent of this transport mapping.

Shop selection observes the existing catalog without a funds gate and supplies
explicit product IDs and service quantity bounds. The evaluator's current
currency gate remains separate from catalog membership. Shop purchase support
is unconnected; no executable purchase or new required-parameter contract is
added by observation. See [Shop observation and retained routes](shops.md#10-selected-shop-observation-and-migration-boundary).

---

## 2. Current evaluator gates and legacy review

The current evaluator uses six gates. This is an implementation snapshot, not certification that all flags reproduce legacy behavior.

**Reviewed currency corrections:** prayer is a free daily blessing (#986), blacksmith seals use crystals (#987), and casino games use coins (#988); none of these catalog entries requires wallet gold. Exact execution prerequisites remain service-owned. Home sleep is free; it is not a paid-inn mechanic.

The gate descriptions below summarize the current implementation:

1. **Dead Gate (`$m{hp} <= 0`)**:
   - In legacy Party2 (`lib/quest.cgi:479, 902`), characters with HP <= 0 are prohibited from creating or joining combat quests and expeditions (`adventure_start`, `challenge_start`, `dungeon_start`, `boss_fight`, `pvp_room_create`).
   - Noncombat town and facility actions (banking `lib/bank.cgi:77–103`, commerce `lib/weapon.cgi:53–85`, crafting, casino, progression, social chat `lib/system.cgi:639–676`, and recovery) do NOT require living HP in legacy dispatch.
   - Deceased characters in town can freely access noncombat facilities, deposit/withdraw, trade, or rest at `home_sleep` to restore full HP and MP (`lib/sleep.cgi:32`).
2. **Fatigue Gate (`$m{tired} >= 100`)**:
   - Exhausted characters cannot initiate stamina-draining tasks (battles, dungeon runs, boss raids, or casino gambling).
   - **Allowed**: Rest (`home_sleep`), food (`tavern_order`), banking, storage management, and shopping.
3. **Sleep Gate (`$m{sleep} > 0`)**:
   - Under `party.cgi:14`, any character with remaining sleep seconds is routed unconditionally to `sleep.cgi`.
   - Ordinary actions remain blocked while the sleep duration is active or wake recovery is pending. `home_wake` is offered only when the duration has elapsed and recovery is pending.
   - **Emergency exception**: `rescue.cgi:53-65` accepts rescue during existing sleep and adds a penalty. `rescue_request` therefore has no awake requirement.
4. **Cooldown Gate (`party2:scheduled:actor:<id>`)**:
   - Characters with Pending or Processing ScheduledActions cannot execute cooldown-gated town or expedition actions. Passing ExecuteAt does not imply settlement; Completed/Failed records do not block. The actor Set is an index, not an exclusive lock ([Scheduled work lifecycle](scheduling.md)).
5. **Currency Gate (`$m{money} > 0`)**:
   - Currency-gated entry actions require positive wallet gold. Exact amounts, selected-item prices, alternate currencies and domain-specific prerequisites remain execution-service checks; the catalog has no request payload to evaluate them.
6. **Location Gate**:
   - Service entry evaluation uses ordinary selected facility and actual activity.
     Ordinary town entries remain discoverable; selected facilities narrow their
     operations. Actual activity blocks new entries and ordinary navigation.
     Displayed selection is not target authorization. Continuation candidates
     require owning membership/role/phase and bypass generic entry gates;
     conflicts retain only verified leave/recovery candidates.

### Approved progressive location and selection rules

[The shared navigation contract](../architecture/client-agent-api.md#approved-navigation-and-progressive-observation-contract)
was approved through #1047 on 2026-10-06. Ordinary selection and the shared
controls composer are implemented; Home also provides verified public/owned
projections and actor-owned mailbox pages. Other facility observations remain separate.
Current place and previous selections narrow the next choices:
town destinations, facility subjects, then applicable operations with the
selected target prefilled. Selected values are still explicit command inputs;
browsing never authorizes a mutation or silently changes its target. Home targets
select the observed house, while sleep and mailbox actions retain the owned actor.
Public Home omits owner-private counters and mailbox choices; the service verifies
the viewer account before private enrichment. [Home visibility](home.md#observation-visibility-and-navigation)
records remaining collection/mutation owners.

The navigation commands use owned actor authorization and existing sleep/pending
recovery/unfinished-work guards. They do not require living HP, wallet gold or
fatigue capacity, and do not add a cooldown or any gameplay mutation. A GET reads
selection without a write or TTL renewal. Expired selection defaults to town;
an unavailable selected subject remains explicit without a persisted fallback.
The composer renders saved selection, distinguishes adapter connection from
entry eligibility and offers only connected scene-appropriate commands. This
uses actual activity before ordinary selection, without converting selection
into membership or a run. A missing or failed selector never hides active work.

Ordinary location/selection uses a small character-scoped Valkey record. Actual
room membership, dungeon/challenge progress and sleep remain feature-owned and
determine active scenes. Entry, continuation and recovery are distinct:

- Facility entry uses current scene and actual activity. Choosing a different
  screen cannot enter/leave a room or abandon a run.
- Continuation uses existing member/observer/leader and run/turn rules. It does
  not inherit the town-only entry gate or a generic unfinished-work exclusion
  that would prevent the very action needed to finish that activity.
- Sleep permits its existing Rescue exception and explicitly ready Wake. Expiry
  and `is_ready` never mean recovery or settlement occurred.
- Missing/expired ordinary selection defaults to town; missing selection does
  not complete or erase an active feature. Conflicting exclusive activities need
  an honest recovery/conflict observation, not arbitrary priority or cleanup.

The original persists facility/home selection (`party2/party.cgi:14–30`,
`lib/system.cgi:267–318,418–435`) and replaces town choices inside games
(`_casino.cgi:10–32`, `vs_dungeon.cgi:33–48`, `vs_challenge.cgi:24–30`). Public
versus own-home actions differ (`home.cgi:26–39,73–105`). These are dispatch
requirements; current Go gates and a TTL alone do not establish complete parity.
Casino detail observation additionally requires owned-character authentication
and existing participant/spectator admission. It never implicitly joins or
spectates, and it does not apply a town-entry or sleep gate to an admitted
viewer. The [Casino visibility contract](casino.md#observation-and-visibility)
defines its phase-specific masking and owner-defined expiry effects.
Ordinary Casino room selection exposes public summaries and explicit admission
targets only. Actual admission supplies masked detail and role/phase controls;
leave is recovery, while conflict/sleep suppress start/kick/play. The unsupported
solo `casino_highlow`/`casino_doppel` catalog entries are replaced by actual
room-game choices; they are not connected commands.
The ephemeral navigation record is an approved simplicity trade-off, not a new
durable Core Character location. Legacy presence/log effects remain explicit
reconciliation work in each owning migration.

---

## 3. Gate Flags

Condition gates are represented via bitmask flags in `internal/playercontext`:

```go
type GateFlags uint32

const (
	GateDeadCheck     GateFlags = 1 << iota // Requires HP > 0 (Alive; combat quests only)
	GateFatigueCheck                        // Requires Tired < 100 (Not exhausted)
	GateSleepCheck                          // Requires Sleep == 0 (Awake)
	GateCooldownCheck                       // Requires no active ScheduledAction
	GateCurrencyCheck                       // Requires positive wallet gold for entry
	GateLocationCheck                       // Requires valid facility / town presence
)
```

---

## 4. Action Catalog Precondition Matrix

The table below documents the entry, navigation and recovery controls in `internal/playercontext/catalog.go`, their OpenAPI 3.1 mapping, required execution parameters, and precondition gates. Activity continuations in `catalog_continuation.go` use membership/role/phase facts instead of these entry gates; their strict parameters are defined in the OpenAPI source and their native-service mapping is owned by [the Gateway contract](../architecture/client-agent-api.md#entry-and-continuation).

| ID | Label | Category | OpenAPI OperationID | Required Params | Dead Gate (HP>0) | Fatigue Gate (<100) | Sleep Gate (Awake) | Cooldown Gate | Currency Gate |
|---|---|---|---|---|:---:|:---:|:---:|:---:|:---:|
| `adventure_start` | 冒険に出る | `adventure` | `executeCharacterAction` | `["stage_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `challenge_start` | 試練の道（連戦チャレンジ） | `adventure` | `startChallengeSession` | `["tier_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `dungeon_start` | ダンジョン探索 | `adventure` | `startDungeonExpedition` | `["dungeon_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `boss_fight` | ボス討伐 | `adventure` | `fightBoss` | `["boss_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `pvp_room_create` | 対戦部屋作成 | `adventure` | `postCharactersIdPvpRooms` | `[]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `home_sleep` | 自宅・宿屋で休む | `home` | `executeCharacterAction` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `home_wake` | 目を覚ます | `home` | `executeCharacterAction` | `[]` | ❌ | ❌ | ❌ | ❌ | ❌ |
| `chapel_pray` | 礼拝堂で祈る（祈願） | `home` | `prayAtChapel` | `["blessing"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `bank_deposit` | 銀行に預金する | `economy` | `executeCharacterAction` | `["amount"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `bank_withdraw` | 銀行から引き出す | `economy` | `executeCharacterAction` | `["amount"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `depot_deposit` | 預かり所にアイテムを預ける | `economy` | `postCharactersIdDepotDeposit` | `["item_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `depot_withdraw` | 預かり所からアイテムを引き出す | `economy` | `postCharactersIdDepotWithdraw` | `["item_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `auction_send` | 仕送り・送金 | `economy` | `postCharactersIdAuctionSend` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `fleamarket_list` | フリーマーケット出品 | `economy` | `createFleaMarketListing` | `["item_id", "price"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `fleamarket_purchase` | フリーマーケット購入 | `economy` | `purchaseFleaMarketListing` | `[]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `shop_purchase` | 店でアイテム購入 | `shop` | `shopPurchase` | `["item_definition_id"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `shop_sell` | 店にアイテム売却 | `shop` | `shopSell` | `["item_instance_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `shop_accessory_buy` | 装飾品購入 | `shop` | `shopAccessoryBuy` | `["item_definition_id"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `gemstore_buy` | 宝石購入 | `shop` | `buyGem` | `["gem_id"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `secretshop_purchase` | ヒミツの店で購入 | `shop` | `executeCharacterAction` | `["item_id", "quantity"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `blackmarket_trade` | 闇市景品交換 | `shop` | `tradeBlackMarketPrize` | `["prize_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `blacksmith_seal` | 鍛冶屋で刻印強化 | `crafting` | `postCharactersIdBlacksmithSeal` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `alchemy_synthesize` | 錬金調合 | `crafting` | `postCharactersIdAlchemySynthesize` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `custom_skill_set` | カスタムスキル作成 | `crafting` | `setCustomSkill` | `["name"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `plantation_sow` | 種菜園に種まき | `crafting` | `postCharactersIdPlantationSow` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `plantation_harvest` | 種菜園から収穫 | `crafting` | `postCharactersIdPlantationHarvest` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `casino_slot` | カジノスロット | `entertainment` | `playCasinoSlot` | `["bet"]` | ❌ | ✅ | ✅ | ✅ | ❌ |
| `casino_exchange` | コイン両替 | `entertainment` | `executeCharacterAction` | `["coins"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `casino_prize_exchange` | 賞品交換 | `entertainment` | `executeCharacterAction` | `["cost_coins", "count"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `casino_room_create` | カジノ部屋作成 | `social` | `executeCharacterAction` | `["name", "game_type", "speed", "max_players", "rate", "allow_spectators"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `casino_room_join` | カジノ部屋参加 | `social` | `executeCharacterAction` | `["room_id"]` | ❌ | ✅ | ✅ | ✅ | ❌ |
| `casino_room_spectate` | カジノ部屋観戦 | `social` | `executeCharacterAction` | `["room_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `lottery_raffle` | 福引を引く | `entertainment` | `playRaffle` | `["raffle_type"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `lottery_takarakuji_buy` | 宝くじ購入 | `entertainment` | `buyTakarakujiTicket` | `[]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `tavern_order` | 酒場で食事注文 | `entertainment` | `orderTavernMeal` | `["item_id"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `contest_enter` | フォトコンテスト応募 | `entertainment` | `enterContest` | `["photo_id", "title"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `wishingwell_exchange` | 願いの泉でSP交換 | `growth` | `exchangeWishingWellSP` | `["stat", "sp"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `altar_pray` | 復活の祭壇（祈り） | `growth` | `prayAltarRamia` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `god_wish` | 神の願い・限界突破 | `growth` | `grantGodWish` | `["wish_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `job_change` | ダーマ神殿で転職 | `growth` | `changeCharacterJob` | `["job_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `medal_claim` | 小さなメダル景品交換 | `growth` | `claimMedalReward` | `["reward_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `monster_tame` | モンスター捕獲・預託 | `social` | `tameMonster` | `["monster_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `helper_complete` | 何でも屋クエスト報告 | `social` | `completeHelperQuest` | `["quest_id"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `park_post` | 交流広場に伝言投稿 | `social` | `postParkMessage` | `["message"]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `rescue_request` | 緊急救出要請 | `social` | `executeCharacterAction` | `["reason"]` | ❌ | ❌ | ❌ | ❌ | ❌ |

*(Note: All facility actions implicitly require `GateLocationCheck` except `rescue_request`, which is a global recovery action. In legacy Party2, only combat/expedition quests under `lib/quest.cgi:479, 902` require living HP; town actions under `lib/system.cgi:639–655` execute regardless of character HP).*

`home_wake` also requires wakeable lifecycle state even though it has no Sleep flag. An already-awake character is not offered a redundant wake action. Dead awake characters retain noncombat entries; sleeping characters have only rescue; characters awaiting wake recovery have wake and rescue, in catalog order. Entry counts follow the current catalog, while exposed controls also require scene eligibility and a connected adapter.

### Evaluation and read contract

SecretShop destination discovery and entry also apply its existing JobLevel ≥7
qualification through the [scene navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery).
Purchase revalidates qualification, item availability, exact funds and quantity
in its service; the catalog gates alone do not establish these guarantees.

The pipeline evaluates Dead → Fatigue → Sleep → Cooldown → Currency → Location, stopping at the first rejecting gate for each action. Each gate honors its RequiredGates exemptions; wake eligibility is an explicit recovery rule. Gate evaluation is pure and changes no state.

`playercontext.Service.Query` authorizes the owning player before reading unfinished ScheduledActions, Sleep duration, Asleep recovery and required public feature activity ports. Character has no invented Sleeping or location field. Pending/Processing work stays visible after its deadline; persisted Completed/Failed records are excluded even when scheduling cleanup is pending. Activity is a whitelist of actor membership/run facts, not raw room or reward state. Missing/expired selection cannot end activity, and ordinary subject reads are skipped while activity is authoritative. Required storage/read failures fail GET and shared refresh; confirmed command results still survive failed refresh.

Scheduling and timers use existing actor-index/timer reads; activity adapters add their owning service reads. An explicitly linked Party roster and one run are one activity; otherwise simultaneous exclusive facts produce conflict recovery without an arbitrary winner. Terminal run buffers remain observed until owner cleanup and GET never finalizes them. This is not a transaction across stores, and action execution must revalidate changing state. [The Gateway contract](../architecture/client-agent-api.md#entry-and-continuation) owns continuation support and scoped legacy dispatch reconciliation.

No CharacterSnapshot or availability-result cache is introduced. The catalog is small; any cache should follow measurements and an explicit invalidation design covering Character updates and timer/lifecycle changes.

---

## 5. Client & Agent Observation and Planned Command Contract (#939/#646)

When clients receive `available_actions` via `GET /context`:

```json
{
  "action": "bank_deposit",
  "label": "銀行に預金する",
  "category": "economy",
  "style": "secondary",
  "required_params": ["amount"],
  "params_schema": {
    "type": "object",
    "additionalProperties": false,
    "properties": {"amount": {"type": "integer", "format": "int64", "minimum": -9223372036854775808, "maximum": 9223372036854775807}},
    "required": ["amount"]
  }
}
```

The AI agent or bot can immediately construct the payload for `POST /characters/{id}/actions`:

```json
{
  "action": "bank_deposit",
  "params": {
    "amount": 5000
  }
}
```

If an action has empty `required_params` (e.g. `home_sleep`, `home_wake`), `params` may be omitted or sent as `{}`.
