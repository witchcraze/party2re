# Guild System Design

## Overview

Guilds (`guild.cgi`, `join_guild.cgi`) are social organizations ranked by Guild Points. Legacy Party2 has **no guild levels, donation EXP or capacity scaling**. Fees, name and title validation, point awards and transport are owned by code and [OpenAPI](../api/openapi.json). Renaming (`＠かいめい`) is commented out of the legacy dispatch (`lib/join_guild.cgi:54,63`), so it is unreachable and not reconstructed.

## Membership

When an applicant has joined another guild, `ataeru` drops the pending row before reaching the approval or rejection branch (`lib/guild.cgi:209–232,251–257`). Go retains the row on approval and sends an ordinary rejection letter on rejection. [#2966](https://github.com/witchcraze/party2re/issues/2966)

### Broadcast Callouts (`よびかける`)

Letters go to every roster row in order, **including pending applicants** and the sender, under the shared [letter rules](home.md#letter-settings). A throttled or failed recipient stops the broadcast (`lib/guild.cgi:50–57`); earlier deliveries stay, and no point is awarded or activity recorded.

### Guild Master Succession & Dissolution News

#### Legacy requirements and provenance

`lib/system.cgi:1124–1183` (`delete_guild_member`) dissolves the guild when the roster has at most one row before removal. When the master leaves a larger roster, it prefers the first remaining row whose title contains `ギルマス`, otherwise the first remaining row. Pending applications are roster rows (`lib/join_guild.cgi:196–214`) and count in both calculations. **Pitfall:** the routine sets the successor's title and the guild's master record but not the successor's personal affiliation, unlike approval (`lib/guild.cgi:209–239`). The pending filter in `lib/join_guild.cgi:340–347` belongs to renaming, not succession.

#### Approved reconstruction rule (#993)

**Decision (#993, 2026-10-04):** the user chose to restore the legacy roster-based rule (「原典方式へ戻す（再構築方針に沿う推奨案）」). Pending rows count toward survival and can inherit leadership; a selected applicant becomes leader with title `ギルマス` and is no longer pending, without an acceptance letter, while unselected applicants stay pending. Go keeps affiliation only in the membership relation, so the legacy personal-affiliation inconsistency is not reproduced. Roster order is `joined_at`, then character ID (the legacy uses file-row order). A candidate who already joined another guild is skipped; if none remains, the guild dissolves.

Dissolution happens on removal from a roster of at most one row, on manual disbandment and after 20 days of inactivity; the last active member leaving does not by itself dissolve a guild with pending rows. Dissolution news (`ギルド『<GuildName>』が解散しました`) appears only if the dissolution commits: removal writes it in the same transaction, and manual or inactivity disbandment publishes it after commit.

## Decorations

**Reconstruction decision:** decorative NPC entries belong to guild display storage, separately from the player roster, because legacy `log_member.cgi` mixes players and display objects. Decorations confer no affiliation or authority and do not participate in player capacity or leadership succession.

## Inactivity and point decay

- Guilds with no member activity for 20 days (`auto_delete_guild_day = 20`) are disbanded; inactivity is rechecked under the guild lock before deletion. Legacy measures the age of the room's member list (`lib/join_guild.cgi:423–433`), which every request inside the guild room rewrites, so presence alone keeps a guild alive. Go therefore counts every command a member sends from the guild room, as well as guild mutations.
- Daily maintenance decays points and, in the same step, records the public guild list (`guild_list.cgi`) that stays unchanged until the next run (`login.cgi` `write_guild_list_html`). **Decision:** the list remains this daily snapshot, as legacy players saw it, not a live read. Legacy runs the step on the first login at least a day after the previous run, and skips it when the player list is due; Go runs it with the scheduled daily decay.

## Operator policy for duplicate affiliations

If historical concurrency bugs left a character with several active affiliations, an operator decides the true one and demotes or removes the others. Automatic guild deletion or confiscation of points or assets is prohibited.
