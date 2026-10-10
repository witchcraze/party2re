# Character Item Depot & Storage Design

## Overview

The Item Depot (預かり所 / 倉庫) provides characters with persistent storage for item instances outside of their active inventory. The reconstruction models dynamic capacity, storage expansion, item sorting, direct item selling, inter-character mailing and collection discovery from `system.cgi:get_depot_c` and `depot.cgi`. Known withdrawal and persisted-order differences remain documented below. Gold is kept in character purses and the bank.

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

### Dynamic Capacity Recalculation (`RefreshCapacity`)

Because `character_depots.capacity` is initialized at character creation and character job levels advance dynamically over time, commerce and delivery modules (`shop`, `secretshop`, `blackmarket`, `fleamarket`, `auction`, `store`, `god`, `altar`, `medal`, `plantation`) MUST synchronize the in-memory depot capacity using `dep.RefreshCapacity(char.JobLevel, char.OverDepot)` before performing capacity boundary checks. This guarantees that high-level characters enjoy their full dynamic depot capacity (up to 500 slots) across all trade and item receipt operations.

### Root Initialization Guarantee & Centralized Retrieval (`FindOrCreate`)

To eliminate uninitialized depot records and stale capacity errors, a two-layer defense-in-depth architecture is enforced:
1. **Root Guarantee (Character Creation Auto-Initialization)**: Whenever a new character is created and saved (`CharacterRepository.Save`), a default `character_depots` record (`capacity = 5`, `ex_depot = 0`) is automatically inserted within the same database transaction.
2. **Centralized Retrieval (`FindOrCreate`)**: Across all commerce and reward domain callers (`store`, `blackmarket`, `plantation`, `casino`, `medal`, `auction`, `alchemy`, `depot/delivery`), services retrieve depots via `depot.FindOrCreate(ctx, repo, char)`. Under pessimistic lock (`FindByCharacterIDForUpdate`), if no record exists it initializes a new `Depot` with dynamic capacity reflecting the character's `JobLevel` and `OverDepot`; if a record already exists, it refreshes the capacity via `dep.RefreshCapacity(char.JobLevel, char.OverDepot)`. This permanently eliminates uninitialized `ErrNotFound` bugs and eliminates repetitive hand-rolled boilerplate across domains.

## Operations & Invariants

