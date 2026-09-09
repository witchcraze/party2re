# Character Resting and Recovery (睡眠・回復)

## Purpose

This document defines the rules and specifications for character resting and recovery in Party2 reconstruction (`party2re`).
In original Party2 (`lib/sleep.cgi`, `home.cgi`), character resting is performed at Home (either one's own private home or a visited player's home) without monetary cost, governed by an online-scaled sleep countdown timer. The fictional paid "Inn" (`internal/inn`) has been decommissioned.

## Core Mechanics

### 1. Free Home Sleeping (`POST /characters/{id}/home/sleep`)

Characters can go to sleep in their own private home or in a visited player's home:
- **Cost**: Completely free (0 Gold).
- **Target Home**: Own house by default, or an existing player's home via `target_home_id`.
- **Duration Scaling**: Sleep duration is dynamically scaled by concurrent logged-in player headcount to throttle load:
  - Headcount < 20: 1x base duration (default 60 seconds)
  - Headcount >= 20: 2x base duration (120 seconds)
  - Headcount >= 30: 3x base duration (180 seconds)
- **Timer & Storage**:
  - `party2:timer:sleep:<character_id>`: Ephemeral lock with native TTL equal to the sleep duration.
  - `party2:timer:asleep:<character_id>`: Ephemeral pending wake flag with 24-hour TTL.
- **Job Memory Reversion**: If a character is currently operating under a temporary Job Memory (e.g. from future job recall), it is immediately reverted upon entering sleep.

### 2. Action Lock During Sleep

While `party2:timer:sleep:<character_id>` is active:
- The character cannot initiate new actions (adventures, battles, etc.).
- The system returns HTTP `409 Conflict` with the message:
  `"お休み中「Zzz...」 目覚めるまで X分YY秒"`
- Status can be queried anytime via `GET /characters/{id}/home/sleep`.

### 3. Awakening and Full Recovery (`POST /characters/{id}/home/wake`)

Once the sleep duration timer expires (`party2:timer:sleep:<character_id>` has lapsed):
- The player wakes up their character.
- **Full Restoration**:
  - `HP` restored to `MaxHP`.
  - `MP` restored to `MaxMP`.
  - `Tired` (疲労度) reset to `0`.
  - Temporary job memory reverted.
- **Cross-Domain Reset Hooks**:
  - **Tavern**: Eating fullness state reset (`tavern.ResetFullness`).
  - **Chapel**: Daily prayer and active blessing cleared (`chapel.ClearBlessing`), allowing new daily prayers.
- The pending wake flag `party2:timer:asleep:<character_id>` is released.
- Subsequent wake requests while awake are idempotent.

## Deprecation Notice

- Fictional **Inn** (`internal/inn` and `POST /characters/{id}/inn`) was an unverified divergence charging gold per level. It has been completely decommissioned and removed in Issue #459.

## Related Documents

- [`docs/design/home.md`](home.md)
- [`docs/architecture/valkey-keyspace.md`](../architecture/valkey-keyspace.md)
- [`docs/migration/legacy-cgi-mapping.md`](../migration/legacy-cgi-mapping.md)
