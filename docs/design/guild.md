# Guild System Design

## Overview

The Guild (ギルド) system enables players to form cooperative social organizations (`guild.cgi`, `join_guild.cgi`). In authentic Party2, there are **no guild levels, donation EXP, or capacity scaling**. Instead, guilds compete for server-wide community influence ranked by dynamic **Guild Points (`gpoint`)** accrued organically through member social, economic, and combat activities.

## Domain Model & Roles

### Guild Creation & Name Validation (`つくる`, `join_guild.cgi:246-250`)

Guild creation adheres strictly to authentic legacy constraints and shared validation security standards:

- **Creation Fee**: 5,000 Gold (`CreationFee`) deducted atomically from the creator's wallet.
- **Pre-condition**: Creator must not currently belong to or lead any guild.
- **Name Constraints**:
  - Length: 1 to 32 runes (`MaxNameLength = 32`).
  - Whitespace: ASCII whitespace (`\s`) and Japanese fullwidth spaces (`\u3000`) are rejected (both surrounding and internal).
  - Prohibited Characters: `[,;\"\'&<>\\\/@＠]` are rejected.
  - Unicode Security: Normalized via Unicode NFC; C0/C1 control characters, zero-width characters, bidirectional overrides, and Zalgo text (>2 consecutive combining marks) are rejected.
  - Uniqueness: Guild names must be unique across the server.

### Guild Authority & Membership

- **Leader (`RoleLeader` / ギルマス)**:
  - Highest and sole administrative authority in the guild.
  - Can assign custom role titles (`あたえる`), customize guild hex color (`からー`), update guild mark (`まーく`), update wallpaper (`かべがみ`), update guild notice (`めっせーじ`), approve applicants, reject/kick members (`追放`), transfer leadership, and disband the guild.
  - Default title is `ギルマス` and cannot be assigned to another member or altered via `あたえる`.
- **Member (`RoleMember` / メンバー)**:
  - Regular guild member with no administrative permissions.
  - Carries an optional player-defined custom title assigned by the leader.
- **Pending Applicant (`is_pending = true` / 参加申請中)**:
  - Prospective recruit awaiting approval by the guild master.
  - Holds temporary title `参加申請中`. Cannot send broadcast callouts or participate in guild actions until formally approved.

### Dynamic Guild Points (`gpoint`)

Guilds do not possess numeric levels or gold treasuries. Community standing is measured by cumulative Guild Points (`gpoint`) accrued through gameplay:

- **Tavern Dining (`bar.cgi`)**: +2 pt per meal consumed by a guild member.
- **Job Change (`job_change.cgi:195`)**: +50 pt per job change by a guild member.
- **Photo Contest Placements (`contest.cgi`)**: +700 pt (1st), +300 pt (2nd), +100 pt (3rd).
- **GvG Combat (`vs_guild.cgi`)**: +3 pt per round win, +match prize pool GP to tournament winner, +4 pt per participant.
- **Helper Quests (`helper.cgi`)**: +100 pt on guild-specific request completion.
- **God Wishes (`god.cgi`)**: +1,000 pt on heaven wish fulfillment (`WishGuildRank`).
- **Home & Store Construction (`_town.cgi`)**: +(`cycle_house_day * 10`) pt on residence founding, +(`cycle_store_day * 10`) pt on boutique construction.
- **Broadcast Callouts (`guild.cgi:よびかける`)**: +1 pt per member dispatch.

Guild influence rankings (`guild_list.cgi`) order all guilds by `points DESC, created_at ASC`.

### Custom Role Titles (`あたえる`)

The guild leader can assign arbitrary custom role titles to any non-leader member:

1. **Authority**: Only the guild leader can assign custom role titles (`ErrUnauthorized`).
2. **Leader Exemption**: The leader cannot assign a custom title to themselves (`ErrCannotAssignToLeader`).
3. **Length & Visual Width**:
   - Up to 6 full-width Japanese characters or 12 half-width ASCII characters (`MaxRoleTitleWidth = 12`).
   - Evaluated by display width where ASCII runes count as 1 and non-ASCII runes count as 2.
4. **Validation Rules**:
   - Cannot be empty (`ErrInvalidRoleTitle`).
   - Cannot contain half-width or full-width whitespace (`/　|\s/`).
   - Unicode Security: Validated via `validation.ValidateSingleLine` (Unicode NFC normalization, max 12 runes, rejects C0/C1 control characters, zero-width characters, bidirectional overrides, and Zalgo text). The validated NFC-normalized title is returned and persisted across role assignment and approval paths.
   - Cannot contain invalid characters (`, ; " ' & < > @ ＠`).
   - Cannot use reserved system strings: `参加申請中` or `ギルマス` (`ErrReservedRoleTitle`).

