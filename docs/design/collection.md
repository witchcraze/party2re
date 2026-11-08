# Monster Book & Collection Design

## Overview

Collections (`internal/collection`; legacy `lib/_add_monster_book.cgi`, `lib/collection.cgi`, `legend.cgi`) track defeated monsters and discovered weapons, armor and items, with completion news and hall-of-fame induction. Fields, totals and transport are owned by code and [OpenAPI](../api/openapi.json).

## Rules

- Completion targets: 180 monsters, 71 weapons (`$#weas`), 55 armors (`$#arms`) and 141 items (`$default_ites`).
- **Pitfall:** only basic items up to No. 141 count toward item completion; higher-numbered items (synthesis or expansion items such as item142, item154, item257) stay visible in the list but never count or compensate for missing basic items (`lib/collection.cgi:45–49,81–87`). Weapons and armor in the shared table never count toward the item compendium.
- A monster's first-defeat stats and rewards are an immutable snapshot; later defeats only increment the count. **Decision:** records predating the snapshot stay empty instead of fabricating stats.
- PvP opponents never enter the monster book. Sealing battles record their bosses (king99 clones as one shadow entry) with habitat 「封印戦」.

## Completion and induction

Hall-of-fame induction is mandatory; completion is marked only after induction succeeds, so a failed induction is retried on the next defeat or discovery, once and without duplicates. News is best effort and published only on the first completion. Characters who completed before this rule and lost their induction are repaired through the collection service's recovery operation, not ad hoc SQL.

Discovery and induction failures propagate out of the enclosing acquisition or battle settlement and roll it back as a whole; adding a discovery caller must keep this. Sealing battles follow the [Boss settlement](boss.md#rules-and-decisions).
