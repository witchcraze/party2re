# Auction Hall Design (Live P2P Trading, Direct Send & Inspect)

## Overview

The Auction Hall module (`internal/auction`) faithfully reproduces the original Party2 specification (`party2/lib/auction.cgi`).

In original Party2, the "Auction Hall" (オークション会場) is not an asynchronous automated auction house with server-side bids, listings, or buyouts. Instead, it is a venue for **live, player-to-player (P2P) trading and unscripted bartering**, overseen by NPC **@ワイルド**.

As NPC @ワイルド explains:
> 「ここはオークション会場です。他のプレイヤーとアイテム交換やアイテム売買をする場所です。」  
> 「入札や出品のようなシステムはないです。自由に競りをしてください。」  
> 「相手が実際にそのアイテムや落札金を持っているのか「＠しらべる」で見ることができます。」

---

## Domain Rules & Mechanics

### 1. Direct Transfer (@おくる / Send)

Players negotiate auction terms in real-time chat and settle trades via direct transfer:

- **Send Gold**:
  - Requires a minimum amount of 1 G (`ErrInvalidSendAmount`).
  - Verifies sender has sufficient funds (`ErrInsufficientMoney`).
  - Atomically deducts gold from the sender's wallet and credits the recipient's wallet, clamping at maximum capacity (999,999 G).
- **Send Item**:
  - The sender specifies an equipped slot (`weapon`, `armor`, `accessory`, or `shield`) or a specific inventory instance ID.
  - If equipped, the item is unequipped from the sender's active equipment and consumed from the sender's inventory.
  - Taboo/restricted items cannot be sent (`ErrTabooItem`).
  - The item is deposited directly into the recipient's **depot** (`depot.Depot`).
  - If the recipient's depot is at capacity, the transfer is rejected with `ErrDepotFull` before deducting any items from the sender.
- **Constraints**:
  - Sending to self is rejected (`ErrCannotSendToSelf`).
  - Target must exist (`ErrTargetNotFound`).

### 2. Player Inspection (@しらべる / Inspect)

To prevent trade fraud and verify solvency during bidding:
- Players can inspect any target player by character ID or name.
- Returns target character's Level, Job, Job Level, Wallet Money, and currently equipped Weapon, Armor, and Accessory (with enhancement levels).

---

## Concurrency & Deadlock Prevention

All transfer operations execute within a single transactional boundary (`TransactionProvider.RunInTx`) strictly following the Global Pessimistic Lock Hierarchy (Rule 05):

1. **Rank 2 (Characters)**: Both sender and recipient characters are locked using `FindByIDForUpdate` in **monotonically ascending character ID order** (`id.Sort2(senderID, targetID)`). This mechanically guarantees that concurrent bidirectional transfers (e.g. A sends to B while B sends to A) never deadlock.
2. **Rank 3 (Inventory)**: Sender's inventory is locked using `FindByCharacterIDForUpdate` to consume the item.
3. **Rank 5 (Depot)**: Recipient's depot is locked using `FindByCharacterIDForUpdate` to pre-check capacity and add the transferred item.
