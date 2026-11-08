# Guild System Design

## Overview

Guilds (`guild.cgi`, `join_guild.cgi`) are social organizations ranked by Guild Points. Legacy Party2 has **no guild levels, donation EXP or capacity scaling**. Fees, name and title validation, point awards and transport are owned by code and [OpenAPI](../api/openapi.json).

## Membership

- Applications are guild roster rows with title `参加申請中` (`lib/join_guild.cgi:182–219`). An unaffiliated character may apply to several guilds; an active member may neither apply nor create a guild. An applicant may still create a guild and becomes its leader. Pending rows earn no action points (`lib/guild.cgi:63`).
- Approval rechecks that the applicant has not joined another guild in the meantime (`lib/guild.cgi:220–224`). A character holds at most one active affiliation, also under concurrent creation, joining and approval.
- The leader cannot be given a custom title, and the reserved titles `参加申請中` and `ギルマス` cannot be assigned. Title width counts ASCII as 1 and other characters as 2, up to 12.
- White (`#FFFFFF`) is the default color and marks a friendly guild that cannot enter GvG; other colors are unique, and NPC pink (`#FF69B4`) is prohibited.

### Broadcast Callouts (`よびかける`)

Any active member may call out; letters go to every roster row in order, **including pending applicants**, and the sender receives a copy. Letters use the shared [throttle and length rules](home.md#letter-settings). A throttled or failed recipient stops the broadcast (`lib/guild.cgi:50–57`); earlier deliveries stay, and no point is awarded or activity recorded. Only complete delivery awards 1 point.

### Guild Master Succession & Dissolution News

#### Legacy requirements and provenance

`lib/system.cgi:1124–1183` (`delete_guild_member`) dissolves the guild when the roster has at most one row before removal. When the master leaves a larger roster, it prefers the first remaining row whose title contains `ギルマス`, otherwise the first remaining row. Pending applications are roster rows (`lib/join_guild.cgi:196–214`) and count in both calculations. **Pitfall:** the routine sets the successor's title and the guild's master record but not the successor's personal affiliation, unlike approval (`lib/guild.cgi:209–239`). The pending filter in `lib/join_guild.cgi:340–347` belongs to renaming, not succession.

#### Approved reconstruction rule (#993)

**Decision (#993, 2026-10-04):** the user chose to restore the legacy roster-based rule (「原典方式へ戻す（再構築方針に沿う推奨案）」). Pending rows count toward survival and can inherit leadership; a selected applicant becomes leader with title `ギルマス` and is no longer pending, without an acceptance letter, while unselected applicants stay pending. Go keeps affiliation only in the membership relation, so the legacy personal-affiliation inconsistency is not reproduced. Roster order is `joined_at`, then character ID (the legacy uses file-row order). A candidate who already joined another guild is skipped; if none remains, the guild dissolves.

Dissolution happens on removal from a roster of at most one row, on manual disbandment and after 20 days of inactivity; the last active member leaving does not by itself dissolve a guild with pending rows. Dissolution news (`ギルド『<GuildName>』が解散しました`) is published only after commit.

## Inactivity and point decay

- Guilds with no member activity for 20 days (`auto_delete_guild_day = 20`) are disbanded; inactivity is rechecked under the guild lock before deletion.
- Daily maintenance decays points: `$gpoint = int($gpoint * 0.8)` (`login.cgi:448`).

## Operator policy for duplicate affiliations

If historical concurrency bugs left a character with several active affiliations, an operator decides the true one and demotes or removes the others. Automatic guild deletion or confiscation of points or assets is prohibited.
