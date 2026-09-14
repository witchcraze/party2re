# Blacksmith (鍛冶屋): Weapon Seals, Naming, and Dedicated Storage

## Overview

The Blacksmith (鍛冶屋, `party2/lib/blacksmith.cgi`) provides equipment customization and dedicated weapon management:
1. **Weapon Seals (こくいん)**: Engraves elemental and tactical seals onto equipped weapons in exchange for crystals (`刻印晶`).
2. **Equipment Naming (なづける)**: Customizes the display name of equipped weapons and armor.
3. **Dedicated Weapon Storage (専用預かり所)**: Stores up to 3 customized weapons with their seals and names preserved.

---

## 1. Weapon Seals (こくいん)

### Mechanics
- Seals can only be engraved on a weapon currently equipped in the main hand slot (`SlotMainHand`).
- Applying a seal consumes crystals (`刻印晶`, `character.crystal`), capped at 999,999.
- A weapon can only hold **one** seal at a time; weapons already sealed cannot receive another seal.

### Authentic 12 Seals Specification (`_data.cgi:2209-2230`, `_battle.cgi:39-44`)

| ID | Name | Combat Effect | Crystal Cost |
|---|---|---|---|
| 1 | 爪の刻印 | 攻撃力 +10 | 50 |
| 2 | 牙の刻印 | 攻撃力 +30, 素早さ -20 | 500 |
| 3 | 竜の刻印 | 攻撃力 +20% (武器攻撃力), 素早さ -30 | 5,000 |
| 4 | 羽の刻印 | 素早さ +10 | 50 |
| 5 | 翼の刻印 | 攻撃力 -10, 素早さ +30 | 500 |
| 6 | 鳳の刻印 | 攻撃力 -20, 素早さ +1〜50 (ランダム) | 5,000 |
| 7 | 炎の刻印 | スキル「しゃくねつ」付与 (MP 40, 威力 180, 火属性, 敵全体) | 100 |
| 8 | 氷の刻印 | スキル「マヒャド」付与 (MP 27, 威力 160, 水属性, 敵全体) | 100 |
| 9 | 雷の刻印 | スキル「ギガデイン」付与 (MP 40, 威力 180, 光属性, 敵全体) | 100 |
| 10 | 神速の刻印 | アビリティ「seal_shinsoku」付与 (通常攻撃時に同ターン内で2回連続攻撃) | 100 |
| 11 | 空の刻印 | アビリティ「seal_kuu」付与 (攻撃命中時 12.5% の確率で対象の全ステータス強化/防御状態を解除) | 100 |
| 12 | 理の刻印 | アビリティ「seal_kotowari」付与 (MP 3 消費、通常攻撃を魔法属性化かつダメージ 80% で実行) | 100 |

### Eligibility Validation (`_can_add_wea_seals`)
Seals cannot be applied if:
- No weapon is equipped in `SlotMainHand`.
- The equipped item is bare hands (`素手`).
- The item definition type is throw (`t`) or poison/trap (`p`).
- The weapon already possesses a seal (`wea_seal != 0`).

### Crystal Drops & Consumable Removal (`_battle.cgi:145-150`, `_item.cgi:140-155`)
- **Monster Crystal Drops**: When an enemy monster is defeated in combat (via direct damage, poison DOT, or job skill like Dejon), a crystal drop is checked:
  - If the enemy is strong (`HP > 10000 || (Atk > 1500 && Def > 1500)`): 4% chance (`rand(50) < 2`).
  - Otherwise: 2% chance (`rand(50) < 1`).
  - Dropped crystals are awarded to participating characters upon victory (`char.crystal += drops`, capped at 999,999).
- **Item 257 (水晶の原石)**:
  - Consumable item usable out-of-combat from Home.
  - If the character has a weapon seal (`char.wea_seal > 0`), the seal is stripped (`char.wea_seal = 0`), and 50% of the crystal cost is refunded to `char.crystal` (capped at 999,999).
  - If no seal is engraved, the item is consumed with message `「しかし、何も起こらなかった…」`.

---

## 2. Equipment Naming (なづける)

### Mechanics
- Allows players to assign a personalized name to their equipped weapon or armor.
- Submitting an empty name resets the custom name to the item's original catalog name.
- Custom names are stored on the character record (`wea_name`, `arm_name`) and preserved across weapon deposits/withdrawals.

### Sanitization & Validation Rules
- **Maximum Length**: 20 runes / Unicode characters.
- **Whitespace Prohibition**: No half-width spaces (` `) or full-width spaces (`　`).
- **Forbidden Characters**: `,`, `;`, `"`, `'`, `&`, `<`, `>` (prevents injection, CSV corruption, and XSS).
- **At-Symbol Prohibition**: `@` and full-width `＠` (prevents delimiter collision with legacy serializations).

---

## 3. Dedicated Weapon Storage (専用預かり所)

### Mechanics
- Dedicated holding area exclusively for weapons (`party2/lib/blacksmith.cgi`).
- **Capacity**: Strictly 3 slots (1, 2, 3).
- Preserves the item's `item_definition_id`, `wea_seal`, and `wea_name`.

### Operations & Invariants
- **Deposit (`あずける`)**:
  - Requires a weapon equipped in `SlotMainHand`.
  - Rejects deposit if the storage already contains 3 weapons (`ErrStorageFull`).
  - Rejects deposit if a stored weapon has the identical effective name (`customName` or catalog `Name`) (`ErrDuplicateStoredName`).
  - Unequips and removes the weapon from the character's inventory, moving it to `blacksmith_deposits`.
  - Clears `wea_seal` and `wea_name` on the character record.
- **Withdraw (`ひきだす`)**:
  - Rejects withdrawal if the character currently has any weapon equipped in `SlotMainHand` (`ErrWeaponSlotOccupied`).
  - Restores the weapon into the character's inventory and automatically equips it.
  - Restores the saved `wea_seal` and `wea_name` onto the character record.
  - Deletes the record from `blacksmith_deposits`.

---

## 4. Concurrency & Row-Lock Hierarchy

All operations execute within a single database transaction conforming strictly to the project lock hierarchy:

```
Rank 2: characters (SELECT ... FOR UPDATE)
  ↓
Rank 3: equipment_slots, inventory_items (SELECT ... FOR UPDATE)
  ↓
Rank 8: blacksmith_deposits (SELECT ... FOR UPDATE)
```

Deadlock is mathematically impossible because locks are always acquired in strictly monotonic rank order.
