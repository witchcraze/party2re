# Guild System Design

## Overview

The Guild (ギルド) system enables players to form cooperative social organizations (`guild.cgi`, `join_guild.cgi`). In authentic Party2, there are **no guild levels, donation EXP, or capacity scaling**. Instead, guilds compete for server-wide community influence ranked by dynamic **Guild Points (`gpoint`)** accrued organically through member social, economic, and combat activities.

## Domain Model & Roles

### Guild Authority & Membership

- **Leader (`RoleLeader` / ギルマス)**:
  - Highest and sole administrative authority in the guild.
  - Can assign custom role titles (`あたえる`), customize guild hex color (`からー`), update guild notice (`めっせーじ`), transfer leadership, kick members (`追放`), and disband the guild.
  - Default title is `ギルマス` and cannot be assigned to another member or altered via `あたえる`.
- **Member (`RoleMember` / メンバー)**:
  - Regular guild member with no administrative permissions.
  - Carries an optional player-defined custom title assigned by the leader.

### Dynamic Guild Points (`gpoint`)

Guilds do not possess numeric levels or gold treasuries. Community standing is measured by cumulative Guild Points (`gpoint`) accrued through gameplay:

- **Tavern Dining (`bar.cgi`)**: +2 pt per meal consumed by a guild member.
- **Photo Contest Placements (`contest.cgi`)**: +700 pt (1st), +300 pt (2nd), +100 pt (3rd).
- **GvG Combat (`vs_guild.cgi`)**: +3 pt per round win, +match prize pool GP to tournament winner, +4 pt per participant.
- **Helper Quests (`helper.cgi`)**: +100 pt on guild-specific request completion.
- **God Wishes (`god.cgi`)**: +1,000 pt on heaven wish fulfillment (`WishGuildRank`).
- **Home & Store Construction (`_town.cgi`)**: +(`cycle_house_day * 10`) pt on residence founding, +(`cycle_store_day * 10`) pt on boutique construction.
- **Broadcast Callouts (`guild.cgi:よびかける`, Part 2)**: +1 pt per server-wide member dispatch.

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

### Guild Color Customization (`からー`)

Each guild possesses a configurable 6-digit hex color code (`#RRGGBB`):

1. **Default Color**: `#FFFFFF` (White). Newly established guilds start with white. White designates friendly status and prohibits GvG battle entry (`ErrFriendlyGuildCannotBattle`). Multiple guilds may share the default white color.
2. **Uniqueness**: Any non-white custom color must be strictly unique across all active guilds on the server (`ErrColorTaken`).
3. **NPC Prohibition**: The reserved NPC pink color (`#FF69B4`) is prohibited (`ErrColorTaken`).
4. **Authority**: Only the guild leader can change the guild color.

### Invariants & Business Rules

1. **Unique Membership**:
   - A character can belong to at most one guild at any time (`guild_members` primary key on `character_id`).
2. **Guild Creation (`つくる`)**:
   - Creation Fee: `5,000` gold deducted atomically from founder wallet.
   - Unique Guild Name: 1 to 32 characters, unique server-wide.
   - Founder automatically assumes `RoleLeader` with title `ギルマス`, initial points `0`, and color `#FFFFFF`.
3. **Member Capacity**:
   - Uncapped in authentic Party2 specification (no fictional leveling caps).
4. **Member Lifecycle**:
   - **Join**: Character must not be in any guild.
   - **Leave (`だったい`)**: Regular members can leave at any time. The leader cannot leave while other members remain unless leadership is transferred first (`ErrLeaderCannotLeaveWithMembers`).
   - **Kick (`追放`)**: Only the leader can kick members. Leaders cannot be kicked.
   - **Transfer Leadership**: Leader transfers leadership to an existing member. The former leader becomes `RoleMember` (title reset), and the new leader becomes `RoleLeader` (title `ギルマス`).
   - **Disband (`かいさん`)**: Only the leader can disband the guild (or automatic disband when the sole member leaves). Deletion cascades to all member records.

## Persistence & Transaction Ordering

- Guild operations obey the deterministic lock acquisition hierarchy (Rank 0 -> 8):
  - Character wallet deduction (Rank 2: `characters`) occurs before guild records (Rank 7: `guilds`, `guild_members`).
- Points increments are performed via atomic SQL arithmetic (`points = points + ?`) to avoid lock contention during concurrent gameplay achievements.
