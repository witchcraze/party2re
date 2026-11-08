# Town Black Market, Rare Point Barter & NPC @闇商人 Design

## Overview

The Black Market module (`internal/blackmarket`) is a clean-room reconstruction of the underground black market barter system matching the original Party2 Perl CGI specification (`party2/lib/black_market.cgi`), located at `闇市場` and operated by NPC `@闇商人`.

In authentic Party2, the Black Market is not a gold shop. It operates purely as a barter exchange where characters sacrifice rare items in exchange for Rare Points and U-Rare Points, and redeem those points for exclusive equipment and artifacts that are sent directly to the character's Depot (`預かり所`).

---

## Domain Rules & Systems

### 1. Eligibility & Access Control

- **Discovery requirement**: Legacy `闇市場` is reachable only through the Oracle's hidden `＠やみいちば` action, which returns without moving the character below `job_lv` 15 (`lib/goods.cgi:51–56`). `CheckEligibility` applies the same threshold to scene entry, town discovery, the two commands and the retained NPC operations; rejected operations return `ErrAccessDenied` without moving assets.
- **Authentication**: All operations require character ownership authentication.

---

### 2. Rare Point & U-Rare Point Sacrifice System (`SacrificeItem` / `@ささげる`)

- **Dual-Source Sacrifice**: Characters can sacrifice eligible rare weapons, armor, or items directly from either active character inventory or Depot (`預かり所`).
- **Sacrifice Eligibility & Yields**:
  - **46 Authentic Eligible Items**: Defined across regular weapons, armors, accessories, and consumables.
  - **Regular Rare Items**: Yields **+1 Rare Point** (e.g. `weapon-29`〜`weapon-40`, `armor-35`〜`armor-40`, `item-028`〜`item-109`).
  - **Ultra-Rare Artifacts**: Yields **+1 to +50 U-Rare Points** (e.g. `item-263`〜`item-268`).
  - **Ineligible Items**: Non-rare items are rejected (`ErrNotSacrificeEligible` / HTTP 400).
- **Transaction & Locking**:
  - Pessimistic lock ordering: Tier 2 `characters` -> Tier 3 `inventory_items` -> Tier 5 `character_depots` -> Tier 8 `blackmarket_character_points`.
  - Atomically consumes the item from inventory or depot and credits points to `blackmarket_character_points`.

---

### 3. Exclusive Prize Trade Exchange (`TradePrize` / `@とりひき`)

- **Prize Redemption**: Accumulated points can be exchanged for exclusive weapons, armor, accessories, and consumables across two catalogs:
  - **12 Regular Rare Prizes**:
    - `bm_prize_087` (時の砂, `item-087`): 1 Rare Point
    - `bm_prize_057` (気合の霊薬, `item-057`): 4 Rare Points
    - `bm_prize_060` (シルバーオーブ, `item-060`): 2 Rare Points
    - `bm_prize_061` (レッドオーブ, `item-061`): 2 Rare Points
    - `bm_prize_062` (ブルーオーブ, `item-062`): 2 Rare Points
    - `bm_prize_063` (グリーンオーブ, `item-063`): 2 Rare Points
    - `bm_prize_064` (イエローオーブ, `item-064`): 2 Rare Points
    - `bm_prize_065` (パープルオーブ, `item-065`): 2 Rare Points
    - `bm_prize_237` (歴戦の褒章, `item-237`): 2 Rare Points
    - `bm_prize_216` (モノリス, `item-216`): 3 Rare Points
    - `bm_prize_243` (陽炎の追憶記, `item-243`): 3 Rare Points
    - `bm_prize_207` (未来のカケラ, `item-207`): 10 Rare Points
  - **12 Ultra-Rare Prizes**:
    - `bm_uprize_059` (天馬のたづな, `item-059`): 5 U-Rare Points
    - `bm_uprize_199` (太古の財宝, `item-199`): 5 U-Rare Points
    - `bm_uprize_246` (クリスタルミラー, `item-246`): 5 U-Rare Points
    - `bm_uprize_185` (封光鏡, `item-185`): 10 U-Rare Points
    - `bm_uprize_194` (廻光鏡, `item-194`): 10 U-Rare Points
    - `bm_uprize_195` (幻光鏡, `item-195`): 10 U-Rare Points
    - `bm_uprize_258` (異封の箱, `item-258`): 10 U-Rare Points
    - `bm_uprize_259` (異封の鍵, `item-259`): 10 U-Rare Points
    - `bm_uprize_255` (妖しい宝珠, `item-255`): 15 U-Rare Points
    - `bm_uprize_210` (智浄衣, `item-210`): 15 U-Rare Points
    - `bm_uprize_204` (無心慧, `item-204`): 15 U-Rare Points
    - `bm_uprize_262` (黒い天馬のたづな, `item-262`): 20 U-Rare Points
