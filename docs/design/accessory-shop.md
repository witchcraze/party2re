# Accessory Shop Design and Synthesis Specification

## Purpose

This document specifies the behavior, economic formulas, catalog progression, depot transfer routing, and accessory synthesis mechanics governing the town Accessory Shop (NPC: `@ミラ`), maintaining 1:1 behavioral parity with the legacy Party2 Perl CGI implementation (`lib/accessory.cgi`, `&acce`).

---

## 1. Shopkeeper NPC and Dialogue

| Shop Type | Title | NPC Name | Dialogue Style / Tone |
| :--- | :--- | :--- | :--- |
| **`accessory`** | 装飾品店 / アクセサリーショップ | ミラ (Mira) | Playful, elegant, self-confident |

### Dialogue Triggers
- **Welcome / Catalog Greeting**: `"いらっしゃい、何か見ていく？"`
- **Inspect NPC (`POST /characters/{id}/shop/accessory/inspect`)**: `"私の美しさに言葉もないのかしら？"`
- **Talk NPC (`POST /characters/{id}/shop/accessory/talk`)**: `"アクセサリーの合成もできるわよ。素材は倉庫から消費するから準備してね。"`
- **Purchase Success (to Inventory)**: `"お買い上げありがとう！大切に使ってね。"`
- **Purchase Success (to Depot)**: `"荷物がいっぱいみたいだから、倉庫に転送しておいたわよ。"`
- **Sale Success**: `"{payout} Gで買い取ったわ"`
- **Synthesis Success**: `"合成は大成功よ！倉庫に送っておいたわ。"`
- **Synthesis Failure**: `"あらら……合成に失敗しちゃったわ。素材は消えちゃったけど、また挑戦してね。"`

---

## 2. Pricing Formulas

### 2.1. Retail Purchase Multipliers
Unlike standard weapon/armor/item shops that charge 2× base catalog price, the accessory shop uses a tiered retail markup:
- **Standard Accessories**: 10× base catalog price.
  $$\text{RetailPrice} = \text{BasePrice} \times 10$$
- **High-Tier Specialty Tomes** (`item-150` 転移の魔術書, `item-151` 盗賊の心得): 1000× base catalog price.
  $$\text{RetailPrice} = \text{BasePrice} \times 1000$$

Both pricing calculations are protected against integer overflow via `math.MaxInt` bounds checks (`ErrPriceOverflow`).

### 2.2. Resale Markdown
When selling items back to the accessory shop:
$$\text{SellPrice} = \lfloor \text{BasePrice} \times 0.5 \rfloor$$

---

## 3. Catalog Progression by Job Level (`job_lv`)

The items available in Mira's shop expand as the adventurer advances their current class level (`job_lv`):

| Job Level Tier | Available Items Count | Catalog Item Definitions |
| :--- | :---: | :--- |
| **`job_lv < 50`** | 6 items | `item-013` .. `item-018` |
| **`50 <= job_lv < 100`** | 18 items | `item-013` .. `item-030` |
| **`job_lv >= 100`** | 28 items | `item-013` .. `item-038`, `item-150`, `item-151` |

Purchasing an item above the character's current tier is rejected with `ErrLevelRequirementNotMet` (HTTP 422).

---

## 4. Purchase Routing & Item Collection

1. **Direct Inventory Delivery**:
   - Purchase quantity is 1 ($\text{quantity} = 1$).
   - Character has an open slot in inventory (`inventory.FirstEmptySlot()`).
   - The purchased item instance is placed in the character's inventory.
   - If configured, item collection discovery is triggered atomically (`RecordItemDiscovered`).
   - Shopkeeper returns standard purchase dialogue.

2. **Depot Auto-Transfer**:
   - Character inventory is full OR purchase quantity is greater than 1 ($\text{quantity} > 1$).
   - Depot record is retrieved or initialized via `depot.FindOrCreate`.
   - Remaining capacity is verified (`depot.RemainingCapacity() >= quantity`). If insufficient, returns `ErrDepotFull`.
   - Items are delivered directly into the character depot.
   - Shopkeeper returns depot transfer dialogue.

---

## 5. Accessory Synthesis (`&acce`)