### Membership Application & Approval Workflow (`参加申請中`)

In legacy Party2, players join guilds via a formal application and approval gating process:

1. **Application (`さんか`)**:
   - A player without an existing guild membership or pending application calls `ApplyToJoin`.
   - The player is registered in `guild_members` with `is_pending = true` and title `参加申請中`.
   - A notification letter is sent to the guild master:
     `【＋参加申請＋】<GuildName> 入団希望者 <ApplicantName>`
2. **Leader Review**:
   - **Approval (`あたえる`)**: The leader assigns a valid role title via `ApproveApplication` (or `AssignCustomRole`). This clears `is_pending = false`, applies the title, and sends an acceptance letter:
     `【＋参加許可証＋】<GuildName> (ギルマス <LeaderName>) から参加許可をもらいました`
   - **Rejection (`追放`)**: The leader rejects the applicant via `RejectApplication` (or `KickMember`). The record is removed and a rejection letter is sent:
     `【＋不合格＋】残念ながら <GuildName> (ギルマス <LeaderName>) から参加を拒否されました`
3. **Member Expulsion (`追放`)**:
   - When kicking an active member, the leader removes the member and an expulsion letter is sent:
     `【＋追放＋】<GuildName> (ギルマス <LeaderName>) から追放されました`

### Broadcast Callouts (`よびかける`)

Active members can broadcast messages to all fellow guild members:

- Any active member (`!is_pending`) can invoke `BroadcastCallout` with a message (up to 200 characters). Sanitized via `internal/validation.ValidateSingleLine` (Unicode NFC normalized, single-line, rejecting control characters, zero-width characters, bidirectional overrides, and Zalgo text).
- Delivers the sanitized letter to every guild member on the roster in roster order, including pending applicants (`is_pending = true`). Pending applicants cannot send broadcast callouts themselves.
- If delivery of a letter fails, the broadcast terminates immediately and returns the delivery error without awarding Guild Points or updating `last_active_at` (earlier successful deliveries to preceding members are preserved and not rolled back).
- Upon complete successful delivery to all roster recipients, awards **+1 Guild Point (`gpoint`)** to the guild and updates `last_active_at` timestamp.

### Visual Customization: Color, Mark & Wallpaper

1. **Hex Color (`からー`)**:
   - Default: `#FFFFFF` (White). White indicates friendly status and prohibits GvG battle entry (`ErrFriendlyGuildCannotBattle`).
   - Uniqueness: Non-white colors must be unique across all active guilds (`ErrColorTaken`).
   - NPC Pink (`#FF69B4`) is prohibited.
2. **Guild Mark (`まーく`)**:
   - Only the leader can change the guild mark (`ErrUnauthorized`).
   - Fee: `3,000` Gold (`MarkChangeFee`) deducted from the leader's wallet (Rank 2).
   - Default mark is `'0'`.
3. **Guild Wallpaper (`かべがみ`)**:
   - Only the leader can change the guild hall background image (`ErrUnauthorized`).
   - Validated against the legacy wallpaper catalog (`%kabes` in `_data.cgi:330-388`).
   - Pricing ranges from `0` Gold (`none.gif`) to `50,000` Gold (`stage20.gif`), deducted atomically from the leader's wallet (Rank 2).

### Guild Master Succession & Dissolution News

#### Legacy requirements and provenance

`lib/system.cgi:1124-1183` (`delete_guild_member`) dissolves the guild when the
roster has at most one row before removal. When the master departs from a larger
roster, it prefers the first remaining row whose title contains `ギルマス`, then
falls back to the first remaining roster row. Pending applications are roster
rows (`lib/join_guild.cgi:196-214`) and are not excluded from either calculation.

The routine replaces the successor's title with `ギルマス` and updates the guild's
master record. It does not update the successor's personal guild affiliation,
unlike ordinary application approval (`lib/guild.cgi:209-239`). Including an
applicant in succession therefore does not establish complete, consistent
membership approval in the reference implementation.

The pending-row filter in `lib/join_guild.cgi:340-347` belongs to guild-name
propagation during renaming; it is not a succession or dissolution rule.

#### Approved reconstruction rule (#993; implemented in #1006)

On 2026-10-04, the user explicitly chose restoration of legacy roster-based
succession in #993: 「原典方式へ戻す（再構築方針に沿う推奨案）」. Pending rows
therefore count toward guild survival and can inherit leadership. The pending
exclusion introduced by #956 / PR #962 was replaced in #1006 with the legacy
roster-based rule.

