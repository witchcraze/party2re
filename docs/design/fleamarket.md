# Flea Market Design

## Overview

The Flea Market (`party2/lib/free.cgi`) lets players list items from their Depot for fixed-price purchase by other players.

## 1. Legacy `@actions` Reconciliation Table

Legacy source: `party2/lib/free.cgi`. Commands use the [Action Gateway](../architecture/client-agent-api.md) and observation uses the `fleamarket` scene. Legacy shows the market only inside the facility, so Flea Market has no public projection.

| Legacy action | Lines | Go operation | Transport | Disposition |
|---|---|---|---|---|
| `しゅっぴん` | 52-138 | `CreateListing` | `fleamarket_list` with `item_id`, `price` | Legacy matches by item name and takes the first row; `item_id` is an instance ID or a definition ID (first match). The legacy prompt listing Depot rows maps to the `depot` scene's instances. |
| `かう` | 143-207 | `PurchaseListing` | `fleamarket_purchase` with `listing_id` | Holds the seller's payment as a pending remittance with the Home money notice (`send_money`, source `fleamarket_payment`). Legacy leaves your own listings out of the prompt; Go rejects them with `FLEAMARKET_OWN_LISTING`. |
| `みる` | 212-237 | `ListActiveListings` | `listings` in the scene | Observation only, with no cooldown, as legacy sets `act_time = 0`. |
| `もどす` | 242-298 | `CancelListing`, `GetActiveCharacterListings` | `fleamarket_cancel` with `listing_id`; `owned_listings` in the scene | Legacy deletes finished rows and offers only active ones, so sold and cancelled rows stay in the table as records but are not observed. |
| `はなす`, NPC `しらべる` | `system.cgi` shared | `npc_talk`, `npc_inspect` | None | Talk logs one of `@FMGKDP`'s words and starts the action wait; inspect returns the legacy default privately. |

Commands select a listing by its opaque ID; the legacy display number (`number_select`, `free.cgi:332–366`) is the lowest number free among active listings when an item is listed, so a finished listing's number is reused. Rows show it, and the `＠かう>` and `＠もどす>` log rows name it (`free.cgi:185,278`).

## 2. Rules

- **Server-wide capacity lock**: creators serialize on the single-row `fleamarket_capacity_lock` before reading the active listings for the 120-listing ceiling (`$MAX = 120` in `free.cgi`) and the free display number. Locking listing rows cannot do this because `READ COMMITTED` has no gap locks; purchase and cancel only free slots and numbers, so they skip it. The lock row stores no game state and is recreated if missing.
- **Listing collection (`free.cgi:130–131`)**: listing runs `add_collection` on the lister's held hand items (`inventory_items`), not the listed Depot item.
- **Instance enhancement preservation**: a listing escrows the unit's enhancement level, and purchase or cancellation delivers an instance with the same level. Legacy rows hold only kind and number, so this is the current instance-preservation contract rather than a legacy rule; pre-column rows read as level 0.
- Deliveries ignore Depot capacity ([delivery capacity](depot.md#delivery-capacity-by-caller)).

## 3. Observation

Listings follow legacy `list_sort` (`free.cgi:367-404`): item kind (weapon, armor, item), item number, price, then age, with the listing ID keeping pages stable.
