# Guild System Design

## Overview

The Guild (ギルド) system enables players to form cooperative social organizations (`guild.cgi`, `join_guild.cgi`). In authentic Party2, there are **no guild levels, donation EXP, or capacity scaling**. Instead, guilds compete for server-wide community influence ranked by dynamic **Guild Points (`gpoint`)** accrued organically through member social, economic, and combat activities.

## Domain Model & Roles

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

- Any active member (`!is_pending`) can invoke `BroadcastCallout` with a message (up to 200 characters).
- Delivers a letter to every active guild member.
- Awards **+1 Guild Point (`gpoint`)** to the guild.
- Updates `last_active_at` timestamp.

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

### 20-Day Inactivity Automatic Disbandment (`auto_delete_guild_day = 20`)

Guilds that have had no member activity for 20 consecutive days are automatically disbanded:

- Each guild tracks `last_active_at TIMESTAMP`.
- Any guild activity (creation, member join/apply, approval, role title assignment, callout, mark/wallpaper update, notice/color update) touches `last_active_at = NOW()`.
- A daily scheduled worker (`guild_inactivity_check`, `scheduling.ActionHandler`) inspects guilds where `last_active_at < NOW() - 20 days` and cleanly disbands them.

## Persistence & Transaction Ordering

- Guild operations obey the deterministic lock acquisition hierarchy (Rank 0 -> 8):
  - Character wallet deduction (Rank 2: `characters`) occurs before guild records (Rank 7: `guilds`, `guild_members`).
- Points increments are performed via atomic SQL arithmetic (`points = points + ?`) to avoid lock contention during concurrent gameplay achievements.
