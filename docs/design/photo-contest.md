# Photo Contest Design

## Overview

Photo contests (`internal/contest`; legacy `photo.cgi`, `contest.cgi`, `screen_shot.cgi`) cover screenshots, entries, voting, settlement with prizes and the hall of fame. Limits, validation and transport are owned by code and [OpenAPI](../api/openapi.json).

## Round lifecycle

- A preparing round becomes active only with at least 5 entries at a settlement check; otherwise entries are kept and the deadline moves 10 days.
- **Postponement origin:** a postponed deadline is the **current check time** plus 10 days, never the old end time (`contest.cgi:255–264,294–300`: `$end_time = $time + 24*60*60*$contest_cycle_day`). This also keeps a delayed settlement from scheduling a deadline in the past.
- An active round with 1–4 entries is postponed together with an under-quorum preparing round. If the preparing round has reached quorum, the active round settles and awards its entrants but skips the hall of fame (`contest.cgi:186`), so entries are never postponed forever.
- A character may not enter the preparing round while it has an entry in the active round. Self-voting is allowed (`lib/photo.cgi:touhyou`), one vote per character per round.
- Guild points go to the guild captured when the entry was submitted, never to a later guild or another guild with the same name; results show the entry-time guild name.

## Prizes

`contest.cgi:18–24` lists 15,000/7,000/3,000 gold, 10/6/3 medals and 700/300/100 guild points for the top three; every voter for the winner gets a medal bonus.

#### Legacy delivery evidence

At legacy revision `ff97fc549932a146cc945d7edf71b44c868a840e`:

| Reward | Original execution | Current Go behavior |
| --- | --- | --- |
| Winner Gold | `contest.cgi:220–221` calls `lib/system.cgi:842–855` to append 15,000 / 7,000 / 3,000 G to the winner's pending remittances. Depot withdrawal receives them (`lib/depot.cgi:208–215,290–307`). | `SettleContest` credits the winner's wallet immediately. Excess over `MaxMoney` is clamped away. |
| Winner medals | `contest.cgi:210–219` appends 9 / 5 / 2 items No.23 directly to each winner's Depot, then attempts one further delivery using an out-of-scope `$vname`. | Credits 10 / 6 / 3 directly to each winner's Small Medals counter. |
| Champion voter bonus | `contest.cgi:203–207` sends one item No.23 to each named voter for the first-place entry, including the winner's own vote. | Credits one Small Medal directly to each champion voter's counter. |

The voter loop's lexical `$vname` is not the package variable used by the later
winner delivery. With that package variable unset, the extra medal normally has
no recipient; with it set to another existing character, that character receives
one extra item per placed entrant. It is not necessarily the last voter. The
9 / 5 / 2 direct append (`213–215`) and the 10 / 6 / 3 prize table must not be
conflated.

Isolated execution of the original reward and delivery routines confirmed the
unset/set package-variable cases, self-vote bonus and pending Gold. This does
not establish a full CGI run under the unavailable operational configuration.
Both the direct winner append and `send_item` (`system.cgi:818–837`) omit a
capacity check; contest Gold delivery also has no Depot-full precondition.

#### Approved delivery contract and implementation gap

In [#1250](https://github.com/witchcraze/party2re/issues/1250), on 2026-10-18 the
user explicitly selected the original delivery model and directed that the
specification remain as close to the original as possible. On that basis, the
contract preserves the verified delivery and winner counts:

- Gold is a pending claim belonging to the placed entrant, not immediate wallet
  income. Reuse the pending-remittance/receipt contract owned by
  [#1109](https://github.com/witchcraze/party2re/issues/1109); preserve unpaid Gold
  at the wallet cap and on failures/retries. Contest awards do not inherit the
  player-to-player sender's Depot-full rejection.
- Medals are `item-023` delivered to each recipient's Depot, not direct counter
  increments. Merge an existing eligible stack, or append a new slot even when
  full or already over capacity. This matches the shared
  [delivery capacity rule](depot.md#delivery-capacity-by-caller) (`Depot.Deliver`).
- Settlement must durably create all awards and commit the round transition
  atomically. Storage errors, retry and concurrent settlement/receipt must not
  lose or duplicate awards. A separate delivery queue is not implied.
- Each placed entrant receives **9 / 5 / 2 medal items**, respectively. Each
  champion voter receives **one additional item**; overlapping eligibility adds
  both awards (10 for a self-voting champion). Preserve the verified direct
  append count rather than silently correcting it to the 10 / 6 / 3 prize
  table. Do not deliver the extra item to an unresolved package-global name.

This deliberately excludes erroneous extra delivery when a package-global
`$vname` happens to identify an existing character. It does not assert that
9 / 5 / 2 was the original author's intent, or that 10 / 6 / 3 was actually
delivered. Increasing the winner awards to the prize table is not approved.

Open (#1496): settlement still credits the wallet and medal counter directly; pending remittances (adopting #1109) and the contest item delivery are not implemented.
