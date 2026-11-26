# Flea Market Design

## Overview

The Flea Market (`party2/lib/free.cgi`) lets players list items from their Depot for fixed-price purchase by other players.

## 1. Legacy `@actions` Reconciliation Table

Legacy source: `party2/lib/free.cgi`. Commands use the [Action Gateway](../architecture/client-agent-api.md) and observation uses the `fleamarket` scene. All former listing REST routes, including the anonymous listing GETs, are retired and return 404. Legacy shows the market only inside the facility, and Flea Market is not an approved public projection.

| Legacy action | Lines | Go operation | Transport | Disposition |
|---|---|---|---|---|
| `しゅっぴん` | 52-138 | `CreateListing` | `fleamarket_list` with `item_id`, `price` | Lists one Depot unit at 1-999,999 G. Legacy matches by item name and takes the first row; `item_id` is an instance ID or a definition ID (first match). The legacy prompt listing Depot rows maps to the `depot` scene's instances. |
| `かう` | 143-207 | `PurchaseListing` | `fleamarket_purchase` with `listing_id` | Charges the buyer, delivers to the buyer's Depot, and holds the seller's payment as a pending remittance with the Home money notice (`send_money`, source `fleamarket_payment`). The action returns the purchased `listing`, `buyer_gold` and `item_instance`. Legacy leaves your own listings out of the prompt; Go rejects them with `FLEAMARKET_OWN_LISTING`. |
| `みる` | 212-237 | `ListActiveListings` | `listings` in the scene | Observation only, with no cooldown, as legacy sets `act_time = 0`. A row carries every fact the legacy table shows, so no detail read replaces the retired one. |
| `もどす` | 242-298 | `CancelListing`, `GetActiveCharacterListings` | `fleamarket_cancel` with `listing_id`; `owned_listings` in the scene | Returns the item to the seller's Depot. Legacy deletes finished rows and offers only active ones, so the former Go read of sold and cancelled history is retired. Those rows stay in the table as records but are not observed. |
| `はなす`, NPC `しらべる` | `system.cgi` shared | `npc_talk`, `npc_inspect` | None | Talk logs one of `@FMGKDP`'s words and starts the action wait; inspect returns the legacy default privately. |

### Known differences

These are recorded, not approved departures:

- **Action wait and facility log**: legacy successful commands keep the default action wait and write the facility log. The Gateway applies the [action wait](../architecture/place-log.md#action-wait); the commands' log rows remain [#1941](https://github.com/witchcraze/party2re/issues/1941).
- **Listing number**: legacy shows the lowest free number. Go uses an opaque listing ID, and order ties use age then ID.

## 2. Rules

- **Server-wide capacity lock**: creators serialize on the single-row `fleamarket_capacity_lock` before counting the 120-listing ceiling (`$MAX = 120` in `free.cgi`). Locking listing rows cannot do this because `READ COMMITTED` has no gap locks; purchase and cancel only free slots so they skip it. The lock row stores no game state and is recreated if missing.
- **Listing capacity**: at most `5 + OverFlea` active listings per character (up to 10).
- **Depot delivery and auto-init**: items originate from and return to the seller's Depot, and purchases deliver to the buyer's Depot (auto-created via `depot.FindOrCreate`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- **Listing collection (`free.cgi:130–131`)**: listing runs `add_collection` on the lister's held hand items (`inventory_items`), not the listed Depot item.
- **Instance enhancement preservation**: a listing escrows the unit's enhancement level, and purchase or cancellation delivers an instance with the same level. Legacy rows hold only kind and number, so this is the current instance-preservation contract rather than a legacy rule; pre-column rows read as level 0.

## 3. Observation

The `fleamarket` scene exposes the current page of active listings, the total, and the actor's own active listings. GET never mutates, and a failed required read fails the whole observation.

Listings follow legacy `list_sort` (`free.cgi:367-404`): item kind (weapon, armor, item), item number, price, then age, with the listing ID keeping pages stable. Listing candidates come from the `depot` scene. Command rejection codes and schemas: [OpenAPI](../api/openapi.json).
