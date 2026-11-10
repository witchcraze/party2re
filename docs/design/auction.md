# Auction Hall Design (Live P2P Trading, Direct Send & Inspect)

## Overview

The Auction Hall module (`internal/auction`) provides direct transfers and player
inspection. Legacy behavior comes from `party2/lib/auction.cgi` and its shared
`system.cgi` routines; the transport migration preserves existing service behavior
without certifying full legacy parity.

In original Party2, the "Auction Hall" (オークション会場) is not an asynchronous automated auction house with server-side bids, listings, or buyouts. Instead, it is a venue for **live, player-to-player (P2P) trading and unscripted bartering**, overseen by NPC **@ワイルド**.

The venue is the `auction` scene; `@ワイルド`'s words are an HTTP-owned NPC
profile in [`action_npc.go`](../../internal/api/http/action_npc.go).

---

## Current Service Rules & Mechanics

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
  - If the recipient's Depot is full, gold and item sends are both rejected with `ErrDepotFull` before any change, even when a matching stack exists (`auction.cgi:57–60`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- **Constraints**:
  - Sending to self is rejected (`ErrCannotSendToSelf`).
  - Target must exist (`ErrTargetNotFound`).
  - Target ID takes precedence over target name; positive gold takes precedence
    over an item, and an equipped slot takes precedence over an instance ID.

### 2. Player Inspection (@しらべる)

`しらべる` on a player is the common `system.cgi` `shiraberu`, available in every
place, not an Auction rule; the venue words only advertise it. It is the
scene-independent `player_inspect` command, whose status projection is
`auction.Service.Inspect` composed with guild, custom-skill gems, login message
and presence at the composition root.

- **Self fallback:** an omitted or nonexistent target shows the actor's own
  status, as legacy does (`system.cgi:385`). Self-only legacy facts (money,
  coins, medals, next-level experience) are already in the actor's context.
- **No wallet money:** legacy never shows another player's money; the "落札金"
  in the venue words cannot actually be checked.
- **Armor defect:** legacy prints armor only `if $p{wea}` (`system.cgi:373,402`),
  a copy of the weapon condition. Armor is shown whenever equipped.

---

## Concurrency & Deadlock Prevention

All transfer operations execute within a single transactional boundary (`TransactionProvider.RunInTx`) strictly following the Global Pessimistic Lock Hierarchy (Rule 05):

1. **Rank 2 (Characters)**: Both sender and recipient characters are locked using `FindByIDForUpdate` in **monotonically ascending character ID order** (`id.Sort2(senderID, targetID)`), avoiding inverse lock order in bidirectional transfers.
2. **Rank 3 (Inventory)**: Sender's inventory is locked using `FindByCharacterIDForUpdate` to consume the item.
3. **Rank 5 (Depot)**: Recipient's depot is locked using `FindByCharacterIDForUpdate` to pre-check capacity and add the transferred item.

## Gateway and retained reads

`auction_send` invokes `AuctionService.Send` with the authenticated owned character
as sender. Parameters are optional `target_character_id`, `target_character_name`,
`slot`, `instance_id` strings and integer `gold`; supplied values cannot be null.
Missing recipients, empty transfers, amounts and owned items remain service
validations. The result preserves the resolved recipient, transferred gold or item
instance/definition/name/enhancement and message. The shared
[command outcome contract](../architecture/client-agent-api.md#command-outcome-and-context-refresh)
preserves known outcomes when refresh fails; recovery reads context without replay.

| Legacy action | Disposition |
| --- | --- |
| `auction.cgi` `おくる` / `okuru` | `auction_send` in the `auction` scene. No body sender override survives. |
| Common `はなす` (`system.cgi:240–246`), venue words | `npc_talk` in the `auction` scene; NPC `しらべる` is `npc_inspect`. |
| Common `しらべる` on a player (`system.cgi:320–410`) | `player_inspect` (above). No Auction REST read remains. |

### Legacy differences still requiring reconciliation

Legacy gold is queued through `system.cgi:842–855` (`send_money`), whereas Go
credits the wallet in the transfer transaction and clamps at its limit. The
queue/destination and wallet-overflow policy are tracked in
[the remittance specification issue](https://github.com/witchcraze/party2re/issues/1815);
this transport change does not resolve them. Go additionally accepts explicit
inventory instances and shield slots; legacy offers the currently equipped weapon,
armor or accessory.
