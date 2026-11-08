# Event Plaza Design

## Overview

The Event Plaza (`internal/eventplaza`, legacy `lib/event.cgi`) is the town plaza with its traveling merchant and the crowd added after a king sealing. Presence uses the ordinary 5-minute window (`party2/config.cgi_origin:81–82`). Catalog, prices and transport are owned by code and [OpenAPI](../api/openapi.json).

## Merchant

- The merchant's catalog depends on 10/20/30 plaza members; a higher tier **replaces** the lower catalog (`@sales` is reassigned), and items cost three times their base price (`$ites[$i][2] *= 3`).
- **Pitfall:** the tier is selected from the raw stored rows **before** presence cleanup, then the merchant row is added; the displayed headcount is computed afterward, so a later request can select a different tier (see below).
- Items requested by ongoing helper quests (`get_helper_item(3)`) are not listed or sold.
- A purchase goes to the hand when the consumable hand is empty and the quantity is 1, otherwise to the Depot, past capacity if needed (`event.cgi:94`; [delivery capacity](depot.md#delivery-capacity-by-caller)).

## 3. Victory Celebrations: Legacy Contract and Implementation Gaps

#### Legacy evidence

Paths below are relative to the original `party2/` tree; the local reference copy is `party2-main/party2/`.

- **Trigger**: `lib/vs_king.cgi:45–74` calls `_win_vs_king.cgi` after successful `ふういん`, alongside the victory news. The crowd is a sealing side effect, not a separately claimable reward.
- **Generation**: `lib/_win_vs_king.cgi:5–11,17–54,56–71` appends 2–5 regular guests and 1–2 props per victory, independently and uniformly selected within each range. The total is 3–7 rows, with mean 5 and probabilities `1/8, 2/8, 2/8, 2/8, 1/8`; it is not a uniform draw from 3–7. Regular guests draw from 65 candidate slots (`@c_randoms`) mapping to 64 unique identities, where 0-based slots 60 and 64 share the duplicate name `ジェイ` (collision probability 67/4225 for two independent draws). Props draw from 32 candidate slots (`@c_fixeds`) mapping to 31 unique identities, where 0-based slots 20 and 24 share the duplicate name `クマチャン` (collision probability 34/1024 for two independent draws). Boss tier does not affect the counts. Successive victories append to existing records.
- **Time marker**: Each added row independently receives a future timestamp `victory_time + 3600 + U{0..3599}` seconds. The initial two-hour setting is decremented before use; it does not give every guest a two-hour lifetime.
- **Actual retention**: `config.cgi_origin:81–82` and `lib/system.cgi:475–530` apply the ordinary 300-second presence window to those future timestamps. A row remains at `now == marker + 300` and is removed when `now > marker + 300`, on the next cleanup. Thus the nominal future marker lies in `[1h, 2h)`, while the cleanup boundary lies in `[65min, 125min)`. Same-name rows are reduced to the first retained row during this cleanup, so generated rows are not guaranteed distinct displayed attendees.
- **Merchant ordering**: `party.cgi:17–20` loads `lib/event.cgi` before `read_member`. `lib/event.cgi:14–56` counts the raw stored rows, including expired or duplicate rows not yet cleaned, selects the 10/20/30 catalog, and appends the merchant's own row when present. Cleanup and display follow. A later request can therefore select a different tier after cleanup; merchant-row presence can also affect the next count. These effects must not be silently replaced by a cleaned-count rule.
- **No toast or separate banquet lifetime**: The only event-specific registered action is `かう` (`lib/event.cgi:76–115`). There is no toast, toast Gold, independent 24-hour banquet record, or separate 24-hour display/claim window in this path.

#### Adopted rule and rationale

Decision: [#1248](https://github.com/witchcraze/party2re/issues/1248). The maintainer explicitly directed: 「できるだけ原典の仕様に合わせる方向でお願いします」. Adopt the observed legacy behavior above, including its retention grace, duplicate handling, candidate slot weights, and merchant-selection order. Generic identities (`Celebration guest %02d` across 65 candidate slots / 64 identities with duplicate slots 60 and 64; `Celebration decoration %02d` across 32 candidate slots / 31 identities with duplicate slots 20 and 24) replace legacy proper names and image assets without adding new catalogs, configuration, or bootstrap infrastructure. Repeated duplicate slot draws retain their authentic double weight in initial stored counts and collapse to the first retained row during subsequent observation cleanup, preventing premature merchant tier progression. No small-server 10/20/30-person boost, strict one-hour expiry, or other gameplay departure is approved by this decision. Existing Go tests and closed #634's incorrect attribution are not approval evidence; #161's victory-log requirement does not authorize toast rewards.

The celebration is represented by the remaining plaza crowd. There is no independent banquet display period to keep at 24 hours or reset to a fixed two hours. Remove the separate banquet listing, toast action and its Gold reward entirely; keeping a zero-Gold toast would still retain an action absent from the original. Preserve the sealing announcement and the crowd hook. Do not claw back previously awarded Gold or destroy historical records as part of retiring their runtime use.


Historical `celebration_banquets` and `banquet_toasts` records and previously awarded toast gold are preserved; nothing reads, lists or claims them at runtime.
