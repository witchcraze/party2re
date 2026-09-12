# Items and Equipment Design

## Purpose

This document defines the 5-category item classification, pricing and inventory ownership models, and the 5-slot equipment system.

## Item System

### Separation of Definition and Instance
- **`ItemDefinition`**: Static catalog definition defining what an item is, its base price, and its equipment slot.
- **`ItemInstance`**: Concrete owned item entity possessing a unique ID, item definition ID, stack quantity, and enhancement level (+0 to +10).

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
- **`UsageCategoryDepotAfterAction` (`4`)**: Special processing during quest participation or depot after-action (e.g. `item-113`, `item-114`, `item-259`). Total: 3 items.

### Usage Location Validation Rules
- **Combat Action Command (`＠どうぐ`)**: Active item execution in combat (`ActionItem`) is strictly restricted to `UsageCategoryCombatOnly` (`1`). Non-combat items (seeds, medals, materials, passives; categories `0`, `2`, `3`, `4`) are rejected with `ErrCannotUseInCombat` (`item cannot be used in combat`) and are never consumed in battle.
- **Home Command (`＠ほーむ`)**: Active item consumption at Home is restricted to `UsageCategoryAnytime` (`2`) and `UsageCategoryDepotAfterAction` (`4`). Combat items (`1`) return `ErrCannotUseHere` (`%sは戦闘中でしか使えません`), and non-usable items (`0`, `3`) return `%sはここでは使えません`.

### Pricing Rules
- **Purchase Price**: Defined in the item catalog (`Price`).
- **Resale Price**: Exactly 50% of the base purchase price ($\lfloor\text{Price} / 2\rfloor$).

---

## Equipment System

### Equipment Slots
A character has 5 distinct equipment slots:
- `SlotMainHand` (`main-hand`): Main-hand weapon.
- `SlotOffHand` (`off-hand`): Off-hand shield or secondary weapon.
- `SlotBody` (`body`): Body armor.
- `SlotAccessory1` / `SlotAccessory` (`accessory`): Accessory slot 1.
- `SlotAccessory2` (`accessory`): Accessory slot 2.

### Invariants & Rules
1. **Ownership Requirement**: Only items currently in the character's active inventory can be equipped.
2. **Slot Compatibility**: An item definition's `Slot` must match the target equipment slot (`SlotNone` items like potions/materials cannot be equipped).
3. **Equip Operation**: Equipping an item into an occupied slot returns the previously equipped instance ID so it can be returned/swapped in inventory.
4. **Unequip Operation**: Removing equipment unlinks the instance from the slot and requires the slot to currently hold an item.
5. **Stackability Domain Invariants**:
   - **Stackable Definitions (`Definition.IsStackable() == true`)**: Items with `Slot == SlotNone` (consumables, craft materials) are stackable (`Quantity >= 1`).
   - **Non-Stackable Definitions (`Definition.IsStackable() == false`)**: Equipment items (`Slot != SlotNone`: weapons, armor, shields, accessories) represent discrete gear. Each equipment piece must have `Quantity == 1` and occupy its own distinct storage slot.
   - **Enhancement Level Invariant**: Any item with `EnhancementLevel > 0` must strictly have `Quantity == 1` and cannot stack with any other item instance (`CanStackWith == false`).
   - **Instance Creation & Validation Guards**: `NewInstanceWithEnhancement`, `Definition.NewInstance`, and `Definition.NewInstanceWithEnhancement` reject `Quantity > 1` for equipment or enhanced items with `ErrInvalidInstance`.

---

## Storage & Item Consumption

Inventory storage (`internal/core/inventory`) encapsulates item mutations and validates quantities:
- **`Consume(instanceID, quantity) error`**: Decrements item quantity by `quantity` or removes the slot when quantity reaches 0. Rejects `quantity <= 0` or insufficient stack with `ErrInvalidQuantity`.
- **`ConsumeItem(instanceID, quantity) (item.Instance, error)`**: Decrements item quantity and returns a copy of the consumed item instance with `Quantity = quantity`.
- **`ConsumeOne(instanceID) error`**: Convenience helper delegating directly to `Consume(instanceID, 1)`.
- **`ConsumeOneItem(instanceID) (item.Instance, error)`**: Convenience helper delegating directly to `ConsumeItem(instanceID, 1)`.
- **Interface Symmetry**: Symmetrically mirrors Depot storage consumption (`depot.Consume`, `depot.ConsumeOne`, `depot.PurgeSlot`) across storage aggregates.

