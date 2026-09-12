# Plantation Seed Cultivation & Depot Delivery Design

## Overview

The Plantation facility (種菜園, NPC @ロータス) reproduces legacy `lib/plantation.cgi` seed cultivation, overnight crop maturation, and direct Depot harvest delivery.
Unlike modern farming mini-games with tile-grids or watering schedules, authentic Party2 cultivation adheres strictly to the original specification:
- **6 Seed Varieties**: Red, Blue, Yellow, Green, Silver, and Gold seeds purchased with Gold directly at the facility.
- **14 Unique Fertilizer Reagents**: 5 purchased with Gold (こめぬか, あぶらかす, 骨粉, 石灰, 化学肥料) and 9 consumed from Depot storage (or active Inventory) (世界樹のしずく, ギザールの野菜, クポの実, ギャンブルハート, 魔法の粉, パデキアの根っこ, 満月草, 馬のフン, 極上肥料).
- **Overnight Maturation**: Crops mature at the next midnight in Japan Standard Time (`timer.NextMidnightJST`).
- **Wither Failure & Yield Multipliers**: Each fertilizer defines a wither probability (`0%` to `40%`), a high-quality probability bonus (`0%` to `60%`), and extra yield bonus (`0` to `10` extra items).
- **Depot-Direct Delivery**: Harvested herbs, seeds, and stat nuts are deposited directly into the player's Depot (`character_depots`, `depot_items`).

---

## Domain Model

### Seed Catalog (`@seeds`)

| ID | Name | Price | High Base | High Rand | High Quality Items | Low Base | Low Rand | Low Quality Items | High Rate |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `red` | 赤の種 | 50 G | 16 | 5 | 命の木の実, 不思議な木の実, 力の種, 守りの種, 素早さの種 (item-016..020) | 2 | 2 | 上薬草, 特薬草 (item-002..003) | 25% |
| `blue` | 青の種 | 50 G | 18 | 5 | 力の種, 守りの種, 素早さの種, スキルの種, 幸せの種 (item-018..022) | 2 | 2 | 上薬草, 特薬草 (item-002..003) | 15% |
| `yellow` | 黄の種 | 80 G | 16 | 2 | 命の木の実, 不思議な木の実 (item-016..017) | 1 | 3 | 薬草, 上薬草, 特薬草 (item-001..003) | 33% |
| `green` | 緑の種 | 80 G | 18 | 3 | 力の種, 守りの種, 素早さの種 (item-018..020) | 1 | 3 | 薬草, 上薬草, 特薬草 (item-001..003) | 33% |
| `silver` | 銀の種 | 500 G | 18 | 5 | 力の種, 守りの種, 素早さの種, スキルの種, 幸せの種 (item-018..022) | 1 | 0 | 薬草 (item-001) | 20% |
| `gold` | 金の種 | 500 G | 21 | 2 | スキルの種, 幸せの種 (item-021..022) | 1 | 0 | 薬草 (item-001) | 18% |

### Fertilizer Catalog (`@ferts`)

| ID | Name | Cost / Source | Item ID | High Prob Bonus | Wither Rate | Yield Bonus | Consumption Priority |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `none` | なし (Unfertilized) | 0 G | - | +0% | 10% | 0 | Default |
| `rice_bran` | こめぬか | 150 G | - | +5% | 8% | 0 | Gold |
| `oil_cake` | あぶらかす | 150 G | - | +3% | 2% | 0 | Gold |
| `bone_meal` | 骨粉 | 200 G | - | +10% | 10% | +1 | Gold |
| `lime` | 石灰 | 200 G | - | +20% | 25% | +1 | Gold |
| `chemical` | 化学肥料 | 500 G | - | +30% | 35% | +2 | Gold |
| `yggdrasil_dew` | 世界樹のしずく | Item | `item-005` | +30% | 10% | +5 | Depot -> Inventory |
| `gysahl_greens` | ギザールの野菜 | Item | `item-037` | +35% | 20% | +7 | Depot -> Inventory |
| `kupo_nut` | クポの実 | Item | `item-038` | +50% | 8% | +3 | Depot -> Inventory |
| `gambler_heart` | ギャンブルハート | Item | `item-039` | +50% | 40% | +10 | Depot -> Inventory |
| `magic_powder` | 魔法の粉 | Item | `item-081` | +28% | 0% | +2 | Depot -> Inventory |
| `padekia_root` | パデキアの根っこ | Item | `item-010` | +10% | 25% | +5 | Depot -> Inventory |
| `full_moon_herb` | 満月草 | Item | `item-008` | +15% | 8% | +2 | Depot -> Inventory |
| `horse_dung` | 馬のフン | Item | `item-130` | +25% | 5% | +3 | Depot -> Inventory |
| `superb` | 極上肥料 | Item | `item-182` | +60% | 3% | +4 | Depot -> Inventory |

---

## Operations & Invariants

### 1. Sow (`POST /characters/{id}/plantation/sow`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 8 `plantation_plots`.
2. **Preconditions**:
   - Character must not have an active plot (`plantation_plots` empty for character).
   - Character must possess sufficient gold (`char.Money >= seed.Price`).
3. **Execution**:
   - Deduct seed price from character wallet (`char.DeductMoney`).
   - Create plot record with `seed_id`, `fertilizer_id = NULL`, `sown_at = now`, `matures_at = timer.NextMidnightJST(now)`.
   - Return confirmation message (e.g. `"赤の種をまいたよ！"`).

### 2. Fertilize (`POST /characters/{id}/plantation/fertilize`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots` -> Rank 8 `plantation_plots`.
2. **Preconditions**:
   - Character must have an active plot.
   - Plot must not already have a fertilizer applied (`plot.FertilizerID == nil`).
   - If gold fertilizer: character has sufficient funds.
   - If item fertilizer: character has the item in Depot or active Inventory.
3. **Execution**:
   - If gold fertilizer: deduct gold from character wallet.
   - If item fertilizer: consume 1 unit from Depot if present, otherwise consume 1 unit from Inventory.
   - Update plot with `fertilizer_id = fert.ID`.
   - Return confirmation message (e.g. `"化学肥料をまくよ！"`).

### 3. Harvest (`POST /characters/{id}/plantation/harvest`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 5 `character_depots` -> Rank 8 `plantation_plots`.
2. **Preconditions**:
   - Character must have an active plot.
   - Plot must be matured (`now >= plot.MaturesAt`).
3. **Execution**:
   - Roll wither chance: `rand(100) < fert.WitherRate`.
     - If withered: delete plot and return message `"〜は芽が出なかったよ…"`.
   - Roll total yield count: `1 + int(rand(fert.YieldBonus + 1))`.
   - For each yielded item:
     - Roll high quality: `rand(100) < (seed.HighRate + fert.ProbBonus)`.
     - If high: item index = `seed.HighBase + rand(seed.HighRand)`.
     - If low: item index = `seed.LowBase + rand(seed.LowRand)`.
     - Map index to `item-%03d`.
   - Add all yielded item instances to character Depot (`depot.AddItem`). If Depot is full, return `depot.ErrDepotFull` and preserve plot.
   - Delete plot from `plantation_plots`.
   - Return harvest summary and message `"収穫したよ！<br>...倉庫に送っておいたよ"`.
