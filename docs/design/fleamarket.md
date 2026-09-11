# Flea Market and Player Item Stall Design Specification

## Overview

The Flea Market (`fleamarket`) subsystem cleanly reconstructs and modernizes the original Party2 `free.cgi` ("フリーマーケット") mechanics.
In contrast to the auction house (bidding and timed auction settlement) and the NPC item shop (fixed 50% resale price), the Flea Market enables players to set up casual player-to-player stalls to list items directly from their **Depot storage (`depot.cgi`)** for direct fixed-price purchase by other adventurers.

---

## 1. Legacy `@actions` Reconciliation Table

| Legacy Action | Legacy Subroutine | Modern Go Domain Method | Modern HTTP Endpoint | Reconciliation Status & Rationale |
|---|---|---|---|---|
| `しゅっぴん` | `&syuppin` | `FleaMarketService.CreateListing` | `POST /characters/{id}/fleamarket/listings` | **Reconciled (1:1 with free.cgi/depot.cgi)**: Lists item from seller's Depot storage at fixed price (1–999,999 G). Seller listing limit is base 5 + `seller.OverFlea` (up to 10). Strictly checks server-wide 120 active listing ceiling (`$MAX = 120`). |
| `かう` | `&kau` | `FleaMarketService.PurchaseListing` | `POST /characters/{id}/fleamarket/listings/{listing_id}/purchase` | **Reconciled (1:1 with free.cgi/depot.cgi)**: Direct purchase by another player. Deducts buyer gold, credits seller gold, and deposits purchased item directly into buyer's Depot (`&send_item`) atomically. |
| `みる` | `&miru` | `FleaMarketService.ListActiveListings`<br>`FleaMarketService.GetListing`<br>`FleaMarketService.GetCharacterListings` | `GET /fleamarket/listings`<br>`GET /fleamarket/listings/{listing_id}`<br>`GET /characters/{id}/fleamarket/listings` | **Reconciled (1:1 & Extended)**: Lists active listings with pagination and allows querying character-specific active listings. |
| `もどす` | `&modosu` | `FleaMarketService.CancelListing` | `DELETE /characters/{id}/fleamarket/listings/{listing_id}` | **Reconciled (1:1 with free.cgi/depot.cgi)**: Seller cancels own active listing; item is safely returned to seller's Depot storage (`&send_item`). Fails cleanly if seller's depot is at capacity. |

---

## 2. Domain Architecture & Invariants

```mermaid
classDiagram
    class Listing {
        +string ID
        +string SellerCharacterID
        +string SellerName
        +string ItemID
        +string ItemName
        +string ItemCategory
        +int Price
        +ListingStatus Status
        +string BuyerCharacterID
        +string BuyerName
        +time.Time CreatedAt
        +time.Time SoldAt
    }

    class PurchaseResult {
        +Listing Listing
        +int BuyerGold
        +int SellerGold
        +Instance ItemInstance
    }
```

### Invariants:
1. **Server-Wide Active Listing Ceiling**: The market enforces a strict server-wide ceiling of at most 120 simultaneous active listings (`ServerMaxListings = 120`, matching `$MAX = 120` in `free.cgi`). If 120 active listings are present, `CreateListing` returns `ErrServerMaxListingsReached` (`400 Bad Request`).
2. **Depot-Backed Item Storage**: In accordance with the original Perl implementation, items listed in the flea market originate from the player's Depot (倉庫), are delivered to the buyer's Depot on purchase, and return to the seller's Depot on cancellation. If the recipient's Depot is full, operations return `ErrDepotFull`.
3. **Seller Listing Capacity & OverFlea Expansion**: A character can maintain at most `5 + OverFlea` active listings simultaneously (`MaxListingsPerCharacter = 5`, expanded up to 10 via character `OverFlea` capacity flag).
4. **Price Range Enforcement**: Listing price must be between 1 G and 999,999 G (`MinListingPrice = 1`, `MaxListingPrice = 999999`).
5. **Pessimistic Locking & Deadlock-Free Ordering**:
   - All state mutations execute inside MariaDB transactions (`RunInTx`).
   - Cross-character locks during purchase are acquired strictly in ascending character ID order: `firstCharID < secondCharID`.
   - Complete global lock hierarchy:
     - Rank 0: `fleamarket_listings` (`GetListingByIDForUpdate`)
     - Rank 2: `characters` (`FindByIDForUpdate`, sorted ascending)
     - Rank 5: `depots` (`FindByCharacterIDForUpdate`)
6. **Self-Purchase Prevention**: Sellers are forbidden from purchasing their own flea market listings (`ErrCannotBuyOwnListing` / `400 Bad Request`).
7. **Ownership-Validated Cancellation**: Only the seller who created the listing can cancel it (`ErrUnauthorizedSeller` / `403 Forbidden`). Cancelled listings immediately return the item to the seller's Depot.
8. **Compare-And-Swap (CAS) Status Transition**: Listings can only be purchased or cancelled from `active` status. Double purchase or concurrent cancel attempts are rejected cleanly (`ErrListingNotActive`).

---

## 3. API Endpoints

| Method | Path | Summary | Authentication |
|---|---|---|---|
| `GET` | `/fleamarket/listings` | List paginated active flea market listings | Public |
| `GET` | `/fleamarket/listings/{listing_id}` | Get specific flea market listing details | Public |
| `GET` | `/characters/{id}/fleamarket/listings` | List authenticated character's own listings | Bearer Token (Character Owner) |
| `POST` | `/characters/{id}/fleamarket/listings` | Create a new flea market listing from Depot | Bearer Token (Character Owner) |
| `POST` | `/characters/{id}/fleamarket/listings/{listing_id}/purchase` | Purchase an active listing from another player into Depot | Bearer Token (Character Owner) |
| `DELETE` | `/characters/{id}/fleamarket/listings/{listing_id}` | Cancel active listing and return item to Depot | Bearer Token (Character Owner) |

---

## 4. Database Schema

- `fleamarket_listings`:
  - `id CHAR(32) PRIMARY KEY`
  - `seller_character_id CHAR(32) NOT NULL` (Foreign key to `characters(id) ON DELETE CASCADE`)
  - `seller_name VARCHAR(64) NOT NULL`
  - `item_id VARCHAR(64) NOT NULL`
  - `item_name VARCHAR(64) NOT NULL`
  - `item_category VARCHAR(32) NOT NULL DEFAULT 'misc'`
  - `price INT NOT NULL`
  - `status VARCHAR(32) NOT NULL DEFAULT 'active'`
  - `buyer_character_id CHAR(32) NULL` (Foreign key to `characters(id) ON DELETE SET NULL`)
  - `buyer_name VARCHAR(64) NULL`
  - `created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`
  - `sold_at TIMESTAMP NULL`
  - Indexes on `(seller_character_id, status)` and `(status, created_at)`.
