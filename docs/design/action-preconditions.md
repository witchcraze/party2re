# Action Catalog & Preconditions Matrix Specification

## 1. Overview & CQRS Integration

In the Party2 Re CQRS Client/Agent Gateway architecture ([`docs/architecture/client-agent-api.md`](../architecture/client-agent-api.md)), client frontends (Web UI, AI Agents, Line/Discord Bots) interact with the backend via two unified operations:
- **Observation**: `GET /api/v1/characters/{id}/context` (returns character snapshot, active cooldowns, and a whitelist of executable actions)
- **Execution**: `POST /api/v1/characters/{id}/actions` (dispatches state transition commands)

To prevent LLM context window bloating (Token Bloat), the Action Catalog registers **strictly top-level facilities, services, and commands** (~30–40 actions). Sub-selections (such as specific item IDs, skill recipes, or shop item choices) are handled within individual action parameter payloads.

Each action is statically linked to its corresponding OpenAPI 3.1 `operationId` and explicit `required_params` keys, enabling AI agents and automated clients to construct valid API payloads without guessing or inspecting massive OpenAPI specs.

---

## 2. Legacy Specification Parity (Ground Truth)

In legacy Party2 (`party.cgi`, `lib/*.cgi`), action availability is governed by five core state gates:

1. **Dead Gate (`$m{hp} <= 0`)**:
   - Deceased characters cannot participate in battles (`vs_monster`, `vs_player`, `boss`), play casino games, purchase items, or perform economic transactions.
   - **Exceptions**: `home_sleep` (revives HP/MP to full in `sleep.cgi`), `home_wake`, `chapel_pray` (church revival), and `rescue_request` (emergency unstick).
2. **Fatigue Gate (`$m{tired} >= 100`)**:
   - Exhausted characters cannot initiate stamina-draining tasks (battles, dungeon runs, boss raids, or casino gambling).
   - **Allowed**: Rest (`home_sleep`), food (`tavern_order`), banking, storage management, and shopping.
3. **Sleep Gate (`$m{sleep} > 0`)**:
   - Under `party.cgi:14`, any character with remaining sleep seconds is routed unconditionally to `sleep.cgi`.
   - **All actions are completely blocked** except `home_wake` (waking up when timer completes).
4. **Cooldown Gate (`party2:scheduled:actor:<id>`)**:
   - Characters actively executing delayed actions (adventures or training) cannot execute concurrent town or expedition actions.
5. **Currency Gate (`$m{money} > 0`)**:
   - Actions requiring non-zero gold or entry fees (e.g., bank deposits, purchases, gambling bets, lottery tickets, tavern meals).

---

## 3. Gate Flags

Condition gates are represented via bitmask flags in `internal/playercontext`:

```go
type GateFlags uint32

const (
	GateDeadCheck     GateFlags = 1 << iota // Requires HP > 0 (Alive)
	GateFatigueCheck                        // Requires Tired < 100 (Not exhausted)
	GateSleepCheck                          // Requires Sleep == 0 (Awake)
	GateCooldownCheck                       // Requires no active ScheduledAction
	GateCurrencyCheck                       // Requires positive currency / sufficient gold
	GateLocationCheck                       // Requires valid facility / town presence
)
```

---

## 4. Action Catalog Precondition Matrix

The table below documents all 42 canonical actions in `internal/playercontext/catalog.go`, their OpenAPI 3.1 mapping, required execution parameters, and precondition gates.

