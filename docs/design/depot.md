# Character Item Depot & Storage Design

## Overview

The Item Depot (預かり所 / 倉庫) provides characters with persistent storage for item instances outside of their active inventory. The reconstruction models dynamic capacity, storage expansion, item sorting, direct item selling, inter-character mailing and collection discovery from `system.cgi:get_depot_c` and `depot.cgi`. Withdrawal swaps with the legacy hand fields. Gold is kept in character purses and the bank.

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

### Delivery Capacity by Caller

Capacity limits what a player may deposit or another player may send. It does
not limit what an NPC delivers. Legacy `system.cgi:818–837` (`send_item`) and
`1463–1480` (`send_gem`) append to the recipient's file without counting it. Only
callers that call `get_depot_c`, read a stored `is_full` flag or call
`get_you_gem_box_c` (`1482–1493`) beforehand ever reject. In Go, "full" means the
slot count at that moment is at or above the calculated capacity (`Depot.IsFull`).
The legacy `is_full` flag can be out of date; Go does not copy that.
Decided in [#1139](https://github.com/witchcraze/party2re/issues/1139)
on 2026-11-08 under the project owner's standing direction to reproduce what
original players experienced. Apart from the current-count and stack notes here,
no caller below departs from legacy behavior.

| Caller | Legacy check | Rule |
|---|---|---|
| Purchases from weapon, armor, item, accessory and secret shops, including batch buying and quantity > 1 (`weapon.cgi:65,148`, `armor.cgi:62`, `item.cgi:84,171`, `accessory.cgi:119`, `secret.cgi:55`) | none | Deliver |
| Accessory synthesis product (`accessory.cgi:200`) | none | Deliver after the materials are consumed |
| Black Market prizes (`black_market.cgi:118,137`), medal (`medal.cgi:80`) and casino (`casino.cgi:609–623`, one row per unit) prizes | none | Deliver |
| Flea Market purchase and cancel (`free.cgi:161–171,257–271`); Player Store purchase, barter (both sides) and withdrawal (`store.cgi:386–394,468–476,544–555`) | none | Deliver |
| Costume goods (`goods.cgi:72`), altar wish (`reborn.cgi:104`), god wishes (`god.cgi:102–103`), event bazaar (`event.cgi:94`), helper quest reward (`helper.cgi:115`), raffle and lottery (`lot.cgi:134`, `takarakuzi.cgi:237`) | none | Hand when the existing path fills an empty hand, otherwise Deliver |
| Alchemy completion (`alchemy.cgi:183–201`) and Plantation harvest (`plantation.cgi:197–204,249–262`) | none | Deliver |
| Combat book return (`_skill.cgi:6829–6840,6868–6899`) | none | Deliver |
| Combat book exchange (`_skill.cgi:10489–10536`) | none | Swap the requested row in place (`InsertDelivery`); an empty hand removes the row. No slot is added. |
| Gem appraisal and synthesis (`gem_store.cgi:309,374`) | none | Deliver to the Gem Box |
| Treasure inspection (`_npc_action.cgi:6–84`) | own `is_full` | A tool found with an empty tool hand goes to the hand. Otherwise, a full Depot forfeits the item (`DeliveredToLost`), even when a matching stack exists; a Depot with space receives it. |
| Auction (`auction.cgi:57–60`) and Depot (`depot.cgi:349–353`) send of gold or an item | recipient `is_full` | Reject before any change when the recipient is full, even when a matching stack exists; otherwise Deliver |
| Gem send (`gem_store.cgi:231–235`) | recipient count vs **sender** maximum | Reject when the recipient's gem count is at or above the gem capacity for the **sender's** job level; otherwise append |
| Gem purchase (`gem_store.cgi:143`) and Depot deposit (`depot.cgi:172`) | own capacity | Keep the existing rejection |

**Deliver** (`Depot.Deliver`, `GemBox.Deliver`, `PolicyDeliverOverCapacity`)
merges an eligible stack. Otherwise it appends a slot, even when that pushes the
Depot past capacity. Quantity stacks and the inventory/equipment split remain
modern representations. Delivery never raises `Capacity`, so later deposits stay
blocked until the slot count drops below it. The gem-send limit keeps the
sender-based quirk because players lived with it. A box can therefore exceed its
own capacity, and a low-level sender can be rejected by a box that still has room.

Each delivery commits atomically with what was paid for it: currency, points,
materials, listing state, plot or synthesis state. A storage failure rolls all of
it back. Because capacity never rejects, there is no partial delivery. Plantation
draws once and clears the plot, so a retry cannot reroll the harvest.

Callers that pin a different representation keep their own owner:
[registration referral](player-and-character.md#210-registration-referral-delivery)
and the
[Home bag/treasure/key contract](home.md#legacy-dispatch-and-consumption-contract)
append separate instances without stacking (`AppendDelivery`).
[Contest awards](photo-contest.md#approved-delivery-contract-and-implementation-gap)
follow the Deliver rule. Still open: held-field placement on acquisition
([#1948](https://github.com/witchcraze/party2re/issues/1948)), Auction gold
destination ([#1815](https://github.com/witchcraze/party2re/issues/1815)) and
native combat book execution
([#1370](https://github.com/witchcraze/party2re/issues/1370)).

### Dynamic Capacity Recalculation (`RefreshCapacity`)

Because `character_depots.capacity` is initialized at character creation and character job levels advance dynamically over time, commerce and delivery modules (`shop`, `secretshop`, `blackmarket`, `fleamarket`, `auction`, `store`, `god`, `altar`, `medal`, `plantation`) MUST synchronize the in-memory depot capacity using `dep.RefreshCapacity(char.JobLevel, char.OverDepot)` before performing capacity boundary checks. This guarantees that high-level characters enjoy their full dynamic depot capacity (up to 500 slots) across all trade and item receipt operations.

### Root Initialization Guarantee & Centralized Retrieval (`FindOrCreate`)

To eliminate uninitialized depot records and stale capacity errors, a two-layer defense-in-depth architecture is enforced:
1. **Root Guarantee (Character Creation Auto-Initialization)**: Character creation initializes `character_depots` with `capacity = 5` and `ex_depot = 0` alongside the character and joining news. The [character creation contract](player-and-character.md#29-transactional-character-creation--joining-news) owns their shared commit/rollback guarantee.
2. **Centralized Retrieval (`FindOrCreate`)**: Across all commerce and reward domain callers (`store`, `blackmarket`, `plantation`, `casino`, `medal`, `auction`, `alchemy`, `depot/delivery`), services retrieve depots via `depot.FindOrCreate(ctx, repo, char)`. Under pessimistic lock (`FindByCharacterIDForUpdate`), if no record exists it initializes a new `Depot` with dynamic capacity reflecting the character's `JobLevel` and `OverDepot`; if a record already exists, it refreshes the capacity via `dep.RefreshCapacity(char.JobLevel, char.OverDepot)`. This permanently eliminates uninitialized `ErrNotFound` bugs and eliminates repetitive hand-rolled boilerplate across domains.

## Operations & Invariants

### 1. Item Deposit & Withdrawal
- **Deposit Item**: Moves an item instance from character inventory to depot storage.
  - Invariant: Item exists in character inventory.
  - Invariant (Issues #452, #558, #563): Stacking is restricted to stackable items (`Definition.IsStackable() == true` / `Slot == SlotNone`) with `EnhancementLevel == 0`. If a stackable unenhanced item already exists in the depot with identical definition ID, it stacks into the existing slot without consuming an extra slot. Equipment items (weapons, armor, shields, accessories; `IsStackable() == false`) and enhanced items (`EnhancementLevel > 0`) represent discrete gear instances and must NEVER merge into a single slot even if unenhanced (+0), always occupying discrete slots ($\text{len}(Items) < Capacity$).
- **Withdraw Item (`hikidasu`)**: Swaps one unit of the selected depot row into
  the matching [legacy hand field](items-and-equipment.md#legacy-hand-fields)
  (`depot.cgi:208–288`).
  - Invariant: The row exists in depot storage, and its definition and every held
    definition resolve to a canonical legacy kind; otherwise the withdrawal fails
    without guessing. Two held rows in the same field are rejected
    (`ErrInventoryFull`) without change.
  - Swap: A held occupant of the same field is unequipped and returns to storage at
    the withdrawn row's position, or merges into an eligible stack. Legacy rewrites
    one row in place and never checks capacity, so the swap is never rejected for a
    full depot; when a stack split adds the returned row, the slot count may exceed
    capacity by one.
  - Stack splitting: Withdrawing from a stack of $N \ge 2$ leaves $N - 1$ units in
    the same row and gives the hand a quantity-1 instance with a new ID. A row of
    $N = 1$ moves with its instance ID.
  - Equipment: Withdrawn gear is equipped in its slot. A weapon withdrawal clears
    the weapon name and seal; an armor withdrawal clears the armor name. Held-item
    swaps leave customization unchanged. The returned instance keeps identity,
    definition and enhancement, but no customization.
  - Side effect: Records the withdrawn definition in the Collection book
    (`_add_collection.cgi`).
  - Atomicity: Equipment is locked with the inventory before the Depot. The swap
    unlinks the occupant before its inventory row is deleted and links new gear
    after its row exists; any failure rolls back hand, equipment, customization,
    storage and collection together.
  - Legacy `hikidasu` first collects pending remittances
    ([#1109](https://github.com/witchcraze/party2re/issues/1109)) and selects rows
    by name; Go selects an explicit instance ID.

Depositing or sending an equipped instance unlinks its equipment slots before
removing it from inventory. Removing main-hand equipment clears the sender's
weapon name and seal; removing body armor clears the armor name. Other slots and
unequipped instances leave those character fields unchanged. Instance ID,
definition, quantity and enhancement are preserved; customization is not mailed
or stored on the Depot instance. Legacy `depot.cgi:167–207,334–405` removes the
held item, and `party.cgi:22–29` then calls `system.cgi:write_user` (`80–87`) to
clear customization for empty weapon/armor fields. Failure, including full
storage or a later save error, rolls back equipment, customization and ownership.

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
- Legacy `depot.cgi:313–332` sorts each stored row by its kind, then by its numeric
  item number. `azukeru` (`167–207`) stores weapons as kind 1, armor as kind 2 and
  every entry of the item catalog as kind 3, so shields and accessories sort with
  consumables by catalog number, not with armor.
- Go derives the same key from canonical catalog identity
  (`item.Definition.LegacyStorageKey`): `weapon-NN` → (1, NN), `armor-NN` → (2, NN),
  `item-NNN` → (3, NNN). This is independent of `Slot.Kind()`'s slot grouping.
  Instances with equal keys keep their existing relative order.
- Legacy rows always carry a kind and number, so a definition that is missing from
  the catalog, is not a canonical identity, or has no configured definition provider
  has no legacy behavior. By approved decision (#1288) the sort then fails with the
  wrapped catalog error and the transaction leaves the depot unchanged; no
  classification is guessed.
- Explicit sorting persists the resulting instance order. Ordinary saves preserve
  slice order, including appended items; reads never implicitly sort by definition.
  Both normal and locked reads use stored position with instance ID as a tie-breaker.
  Rows predating the position migration retain their former ID order until saved.

### 5. Mailing Items and Money (`おくる`)
- **Send Money (`SendMoney`)**: Transits gold directly from the sender's purse to the recipient's purse.
  - Invariant: `sender.Money >= amount` and `amount > 0` and `sender != recipient`.
- **Send Item (`SendItem`)**: Transits an item directly from sender's inventory into the recipient's depot storage.
  - Invariant: Item exists in sender inventory.
- Both reject a full recipient Depot before any change, even when a matching
  stack exists ([delivery capacity](#delivery-capacity-by-caller)).

### 6. Storage Item Consumption (`Consume`, `ConsumeOne`, `PurgeSlot`)
- Town facilities and trade actions (e.g., Home consumable item usage, Black Market rare item sacrifices, Gem Store synthesis crafting, Flea Market and Player Store listings) consume items directly from Depot storage.
- Standardized consumption method `dep.Consume(instanceID, quantity)` safely decrements `Quantity` by `quantity` when `existing.Quantity >= quantity`, returning a copy of the consumed instance with `Quantity = quantity`.
- If the remaining quantity reaches 0, the item slot is removed from depot storage.
- If `quantity <= 0` or `existing.Quantity < quantity`, returns `ErrInvalidQuantity` without mutating depot state.
- `dep.ConsumeOne(instanceID)` delegates directly to `dep.Consume(instanceID, 1)` and is also used by `WithdrawItem` so withdrawing from a stack moves a single unit to the hand.
- Intentional whole-slot removal (e.g., selling an entire depot stack) must use `PurgeSlot(instanceID)` (or legacy `RemoveItem`, which delegates to `PurgeSlot`). `RemoveItem` is marked deprecated for consumption purposes to prevent accidental stack deletion.

## Atomicity, Concurrency & Lock Hierarchy

Production single-character mutations use `economy.TransactionRunner`: character
locks (Rank 2), inventory/equipment locks when needed (Rank 3), then Depot root/item locks
(Rank 5). Sales lock no inventory. The runner credits the wallet after the Depot
save in the same transaction; a later character-save failure rolls both back.
Concurrent single/batch sales of an overlapping instance can settle only once.
Cross-character transfers (`SendMoney`, `SendItem`) use the injected transaction
provider and enforce global lock hierarchy ordering:
1. **Rank 2 (`characters`)**: Both sender and recipient row locks acquired via `id.Sort2(fromID, toID)` in ascending lexicographical order to prevent deadlocks.
2. **Rank 3 (`inventory_items`, `equipment_slots`)**: Sender inventory, then equipment locked for item transfers. Equipment references are saved before inventory deletion.
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
the mapping below owns individual route dispositions. Connected Gateway
commands delegate to the listed services and retain their Depot result,
including gold earned for sales and trimmed recipient/item IDs for sending.
`depot_expand` and `depot_sort` accept omitted params or an empty object.
`depot_deposit`, `depot_withdraw` and `depot_sell` require a non-null string `item_id`;
`depot_sell_batch` requires an explicit array of string `item_ids`.
`depot_send_item` requires non-null string `item_id` and
`recipient_character_id`; the actor remains the authenticated path owner.
Discovery never selects items or recipients. Exact ownership, capacity,
stacking and equipment detachment remain service-owned. The shared
[command contract](../architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions)
owns explicit intent, guards and outcome recovery. Observation does not execute
NPC, delivery, collection or scheduling effects. Legacy `depot.cgi:68–81` dispatch
maps to the existing services as follows:

| Legacy routine | Existing service | Transport |
| --- | --- | --- |
| Listing/header, `get_depot_c` | GetDepot | Selected Depot GET context; whole-list REST retired |
| `azukeru` | DepositItem | Gateway `depot_deposit`; deposit REST retired |
| `hikidasu` | WithdrawItem | Gateway `depot_withdraw`; withdrawal REST retired |
| `uru` | SellItem | Gateway `depot_sell`; sale REST retired |
| `matomete_uru` | SellItems | Gateway `depot_sell_batch`; batch-sale REST retired |
| `seiton` | SortItems | Gateway `depot_sort`; sort REST retired |
| `okuru` (item) | SendItem | Gateway `depot_send_item`; item-send REST retired |
| `okuru` (money) | SendMoney | POST `/characters/{id}/depot/send-money` |
| `expansion_depot` | Expand | Gateway `depot_expand`; expansion REST retired |

This mapping records transport coverage, not full legacy parity. Sorting and
equipped-item movement use the domain contracts above.
Withdrawal follows the swap contract above; equipped item deposit/send uses the
atomic equipment-detachment contract. Shared
NPC/presence/log effects and remaining command/route migration stay under #947.
Legacy `okuru` (`depot.cgi:17–21,371–405`) has configurable forbidden-item
lists, all empty in the reference checkout. Current `SendItem` has no such
configuration; nonempty deployment-specific lists are unverified. Recipient
Home delivery notices remain under
[#1110](https://github.com/witchcraze/party2re/issues/1110).
