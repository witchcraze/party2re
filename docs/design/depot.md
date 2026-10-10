# Character Item Depot & Storage Design

## Overview

The Item Depot (預かり所 / 倉庫) provides characters with persistent storage for item instances outside of their active inventory. The reconstruction models dynamic capacity, storage expansion, item sorting, direct item selling, inter-character mailing and collection discovery from `system.cgi:get_depot_c` and `depot.cgi`. Known withdrawal and sort-classification differences remain documented below. Gold is kept in character purses and the bank.

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
- `SellItem` delegates to the same batch implementation. Before removing any
  instance, every selected ID must exist exactly once in the request, its catalog
  lookup must succeed, its base price must be nonnegative and its quantity positive.
  A missing catalog provider or failed lookup aborts the sale; a successfully read
  0G definition remains sellable. Integer division computes the per-unit half-price;
  shared checked multiplication/addition reject overflow before mutation.
- Sales remove whole selected instances, including all units in a stack, and ignore
  enhancement when valuing the base price. Existing wallet saturation at `MaxMoney`
  remains in force. This stack representation is the reconstruction contract;
  legacy `depot.cgi:105–164,420–519` operates on individual stored rows.
- Missing/repeated IDs, invalid valuation and persistence failures leave the wallet
  and Depot unchanged in the production transaction. Catalog failures retain their
  original error for callers rather than becoming successful zero-price sales.

### 4. Depot Sorting (`せいとん`)
- The current Go comparator orders items deterministically using the equipment-slot kind:
  1. **Kind 1 (Weapons)**: Items equipped in `SlotMainHand`.
  2. **Kind 2 (Armors)**: Items equipped in `SlotOffHand`, `SlotBody`, or `SlotAccessory`.
  3. **Kind 3 (Consumables & Misc)**: Items with `SlotNone`.
- Items within the same kind are ordered ascending by `DefinitionID`.
- Explicit sorting persists the resulting instance order. Ordinary saves preserve
  slice order, including appended items; reads never implicitly sort by definition.
  Both normal and locked reads use stored position with instance ID as a tie-breaker.
  Rows predating the position migration retain their former ID order until saved.
- Legacy `depot.cgi:313–332` sorts stored kind and numeric item number before
  saving. Its `azukeru` (`167–207`) stores weapons as kind 1, armor as kind 2 and
  entries from the item catalog as kind 3. The current slot-based comparator groups
  shields/accessories with armor instead; exact catalog-key reconciliation remains
  #1288 work under #947. Persisting the existing comparator's output does not claim full parity.

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

Production single-character mutations use `economy.TransactionRunner`: character
locks (Rank 2), inventory locks when needed (Rank 3), then Depot root/item locks
(Rank 5). Sales lock no inventory. The runner credits the wallet after the Depot
save in the same transaction; a later character-save failure rolls both back.
Concurrent single/batch sales of an overlapping instance can settle only once.
Cross-character transfers (`SendMoney`, `SendItem`) use the injected transaction
provider and enforce global lock hierarchy ordering:
1. **Rank 2 (`characters`)**: Both sender and recipient row locks acquired via `id.Sort2(fromID, toID)` in ascending lexicographical order to prevent deadlocks.
2. **Rank 3 (`inventory_items`)**: Sender inventory items locked.
3. **Rank 5 (`character_depots`)**: Target depot locked with `FOR UPDATE`.

The [Depot navigation index](../../.arch/modules/depot.json) records the verified
boundaries, including SendItem's subsequent sender-Depot read under lock. Tests
may explicitly inject adapters without a database transaction; that configuration
does not provide production rollback guarantees.

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
reader's persisted instance order, with ID breaking position ties. They do not
promise a snapshot across concurrent changes. The adapter does not call
`SortItems` or add a separate ordering rule. Missing storage produces an empty
in-memory view with calculated capacity; GET never persists that view or creates
a depot. Required character/storage errors fail observation, and actual
activities/conflicts take priority over ordinary selection. The
[navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery)
owns input, ownership and GET-only refresh recovery behavior.

Owned status uses explicit `scene_enter` for Depot followed by GET context;
the old GET `/characters/{id}/depot` route is retired. The `depot_expand`
Gateway command delegates to `Expand` and replaces POST
`/characters/{id}/depot/expand`; owned context supplies the read-only quote.
The `depot_sell` and `depot_sell_batch` commands delegate explicit instance
selection to `SellItem` and `SellItems` and return the existing Depot/gold-earned
result, replacing the former single/batch sale POST routes. Discovery never
chooses all visible instances for a batch. Five inventory/transfer REST operations
remain. Explicit `depot_sort` accepts omitted params or an empty object, delegates
to `SortItems` and returns the existing Depot result. Its REST route awaits
separate verified retirement; deposit/withdraw/send command adapters are pending.
The shared
[command contract](../architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions)
owns explicit intent, guards and outcome recovery. Observation does not execute
NPC, delivery, collection or scheduling effects. Legacy `depot.cgi:68–81` dispatch
maps to the existing services as follows:

| Legacy routine | Existing service | Transport |
| --- | --- | --- |
| Listing/header, `get_depot_c` | GetDepot | Selected Depot GET context; whole-list REST retired |
| `azukeru` | DepositItem | POST `/characters/{id}/depot/deposit` |
| `hikidasu` | WithdrawItem | POST `/characters/{id}/depot/withdraw` |
| `uru` | SellItem | Gateway `depot_sell`; sale REST retired |
| `matomete_uru` | SellItems | Gateway `depot_sell_batch`; batch-sale REST retired |
| `seiton` | SortItems | Gateway `depot_sort`; POST `/characters/{id}/depot/sort` retained |
| `okuru` | SendMoney, SendItem | POST `/characters/{id}/depot/send-money`, `/characters/{id}/depot/send-item` |
| `expansion_depot` | Expand | Gateway `depot_expand`; expansion REST retired |

This mapping records transport coverage, not full legacy parity. Legacy
`seiton` (`depot.cgi:313–332`) persists kind/item-number ordering. SortItems results
now survive repository reload, and context pages preserve that order. The remaining
slot/catalog classification difference described above belongs to the feature
comparator; HTTP adds no separate sorting rule.
Legacy `hikidasu` (`depot.cgi:208–288`) swaps held equipment/items back into
storage, while current WithdrawItem moves into inventory and rejects a full
slot; the equivalent atomic swap contract remains undecided in #1127. Equipped
item deposit/send also retains a known foreign-key failure (#1112). Shared
NPC/presence/log effects and remaining command/route migration stay under #947.
