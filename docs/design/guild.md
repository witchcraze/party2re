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
- Delivers the sanitized letter to every active guild member.
- If delivery of a letter fails, the broadcast terminates immediately and returns the delivery error without awarding Guild Points or updating `last_active_at` (earlier successful deliveries to preceding members are preserved and not rolled back).
- Upon complete successful delivery to all active members, awards **+1 Guild Point (`gpoint`)** to the guild and updates `last_active_at` timestamp.

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

### Guild Master Succession & Dissolution News (`system.cgi:1124-1183`, `join_guild.cgi:367-436`)

1. **Guild Master Succession**:
   - When a Guild Master leaves the guild (`POST /guilds/{id}/leave`) or their character is deleted (`RemoveCharacterFromGuild` cleanup hook), leadership is automatically transferred to another active member if other active members remain.
   - Pending applicants (`is_pending == true`) are strictly excluded from leadership succession candidates.
   - **Successor Priority**:
     1. An active member (`!is_pending`) whose custom title contains `ギルマス` (e.g. `副ギルマス`, `ギルマス補佐`).
     2. If no member matches, the next eligible active member in the roster (by order of joining).
   - Once leadership is successfully transferred, the departing leader is removed from the roster.
2. **Auto-Dissolution & Server News Announcement**:
   - When the last active member of a guild leaves or is deleted (including when only the leader and pending applicants exist, or when the guild is empty), or when the leader manually disbands the guild (`DELETE /guilds/{id}`), or upon 20-day inactivity auto-disbandment, the guild is dissolved. Pending applicants cannot maintain or inherit a leaderless guild.
   - Upon dissolution, a system-wide server news announcement is published via `NewsPublisher`:
     `ギルド『<GuildName>』が解散しました` (Category: `guild`, Author: `System`).
3. **Database Cascade Invariant**:
   - Character deletion (`character_repository.Delete`) no longer runs raw `UPDATE guilds SET leader_character_id = NULL`. Guild leadership and dissolution are handled strictly through the domain cleanup hook prior to physical character deletion, preserving the invariant that every existing guild has a valid leader.

### 20-Day Inactivity Automatic Disbandment (`auto_delete_guild_day = 20`)

Guilds that have had no member activity for 20 consecutive days are automatically disbanded:

- Each guild tracks `last_active_at TIMESTAMP`.
- Any guild activity (creation, member join/apply, approval, role title assignment, callout, mark/wallpaper update, notice/color update) touches `last_active_at = NOW()`.
- A daily scheduled worker (`guild_inactivity_check`, `scheduling.ActionHandler`) inspects guilds where `last_active_at < NOW() - 20 days` and cleanly disbands them, publishing the dissolution news announcement.

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



