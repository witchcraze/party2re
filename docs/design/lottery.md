# Lottery & Raffle Design

## Overview

Two town games: the raffle (福引, `party2/lib/lot.cgi`) paid with tavern coupons, and the 20-ticket lottery (宝くじ, `party2/lib/takarakuzi.cgi`). Prize tables, prices and transport are owned by code and [OpenAPI](../api/openapi.json).

## Raffle

- There is no tier choice: a character with 300 or more coupons always plays the special raffle (300 coupons); otherwise the standard raffle (3 coupons). The mode is decided against the locked balance, so concurrent draws cannot use a stale balance. Coupons come only from tavern meals and wishes; there is no gold purchase or gold prize.
- A prize goes to the inventory when the tool hand is empty, otherwise to the Depot, past capacity if needed (`lot.cgi:134`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- The standard grand prize depends on the JST weekday.

## Lottery

- 30,000 gold per ticket, one per character, at most 20 per round. Drawings fall on the 1st, 11th and 21st at 00:00 JST, never early. Rounds with fewer than 20 tickets are filled with dummy entries; a character or dummy wins at most once per round. Winner quotas are 1, then 1–2, then 2–4 (`takarakuzi.cgi:301–303`), dummies included.
- Prizes go straight to the winner's Depot, past capacity if needed (`takarakuzi.cgi:237`; [delivery capacity](depot.md#delivery-capacity-by-caller)), and register the collection by legacy hand kind. A storage failure rolls back the whole drawing; the next round starts with a new lineup.

### Prize lineup and winner selection

**Approved decision ([#1249](https://github.com/witchcraze/party2re/issues/1249), 2026-10-18):** retain the current Go selection across all candidate slots and all remaining participant slots (`internal/lottery/takarakuji.go`). This intentionally departs from the legacy execution behavior. Duplicate prize entries retain their weights; unique items are **not** equally likely. The rationale is to keep every participant slot and every listed prize reachable, without changing the current Go draw behavior. Restoring the legacy exclusions was considered and rejected. Depot-full behavior remains separately tracked in [#1139](https://github.com/witchcraze/party2re/issues/1139).

- **Candidate selection:** Legacy `party2/lib/takarakuzi.cgi:23-53,297-299,308-310` defines 5/9/9 candidate rows with one-based labels, draws `rand($#list)` (indices `0..3` or `0..7`), and indexes by the selected row's one-based label, making index 0 (`item-129`, `item-168`, `item-126`) unreachable (`0` vs `1/4` or `1/8` per remaining row). Go selects uniformly across all candidate slots (`1/5` or `1/9` per slot, preserving duplicate-slot weights).
- **Winner selection:** Legacy `party2/lib/takarakuzi.cgi:214-245` draws `rand($#players)` (`0..N-2`), leaving the final slot (`N-1`) unreachable (`0` vs `M/19` for `M` total draws in `4..7`). When 20 tickets sell, the 20th purchaser cannot win; when fewer sell, the excluded slot is the last appended dummy. Go draws uniformly without replacement across all 20 slots (`M/20`, with rank probabilities `1/20`, `amount2/20`, `amount3/20`).
