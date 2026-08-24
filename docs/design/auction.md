# Player Auction House & Marketplace Design

## Overview

The Player Auction House and Marketplace Feature Module (`internal/auction`) enables player-driven trading, bidding, and instant buyouts of rare equipment, weapons, and items.

---

## Domain Rules & Mechanics

### Auction Listings

- A player lists an owned item with:
  - **Starting Bid ($S$)**: Minimum opening bid ($S > 0$).
  - **Buyout Price ($B$)**: Optional instant-purchase price ($B = 0$ for no buyout, or $B \ge S$).
  - **Duration**: Listing validity window (e.g. 24 hours, 48 hours).
- **Status Lifecycle**:
  - `ACTIVE`: Open for bidding and buyouts.
  - `SOLD`: Successfully purchased via buyout or won at expiration by highest bidder.
  - `EXPIRED`: Duration elapsed with no bids; item eligible for reclamation by seller.
  - `CANCELLED`: Cancelled by seller before any bids were placed.

---

## Bidding & Outbid Refunds

1. **Placing a Bid**:
   - A bidder submits an amount $A \ge S$ and $A > \text{CurrentBid}$.
   - The seller cannot place bids on their own listing.
   - $A$ gold is deducted and held in escrow from the bidder's wallet.
2. **Outbid Handling**:
   - When a new highest bid $A_{\text{new}}$ is placed, the previous highest bidder is immediately and atomically refunded their locked bid $A_{\text{previous}}$.
3. **Buyout Execution**:
   - If $A \ge B$ (or a player invokes instant `Buyout`), the auction concludes immediately:
     - Previous bidder (if any) is refunded.
     - Buyer pays $B$ gold.
     - Seller receives $B$ gold.
     - Listing status transitions to `SOLD`.

---

## Settlement & Expiration

- **Auction Settlement**:
  - Upon reaching `ExpiresAt`:
    - If `HighestBidderID != nil`: Seller receives the winning bid amount, and listing transitions to `SOLD`.
    - If `HighestBidderID == nil`: Listing transitions to `EXPIRED`.
- **Cancellation**:
  - A seller may cancel an `ACTIVE` listing only if no bids have been placed (`HighestBidderID == nil`).

---

## Concurrency & Persistence

- All bidding, buyout, and settlement operations execute within MariaDB transactions (`auction_listings`) with row-level locks (`SELECT ... FOR UPDATE`), preventing double-bidding or concurrent race conditions.
