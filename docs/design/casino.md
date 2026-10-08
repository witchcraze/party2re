# Casino Architecture & Multi-Player Facility Design

## Overview

The Casino (カジノ) in Party2 is an entertainment and wagering facility based faithfully on `party2/lib/casino.cgi`, `party2/lib/_casino.cgi`, `party2/lib/casino_indian.cgi`, `party2/lib/casino_highlow.cgi`, and `party2/lib/casino_doppel.cgi`.

It provides:
1. **Casino Currency Exchange**: One-way exchange from character gold to casino coins (1 Coin = 20 Gold; cashing out coins back to gold is prohibited per legacy clean-room specification).
2. **Multi-Player Room Lobby**: Real-time room recruitment and turn-based games for 2 to 8 players.
3. **Authentic Mini-Games**:
   - Multi-Player Indian Poker (`indian`) — 13-card blind bluffing game with forehead placement and pot distribution.
   - Multi-Player High & Low (`highlow`) — 13-card rank prediction and split pot showdown.
   - Multi-Player Doppelganger (`doppel`) — 8-symbol dealer matching and leadership transfer.
   - Slot Machine (`slot`) — Solo 3-reel, 5-symbol paytable with 100x jackpot.
4. **Prize Exchange & Depot Routing**: 18 authentic prizes delivered directly into long-term Depot storage (`character_depots`).

---

## 1. Currency & Account Invariants

