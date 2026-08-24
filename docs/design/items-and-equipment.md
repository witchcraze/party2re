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
