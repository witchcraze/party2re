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
- A weapon holds one seal. Applying another replaces it and charges the new seal's full crystal cost (`blacksmith.cgi:110–140`); there is no existing-seal rejection.

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
An equipped weapon is required. Legacy eligibility comes from each seal's weapon
ID list (`blacksmith.cgi:239–252`, `_data.cgi:2209–2223`): seals 1–6 permit all
nonzero catalog weapon IDs; seals 7–12 restrict specific weapons. Neither weapon
type nor an existing seal supplies an additional rejection.

### Crystal Drops & Consumable Removal (`_battle.cgi:145-150`, `_item.cgi:140-155`)
- **Monster Crystal Drops**: When an enemy monster is defeated in combat (via direct damage, poison DOT, or job skill like Dejon), a crystal drop is checked:
  - If the enemy is strong (`HP > 10000 || (Atk > 1500 && Def > 1500)`): 4% chance (`rand(50) < 2`).
  - Otherwise: 2% chance (`rand(50) < 1`).
  - Dropped crystals are awarded to participating characters upon victory (`char.crystal += drops`, capped at 999,999).
- **Item 257 (水晶の原石)**:
  - Consumable item usable out-of-combat from Home.
  - If the character has a weapon seal (`char.wea_seal > 0`), the seal is stripped (`char.wea_seal = 0`), and 50% of the crystal cost is refunded to `char.crystal` (capped at 999,999).
  - If no seal is engraved, **the item is NOT consumed**; only the failure message `「しかし、何も起こらなかった…」` is shown (fixed by #1121).

---

## 2. Equipment Naming (なづける)

### Mechanics
- Allows players to assign a personalized name to their equipped weapon or armor.
- The Go service accepts an explicit empty name to reset to the catalog name;
  legacy `nazukeru` instead opens the naming prompt for an empty name. This
  existing difference is retained by transport migration, not certified as parity.
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
  - Clears `wea_seal` and `wea_name` on the character record, matching legacy
    `write_user` cleanup after unequipping (`system.cgi:80–83`).
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

The composition root injects the transaction provider. Character locking
serializes customization and storage operations for the same owner; transaction
rollback must preserve equipment, inventory, storage, crystals and names together.

## 5. Gateway commands and observation

The authenticated path character owns every command. Strict parameters reject
actor overrides and missing/null required fields before service execution.
Integer inputs retain signed int64 transport bounds; seal/slot ranges, equipment,
names, storage capacity and crystal costs remain service validations. Wallet gold
is not required. Successful results retain HTTP-owned dialogue and service facts;
refresh failure preserves the known outcome and permits GET-only recovery under
the [shared Gateway contract](../architecture/client-agent-api.md#input-and-execution-boundary).

| Legacy dispatch / source | Gateway replacement and result |
| --- | --- |
| `こくいん` → `kokuin`, `blacksmith.cgi:110–140` | `blacksmith_seal` → `ApplySeal`; required `seal_id`, returns `seal` and `message` |
| `なづける` → `nazukeru`, `blacksmith.cgi:55–105` | `blacksmith_name` → `NameEquipment`; required `target` and `name`, returns canonical `target`, trimmed `name` and `message`; explicit `""` resets the name |
| `あずける` → `azukeru`, `blacksmith.cgi:144–188` | `blacksmith_deposit` → `DepositWeapon`; omitted/empty params, returns `deposit` and `message`; the equipped main-hand weapon is the target |
| `ひきだす` → `hikidasu`, `blacksmith.cgi:192–237` | `blacksmith_withdraw` → `WithdrawWeapon`; required owned `slot`, returns `slot` and `message`; slot identity replaces legacy effective-name selection |

These are the four additions to `@actions`/`%actions` at
`blacksmith.cgi:24–31`, loaded through `party.cgi:13–22`. Their former character
POSTs (`blacksmith/seal`, `blacksmith/name`, `blacksmith/storage/deposit` and
`blacksmith/storage/withdraw`) are retired. Independent ActionID schemas and
resolver links use `executeCharacterAction`.

Selecting `blacksmith` observes the owner's crystals, equipped weapon/armor with
effective names, current seal, the 12-seal catalog, seal IDs applicable to the
equipped weapon and stored weapons with effective names, under the
[selection and read-failure contract](../architecture/client-agent-api.md#actor-target-paging-and-read-failures).
These supply the facts behind the legacy header (`blacksmith.cgi:37–50`) and the
seal, naming and withdrawal prompts, whose selectable inputs are the explicit
command parameters above. Stored-weapon `slot` identifies a withdrawal. Crystal
sufficiency, duplicate names and capacity remain command-time service checks.
`GET /blacksmith/seals` and `GET /characters/{id}/blacksmith/storage` are retired;
the catalog now requires the owned context instead of an anonymous read.

Legacy `system.cgi:9–25` also registers `br`, `いどう`, `まち`, `ほーむ`, optional
`ぎるど`, `ささやき`, `はなす`, `しらべる`, `ろぐあうと` and `すくしょ`.
Layout separators and screenshots belong to presentation; session logout replaces
the logout action; navigation, Home, Guild and whisper retain their shared
migration owners (#947/#949/#950), rather than becoming equipment commands.
Legacy `はなす` (`@words`, `blacksmith.cgi:17–19`) and NPC `しらべる` map to the
shared `npc_talk` / `npc_inspect` commands.

Transport migration preserves existing service behavior, including equipped-item
naming, leading/trailing whitespace trimming and ASCII `@` rejection. Legacy
differs on these points, uses a wall-clock action cooldown and venue comment/NPC
logs (`system.cgi:639–680`), and includes weapon names in storage dialogue.
The Gateway applies the [action wait](../architecture/place-log.md#action-wait-1939);
the commands' log rows (#1941), presence and the naming differences remain open,
so there is no full gameplay-parity claim.

`WithdrawWeapon` checks only the main-hand slot and does not reject a full
inventory, matching the legacy equipped-weapon check (`blacksmith.cgi:192–237`).
Storage policy beyond this remains [#1127](https://github.com/witchcraze/party2re/issues/1127).