### 1. Item Deposit & Withdrawal
- **Deposit Item**: Moves an item instance from character inventory to depot storage.
  - Invariant: Item exists in character inventory.
  - Invariant (Issues #452, #558, #563): Stacking is restricted to stackable items (`Definition.IsStackable() == true` / `Slot == SlotNone`) with `EnhancementLevel == 0`. If a stackable unenhanced item already exists in the depot with identical definition ID, it stacks into the existing slot without consuming an extra slot. Equipment items (weapons, armor, shields, accessories; `IsStackable() == false`) and enhanced items (`EnhancementLevel > 0`) represent discrete gear instances and must NEVER merge into a single slot even if unenhanced (+0), always occupying discrete slots ($\text{len}(Items) < Capacity$).
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
- Legacy evidence: `depot.cgi:23–46,521–542` quotes the current tier before an
  explicit affirmative purchase. Observation never purchases; the explicit
  command requests one expansion, charging the service's current tier atomically
  with incrementing the purchased count. A displayed quote is not a reservation.
  Insufficient funds or the maximum count leaves both funds and count unchanged.

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

### 6. Storage Item Consumption (`Consume`, `ConsumeOne`, `PurgeSlot`)
- Town facilities and trade actions (e.g., Home consumable item usage, Black Market rare item sacrifices, Gem Store synthesis crafting, Flea Market and Player Store listings) consume items directly from Depot storage.
- Standardized consumption method `dep.Consume(instanceID, quantity)` safely decrements `Quantity` by `quantity` when `existing.Quantity >= quantity`, returning a copy of the consumed instance with `Quantity = quantity`.
- If the remaining quantity reaches 0, the item slot is removed from depot storage.
- If `quantity <= 0` or `existing.Quantity < quantity`, returns `ErrInvalidQuantity` without mutating depot state.
- `dep.ConsumeOne(instanceID)` delegates directly to `dep.Consume(instanceID, 1)`.
- Intentional whole-slot removal (e.g., withdrawing an entire stack to inventory) must use `PurgeSlot(instanceID)` (or legacy `RemoveItem`, which delegates to `PurgeSlot`). `RemoveItem` is marked deprecated for consumption purposes to prevent accidental stack deletion.

## Atomicity, Concurrency & Lock Hierarchy

All depot transactions execute inside an explicit database transaction (`*sql.Tx`). Cross-character transfers (`SendMoney`, `SendItem`) enforce global lock hierarchy ordering:
1. **Rank 2 (`characters`)**: Both sender and recipient row locks acquired via `id.Sort2(fromID, toID)` in ascending lexicographical order to prevent deadlocks.
2. **Rank 3 (`inventory_items`)**: Sender inventory items locked.
3. **Rank 5 (`character_depots`)**: Target depot locked with `FOR UPDATE`.

## Gateway Observation and Retained Operations

The owned `depot` facility is a pageable town destination. Its typed context
projection reads the existing `GetDepot` service and returns dynamic capacity,
purchased expansion count, the next expansion's gold cost (null at the cap),
total occupied slots and a bounded page of item
instances (ID, definition ID, quantity, enhancement level). Slot count refers
to all stored instances before paging; a stack still occupies one slot.
Legacy `depot.cgi:49–99` reports owned count/capacity/expansions, and
`system.cgi:1267–1288` supplies the capacity and occupied-row rule.

Offset pages use the shared default20/max100 bounds and preserve the public
reader's order; current persistence reads instance ID ascending. They do not
promise a snapshot across concurrent changes. The adapter does not call
`SortItems` or add a separate ordering rule. Missing storage produces an empty
in-memory view with calculated capacity; GET never persists that view or creates
a depot. Required character/storage errors fail observation, and actual
activities/conflicts take priority over ordinary selection. The
[navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery)
owns input, ownership and GET-only refresh recovery behavior.

All nine Depot REST operations remain registered pending their own verified
retirement. The `depot_expand` Gateway command delegates to `Expand`; owned
context supplies the read-only quote and the shared
[command contract](../architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions)
owns explicit intent, guards and outcome recovery. Observation does not execute
NPC, delivery, collection or scheduling effects. Legacy `depot.cgi:68–81` dispatch
maps to the existing services as follows:

| Legacy routine | Existing service | Retained operation |
| --- | --- | --- |
| Listing/header, `get_depot_c` | GetDepot | GET `/characters/{id}/depot` |
| `azukeru` | DepositItem | POST `/characters/{id}/depot/deposit` |
| `hikidasu` | WithdrawItem | POST `/characters/{id}/depot/withdraw` |
| `uru` | SellItem | POST `/characters/{id}/depot/sell` |
| `matomete_uru` | SellItems | POST `/characters/{id}/depot/sell-batch` |
| `seiton` | SortItems | POST `/characters/{id}/depot/sort` |
| `okuru` | SendMoney, SendItem | POST `/characters/{id}/depot/send-money`, `/characters/{id}/depot/send-item` |
| `expansion_depot` | Expand, also through Gateway `depot_expand` | POST `/characters/{id}/depot/expand` |

This mapping records transport coverage, not full legacy parity. Legacy
`seiton` (`depot.cgi:313–332`) persists kind/item-number ordering; current
SortItems results lose that order on repository reload (#1111). The context
adapter preserves reader order so a repair belongs to that owning boundary.
Legacy `hikidasu` (`depot.cgi:208–288`) swaps held equipment/items back into
storage, while current WithdrawItem moves into inventory and rejects a full
slot; the equivalent atomic swap contract remains undecided in #1127. Equipped
item deposit/send also retains a known foreign-key failure (#1112). Shared
NPC/presence/log effects and remaining command/route migration stay under #947.