| ID | Label | Category | OpenAPI OperationID | Required Params | Dead Gate (HP>0) | Fatigue Gate (<100) | Sleep Gate (Awake) | Cooldown Gate | Currency Gate |
|---|---|---|---|---|:---:|:---:|:---:|:---:|:---:|
| `adventure_start` | 冒険に出る | `adventure` | `startAdventure` | `["stage_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `challenge_start` | 試練の道（連戦チャレンジ） | `adventure` | `startChallengeSession` | `["tier_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `dungeon_start` | ダンジョン探索 | `adventure` | `startDungeonExpedition` | `["dungeon_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `boss_fight` | ボス討伐 | `adventure` | `fightBoss` | `["boss_id"]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `pvp_room_create` | 対戦部屋作成 | `adventure` | `postCharactersIdPvpRooms` | `[]` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `home_sleep` | 自宅・宿屋で休む | `home` | `homeSleep` | `[]` | ❌ | ❌ | ✅ | ✅ | ❌ |
| `home_wake` | 目を覚ます | `home` | `homeWake` | `[]` | ❌ | ❌ | ❌ | ❌ | ❌ |
| `chapel_pray` | 礼拝堂で祈る（祈願・蘇生） | `home` | `prayAtChapel` | `["blessing"]` | ❌ | ❌ | ✅ | ✅ | ✅ |
| `bank_deposit` | 銀行に預金する | `economy` | `postCharactersIdBankDeposit` | `["amount"]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `bank_withdraw` | 銀行から引き出す | `economy` | `postCharactersIdBankWithdraw` | `["amount"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `depot_deposit` | 預かり所にアイテムを預ける | `economy` | `postCharactersIdDepotDeposit` | `["item_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `depot_withdraw` | 預かり所からアイテムを引き出す | `economy` | `postCharactersIdDepotWithdraw` | `["item_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `auction_send` | 仕送り・送金 | `economy` | `postCharactersIdAuctionSend` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `fleamarket_list` | フリーマーケット出品 | `economy` | `createFleaMarketListing` | `["item_id", "price"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `fleamarket_purchase` | フリーマーケット購入 | `economy` | `purchaseFleaMarketListing` | `[]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `shop_purchase` | 店でアイテム購入 | `shop` | `shopPurchase` | `["item_definition_id"]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `shop_sell` | 店にアイテム売却 | `shop` | `shopSell` | `["item_instance_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `shop_accessory_buy` | 装飾品購入 | `shop` | `shopAccessoryBuy` | `["item_definition_id"]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `gemstore_buy` | 宝石購入 | `shop` | `buyGem` | `["gem_id"]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `secretshop_purchase` | ヒミツの店で購入 | `shop` | `purchaseSecretShopItem` | `["item_id"]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `blackmarket_trade` | 闇市景品交換 | `shop` | `tradeBlackMarketPrize` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `blacksmith_seal` | 鍛冶屋で刻印強化 | `crafting` | `postCharactersIdBlacksmithSeal` | `[]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `alchemy_synthesize` | 錬金調合 | `crafting` | `postCharactersIdAlchemySynthesize` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `custom_skill_set` | カスタムスキル作成 | `crafting` | `setCustomSkill` | `["name"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `plantation_sow` | 種菜園に種まき | `crafting` | `postCharactersIdPlantationSow` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `plantation_harvest` | 種菜園から収穫 | `crafting` | `postCharactersIdPlantationHarvest` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `casino_slot` | カジノスロット | `entertainment` | `playCasinoSlot` | `["bet"]` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `casino_highlow` | ハイ＆ロー | `entertainment` | `playCasinoHighLow` | `["bet", "guess"]` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `casino_doppel` | ドッペルゲンガー | `entertainment` | `playCasinoDoppel` | `["bet", "pool_size", "player_mark"]` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `lottery_raffle` | 福引を引く | `entertainment` | `playRaffle` | `["raffle_type"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `lottery_takarakuji_buy` | 宝くじ購入 | `entertainment` | `buyTakarakujiTicket` | `[]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `tavern_order` | 酒場で食事注文 | `entertainment` | `orderTavernMeal` | `[]` | ✅ | ❌ | ✅ | ✅ | ✅ |
| `contest_enter` | フォトコンテスト応募 | `entertainment` | `enterContest` | `["photo_id", "title"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `wishingwell_exchange` | 願いの泉でSP交換 | `growth` | `exchangeWishingWellSP` | `["stat", "sp"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `altar_pray` | 復活の祭壇（祈り） | `growth` | `prayAltarRamia` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `god_wish` | 神の願い・限界突破 | `growth` | `grantGodWish` | `["wish_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `job_change` | ダーマ神殿で転職 | `growth` | `changeCharacterJob` | `[]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `medal_claim` | 小さなメダル景品交換 | `growth` | `claimMedalReward` | `["reward_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `monster_tame` | モンスター捕獲・預託 | `social` | `tameMonster` | `["monster_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `helper_complete` | 何でも屋クエスト報告 | `social` | `completeHelperQuest` | `["quest_id"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `park_post` | 交流広場に伝言投稿 | `social` | `postParkMessage` | `["message"]` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `rescue_request` | 緊急救出要請 | `social` | `requestEmergencyRescue` | `[]` | ❌ | ❌ | ✅ | ❌ | ❌ |

*(Note: All facility actions implicitly require `GateLocationCheck` except `rescue_request`, which is a global recovery action).*

---

## 5. Client & Agent Tool Calling Contract

When clients receive `available_actions` via `GET /context`:

```json
{
  "action": "bank_deposit",
  "label": "銀行に預金する",
  "category": "economy",
  "required_params": ["amount"]
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

If an action has empty `required_params` (e.g. `home_sleep`, `job_change`), `params` may be omitted or sent as `{}`.
