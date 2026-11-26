# Photo Contest Design

## Overview

Photo contests (`internal/contest`; legacy `photo.cgi`, `contest.cgi`, `screen_shot.cgi`) cover screenshots, entries, voting, settlement with prizes, and the hall of fame. Validation, limits, and transport are owned by code and [OpenAPI](../api/openapi.json).

## Round lifecycle

- **Postponement origin:** a postponed deadline is the **current check time** plus 10 days rather than the old end time (`contest.cgi:256`), preventing delayed settlements from scheduling deadlines in the past.
- **Under-quorum termination:** an active round with 1–4 entries postpones alongside an under-quorum preparing round, but settles and awards entrants while skipping the hall of fame (`contest.cgi:186`) once the preparing round reaches quorum, so entries are never postponed forever.

## Prizes

Gold awards are [held](depot.md#6-held-gold) for Depot collection, and medal awards are `item-023` Depot [deliveries](depot.md#delivery-capacity-by-caller) (tier amounts live in `internal/contest/prize.go`).

- **Legacy finding** (`contest.cgi:18–24, 203–219`): `@prizes` lists `10/6/3` medals for 1st–3rd place (counts not shown to players in `contest.cgi` or `lib/photo.cgi`). However, `__send_prize` appends only `$prizes[$count-1][0] - 1` (`9/5/2`) medals directly to `depot.cgi` (`213–215`) and calls `&send_item($vname, 3, 23, 'フォトコンテスト')` at line `218` outside the lexical `for my $vname` voter loop (`204–207`) without `use strict`. Because `vname` appears nowhere else in `party2/`, package-global `$vname` is unconditionally `undef` (a typo for `$name`, not an active recipient), so the extra item reaches nobody and actual placement payouts are `9/5/2`.
- **Approved decision** ([#1250](https://github.com/witchcraze/party2re/issues/1250), 2026-10-18): retain the legacy delivery model and actual payouts (`9/5/2` items to placed entrants and `1` item to each champion voter, additive when overlapping—`10` for a self-voting champion), without substituting the `10/6/3` table or replicating the dead `$vname` call.
