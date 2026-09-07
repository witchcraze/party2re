# Character Item Depot & Storage Design

## Overview

The Item Depot (預かり所 / 倉庫) provides characters with persistent storage for item instances outside of their active inventory. It faithfully reproduces the original Party2 Perl CGI specification (`system.cgi:get_depot_c`, `town.cgi` depot routines), featuring dynamic capacity scaling, storage expansion, item sorting, direct item selling, inter-character mailing (money and items), and collection book synchronization. Gold is kept strictly in player/character purses and the bank—fictional gold deposits in the depot are eliminated.

## Domain Model

### `Depot`
- **`CharacterID`** (`string`): Unique identifier of the character owning the depot.
- **`Capacity`** (`int`): Dynamic maximum item capacity slots (range: 5–500).
- **`ExDepot`** (`int`): Purchased depot expansion count (0–20).
- **`Items`** (`[]item.Instance`): Stored item instances (each with unique instance ID, definition ID, and quantity).

## Capacity Formula

Depot capacity dynamically reflects character progression, purchased expansions, and god limit breaks:

$$\text{Capacity} = \min(500, \text{Base}(jobLv) + (ex\_depot \times 5) + (over\_depot \times 50))$$

Where:
- **`Base(jobLv)`**:
  - If $jobLv \ge 29$: $150$ slots
  - If $jobLv > 0$: $jobLv \times 5 + 5$ slots
  - Default / minimum base: $5$ slots
  *(In Party2Re, $jobLv$ corresponds to the character's effective job level)*
- **`ex_depot`**: Number of paid depot expansions (0 to 20, granting +5 slots each, up to +100 slots).
- **`over_depot`**: Number of god limit breaks applied to depot capacity (0 to 5, granting +50 slots each, up to +250 slots).
- Absolute minimum capacity: 5 slots.
- Absolute maximum capacity: 500 slots.

## Operations & Invariants

### 1. Item Deposit & Withdrawal
- **Deposit Item**: Moves an item instance from character inventory to depot storage.
  - Invariant: Item exists in character inventory.
  - Invariant (Issue #452): If the item definition ID already exists in the depot, it stacks into the existing slot without consuming an extra slot. Only new unique definitions require $\text{len}(Items) < Capacity$.
- **Withdraw Item**: Moves an item instance from depot storage to character inventory.
  - Invariant: Item exists in depot storage.
  - Invariant: Character inventory has available space ($\text{len}(Inventory.Items) < Inventory.Capacity$).
  - Side effect: Automatically registers the item in the character's Collection book upon withdrawal if configured.

### 2. Depot Expansion (`かくちょう`)
- Characters can purchase up to 20 expansions (`MaxExDepot = 20`), each granting +5 capacity slots.
- Tiered expansion cost table:
  - Expansions 0, 1: 200,000 gold each
  - Expansions 2, 3: 400,000 gold each
  - Expansions 4, 5: 600,000 gold each
  - Expansions 6, 7: 800,000 gold each
  - Expansions 8..19: 999,999 gold each
- Invariant: `ExDepot < 20` and `Character.Money >= cost`.

### 3. Depot Item Sales (`うる` / `まとめてうる`)
- Stored items can be sold directly from the depot without withdrawing them first.
- Sale price is 50% of the item definition base price per unit: $\lfloor \text{Price} \times 0.5 \rfloor \times \text{Quantity}$.
- Supports atomic batch selling (`SellItems`) for multiple selected depot item instances.

### 4. Depot Sorting (`せいとん`)
- Re-orders items in the depot deterministically according to the legacy item kind hierarchy:
  1. **Kind 1 (Weapons)**: Items equipped in `SlotMainHand`.
  2. **Kind 2 (Armors)**: Items equipped in `SlotOffHand`, `SlotBody`, or `SlotAccessory`.
  3. **Kind 3 (Consumables & Misc)**: Items with `SlotNone`.
- Items within the same kind are ordered ascending by `DefinitionID`.

### 5. Mailing Items and Money (`おくる`)
- **Send Money (`SendMoney`)**: Transits gold directly from the sender's purse to the recipient's purse.
  - Invariant: `sender.Money >= amount` and `amount > 0` and `sender != recipient`.
- **Send Item (`SendItem`)**: Transits an item directly from sender's inventory into the recipient's depot storage.
  - Invariant: Item exists in sender inventory.
  - Invariant: Recipient depot has available capacity (respecting stack merging).

## Atomicity, Concurrency & Lock Hierarchy

All depot transactions execute inside an explicit database transaction (`*sql.Tx`). Cross-character transfers (`SendMoney`, `SendItem`) enforce global lock hierarchy ordering:
1. **Rank 2 (`characters`)**: Both sender and recipient row locks acquired via `id.Sort2(fromID, toID)` in ascending lexicographical order to prevent deadlocks.
2. **Rank 3 (`inventory_items`)**: Sender inventory items locked.
3. **Rank 5 (`character_depots`)**: Target depot locked with `FOR UPDATE`.
