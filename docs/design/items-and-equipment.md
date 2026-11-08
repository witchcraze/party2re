# Items and Equipment Design

## Purpose

This document defines the 5-category item classification, pricing and inventory ownership models, and the current Go equipment representation. Legacy equipment uses weapon, armor, and held-item fields; additional slot categories are implementation metadata whose gameplay equivalence must be checked.

## Item System

### Separation of Definition and Instance
- **`ItemDefinition`**: Static catalog definition defining what an item is, its base price, and its equipment slot.
- **`ItemInstance`**: Concrete owned item entity possessing a unique ID, item definition ID, stack quantity, and an enhancement metadata field (+0 to +10 in current Go). The metadata does not establish a legacy gold/material enhancement mechanic; authentic weapon seals are documented in [blacksmith.md](blacksmith.md).

### 5-Category Item Catalog
Definitions are organized into five JSON data catalogs:
1. `weapons.json`: Main-hand weapons (swords, staves, axes, bows).
2. `armors.json`: Body armor (plate, robes, leather).
3. `shields.json`: Off-hand shields and defensive off-hand gear.
4. `accessories.json`: Rings, amulets, and trinkets.
5. `consumables.json`: Potions, herbs, craft materials, and quest items.

### Usage Location Categories (`UsageCategory`)
Derived from the authentic Party2 Perl CGI `@ites` table (`$ites[no][3]`), every item definition specifies its authorized usage context:
- **`UsageCategoryNone` (`0`)**: Not actively usable via menus (materials, coins, job emblems, auto-consumed quest items). Total: 43 items (including 0: なし).
- **`UsageCategoryCombatOnly` (`1`)**: Usable strictly during active battle commands (`＠どうぐ`). Total: 53 items (50 consumables like herbs/drops/waters and 3 combat gear).
- **`UsageCategoryAnytime` (`2`)**: Usable anytime / at Home (`＠ほーむ`) (seeds, medals, fight elixir, recipes). Total: 53 items.
- **`UsageCategoryCombatPassive` (`3`)**: Automatically triggers or provides passive protection in combat (charms, talismans, elemental orbs). Total: 117 items.
- **`UsageCategoryDepotAfterAction` (`4`)**: Quest equipment callbacks or deferred Home Depot callbacks (`item-113`, `item-114`, `item-259`). Total: 3 items. Home discards callback return values; category 4 does not imply a permanent stat increase. The [Home consumption contract](home.md#legacy-dispatch-and-consumption-contract) distinguishes held and Depot behavior.

### Usage Location Validation Rules
- **Combat Action Command (`＠どうぐ`)**: Active item execution in combat (`ActionItem`) is strictly restricted to `UsageCategoryCombatOnly` (`1`). Non-combat items (seeds, medals, materials, passives; categories `0`, `2`, `3`, `4`) are rejected with `ErrCannotUseInCombat` (`item cannot be used in combat`) and are never consumed in battle.
- **Home Command (`＠ほーむ`)**: Categories `2` and `4` are eligible for dispatch; Go currently implements only a whitelist. [Home](home.md#legacy-dispatch-and-consumption-contract) owns the complete reconciliation, including missing callbacks and intentional no-stat consumption. Combat items (`1`) return `ErrCannotUseHere` (`%sは戦闘中でしか使えません`), and non-usable items (`0`, `3`) return `%sはここでは使えません`.

### Pricing Rules
- **Purchase Price**: Defined in the item catalog (`Price`).
- **Resale Price**: Exactly 50% of the base purchase price ($\lfloor\text{Price} / 2\rfloor$).

---

## Equipment System

### Current Go equipment slots
The exported slot enum has four equipment categories; no `SlotAccessory1` or `SlotAccessory2` exists:
- `SlotMainHand` (`main-hand`): Main-hand weapon.
- `SlotOffHand` (`off-hand`): Off-hand shield or secondary weapon.
- `SlotBody` (`body`): Body armor.
- `SlotAccessory` (`accessory`): One accessory category. Do not invent a second accessory slot from this enum.

### Invariants & Rules
1. **Ownership Requirement**: Only items currently in the character's active inventory can be equipped.
2. **Slot Compatibility**: An item definition's `Slot` must match the target equipment slot (`SlotNone` items like potions/materials cannot be equipped).
3. **Equip Operation**: Equipping an item into an occupied slot returns the previously equipped instance ID so it can be returned/swapped in inventory.
4. **Unequip Operation**: Removing equipment unlinks the instance from the slot and requires the slot to currently hold an item.
   Depot deposit/send removes links to the moved instance before deleting its
   inventory row, within the same transaction as ownership and character
   customization changes. See [Depot movement invariants](depot.md#1-item-deposit--withdrawal)
   for metadata clearing and failure rollback; withdrawal swaps follow the
   [Depot withdrawal contract](depot.md#1-item-deposit--withdrawal).
5. **Stackability Domain Invariants**:
   - **Stackable Definitions (`Definition.IsStackable() == true`)**: Items with `Slot == SlotNone` (consumables, craft materials) are stackable (`Quantity >= 1`).
   - **Non-Stackable Definitions (`Definition.IsStackable() == false`)**: Equipment items (`Slot != SlotNone`: weapons, armor, shields, accessories) represent discrete gear. Each equipment piece must have `Quantity == 1` and occupy its own distinct storage slot.
   - **Enhancement Level Invariant**: Any item with `EnhancementLevel > 0` must strictly have `Quantity == 1` and cannot stack with any other item instance (`CanStackWith == false`).
   - **Instance Creation & Validation Guards**: `NewInstanceWithEnhancement`, `Definition.NewInstance`, and `Definition.NewInstanceWithEnhancement` reject `Quantity > 1` for equipment or enhanced items with `ErrInvalidInstance`.

### Legacy hand fields
Legacy characters hold one weapon (`$m{wea}`), one armor (`$m{arm}`) and one
held item (`$m{ite}`); entering a field is equipping it. Go represents them as
inventory rows classified by `Definition.LegacyStorageKey` kind: kind 1 weapons
(main-hand), kind 2 armor (body) and kind 3 every `item-NNN` entry, so shields
(off-hand), accessories and consumables share the single held field. This kind
differs from `Slot.Kind()`, which groups shields and accessories with armor.
[Depot withdrawal](depot.md#1-item-deposit--withdrawal) applies this mapping.
Other acquisition paths still use the single-row inventory capacity or
`Slot.Kind()` and do not equip delivered gear
([#1948](https://github.com/witchcraze/party2re/issues/1948)).

---

## Storage & Item Consumption

Inventory storage (`internal/core/inventory`) encapsulates item mutations and validates quantities:
- **`Consume(instanceID, quantity) error`**: Decrements item quantity by `quantity` or removes the slot when quantity reaches 0. Rejects `quantity <= 0` or insufficient stack with `ErrInvalidQuantity`.
- **`ConsumeItem(instanceID, quantity) (item.Instance, error)`**: Decrements item quantity and returns a copy of the consumed item instance with `Quantity = quantity`.
- **`ConsumeOne(instanceID) error`**: Convenience helper delegating directly to `Consume(instanceID, 1)`.
- **`ConsumeOneItem(instanceID) (item.Instance, error)`**: Convenience helper delegating directly to `ConsumeItem(instanceID, 1)`.
- **Interface Symmetry**: Symmetrically mirrors Depot storage consumption (`depot.Consume`, `depot.ConsumeOne`, `depot.PurgeSlot`) across storage aggregates.

