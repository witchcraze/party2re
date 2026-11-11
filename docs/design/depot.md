# Character Item Depot & Storage Design

## Overview

The Depot (預かり所) stores item instances outside the inventory (`depot.cgi`, `system.cgi:get_depot_c`). Withdrawal swaps with the legacy hand fields. Gold sent to a character is [held](#6-held-gold) until collected here. Shapes, prices and transport are owned by code and [OpenAPI](../api/openapi.json).

## Capacity Formula

Capacity grows with job level, paid expansions (+5 each, at most 20) and god limit breaks (+50 each, at most 5), between 5 and 500 slots (`system.cgi:1267–1291`). Capacity is computed from current values, not the value stored at character creation.

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
follow the Deliver rule. Still open: Auction gold
destination ([#1815](https://github.com/witchcraze/party2re/issues/1815)) and
native combat book execution
([#1370](https://github.com/witchcraze/party2re/issues/1370)).

## Operations & Invariants

### 1. Item Deposit & Withdrawal
- **Deposit** (`depot.cgi:167–207`) is rejected at capacity. Only unenhanced stackable items merge into an existing stack; equipment and enhanced items always occupy their own slot.
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
  - Held gold is collected before any item ([§6](#6-held-gold)). Legacy selects
    rows by name; Go selects an explicit instance ID.

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

`depot.cgi:23–46,521–542` quotes the current tier before an explicit purchase. A displayed quote is not a reservation: the command charges the tier current at execution, atomically with incrementing the count.

### 3. Depot Item Sales (`うる` / `まとめてうる`)

Stored items sell at floor(base price × 0.5) per unit without withdrawal. Legacy `depot.cgi:105–164,420–519` sells individual stored rows; the reconstruction sells whole selected instances, including every unit of a stack, ignoring enhancement. A batch validates every selected item first and changes nothing if any is missing, repeated or unpriceable; a successfully read 0G definition stays sellable.

### 4. Depot Sorting (`せいとん`)
- Legacy `depot.cgi:313–332` sorts each stored row by its kind, then by its numeric
  item number. `azukeru` (`167–207`) stores weapons as kind 1, armor as kind 2 and
  every entry of the item catalog as kind 3, so shields and accessories sort with
  consumables by catalog number, not with armor.
- Go uses the same key from catalog identity (`weapon-NN` → (1, NN), `armor-NN` → (2, NN), `item-NNN` → (3, NNN)), independent of slot grouping; equal keys keep their relative order.
- Legacy rows always carry a kind and number, so a definition that is missing from
  the catalog, is not a canonical identity, or has no configured definition provider
  has no legacy behavior. By approved decision (#1288) the sort then fails with the
  wrapped catalog error and the transaction leaves the depot unchanged; no
  classification is guessed.
- Only an explicit sort reorders the Depot; reads and ordinary saves keep stored order.

### 5. Mailing Items and Money (`おくる`)

Sending gold or an item rejects a full recipient Depot before any change, even when a matching stack exists ([delivery capacity](#delivery-capacity-by-caller)). Legacy `okuru` (`depot.cgi:17–21,371–405`) has configurable forbidden-item lists, all empty in the reference checkout; Go has no such configuration. Sent gold leaves the sender's wallet and is [held](#6-held-gold) for the recipient.

### 6. Held Gold

Legacy `send_money` (`system.cgi:842–855`) queues gold for the recipient, and `hikidasu` collects the whole queue before any item. `_get_money` (`depot.cgi:290–307`) empties the queue after adding every entry, and `write_user` then clamps the wallet at 999,999 (`system.cgi:71`), so gold over the cap is lost.

Approved departure (#1109): collection takes entries oldest first only up to the wallet cap and keeps the remainder held. When nothing fits, the withdrawal proceeds as an item withdrawal, so a full wallet never blocks items. Entries store the sender name and source instead of a rendered message.

## Gateway Observation and Retained Operations

Depot is a pageable town destination observed through character context (`depot.cgi:49–99`; capacity and occupied-row rule `system.cgi:1267–1288`). A stack occupies one slot. GET never creates or saves a Depot, sorts it or moves items. All Depot operations, including gold sending (`depot_send_money` with explicit `amount` and `recipient_character_id`), use Gateway commands; no Depot REST route remains. Command place-log echoes (#1941) and presence (#1943) remain open.
