# Rankings Design

## Overview

Rankings (`internal/ranking`; legacy `ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`, `legend.cgi`) cover leaderboards, the hall of fame and the weekly job-change ranking. Categories, ordering, pagination and caching are owned by code and [OpenAPI](../api/openapi.json); schemas by the migrations.

## Weekly job-change ranking

**Legacy timing.** `party2/login.cgi:14,112–120` rotates on a login when `log/week_ranking.cgi` is at least seven elapsed days old, and only when neither daily maintenance branch runs; `login.cgi:482–487` publishes and clears the active counts; `week_ranking.cgi:17–36` reads them. There is no calendar anchor, and delayed logins or maintenance extend the period beyond seven days.

**Decision (#1252, 2026-10-11): JST Sunday midnight with one consolidated delayed rotation.** This deliberately departs from the drifting login-dependent rule; scheduling seven days after each actual rotation was considered and rejected.

- Weeks are `[Sunday 00:00 JST, next Sunday 00:00 JST)`, independent of host or player timezone.
- The effective cutoff is when the rotation takes its exclusive lock, not individual event times; writes admitted before it close in the old week. Exact event-time partitioning is not claimed.
- After downtime, all active counts are finalized once at recovery, including writes after the missed boundary. Separate results for missed weeks are never fabricated, and the next rotation is the first Sunday strictly after recovery.
- A restart, duplicate action or retry for a settled boundary never rotates again. A failed rotation keeps its counts and the previous snapshot. A rotation with empty counts keeps a nonempty previous snapshot and still advances the deadline. The atomic rotation and writer isolation of #1242 apply.

## Job popularity

Each job change credits floor(previous level × 0.5) points to the **previous** job and the character's gender (`lib/job_change.cgi:319`), inside the job-change transaction. **Decision:** past job changes cannot be reconstructed, so popularity starts at zero and accumulates only from changes after migration 097.

## Hall of fame

Completion of all jobs, the monster book, the weapon, armor and item encyclopedias, or all alchemy recipes inducts a character permanently and once per category, in induction order.
