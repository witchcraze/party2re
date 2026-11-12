# Endgame Wishes & Limit Breaks Design (天界・裏天界)

The God system (`internal/god`, legacy `god.cgi` and `u_god.cgi`) grants one wish per audience in Heaven (`@神`, reached with item059) or the Underworld (`@神?`, reached with item262). The wish catalog, costs and effects are owned by `internal/god`; this document keeps decisions and non-obvious legacy findings. A granted wish returns the character home; a rejected or joke wish (`wish_lover`) mutates nothing and stays.

## Heaven wishes

- `wish_stats` is refused while limit-broken and adds to maximum stats only; current HP/MP are unchanged.
- `wish_refresh` subtracts 150 fatigue with no lower bound, so fatigue can go negative and absorb later fatigue; HP/MP are unchanged. The HP/MP restore near it in `god.cgi` belongs to `job_change` and is not reached. **Decision:** `wish_full_recovery` has no legacy origin and is rejected as an unknown wish (#1167).
- `wish_all_orbs` replaces the orb string, overwriting Ramia's awakened state, so the character must pray at the [Altar of Rebirth](altar-of-rebirth.md) again before its travel wishes unlock.
- `wish_limit_break_level` is blocked only by a temporary job memory (item243); a persistent memory (item168) does not block it (`god.cgi:121`). Level-cap effects are owned by [Progression](progression.md#celestial-overlevel-天界限界突破).
- `wish_celestial_dragon` changes to job70 through the same transition as a normal change: the departing job's SP and first-time mastery are recorded, a mastered job70's retained SP is restored, and the job history and current job move with the character ([Jobs and Skills](jobs-and-skills.md#job-changes)). The normal change's level 20, item, guild-point and weekly-ranking effects live in `job_change.cgi`'s caller and do not apply. Unlike a normal cross-job change it keeps the limit-break flag.
- **Pitfall:** the legacy wish path checks job completion as exactly `$#jobs − 1` (86) mastered jobs against the flag shared with the normal 72-job check, so it practically never fires. **Decision:** the wish does not evaluate completion; the next normal departure does, using the 72-job rule.

## Underworld wishes

Each Underworld wish raises one of `OverDepot`, `OverMonster`, `OverFuture`, `OverFlea` or `OverStore` by one tier up to 5; the capacity each tier adds is owned by the affected feature.