| Remaining roster after master departure | Approved roster-based selection | Historical Go selection (#956, corrected in #1006) |
|---|---|---|
| Pending applicants only | Prefer a title containing `ギルマス`, otherwise the first row; guild survives | Dissolve the guild |
| Active members and pending applicants | Prefer a title containing `ギルマス` across all rows, otherwise the first row | Apply the same priority to active members only |
| Multiple preferred-title candidates | First matching row in roster order | First matching active row in roster order |

The ordinary application flow gives pending rows the title `参加申請中`; preferred
titles on pending rows are not produced by that flow. Selection itself does not
add an eligibility exception for such rows.

In the reconstruction, replacing an applicant's pending title with `ギルマス`
maps to `RoleLeader`, title `ギルマス`, and `is_pending = false`, using the existing
leadership transfer contract. Unselected applicants remain pending. Succession
does not invoke ordinary approval or add an acceptance letter. Go stores guild
affiliation in the membership relation and has no second personal-affiliation
field; the reference's inconsistent personal affiliation is recorded above,
rather than introducing duplicate membership state to reproduce it.

Roster priority uses the existing repository order (`joined_at ASC`, then
`character_id ASC` for timestamp ties). The reference uses file-row order.

1. **Guild Master Succession**:
   - When a Guild Master leaves the guild (`POST /guilds/{id}/leave`) or their character is deleted (`RemoveCharacterFromGuild` cleanup hook), leadership transfers to a remaining roster row, including a pending applicant.
   - **Successor Priority**:
     1. The first remaining row whose title contains `ギルマス` (e.g. `副ギルマス`, `ギルマス補佐`).
     2. If no title matches, the first remaining row in roster order.
   - **Serialized Mutation**: Roster snapshot selection, successor determination, leadership transfer, and member removal or dissolution execute within a single serialized database transaction (`s.runInTx`). `TransferLeadership` enforces atomic conditional updates (`RowsAffected`) so that stale leader attempts or non-member promotions fail fast and roll back without partial mutation.
   - Once leadership is successfully transferred, the departing leader is removed from the roster.
2. **Auto-Dissolution & Server News Announcement**:
   - Removal from a roster containing at most one row dissolves the guild. Removing a non-leader from a larger roster only removes that row. Pending rows count in both rules; the last active member's departure does not by itself imply dissolution.
   - Manual disbandment by the leader (`DELETE /guilds/{id}`) and 20-day inactivity disbandment also dissolve the guild.
   - **Post-Commit News Semantics**: Upon dissolution, a system-wide server news announcement is published via `NewsPublisher` strictly after the database transaction commits successfully:
     `ギルド『<GuildName>』が解散しました` (Category: `guild`, Author: `System`).
3. **Database Cascade Invariant**:
   - Character deletion (`character_repository.Delete`) no longer runs raw `UPDATE guilds SET leader_character_id = NULL`. Guild leadership and dissolution are handled strictly through the domain cleanup hook prior to physical character deletion, preserving the invariant that every existing guild has a valid leader.

**Implementation status**: Implemented in #1006 and hardened in #1036. `removeMemberInternal` evaluates
the entire remaining roster for both succession and dissolution without a pending filter.
Roster selection and departure mutations are fully serialized under Rank-7 row locks (`GetGuildForUpdate`),
preventing concurrent departure races that could leave orphan guilds. Unselected applicants remain pending;
sole-member departure disbands the guild and publishes server news post-commit.

### 20-Day Inactivity Automatic Disbandment (`auto_delete_guild_day = 20`)

Guilds that have had no member activity for 20 consecutive days are automatically disbanded:

- Each guild tracks `last_active_at TIMESTAMP`.
- Any guild activity (creation, member join/apply, approval, role title assignment, callout, mark/wallpaper update, notice/color update) touches `last_active_at = NOW()`.
- A daily scheduled worker (`guild_inactivity_check`, `scheduling.ActionHandler`) inspects guilds where `last_active_at < NOW() - 20 days` and cleanly disbands them, publishing the dissolution news announcement post-commit.

### Daily 20% Guild Point Decay (`login.cgi:448`)

In authentic Party2 (`party2/login.cgi:448`), during the daily maintenance cycle (`$update_cycle_day`), all guild records have their Guild Points decayed by 20% to prevent indefinite hoarding and maintain dynamic guild ranking competition:

```perl
$gpoint = int( $gpoint * 0.8 );
```

- **Formula**: `floor(current_points * factor)`. Defaults to factor `0.8` (20% decay). Guilds with 0 GP remain at 0 and do not drop below 0.
- **Scheduled Batch**: Executed automatically once daily at 00:00:00 JST via scheduled action `guild_point_decay` (`ActionTypeGuildPointDecay`).
- **Administrative Endpoint**: Can also be executed or parameterized manually via `POST /admin/guilds/decay-points`.

## Persistence & Transaction Ordering

- Guild operations obey the deterministic lock acquisition hierarchy (Rank 0 -> 8):
  - Character wallet deduction (Rank 2: `characters`) occurs before guild records (Rank 7: `guilds`, `guild_members`).
- **Membership Administration, Removal & Succession Serialization (Unit of Work)**:
  - Member departure (`Leave`), character-cleanup departure (`RemoveCharacterFromGuild`), leadership transfer (`TransferLeadership`), expulsion (`Kick`), applicant rejection (`RejectApplication`), role title assignment (`AssignCustomRole`), applicant approval (`ApproveApplication`), and manual disbandment (`Disband`) use ambient transaction propagation (`s.runInTx`).
  - Pessimistic locking (Rank 7): `GetGuildForUpdate` acquires exclusive locks (`SELECT ... FOR UPDATE`) on the guild record in `guilds` and all its member rows in `guild_members` ordered deterministically by `joined_at ASC, character_id ASC`.
  - Atomicity of administrative mutations: Current guild leadership, requester role, target membership/role and pending state are verified under the same exclusive row lock as the mutation, preventing former-leader bypasses from interleaved leadership transfers.
  - Role title assignment (`AssignCustomRole`) validates requester leadership, target membership, and non-leader target role before updating member title. When targeting a pending applicant (`is_pending = true`), it delegates to applicant approval within the same transaction boundary.
  - Applicant approval (`ApproveApplication`) validates current leadership and applicant pending state (`is_pending = true`) before clearing the pending flag and assigning the role title. Attempting to approve an active member returns `ErrMemberNotPending`. Acceptance letters (`【＋参加許可証＋】`) are attempted after successful approval, propagating ambient context so rollback leaves no orphan letter.
  - Expulsion and applicant rejection share a locked removal workflow. Current guild leadership, requester membership/role, target membership/role and pending state are checked against the locked roster before deletion. A former leader cannot remove a promoted successor. `Kick` selects expulsion versus rejection from the locked target state; `RejectApplication` rejects an already approved member with `ErrMemberNotPending`.
  - Expulsion/rejection letters retain their existing content and best-effort delivery. They are attempted only after the removal transaction succeeds. With an incoming outer SQL transaction, letter persistence joins that transaction and is rolled back with membership removal; notification visibility then waits for the outer commit. A failed removal sends no success letter.
  - Strict conditional validation (`RowsAffected`): `TransferLeadership` and `RemoveMember` verify that exactly one row was affected. Stale leader attempts return `ErrUnauthorized`, non-member successor promotions return `ErrTargetNotMember`, and missing members return `ErrCharacterNotInGuild`, rolling back the transaction with zero partial leadership mutation.
  - Dissolution Server News: Server news broadcasts occur strictly after a successful database transaction commit, preventing false news broadcasts on rolled-back transactions.
- Points increments are performed via atomic SQL arithmetic (`points = points + ?`) to avoid lock contention during concurrent gameplay achievements.
- Point decay executes via atomic SQL batch update (`UPDATE guilds SET points = FLOOR(points * ?), updated_at = ? WHERE points > 0`) wrapped in a Unit of Work transaction.

## HTTP REST API Endpoints

| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/guilds` | List guilds (paginated) | Public |
| `POST` | `/guilds` | Create new guild | Bearer Token |
| `GET` | `/guilds/{id}` | Get guild detail with roster | Public |
| `DELETE` | `/guilds/{id}` | Disband guild (leader only) | Bearer Token |
| `GET` | `/characters/{id}/guild` | Get character's current guild & membership | Public |
| `POST` | `/guilds/{id}/apply` | Apply to join guild | Bearer Token |
| `POST` | `/guilds/{id}/applications/{applicant_id}/approve` | Approve applicant & assign title (leader only) | Bearer Token |
| `POST` | `/guilds/{id}/applications/{applicant_id}/reject` | Reject applicant (leader only) | Bearer Token |
| `POST` | `/guilds/{id}/callout` | Broadcast callout message to members (+1 GP) | Bearer Token |
| `PUT` | `/guilds/{id}/members/{char_id}/title` | Assign custom role title to member (leader only) | Bearer Token |
| `PUT` | `/guilds/{id}/customization` | Update color, mark, wallpaper, or notice (leader only) | Bearer Token |
| `POST` | `/guilds/{id}/leave` | Leave guild (triggers succession if leader, or dissolution if last member) | Bearer Token |
| `DELETE` | `/guilds/{id}/members/{char_id}` | Kick member from guild (leader only) | Bearer Token |
| `POST` | `/admin/guilds/decay-points` | Batch decay guild points by 20% (Admin) | Admin Key |
