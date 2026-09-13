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

### Authentic 12 Seals Specification (`_data.cgi:2209-2230`)

| ID | Name | Description | Crystal Cost |
|---|---|---|---|
| 1 | 赤の刻印 | 攻撃力上昇小 | 100 |
| 2 | 青の刻印 | 防御力上昇小 | 100 |
| 3 | 緑の刻印 | 技＋１ | 200 |
| 4 | 黒の刻印 | 会心の一撃率上昇中 | 300 |
| 5 | 白の刻印 | 命中率上昇中 | 300 |
| 6 | 闇の刻印 | 攻撃力上昇中 | 400 |
| 7 | 魔の刻印 | 魔法攻撃力上昇 | 500 |
| 8 | 風の刻印 | 回避率上昇中 | 500 |
| 9 | 妖の刻印 | 魔法防御力上昇 | 600 |
| 10 | 雷の刻印 | 攻撃回数＋１ | 700 |
| 11 | 獣の刻印 | 攻撃力上昇特大 | 1,000 |
| 12 | 覇の刻印 | 全ての能力が上昇 | 1,500 |

### Eligibility Validation (`_can_add_wea_seals`)
Seals cannot be applied if:
- No weapon is equipped in `SlotMainHand`.
- The equipped item is bare hands (`素手`).
- The item definition type is throw (`t`) or poison/trap (`p`).
- The weapon already possesses a seal (`wea_seal != 0`).

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
