# Player and Character Deletion Specification

## 1. Overview

In Party2, player accounts and individual characters can be permanently removed from the system. Due to the modular monolithic design and relational database integrity constraints, deletion must handle cascading dependency cleanup in a deterministic, consistent order with explicit SQL and external cleanup boundaries.

---

## 2. Character Deletion (`DELETE /characters/{id}`)

### 2.1 Authorization & Invariants
- An authenticated player can only delete characters owned by their account (`char.PlayerID == player.ID`).
- Attempting to delete another player's character returns HTTP `403 Forbidden`.
- Attempting to delete a nonexistent character returns HTTP `404 Not Found`.

### 2.2 Cleanup boundary

`CleanupHook` lets feature owners release associations and ephemeral state before
character deletion; SQL repository cleanup removes dependent records before the
character root in one transaction. The concrete table list belongs to the
[repository](../../internal/database/character_repository.go) and migrations.

Challenge admission is shared run evidence: deleting a departed original leader
preserves the admission row while another participant's durable claim remains,
including completed settlement awaiting publication. Surviving participants retain
their existing run and rewards. See the [run lifetime contract](../architecture/transient-run-state.md#identity-and-admission).

Current `character.Service.Delete` invokes hooks **before** its SQL transaction
and treats hook errors as best-effort. SQL rollback therefore does not restore
Valkey/external cleanup. This is not an atomic cross-store cascade; callers must
not infer full rollback from a later deletion failure.

## 3. Player Account Deletion (`DELETE /players/me` and `DELETE /players/{id}`)

### 3.1 Authorization & Invariants
- `DELETE /players/me`: Authenticated player initiates self-deletion. If a password is provided, it is verified against the player's hashed password.
- `DELETE /players/{id}`: Deletion of a specific player account by ID. Allowed if the authenticated player matches the target ID or if the request contains valid administrator credentials.
- Invalid password returns HTTP `401 Unauthorized`.

### 3.2 Cascading Cleanup Order
Current account deletion runs the following sequence; only the final player
repository deletion is wrapped by the account service transaction. Character
deletions and external invalidation occur earlier. Full persistent rollback is
a known gap tracked in [#1095](https://github.com/witchcraze/party2re/issues/1095):
1. Retrieves all characters owned by the player via `FindByPlayerID`.
2. Iterates through and invokes `characterService.Delete(ctx, playerID, char.ID)` for each character, triggering all character cleanup hooks and cascading SQL deletions.
3. Invalidate all active player sessions in Valkey Master via `SessionRepository.DeleteByPlayerID` (O(1) player session set and token deletion).
4. Player-linked relational tables in MariaDB:
   - `player_notifications`
   - `player_api_tokens`
5. Primary player record in `players`.

---

## 4. Inactivity Retention

### Legacy evidence and current gap

Original `party2/login.cgi:112–119,190–223` checks expired users during a
login-triggered player-list refresh (at least one day since the list file changed).
For each user, deletion follows either strict condition:

- `now > last_save + auto_delete_day * 86400`;
- `job_lv <= 0 && lv <= 2 && now > last_save + 7 * 86400`.

Guild departure precedes directory deletion. `lib/system.cgi:29–46` updates
`ltime` on every `write_user`; `party.cgi:20–30` saves after rendering even without
a command, and `login.cgi:106–108` saves the logged-in character. This is a game
save clock, not an account authentication clock. `config.cgi_origin:55` gives
**365 days as a reference default only**; the active legacy configuration is
unavailable. The deletion branch has no administrator or ban exemption.

The Go character model currently has no durable gameplay activity timestamp,
and the recurring jobs in `cmd/party2/wire.go` / `services_soc.go` have no character
expiry sweep. `player.Service.Login` can refresh account `updated_at` and `last_ip`
when supplied an IP; neither proves character activity. Automatic deletion is
therefore **not implemented or enabled**.

### Adopted policy and activity ownership

The approved decision in [#1103](https://github.com/witchcraze/party2re/issues/1103)
preserves the legacy expiry conditions, mapped to **individual Characters**:

- Each character owns a durable `last_gameplay_at`, initialized on creation.
  Update it when that character's authenticated gameplay reaches a committed
  save-equivalent boundary, including game entry and game-screen refresh with
  no state-changing command. A save before a domain rejection still counts;
  authentication/authorization failures and rolled-back saves do not.
- Reconcile legacy `write_user` callers and their dispatch paths with current
  gameplay commands/observations before enabling cleanup. HTTP method or a generic
  repository save is insufficient evidence: background settlement, incoming
  transfers and another character's actions do not establish owner activity.
  Account login alone, token refresh and unrelated/read-only API polling do not
  refresh every owned character. Game-screen observations replacing legacy saved
  refreshes must explicitly participate through owned commands; public/owned GET
  polling stays read-only for activity. Do not require repeated account login.
- Activity and its corresponding durable gameplay writes commit together. Concurrent
  updates cannot move the clock backward. Persistence failure must not report a
  completed gameplay save while leaving an older deletion clock.
- Delete only the expired character and its owned resources through the existing
  character cleanup contract. Preserve its Player, credentials and sibling characters,
  including when the last character expires. Registration capacity remains account
  based; character expiry does not free an account slot.
- The beginner predicate is `Character.JobLevel <= 0 && Character.Level <= 2`;
  `JobLevel` counts completed job changes, not the current job's skill level.
  The general and beginner predicates are ORed, so a general period shorter than
  seven days can expire a beginner earlier. Administrators and banned accounts
  have no special expiry exemption; ban/unban does not reset activity.

This is the initial reconstruction policy. A future retention change requires an
explicit decision, migration/disable plan and updated tests; [the policy
backlog](https://github.com/witchcraze/party2re/issues/1799) tracks that work.

### Configuration, scheduling and safe introduction

- Planned configuration: `PARTY2_INACTIVITY_DELETE_ENABLED` defaults to `false`.
  Enabling requires an explicitly supplied positive integer
  `PARTY2_INACTIVITY_RETENTION_DAYS`; missing, malformed, nonpositive or duration-
  overflowing values reject enabled configuration. Do not silently substitute 365
  or interpret zero as immediate deletion. Beginner expiry remains seven days.
- Compare UTC instants at persisted precision using elapsed periods of `86400`
  seconds per day. Before and exactly at either deadline, preserve the character;
  only strictly after it may deletion occur. Timezone does not change the cutoff.
- Run once daily at `00:00 Asia/Tokyo`, independently of login or list requests,
  through the existing scheduler. This is an approved scheduling adaptation from
  the legacy login-triggered scan. Disabled mode neither schedules nor executes
  destructive work, including previously queued actions. Retries re-evaluate current
  eligibility rather than replay an old list of candidates.
- Deploy activity tracking first. Give existing characters a recorded rollout-time
  baseline (never infer inactivity from account `updated_at`), and keep deletion
  disabled until all save-equivalent paths are covered and both beginner/general
  grace periods have elapsed since that baseline. Missing or invalid activity
  evidence preserves the character and reports an operational error.
- Public presence shares save events and the UTC clock, but preserves a separate
  [sampled record lifecycle](player-and-character.md#212-public-presence-and-login-message).
  Retention rollout baselines must not publish online presence.
- Resolve [persistent deletion atomicity](https://github.com/witchcraze/party2re/issues/1095)
  before enabling. Recheck current activity and beginner status under the same
  transaction/locking boundary as deletion, coordinated with gameplay saves. SQL
  failure rolls back persistent cleanup; external cleanup/recovery follows the
  actual deletion contract, without claiming a SQL/Valkey atomic transaction.
  Shared run evidence needed by surviving participants must remain intact.

### Implementation handoff and verification

Implementation is deliberately separate from this decision: activity tracking
must precede destructive scheduling. [Activity tracking](https://github.com/witchcraze/party2re/issues/1797)
and [guarded expiry](https://github.com/witchcraze/party2re/issues/1798) own code
changes; no destructive cleanup is introduced by this document.

Use fixed clocks to check before/equal/after both deadlines, OR behavior, beginner
level/job-change boundaries, UTC/JST transitions, and configuration rejection.
Verify committed/rolled-back save activity, unchanged game-screen refresh, rejected
requests, continued play without relogin, sibling isolation, last-character expiry,
admin/ban parity and existing-character grace. Persist/reload the activity clock;
race a gameplay save against candidate selection/deletion and prove that newly
active characters survive. Inject clock-write, listing, character/guild cleanup,
SQL commit and scheduler retry failures; missing evidence and disabled/stale jobs
must preserve data. Use the established shared-resource stress harness where
cleanup affects guilds or other shared resources.

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
