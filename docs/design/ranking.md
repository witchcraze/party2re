# Rankings Design

## Overview

Rankings (`internal/ranking`; legacy `ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`, `legend.cgi`) cover leaderboards, the hall of fame and the weekly job-change ranking. Categories, ordering, pagination and caching are owned by code and [OpenAPI](../api/openapi.json); schemas by the migrations.

## `ranking.cgi` boards

**Decision (#2540): legacy board content.** Each of the seven boards holds only the top 10 characters; `kill_m` skips the characters on the `kill_p` board and `hero_c` skips those on the `mao_c` board so one leader never occupies both boards of a pair (with ten or fewer characters, `kill_m` and `hero_c` are empty). Legacy ties follow unspecified directory-read order, so Go's tie order is a native choice. Each exclusion pair is published atomically from one leader query, so single-board administrative refreshes (`POST /rankings/refresh`) update the whole pair together (`weekly_job_change` keeps its separate rotation owner).

**Decision (#2801): daily JST midnight publication.** Legacy `party2/login.cgi:11,112–114` republishes the boards on login when `player_list.html` is at least one day old (`$update_cycle_day = 1`) and `ranking.cgi` reads only those files. Go deliberately departs from the login-driven trigger by publishing once a day at **00:00 JST** via `party2:ranking:refresh` (reconciling missing or pre-boundary snapshots on startup/recovery). Public reads return only published snapshots (empty before initial publication) and never query live counters.

## Weekly job-change ranking

**Legacy timing.** `party2/login.cgi:14,112–120` rotates on a login when `log/week_ranking.cgi` is at least seven elapsed days old, and only when neither daily maintenance branch runs; `week_ranking.cgi` reads only that published file. There is no calendar anchor, and delayed logins or maintenance extend the period beyond seven days.

**Decision (#1252, 2026-10-11): JST Sunday midnight with one consolidated delayed rotation.** This deliberately departs from the drifting login-dependent rule; scheduling seven days after each actual rotation was considered and rejected.

- Weeks are `[Sunday 00:00 JST, next Sunday 00:00 JST)`, independent of host or player timezone.
- The effective cutoff is when the rotation takes its exclusive lock, not individual event times; writes admitted before it close in the old week. Exact event-time partitioning is not claimed.
- After downtime, all active counts are finalized once at recovery, including writes after the missed boundary. Separate results for missed weeks are never fabricated, and the next rotation is the first Sunday strictly after recovery.
- Reads return only the finalized previous-week snapshot (empty before the first rotation; #2801).

## Job popularity

**Decisions:** Past job changes cannot be reconstructed, so popularity starts at zero and accumulates only from changes after migration 097. Unlike legacy `job_ranking.cgi` (updated immediately on job change because its per-job log is small), Go publishes `job_popularity` in the same daily **00:00 JST** snapshot (`#2801`) so no public ranking endpoint exposes live reads.

## Hall of fame

- **Known gap ([#1837](https://github.com/witchcraze/party2re/issues/1837), [#1924](https://github.com/witchcraze/party2re/issues/1924)):** Temporary costume and God-wish appearance overrides are not yet applied to Hall of Fame icons until appearance persistence is unified.
