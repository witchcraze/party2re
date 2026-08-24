# Guild System Design

## Overview

The Guild (ギルド) system enables players to form cooperative social organizations. Members contribute resources through donations to level up the guild hall, expand membership capacity, and coordinate activities under a role-based hierarchy.

## Domain Model & Roles

### Guild Hierarchy (`Role`)

- **Leader (`RoleLeader` / ギルマス)**:
  - Highest authority in the guild.
  - Can edit guild notice / messages, transfer leadership, promote/demote officers, kick members and officers, and disband the guild.
- **Officer (`RoleOfficer` / 役職者)**:
  - Can edit guild notice / messages and kick regular members.
  - Cannot kick the leader or other officers.
- **Member (`RoleMember` / メンバー)**:
  - Regular guild member.
  - Can donate gold, view guild details/members, and leave the guild.

### Invariants & Business Rules

1. **Unique Membership**:
   - A character can belong to at most one guild at any time.
   - Enforced at the database level via a primary key on `guild_members.character_id`.
2. **Guild Creation**:
   - Creation Fee: `5,000` gold (standard value from reference implementation). Deducted atomically from character wallet.
   - Unique Guild Name: 1 to 32 characters, unique across all guilds.
   - Creator immediately becomes the `RoleLeader`.
3. **Capacity & Leveling Formula**:
   - Base Capacity: `10` members at Level 1.
   - Capacity Scaling: `Capacity = 10 + (Level - 1) * 2` (up to 28 members at Level 10).
   - Experience: `1 Gold donated = 1 EXP`.
   - Level Requirement Curve:
     - Level 1: 0 EXP
     - Level $L$ ($L \ge 2$): Cumulative EXP required is $(L - 1)^2 \times 10,000$.
     - Maximum Level: 10 (`MaxLevel`).
4. **Member Lifecycle**:
   - **Join**: Character cannot be in any guild; guild must have open capacity (`len(members) < capacity`).
   - **Leave**:
     - Regular members and officers can leave at any time.
     - The leader cannot leave while other members remain unless leadership is transferred first (`ErrLeaderCannotLeaveWithMembers`).
     - If the leader is the only remaining member, leaving automatically disbands the guild.
   - **Kick**:
     - Leaders can kick officers and members.
     - Officers can kick members.
     - Members cannot kick anyone.
     - Leaders cannot be kicked.
   - **Transfer Leadership**:
     - Only the current leader can transfer leadership to another existing member.
     - The former leader becomes an officer.
   - **Disband**:
     - Leaders can disband the guild, removing all member associations.

## Persistence & Transactions

- All mutations affecting multiple tables (e.g. guild creation with fee deduction, gold donation with level progression and character wallet deduction) are performed in atomic database transactions (`*sql.Tx`).
- Foreign keys with `ON DELETE CASCADE` ensure that guild deletion cascades cleanly to member associations.