- **Exchange Rate**: `1 Casino Coin = 20 Gold`.
- **Wallet vs. Bank Balance**:
  - Buying coins deducts from character wallet gold (`characters.money`).
  - Selling coins is strictly prohibited per legacy clean-room specification (one-way exchange only, purged in #630).
- **Non-negative Balance**:
  - `casino_accounts.coins >= 0` enforced by table schema and transactional checks.

---

## 2. Multi-Player Room Lobby (Candidate C: Ephemeral Valkey Master)

Rooms serialize multi-player games using Valkey Master (`ValkeyRoomRepository`, Candidate C Ephemeral Turn & Session Lobby Architecture, SSOT: [`docs/architecture/transient-run-state.md`](../architecture/transient-run-state.md)). Legacy MariaDB tables `casino_rooms` and `casino_members` were dropped in Migration 085.

### Creation Rules (`@つくる` / `POST /characters/{id}/casino/rooms`)
- **Name**: 1–50 runes, unique among non-disbanded rooms, cannot contain spaces, tabs, newlines, or delimiters (`,;\&<>\\/@＠`).
- **Game Types**: `indian`, `highlow`, `doppel`.
- **Turn Speed**:
  - `12` seconds (さくさく / Fast)
  - `18` seconds (まったり / Normal)
  - `28` seconds (じっくり / Slow)
- **Capacity**: 2 to 8 players.
- **Base Rate**:
  - Standard (`indian`, `highlow`): `1`, `5`, `10`, `20`, `50`, `100`, `500`, `1000`, `5000` coins.
  - Doppelganger (`doppel`): Minimum 10 coins.
  - Creator minimum balance: `rate * 5` coins (`rate` for doppel).
- **Aikotoba (Password)**: Optional plaintext password hashed via SHA-256.
- **Spectator Mode**: Optional boolean flag.

### Participant Lifecycle
- **Join (`@さんか`)**: Gated by `tired < 100`, `coins >= rate`, and available capacity.
- **Spectate (`@けんがく`)**: Gated by room spectator allowance. Spectators cannot act or receive pot rewards.
- **Leave (`@にげる`)**: Removes player. Participating players incur a +1 fatigue penalty (`char.AddTired(1)` per `party2/lib/_casino.cgi:216-218`). Spectators leave freely without fatigue penalty. Automatically reassigns room leader to next active member; disbands room if 0 active members remain.
- **Kick (`@きっく`)**: Leader-only eviction of non-leader members prior to game start (`round == 0`).
- **Unified Game Dispatch**:
  - `POST /characters/{id}/casino/rooms/{roomId}/start`: Starts the game according to configured `game_type`.
  - `POST /characters/{id}/casino/rooms/{roomId}/action`: Dispatches player actions according to configured `game_type`.

### Observation and visibility

The anonymous lobby is a window of at most 100 active rooms containing identity,
game, speed, capacity, base rate, password requirement, spectator allowance,
status and participant/spectator counts. It excludes member identities, cards,
marks, actions, pot and private timestamps. Empty lists are non-null arrays.
Gateway ordinary selection uses the existing shared navigation record. Lobby
pages contain at most 100 rooms from that public window, ordered by room ID,
with explicit offset/limit and next-page parameters. This is not discovery of
all rooms or a cross-page snapshot. A selected room exposes only its public
summary and explicit join/spectate target parameters; it grants no admission.

Detail requires a session and an owned `character_id`, verified again at the
Casino service boundary, plus actual participant or admitted spectator
membership. Anonymous requests return 401, nonowned/nonmember characters 403,
and absent/disbanded/expired rooms 404. Selection never grants admission: join
and spectate remain explicit mutations enforcing password; spectate also checks
spectator allowance.

| Game / phase | Participant | Admitted spectator |
| --- | --- | --- |
| Indian, active | Own card hidden unless folded or waiting; other cards and declarations visible | Participant cards/declarations visible |
| Highlow, active | Own card/action visible; other cards hidden except waiting members; competitive declarations hidden, continue/wait visible | Same rules without an own participant card |
| Doppel, active | Own mark visible; other marks hidden | All participant marks hidden |
| Before start / after settlement (`round <= 0`) | Remaining cards/marks and declarations visible | Same visibility |

Spectator records expose no playable cards/actions. Hidden cards/marks use
`card: -1` and `card_display: "？"`. Go Doppel actions encode the selected mark
itself, so other participants' mark actions are masked as `"？？？"`; the legacy
routine displays actions separately from hidden cards. Evidence is
`lib/casino_indian.cgi:22–33`, `lib/casino_highlow.cgi:33–50`,
`lib/casino_doppel.cgi:23–36`, admission in `lib/casino.cgi:382–516`, and common
dispatch in `lib/_casino.cgi:10–32,100–111`. Current settlement returns to
waiting with round zero; there is no separate finished room status. Formula
parity and settlement history are outside this read-boundary inspection.

Reads never start, advance, wager or settle.

Gateway activity discovers the owned actor's existing character-room mapping
through `GetCharacterRoomView`, then applies `GetRoomView` authorization/masking.
Context adds the authorized masked room view, the actor's coin balance and
service-owned role/phase controls to the activity facts, including conflicts.
Actual admission overrides ordinary selection, even if the selector cannot be
read. Spectators get leave only. Waiting leaders get start and kick targets;
active participants get game action values, with Low only for more than two
participants and Doppel marks bounded by participant count. Conflicts or sleep
retain room facts and leave recovery while suppressing start/kick/play choices.
If admission/phase/turn eligibility changes between the activity and detail
reads, context fails rather than presenting mixed facts; retry observation with
GET. These are unconnected Gateway controls until mutation migration; retained
REST operations remain available. Lobby menus expose existing account coins,
one-way exchange rate, prize catalog and job-gated slot rates without executing
any of those operations.

Detail uses the existing room lock
for a consistent phase/member snapshot and does not renew activity/TTL. Casino
owns the 30-minute idle lifetime: lobby reads invoke expiry, which rechecks
activity under that lock before deletion, then prune stale index entries.
Expiry, store and required detail-name enrichment failures propagate instead
of becoming empty observations. Lobby counts require no name enrichment.
Valkey TTL remains the live-state lifetime authority; no cross-store settlement
guarantee is added. Legacy expiry also applied a sleep penalty
(`lib/casino.cgi:178–190`); that side effect remains reconciliation work under
[#949](https://github.com/witchcraze/party2re/issues/949).

### Scoped legacy dispatch reconciliation

This maps the inspected observation/admission/choice paths, not wager or
settlement formulas. Paths are relative to the original `party2` distribution.
All operational REST routes remain until their mutation replacements are verified.

| Legacy action / call path | Observation / retained service and transport / remaining owner |
| --- | --- |
| `lib/casino.cgi:94–113,195–381`: `つくる`, `インディアン`, `ハイロウ`, `ドッペル` creation selectors | Lobby navigation is separate from `CreateRoom` with explicit `game_type`; retained POST `/characters/{id}/casino/rooms`. Gateway `casino_room_create` candidate is unconnected; adapter belongs to #949. |
| `lib/casino.cgi:95–96,382–518`: `さんか`, `けんがく`, optional `あいことば` | Public selected summary supplies explicit room targets; `JoinRoom`/`SpectateRoom` enforce password, capacity/balance/fatigue or spectator allowance. Retained join/spectate POST routes; Gateway admission candidates remain unconnected under #949. |
| `lib/casino.cgi:97–113,523–594`: `＄1/10/50/100/200すろっと` | Read menu uses existing wager rates and Job 46's 200-coin gate. `SpinSlot` and retained POST `/characters/{id}/casino/slot`; Gateway connection remains #949. |
| `lib/casino.cgi:97–113,596–688`: `こうかん`, `りょうがえ` | Account, prize catalog and `GoldPerCoin` facts; `ExchangePrize`/`ExchangeGoldToCoins` remain prize-exchange/exchange POST operations. Explicit costs/counts/coins are command inputs; reading never exchanges. Gateway adapters remain #949. |
| `lib/_casino.cgi:10–32,100–111,148–239`: `にげる`, `かいし`, leader `きっく` | Owned admitted activity controls map to `LeaveRoom`/`StartGame`/`KickMember`, retaining leave/start/kick POST routes. Selection/location cannot block leave or ongoing play. Mutation connection and progression-on-leave reconciliation remain #949. |
| `lib/_casino.cgi:10–32,124–145`: `すくしょ`, `さそう` | No Casino service/route replacements. Screenshot is presentation and invitation is social reconciliation; explicitly deferred to #949/#140. |
| `lib/casino_indian.cgi:10–33`: `つづける`, `しょうぶ`, `おりる` and forehead masking | `call`/`showdown`/`fold` candidates and authorized masked `GetRoomView`; retained room action POST → `PlayRoomAction` → Indian service. Gateway adapter remains #949. |
| `lib/casino_highlow.cgi:10–50`: `つづける`, `ハイ`, conditional `ロウ`, `おりる` and own-card/action masking | Current room `call`/`high`/`low`/`fold` candidates and masked view; retained room action POST → Highlow service. Legacy hides continue at maximum bet or insufficient coins; current service accepts all-in calls. That menu/service difference remains #949 reconciliation, not a formula change here. |
| `lib/casino_doppel.cgi:10–99`: generated marks and own-mark masking | Participant-count mark choices and masked view; retained room action POST → Doppel service. Legacy clears declarations on start and stores mark changes in the card field, permitting reselection before all cards are selected. Go stores marks in Action too, so its viewer masks other mark actions. Waiting entrants' eligibility remains existing service behavior for #949 reconciliation. |
| `lib/casino.cgi:118–190`, game `member_html`, `_casino.cgi:48–97` | Lobby/selected summary and authorized activity detail replace observation through context, without advancing or settling. Idle expiry remains feature-owned; its legacy sleep penalty is deferred to #949. |

The former OpenAPI singles-play Highlow/Doppel POST paths had no registered
handlers and are removed along with their fictional solo catalog entries.
Both games use explicit room creation/start/action commands. No new handler is
invented; Chapel blessing remains Chapel's responsibility.

### Writer-side persistence and partial failure boundary

Room mutation operations (`CreateRoom`, `UpdateRoom`, `AddMember`, `UpdateMember`, `RemoveMember`, `DeleteRoom`) synchronize both the room detail payload (`party2:casino:room:<room_id>`) with sliding TTL (1800s) and the active index (`party2:casino:rooms:active`).

In `ValkeyRoomRepository.saveRoomDetail`:
1. The room JSON payload is written via `SET ... EX 1800`.
2. The active index is updated: `ZADD` with `UpdatedAt.Unix()` score for active rooms, or `ZREM` and member key deletion for disbanded rooms.
3. Errors from both the payload SET and the subsequent active index ZADD/ZREM or member key operations are propagated truthfully to callers.

Because Valkey operations are not cross-key transactional here:
- If active index renewal (`ZADD`) fails, the room payload `SET` may already have succeeded in Valkey.
- Write failures must never authorize blind command replay; clients must not assume payload rollback, as blindly retrying a failed command could double-deduct wagers or violate round action sequencing on an already-mutated payload.
- In this partial-write state, the renewed room remains intact and queryable by ID (`GetRoom`), while lobby discovery (`ListRooms` / `ListActiveRooms`) relies on the active index score. When that score crosses the 30-minute cutoff, `PurgeIdleRooms` rechecks the locked payload timestamp and preserves the room, while `ListActiveRooms` prunes the stale index from lobby discovery until a subsequent successful renewal write restores the index score.

---

## 3. Multi-Player Indian Poker (`party2/lib/casino_indian.cgi`)

### Rules & Mechanics
- **Deck**: 13 unique cards (`Ａ`, `２`, `３`, `４`, `５`, `６`, `７`, `８`, `９`, `10`, `Ｊ`, `Ｑ`, `Ｋ`).
- **Forehead Card Rule**: Each player cannot view their own dealt card (masked as `？` / `-1` in API client views), but sees all opponent cards.
- **Actions**:
  - `call` (`つづける`): Pay current round bet into pot and stay in the hand.
  - `showdown` (`しょうぶ`): Pay current round bet into pot and vote to conclude.
  - `fold` (`おりる`): Forfeit hand and pay current round bet into pot (`party2/lib/casino_indian.cgi:102`); reveals card immediately.
- **Round Flow & Settlement**:
  - When all active members declare actions:
    - If all but one fold $\rightarrow$ remaining player wins.
    - If $\ge 50\%$ vote showdown, or maximum bet reached, or a player exhausts coins $\rightarrow$ Showdown triggers.
    - Otherwise $\rightarrow$ `round++`, `current_bet += rate`.
  - Highest card among surviving non-folded players wins entire pot.
  - Ties split the pot equally.
  - Members with 0 coins remaining are automatically ejected.

---

## 4. Multi-Player High & Low (`party2/lib/casino_highlow.cgi`)

### Rules & Mechanics
- **Deck**: 13 unique cards (`Ａ`..`Ｋ`), dealt 1 per player secretly.
- **Card & Action Visibility**:
  - During an active round, player sees their **own** card; opponents' cards are masked as `？` / `-1`, except waiting members whose cards remain visible (`lib/casino_highlow.cgi:45`). At round zero, remaining cards are revealed to admitted viewers.
  - Opponents' declared competitive actions (`high`, `low`, `fold`) are masked as `？？？` during active round to prevent information leakage, while `call`/`tsuzukeru` and `待機中` remain visible.
- **Actions**:
  - `call` (`つづける`): Match current round bet into pot and stay in the hand.
  - `high` (`ハイ`): Bet current round bet that own card is highest.
  - `low` (`ロウ`): Bet current round bet that own card is lowest (requires $> 2$ participants).
  - `fold` (`おりる`): Forfeit hand and pay current bet into pot.
- **Round Flow & Progression**:
  - When all active participants have acted in the round:
    - Showdown triggers if:
      - Active non-folded players $\le 1$, OR
      - Any non-folded player exhausts coins (`coins <= 0`), OR
      - Current bet reaches maximum (`current_bet >= rate * 5`), OR
      - Number of High + Low declarations $\ge 50\%$ of active non-folded players.
    - Otherwise: `round++`, `current_bet += rate`. Non-folded participants reset action to `""` and keep cards.
- **Showdown Resolution**:
  - Determine highest card among `high` callers (`higher`) and lowest card among `low` callers (`lower`).
  - **Split Pot (3+ players)**: If both `higher` and `lower` exist and room has $> 2$ players, the pot is divided 50/50 (`pot / 2`) between both winners.
  - **Single Winner**: If only `higher` exists, `higher` wins full pot. If only `lower` exists, `lower` wins full pot.
  - **All Fold**: If all players folded, game ends with no winner ("お流れ") and pot clears.
  - Members with 0 coins remaining are ejected from room; survivors reset to `待機中`.

---

## 5. Multi-Player Doppelganger (`party2/lib/casino_doppel.cgi`)

### Rules & Mechanics
- **Marks**: 8 symbols (`★`, `●`, `◆`, `♪`, `■`, `▲`, `†`, `▼`).
- **Selectable Pool**: `0..min(len(participants), 7)`. For 2 players: 3 marks (`★`, `●`, `◆`); for 3 players: 4 marks; for 7+ players: all 8 marks.
- **Mark Visibility**: Each player sees only their **own** chosen mark; all opponents' marks and actions are masked as `？` / `？？？` during the round.
- **Wager & Selection**:
  - Bet rate is fixed (minimum 10 coins).
  - Coin deduction occurs on a player's **first** mark selection in the round.
  - Active players may change their selected mark before the other players
    finish selecting, without another coin deduction. Legacy clears declarations
    at start and changes only the card field (`lib/casino_doppel.cgi:41–99`).
- **Showdown Resolution**:
  - Triggers immediately when all active participants have chosen a mark.
  - The room leader is the "親" (dealer / target).
  - Any non-leader participant ("子") whose mark matches the leader's mark is a winner:
    - **Children Win**: If $\ge 1$ children match the leader, they split the entire pot equally (`pot / len(winners)`).
    - **Leadership Transfer**: A winner chosen at random becomes the new room leader.
    - **Parent Wins**: If 0 children match the leader, the leader wins the entire pot and retains room leadership.
  - Members with 0 coins remaining are ejected from room; survivors reset to `待機中`.

---

## 6. Solo 3-Reel Slot Machine (`party2/lib/casino_slot.cgi`)

### Rules & Paytable
The Slot Machine (スロットマシン) is a single-player casino wagering game. Players wager casino coins to spin 3 reels containing 5 distinct symbols. Symbol combinations along the payline award multipliers and coin payouts.

- **Reels & Symbols**: 3 reels ($R_1, R_2, R_3$) with 5 symbols:
  - **Seven (`SymbolSeven` / ７)**
  - **Star (`SymbolStar` / ★)**
  - **Dagger (`SymbolDagger` / †)**
  - **Note (`SymbolNote` / ♪)**
  - **Cherry (`SymbolCherry` / ∞)**

### Payout Multipliers

| Combination | Multiplier | Payout on Bet $B$ | Description |
| :--- | :---: | :---: | :--- |
| **７ ７ ７** | **100x** | $100 \times B$ | 777 Jackpot |
| **★ ★ ★** | **70x** | $70 \times B$ | Super Win |
| **† † †** | **50x** | $50 \times B$ | Big Win |
| **♪ ♪ ♪** | **20x** | $20 \times B$ | Standard Win |
| **∞ ∞ ∞** | **10x** | $10 \times B$ | Triple Cherry |
| **∞ ∞ [Any]** | **3x** | $3 \times B$ | Double Cherry (First 2 reels) |
| Any other | **0x** | $0$ | Miss (Loss of wagered bet $B$) |

### Betting Rates & Job Restrictions
- Allowed Bet Rates: **1**, **10**, **50**, **100**, **200** coins.
- **Job 46 Gate**: The 200-coin slot bet is restricted to characters in Job 46 (Gambler / ギャンブラー) (`party2/lib/casino.cgi:98, 109-111`). Other jobs attempting a 200-coin bet are rejected with `ErrJobNotEligibleForSlot200`.

### Fatigue & Rest Interaction
- **Fatigue Gate**: Spinning is blocked if character fatigue `Tired >= 100` (`ErrCharacterExhausted`).
- **Fatigue Penalty on Miss**: When a spin results in a Miss, character fatigue increases by +1 (`char.AddTired(1)` per `party2/lib/casino.cgi:583, 588`).

### Celestial Wish 5 Bonus
- Characters with the celestial Casino Blessing (Wish 5 from `internal/god`) receive a 25% chance of a +50% payout bonus on winning spins (`party2/lib/casino.cgi:565-568, 577-580`).

---

## 7. Authentic Prize Catalog & Depot Routing

### 18 Authentic Prizes (`party2/lib/casino.cgi:41-65`)

| Coins | Item ID | Item Name | Category |
| :---: | :--- | :--- | :---: |
| **100** | `item-004` | 賢者の石 | Consumable |
| **300** | `item-012` | 祈りの指輪 | Accessory |
| **700** | `item-006` | 霊樹の葉 | Consumable |
| **2,000** | `item-032` | 物真似の心 | Consumable |
| **4,000** | `item-038` | 幻獣の実 | Consumable |
| **5,000** | `item-039` | ギャンブルハート | Consumable |
| **8,000** | `armor-34` | 危ない水着 | Armor |
| **30,000** | `weapon-31` | 必殺のピアス | Weapon |
| **70,000** | `weapon-40` | 流銀の剣 | Weapon |
| **80,000** | `weapon-38` | 茨の霊鞭 | Weapon |
| **180,000** | `item-106` | 金の鶏 | Consumable |
| **200,000** | `item-105` | 幸せのくつ | Consumable |
| **1,000,000** | `item-231` | 宇宙の壁紙 | Consumable |
| **1,000,001** | `item-232` | 蟻地獄の壁紙 | Consumable |
| **1,000,002** | `item-233` | 炎の壁紙 | Consumable |
| **1,000,003** | `item-234` | 墓場の壁紙 | Consumable |
| **1,000,004** | `item-235` | 図書館の壁紙 | Consumable |
| **1,000,005** | `item-236` | 要塞の壁紙 | Consumable |

### Atomic Depot Storage Routing
- Items are deposited directly into character depot storage (`character_depots`, `depot_items`).
- If depot is full, `depot.ErrDepotFull` is returned and zero coins are deducted.
- Stackable items are automatically merged with existing inventory slots.

---

## 8. Concurrency & Lock Acquisition Hierarchy
 
Multi-player room lobbies and in-flight turns reside exclusively in Valkey Master (Candidate C Ephemeral Turn & Session Lobby Architecture). MariaDB transactional operations strictly follow the global lock acquisition hierarchy for financial settlements and prize exchange:
```text
Rank 2: characters (Character Primary Entity)
  ↓
Rank 5: character_depots, depot_items (Depot Storage)
  ↓
Rank 8: casino_accounts (Secondary Feature Records)
```
In `ExchangePrize`, Depot lock (Rank 5) is acquired before Casino account lock (Rank 8), preventing deadlocks with concurrent transactions.

### Showdown Settlement Atomicity & Error Boundary

During multiplayer game actions and showdown resolution (`highlow`, `doppel`, `indian_poker`), financial updates (coin deductions, payouts, casino wins) in MariaDB and transient room/member updates in Valkey operate under a strict settlement failure boundary:

- **Pre-Payload Failures (Authoritative Write Failure)**:
  If balance checks (`s.repo.GetAccount`), coin deductions/payouts, or the Valkey room/member payload `SET` fails, the error is propagated immediately and the entire MariaDB transaction rolls back cleanly. Unsettled coins remain in players' balances, and room state is not advanced.
- **Post-Payload Secondary Index Failures (`SecondarySyncError`)**:
  Once the Valkey room payload `SET` succeeds (e.g. pot is committed as settled and room status transitions to waiting/round 0), any failure in secondary indices (active room index `ZADD`/`ZREM` or member mapping `SET`/`DEL`) is wrapped as `ErrSecondarySync`. Because the game state has already transitioned authoritatively in Valkey, rolling back SQL would destroy the pot and leave balances short. The service therefore allows the durable SQL transaction to commit (awarding coins and casino wins to the rightful winner), while returning `ErrSecondarySync` to the caller.
- **Coin Conservation & Replay Rejection**:
  Total coins across all participants are strictly conserved in both healthy settlements and secondary synchronization failure scenarios. Replaying the action after a secondary sync failure is rejected with `ErrGameNotInRound` or `ErrAlreadyActed`, preventing duplicate payouts or lost wagers.

---

## 9. Casino Wins (`cas_c`) & Job Advancement

- **Casino Wins Tracking**:
  - Showdown winners in Indian Poker (`party2/lib/casino_indian.cgi:165`), High & Low (`party2/lib/casino_highlow.cgi:216`), and Doppelganger (`party2/lib/casino_doppel.cgi:120,138`) increment their character's `CasinoWins` (`cas_c`) by 1.
  - Tracked persistently in `characters.casino_wins`.
- **Gambler Job Advancement (Job 46)**:
  - Advancing to Job 46 (Gambler / ギャンブラー) requires `CasinoWins >= 10` alongside holding `item-039` (ギャンブルハート) (`party2/lib/_data.cgi:134`). Characters with `< 10` casino wins cannot unlock Job 46 (`ErrJobUnavailable`).
- **Rankings**:
  - `characters.casino_wins` feeds into the 勝負師ランキング leaderboard (`ranking.cgi:16`).
