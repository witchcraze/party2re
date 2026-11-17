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

`contest.cgi:18–24` lists 15,000/7,000/3,000 gold, 10/6/3 medals and 700/300/100 guild points for the top three. Gold is [held](depot.md#6-held-gold) for Depot collection, and medals are `item-023` Depot [deliveries](depot.md#delivery-capacity-by-caller).

Legacy finding (revision `ff97fc549932a146cc945d7edf71b44c868a840e`): `__send_prize` appends only 9/5/2 medals to each placed entrant's Depot (`contest.cgi:213–215`), then sends one more to `$vname`. Outside the voter loop that is the unset package variable, not the loop's lexical name, so the extra item normally reaches nobody; if it named an existing character, that character would get one per placement.

Approved decision ([#1250](https://github.com/witchcraze/party2re/issues/1250), 2026-10-18, choosing the original delivery model): placed entrants receive 9/5/2 items and each champion voter one more, adding both when eligibility overlaps (10 for a self-voting champion). Neither correct the count to the 10/6/3 table nor deliver the unresolved extra item. This does not claim 9/5/2 was the author's intent.
