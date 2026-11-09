# Rescue & Helper Quest Design

## Overview

Helper quests (`helper.cgi`) are a request board where players turn in Depot items or ranch monsters for rewards; emergency rescue (`rescue.cgi`) recovers a character stuck in an activity. Candidate pools, reward weights, transport and storage are owned by code, [OpenAPI](../api/openapi.json) and migrations. Evidence: `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

## Helper quests

- Candidate ranges and reward weights follow `lib/helper.cgi:22–44,154–211`; repeated reward slots keep their weight. **Pitfall:** a monster target draws its primary slot first; a missing normal monster then draws uniformly from 1–3 and a missing rare monster from 160–165. Primary slots are not filtered or replaced with another pool. The native catalog has no 500–579 monsters yet, so rare monster requests always use that fallback.
- Item turn-ins come from the **Depot** (`check_depot`), monsters from the ranch box (`check_farm`); rewards go to the Depot, past capacity if needed ([delivery capacity](depot.md#delivery-capacity-by-caller)).

### Lifecycle and Expiration

- Requests expire after 6 days, strictly (`now > expires_at`).
- **Pitfall:** `kaiketsu` (`lib/helper.cgi:92–145`) checks materials and conditions **before** the deadline, so an eligible player can still complete a target after it expires.
- Completing a target also exchanges every expired non-target request one for one. Submitting an empty target (legacy `かいけつ` without arguments) exchanges all expired requests without materials, rewards, help count or guild points.
- Targets of stored uncompleted requests are not sold in shops, also after their deadline (`system.cgi:1424–1438`, `get_helper_item`), until no uncompleted request needs them.
- Each request is completed by one character only; completion, replacement and exchanges commit together.

### Initial supply (#1226)

**Decision (#1226):** following 「原点に合わせてね」, automatic initial supply is **zero**: no startup seeding, minimum board size, periodic top-up or read-triggered generation. The recovered container (`docker-entrypoint.sh:7–10`) creates an empty `helper_quest.cgi`, and `lib/helper.cgi:60–86,92–148` only generates replacements when resolving or renewing existing requests, so an empty board stays empty. Historical live-server seed data is unknown; any automated provisioning needs its own decision.

## Emergency rescue

`rescue.cgi:17–21,53–70` clears saved activity and adds sleep; an idle player gets no penalty. The reconstruction adds 600 seconds to any remaining sleep and never shortens it (`rescue.cgi:58–59`); with nothing to clear there is no penalty, timer or record (`rescue.cgi:56–65`). Rescue is allowed during sleep.

**Pitfall:** legacy `refresh_player` empties only the screen module `$m{lib}`; quest/room membership (`$m{quest}`) and its files survive, so rescue never ended activities.

**Decision (#1099):** rescue (player and admin unstuck) clears only scheduled actions. Before any change it reads the same activity facts as the character context (adventure, party, PvP including SQL claims without a live room, GvG, dungeon, challenge, casino); if any remain it returns `RESCUE_ACTIVITY_REMAINING` (admin: 409) and changes nothing. Those activities end only through their owners' commands; rescue adds no forced exit or settlement rule. A PvP claim left after live-room loss has no owner exit yet (#2012).

Open: cleanup, record and timer are not atomic, and a failed sleep read can overwrite existing sleep (#1100).