Mira offers accessory synthesis using materials stored in the character's **Depot**.

### 5.1. Synthesis Rules & Mechanics
1. **Material Source**: Materials are exclusively consumed from the player's depot (`character_depots`).
2. **Guarantee Mechanic (`item-180`, 合成の秘薬)**:
   - If the player holds `item-180` in their **character inventory**, it is consumed during the attempt.
   - Consuming `item-180` guarantees a **100% success rate**, bypassing the recipe's base rate.
   - Only 1 `item-180` is consumed per synthesis operation.
3. **Probability Roll**:
   - If `item-180` is not present in inventory, a deterministic PRNG roll (`rand.Intn(100) + 1`) is evaluated against `recipe.SuccessRate`.
   - Success condition: $\text{roll} \le \text{SuccessRate}$.
4. **Material Consumption**:
   - Required quantities for Material 1 and Material 2 are deducted from depot.
   - On **failure**, materials are permanently lost.
5. **Product Delivery**:
   - On **success**, the synthesized item instance is placed directly into the character's depot.

### 5.2. Lock Ordering (Rule 05)
To prevent deadlocks during synthesis spanning inventory and depot, operations adhere strictly to the system lock rank hierarchy:
1. **Character Rank 2**: `characters` row lock.
2. **Inventory Rank 3**: `inventory_items` lock (searches and consumes `item-180` if present).
3. **Depot Rank 5**: `depot_items` lock (verifies, consumes materials, and inserts finished product).

---

## 6. The 48 Canonical Synthesis Recipes

All 48 recipes adhere to the Clean-Room naming requirements (Rule 00), substituting proprietary legacy names with generic medieval-fantasy clean-room identifiers.

