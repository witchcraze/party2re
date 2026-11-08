# Lottery & Raffle Design

## Overview

Two town games: the raffle (福引, `party2/lib/lot.cgi`) paid with tavern coupons, and the 20-ticket lottery (宝くじ, `party2/lib/takarakuzi.cgi`). Prize tables, prices and transport are owned by code and [OpenAPI](../api/openapi.json).

## Raffle

- There is no tier choice: a character with 300 or more coupons always plays the special raffle (300 coupons); otherwise the standard raffle (3 coupons). The mode is decided against the locked balance, so concurrent draws cannot use a stale balance. Coupons come only from tavern meals and wishes; there is no gold purchase or gold prize.
- A prize goes to the inventory when the consumable hand is empty, otherwise to the Depot, past capacity if needed (`lot.cgi:134`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- The standard grand prize depends on the JST weekday.

## Lottery

- 30,000 gold per ticket, one per character, at most 20 per round. Drawings fall on the 1st, 11th and 21st at 00:00 JST, never early. Rounds with fewer than 20 tickets are filled with dummy entries; a character or dummy wins at most once per round. Winner quotas are 1, then 1–2, then 2–4 (`takarakuzi.cgi:301–303`), dummies included.
- Prizes go straight to the winner's Depot, past capacity if needed (`takarakuzi.cgi:237`; [delivery capacity](depot.md#delivery-capacity-by-caller)), and register the collection by slot, so equipment prizes do not count as items. A storage failure rolls back the whole drawing; the next round starts with a new lineup.

### Prize lineup and winner selection

**Approved decision ([#1249](https://github.com/witchcraze/party2re/issues/1249), 2026-10-18):** retain the current Go selection across all candidate slots and all remaining participant slots. This intentionally departs from the legacy execution behavior. Duplicate prize entries retain their weights; unique items are **not** equally likely. The rationale is to keep every participant slot and every listed prize reachable, without changing the current Go draw behavior. Restoring the legacy exclusions was considered and rejected. Depot-full behavior remains separately tracked in [#1139](https://github.com/witchcraze/party2re/issues/1139).

Each tier selects one prize item per round. Winner quotas remain 1 for first prize, 1 or 2 with equal probability for second prize, and 2, 3, or 4 with equal probability for third prize (`party2/lib/takarakuzi.cgi:301-303`). These quotas include dummy winners.

The legacy candidate tables (`party2/lib/takarakuzi.cgi:23-53`) have 5/9/9 rows with one-based labels. At lines 297-299, `rand($#list)` initially selects indices 0..3 or 0..7. Lines 308-310 then use the selected row's one-based label as an array index, so the **effective** prize indices are 1..4 or 1..8. Consequently the first prize entry is unreachable, while the last remains reachable. Merely restoring duplicate entries, as described in closed #800, does not reproduce these execution probabilities.

The tables enumerate the complete prize support and weights for each tier, conditional on creating a new lineup. A zero probability denotes an unreachable prize. Go directly selects indices 0..4 or 0..8, with probability 1/5 or 1/9 per candidate slot.

| First prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-129` (神の錬金レシピ) | 0 | 1/5 (20%) |
| `item-266` (奇跡の錬金レシピ) | 1/4 (25%) | 1/5 (20%) |
| `item-265` (聖なる秘石) | 2/4 (50%) | 2/5 (40%) |
| `item-255` (黄金の林檎) | 1/4 (25%) | 1/5 (20%) |

| Second prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-168` | 0 | 1/9 |
| `item-264` | 2/8 (25%) | 2/9 |
| `item-150` | 1/8 (12.5%) | 1/9 |
| `item-173` | 1/8 (12.5%) | 1/9 |
| `item-207` | 1/8 (12.5%) | 1/9 |
| `armor-40` (流銀の鎧) | 1/8 (12.5%) | 1/9 |
| `weapon-40` (流銀の剣) | 1/8 (12.5%) | 1/9 |
| `item-265` (聖なる秘石) | 1/8 (12.5%) | 1/9 |

| Third prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-126` (超魔力水) | 0 | 1/9 |
| `item-244` | 1/8 (12.5%) | 1/9 |
| `item-243` | 1/8 (12.5%) | 1/9 |
| `item-199` | 1/8 (12.5%) | 1/9 |
| `item-253` | 1/8 (12.5%) | 1/9 |
| `item-254` | 1/8 (12.5%) | 1/9 |
| `item-263` | 2/8 (25%) | 2/9 |
| `item-264` | 1/8 (12.5%) | 1/9 |

**Winner selection.** Adopted Go selects uniformly from every remaining slot.
   - **Legacy execution** (`party2/lib/takarakuzi.cgi:214-245`): `rand($#players)` selects only indices 0..N−2, each with probability 1/(N−1). Removing selected entries preserves the original final slot, which remains unreachable throughout the draw. With 20 sold tickets, the last purchaser cannot win; with fewer than 20, the excluded slot is the last appended dummy, so all real purchasers remain eligible.
   - Conditional on quotas totaling M draws (4..7), an adopted Go slot has total winning probability M/20 and rank probabilities 1/20, amount2/20, amount3/20. Legacy eligible slots instead have M/19 and rank probabilities 1/19, amount2/19, amount3/19; the excluded final slot has zero for every rank. Selection is without replacement in both cases. This exclusion is intentionally not restored under #1249.
