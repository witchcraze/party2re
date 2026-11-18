# Character Progression & Level Growth Design

## Purpose

This document defines the language-agnostic progression formulas, experience thresholds, level-up stat growth mechanics, skill point (SP) accumulation, and celestial OverLevel rules for characters.

## Initial Character State

A newly created character starts with:
- **Level**: 1
- **Experience**: 0
- **Skill Points (SP)**: 0
- **Starting Gold**: 200
- **Base Stats Range**:
  - `MaxHP`: 30–32 (HP is initialized to MaxHP)
  - `MaxMP`: 6–8 (MP is initialized to MaxMP)
  - `Attack`: 6–8
  - `Defense`: 6–8
  - `Agility`: 6–8

## Experience & Level Thresholds

Experience is cumulative. To advance from a given `level` to `level + 1`, a character's total accumulated experience must reach the threshold:

$$\text{RequiredExp}(\text{level}) = \text{level}^2 \times 10$$

### Key Rules
- Progression begins at Level 1.
- Default level cap is Level 99 (`MaxLevel = 99`).
- If the character has unlocked celestial OverLevel (`OverLevel == true`), the level cap is extended to Level 150 (`OverMaxLevel = 150`).
- Multiple levels can be gained simultaneously if awarded experience exceeds multiple cumulative thresholds.
- When maximum level (99, or 150 with OverLevel) is reached, no further level advancement occurs.

## Level-Up Growth & Orb Timing

- **Active-hand orb scope (`_battle.cgi:316,327,1747–1760`)**: Level-up orb modifiers (`item-152`–`item-157`) inspect only the active combat hand (`$ms{$m}{ite}`), not unequipped or ambiguous bag items.
- **Request-start Job-49 bound (`party.cgi:11–13`, `_data.cgi:145–153`, `_battle.cgi:314–330`)**: Job 49 (`たまねぎ剣士`) growth bounds (`$m{sp} * 0.02`) are evaluated once when `_data.cgi` loads after `read_user`, so every level gained in that request reuses the fractional request-start SP instead of recomputing after `lv_up` increments `$m{sp}`.
- **Current HP/MP preservation**: Level advancement raises `MaxHP` and `MaxMP` without restoring current `HP` or `MP` (recovery belongs to healing items and [Home](home.md)).

## Celestial OverLevel (天界限界突破)

**Pitfall:** the Heaven wishes only toggle `over_lv` (`god.cgi:121–130`); the actual stat and level limits are applied later by `system.cgi`'s `write_user` (`49–69,92–97`). Restoring the normal cap therefore reduces level and stats without resetting experience or SP.

OverLevel allows characters that have reached the celestial realm to progress beyond the standard Lv99 cap:
- **Maximum Level**: Extends from Lv99 to Lv150.
- **Job Change Reset & Preservation**: Normal cross-job transitions (`targetJobID != currentJobID`) reset the `OverLevel` flag to `false`, requiring re-ascension. Re-changing to the same job (`targetJobID == currentJobID`) and celestial Dragon Noble changes (`WishCelestialDragon` / 天竜人) preserve `OverLevel` status (legacy `party2/lib/job_change.cgi:214-226` and `party2/lib/god.cgi:196-218`).
- **Fictional Rebirth Eliminated**: The original Party2 CGI contains no reincarnation/rebirth loop. The clicker-style `RebirthCount` and rebirth stat multipliers are completely eliminated in favor of celestial OverLevel.