| # | Recipe ID | Target ID | Clean-Room Product | Material 1 | Material 2 | Rate |
| :-: | :--- | :--- | :--- | :--- | :--- | :-: |
| 1 | `recipe-01` | `item-101` | 魔よけの鈴 (Warding Bell) | `item-013` 力の指輪 ×1 | `item-014` 守りの指輪 ×1 | 70% |
| 2 | `recipe-02` | `item-102` | 疾風の腕輪 (Gale Bracer) | `item-015` 素早さの指輪 ×1 | `item-016` 幸運の指輪 ×1 | 70% |
| 3 | `recipe-03` | `item-103` | 炎のリング (Flame Ring) | `item-017` 知力の指輪 ×1 | `item-018` 精神の指輪 ×1 | 70% |
| 4 | `recipe-04` | `item-104` | 吹雪のリング (Blizzard Ring) | `item-019` 闘志の腕輪 ×1 | `item-020` 鉄壁の腕輪 ×1 | 70% |
| 5 | `recipe-05` | `item-105` | 雷神のリング (Thunder Ring) | `item-021` 迅速の腕輪 ×1 | `item-022` 豪運の腕輪 ×1 | 70% |
| 6 | `recipe-06` | `item-106` | 聖なる首飾り (Holy Necklace) | `item-023` 英知の腕輪 ×1 | `item-024` 敬虔の腕輪 ×1 | 70% |
| 7 | `recipe-07` | `item-107` | 邪心の首飾り (Cursed Pendant) | `item-025` 鬼神の指輪 ×1 | `item-026` 守護神の指輪 ×1 | 65% |
| 8 | `recipe-08` | `item-108` | 天使の首飾り (Angel Pendant) | `item-027` 神速の指輪 ×1 | `item-028` 天運の指輪 ×1 | 65% |
| 9 | `recipe-09` | `item-109` | 魔導士の首飾り (Mage Pendant) | `item-029` 賢哲の指輪 ×1 | `item-030` 聖者の指輪 ×1 | 65% |
| 10 | `recipe-10` | `item-110` | 破邪の首飾り (Purity Charm) | `item-031` 覇王の首飾り ×1 | `item-032` 聖域の首飾り ×1 | 60% |
| 11 | `recipe-11` | `item-111` | 精霊のペンダント (Spirit Pendant) | `item-033` 飛燕の首飾り ×1 | `item-034` 奇跡の首飾り ×1 | 60% |
| 12 | `recipe-12` | `item-112` | 闘神のペンダント (War God Pendant) | `item-035` 全知の首飾り ×1 | `item-036` 大樹の首飾り ×1 | 60% |
| 13 | `recipe-13` | `item-113` | 竜の牙 (Dragon Fang) | `item-037` 混沌の首飾り ×1 | `item-038` 創世の首飾り ×1 | 50% |
| 14 | `recipe-14` | `item-114` | 守護のオーブ (Orb of Warding) | `item-101` 魔よけの鈴 ×1 | `item-102` 疾風の腕輪 ×1 | 60% |
| 15 | `recipe-15` | `item-115` | 神秘のオーブ (Mystic Orb) | `item-103` 炎のリング ×1 | `item-104` 吹雪のリング ×1 | 60% |
| 16 | `recipe-16` | `item-116` | 叡智の宝珠 (Orb of Wisdom) | `item-105` 雷神のリング ×1 | `item-106` 聖なる首飾り ×1 | 60% |
| 17 | `recipe-17` | `item-117` | 覇道の宝珠 (Orb of Dominance) | `item-107` 邪心の首飾り ×1 | `item-108` 天使の首飾り ×1 | 55% |
| 18 | `recipe-18` | `item-118` | 創世の宝珠 (Orb of Genesis) | `item-109` 魔導士の首飾り ×1 | `item-110` 破邪の首飾り ×1 | 55% |
| 19 | `recipe-19` | `item-119` | 万物の宝珠 (Orb of All Creation) | `item-111` 精霊のペンダント ×1 | `item-112` 闘神のペンダント ×1 | 50% |
| 20 | `recipe-20` | `item-120` | 神竜の宝珠 (Divine Dragon Orb) | `item-113` 竜の牙 ×1 | `item-119` 万物の宝珠 ×1 | 40% |
| 21 | `recipe-21` | `item-121` | 不死鳥の羽 (Phoenix Feather) | `item-025` 鬼神の指輪 ×1 | `item-037` 混沌の首飾り ×1 | 45% |
| 22 | `recipe-22` | `item-122` | 幻影の羽 (Phantom Feather) | `item-026` 守護神の指輪 ×1 | `item-038` 創世の首飾り ×1 | 45% |
| 23 | `recipe-23` | `item-123` | 運命の輪 (Wheel of Fate) | `item-027` 神速の指輪 ×1 | `item-033` 飛燕の首飾り ×1 | 45% |
| 24 | `recipe-24` | `item-124` | 封魔の札 (Magic Ward Talisman) | `item-028` 天運の指輪 ×1 | `item-034` 奇跡の首飾り ×1 | 45% |
| 25 | `recipe-25` | `item-125` | 賢者の紋章 (Sage Crest) | `item-029` 賢哲の指輪 ×1 | `item-035` 全知の首飾り ×1 | 45% |
| 26 | `recipe-26` | `item-126` | 救済の紋章 (Salvation Crest) | `item-030` 聖者の指輪 ×1 | `item-036` 大樹の首飾り ×1 | 45% |
| 27 | `recipe-27` | `item-127` | 破滅の烙印 (Brand of Ruin) | `item-031` 覇王の首飾り ×1 | `item-037` 混沌の首飾り ×1 | 40% |
| 28 | `recipe-28` | `item-128` | 悠久の護符 (Eternal Charm) | `item-032` 聖域の首飾り ×1 | `item-038` 創世の首飾り ×1 | 40% |
| 29 | `recipe-29` | `item-129` | 冥界の護符 (Underworld Charm) | `item-114` 守護のオーブ ×1 | `item-115` 神秘のオーブ ×1 | 40% |
| 30 | `recipe-30` | `item-130` | 星霊の護符 (Astral Charm) | `item-116` 叡智の宝珠 ×1 | `item-117` 覇道の宝珠 ×1 | 40% |
| 31 | `recipe-31` | `item-131` | 聖遺物の欠片 (Relic Shard) | `item-118` 創世の宝珠 ×1 | `item-119` 万物の宝珠 ×1 | 35% |
| 32 | `recipe-32` | `item-132` | 深淵の魔石 (Abyssal Stone) | `item-120` 神竜の宝珠 ×1 | `item-121` 不死鳥の羽 ×1 | 30% |
| 33 | `recipe-33` | `item-133` | 灼熱の魔石 (Blazing Stone) | `item-122` 幻影の羽 ×1 | `item-123` 運命の輪 ×1 | 35% |
| 34 | `recipe-34` | `item-134` | 氷獄の魔石 (Glacial Stone) | `item-124` 封魔の札 ×1 | `item-125` 賢者の紋章 ×1 | 35% |
| 35 | `recipe-35` | `item-135` | 雷叫の魔石 (Thunderclap Stone) | `item-126` 救済の紋章 ×1 | `item-127` 破滅の烙印 ×1 | 35% |
| 36 | `recipe-36` | `item-136` | 虚空の魔石 (Void Stone) | `item-128` 悠久の護符 ×1 | `item-129` 冥界の護符 ×1 | 30% |
| 37 | `recipe-37` | `item-137` | 光輝の魔石 (Radiant Stone) | `item-130` 星霊の護符 ×1 | `item-131` 聖遺物の欠片 ×1 | 30% |
| 38 | `recipe-38` | `item-138` | 暗黒の魔石 (Dark Stone) | `item-132` 深淵の魔石 ×1 | `item-133` 灼熱の魔石 ×1 | 25% |
| 39 | `recipe-39` | `item-139` | 天幻の結晶 (Illusion Crystal) | `item-134` 氷獄の魔石 ×1 | `item-135` 雷叫の魔石 ×1 | 25% |
| 40 | `recipe-40` | `item-140` | 終焉の結晶 (Omega Crystal) | `item-136` 虚空の魔石 ×1 | `item-137` 光輝の魔石 ×1 | 20% |
| 41 | `recipe-41` | `item-141` | 創世神の雫 (Genesis Dew) | `item-138` 暗黒の魔石 ×1 | `item-139` 天幻の結晶 ×1 | 15% |
| 42 | `recipe-42` | `weapon-71` | 聖剣カリバーン (Caliburn) | `weapon-40` 流銀の剣 ×1 | `item-140` 終焉の結晶 ×1 | 20% |
| 43 | `recipe-43` | `armor-55` | 聖鎧アイギス (Aegis Armor) | `armor-40` 流銀の鎧 ×1 | `item-140` 終焉の結晶 ×1 | 20% |
| 44 | `recipe-44` | `armor-51` | 巨人の鎧 (Giant Armor) | `armor-30` 巨人の籠手 ×1 | `armor-35` 巨人の兜 ×1 | 30% |
| 45 | `recipe-45` | `item-170` | 中回復の魔導書 (Mid-Heal Tome) | `item-160` 初級回復書 ×2 | `item-004` 魔力水 ×2 | 50% |
| 46 | `recipe-46` | `item-171` | 完全回復の魔導書 (Full-Heal Tome) | `item-170` 中回復の魔導書 ×2 | `item-005` 賢者の秘薬 ×2 | 30% |
| 47 | `recipe-47` | `item-180` | 合成の秘薬 (Elixir of Synthesis) | `item-141` 創世神の雫 ×1 | `item-120` 神竜の宝珠 ×1 | 10% |
| 48 | `recipe-48` | `item-150` | 転移の魔術書 (Teleport Codex) | `item-102` 疾風の腕輪 ×2 | `item-123` 運命の輪 ×2 | 25% |

---

## 7. HTTP API Reference

All requests and responses use the standard `SuccessResponse[T]` wrapper.

- `GET /characters/{id}/shop/accessory` — Catalog listing filtered by character's `job_lv` (Tiers 0, 50, 100).
- `POST /characters/{id}/shop/accessory/talk` — Mira's advice and guidance.
- `POST /characters/{id}/shop/accessory/inspect` — Inspect reaction dialogue.
- `POST /characters/{id}/shop/accessory/buy` — Buy accessory with 10× or 1000× markup; routes to inventory or depot.
- `POST /characters/{id}/shop/accessory/sell` — Sell items at 50% markdown.
- `POST /characters/{id}/shop/accessory/synthesize` — Execute synthesis consuming materials from depot (auto-consumes `item-180` if held in inventory).
- `GET /characters/{id}/shop/accessory/recipes` — List all 48 synthesis recipes.