- **Depot Delivery & Capacity Verification**:
  - Prizes are delivered directly into the character's Depot (`預かり所`), NOT character inventory.
  - Depot capacity is verified via `depot.CalculateCapacity(char.JobLevel, dep.ExDepot, char.OverDepot)`. If depot capacity is exceeded, the trade is rejected with `ErrDepotFull`.
  - Transaction lock sequence: Tier 2 `characters` -> Tier 5 `character_depots` -> Tier 8 `blackmarket_character_points`.

---

### 4. Authentic Underworld NPC `@闇商人` Dialogue

- **Presentation Decoupling**: In adherence to `.agents/rules/03-architecture.md §5`, domain service results return pure structured data (`CharacterID`, `NPCName`), while human-facing dialogue lines are formatted at the HTTP transport/presentation layer (`internal/api/http`).
- **Atmospheric Talk (`Talk`)**: Formats authentic atmospheric underworld quotes matching the legacy CGI:
  - `"よく来たな…。ここは闇市場だ…"`
  - `"表の世界では手に入れられない物を取引している…"`
  - `"物の取引は金では買えないもの…。つまり、魂…ゴホッゴホッ…ではなく、レアアイテムだ…"`
  - `"お前の魂…ではなく、お前が装備しているレアアイテムをささげろ…"`
  - `"レアアイテムをささげることによって…お前のレアポイントが増える…"`
  - `"レアポイントにより取引できるアイテムが違う…"`
- **Inspection (`Inspect`)**: Formats legacy CGI inspection quote:
  - `"…お前の魂で取引したいのか？"`

---

## 5. Gateway and Retained Routes

Selecting `blackmarket` exposes owned Rare/U-Rare points, the visible prizes and a paged list of eligible sacrifice candidates through character context. Observation never sacrifices, trades, chooses dialogue or creates a Depot. A required read failure fails the whole observation, and a qualification loss shows `selection_unavailable` with a safe back. See the [context and recovery contract](../architecture/client-agent-api.md).

| Former or retained REST operation | Supported replacement / disposition |
| --- | --- |
| GET `/characters/{id}/blackmarket`, GET `.../blackmarket/points` (retired) | POST `scene_enter` with `destination:blackmarket`, then GET `/api/v1/characters/{id}/context` |
| POST `.../blackmarket/sacrifice` (retired) | POST `/api/v1/characters/{id}/actions`, `action:blackmarket_sacrifice`, explicit `params.item_instance_id` |
| POST `.../blackmarket/trade` (retired) | Same Gateway, `action:blackmarket_trade`, explicit `params.prize_id`; the result keeps the delivered Depot instance and remaining points |
| POST `.../blackmarket/talk`, POST `.../blackmarket/inspect` (retired) | Same Gateway, `action:npc_talk` / `action:npc_inspect` in the Black Market scene; both require the scene's qualification |

### Scoped legacy action reconciliation

| Legacy action (`lib/black_market.cgi`) | Go disposition | Known difference |
| --- | --- | --- |
| `header_html` (97–106) | `rare_points`, `u_rare_points` | Equipped names are not repeated in the scene. |
| `とりひき` without a target (132–152) | `prizes`, `u_prizes`; `u_prizes` is empty while U-Rare points are zero | None for listing visibility. |
| `とりひき` with a target (116–125, 135–144) | `blackmarket_trade` | Explicit prize ID replaces the name. A U-Rare trade at zero points rejects instead of redisplaying the list; neither moves assets. Full-Depot delivery remains #1139's specification. |
| `ささげる` without a target (168–172, 200–250, 310) | `sacrifice_candidates` | Lists eligible instances only, inventory then Depot; legacy lists every owned item and rejects common ones when offered. The empty-handed message is an empty array. |
| `ささげる` with a target (179–300) | `blackmarket_sacrifice` | Explicit instance ID replaces legacy's Depot-first name match. Sacrificing an equipped item is still blocked by #1140, and the candidate list does not mark equipped rows. |
| Shared `はなす` (`lib/system.cgi:240–246`), `shiraberu_npc` (81–83) | `npc_talk`, `npc_inspect` | None: talk logs the word and starts the wait; inspect is private. |

Legacy successful actions also apply the shared action wait and write `npc_com` to the facility log, while list views set `$act_time = 0`. The Gateway now applies the [action wait](../architecture/place-log.md#action-wait-1939); writing these commands' log rows remains [#1941](https://github.com/witchcraze/party2re/issues/1941), and presence remains open.

---

## Persistence Architecture

### MariaDB Schemas

SQL definitions are owned by [migrations](../../migrations/).
