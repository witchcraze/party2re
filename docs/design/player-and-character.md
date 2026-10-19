# Player and Character Specification

## 1. Domain Overview

In Party2, identity and game progression are strictly separated across two domain entities:

- **Player**: The account-level entity responsible for authentication, credentials, and session management.
- **Character**: The in-game persona holding stats, progression, inventory, equipment, jobs, and feature-specific associations.

Every `Character` belongs to exactly one `Player`. A `Player` may own multiple characters over time.

---

## 2. Invariants and Rules

### 2.1 Ownership Invariant
- A `Character` cannot exist without being associated with a valid `Player` (`player_id`).
- When a `Character` is created, its `player_id` must be explicitly specified and must refer to an existing `Player`.
- A `Character`'s ownership cannot be transferred arbitrarily; `player_id` remains immutable throughout ordinary gameplay operations.

### 2.2 Authorization & Access Control
- All player-initiated operations acting upon a `Character` (such as starting an adventure, buying/selling in shops, depositing/withdrawing in depot or bank) require an authenticated session belonging to that character's owning `Player`.
- If an authenticated session attempts to inspect or mutate a `Character` owned by a different `Player`, the request is rejected with `Forbidden` (`403`).

### 2.3 Character Listing & Querying
- A `Player` can query all characters associated with their account (`FindByPlayerID`).
- Each `Character` response contains `id`, `player_id`, `name`, `job_id`, `gender`, `level`, `experience`, `money`, `sp`, and `stats`.

### 2.4 Currency & Wallet Ceiling
- `Character.Money` holds the character's active wallet gold, strictly capped at an authentic maximum ceiling of `999,999 G` (`corecharacter.MaxMoney`, reproducing legacy Party2 `system.cgi:71`).
- All currency-crediting pathways (`Character.AddMoney`, `economy.Service.AddGold`, shops, adventures, depot, battles, courier) respect this ceiling, clamping any overflowed gold to `999,999 G`.
- Wealth beyond 999,999 G must be deposited into the Bank (`Character.Deposit`), which supports large-scale gold savings up to 99兆9999億9999万9999 G (`bank.MaxDeposit`).
- `Character.SmallMedals` is capped at `999,999,999` (`corecharacter.MaxSmallMedals`).

### 2.5 Vitality & Fatigue Invariants
- **Vitality Bounds**: HP and MP are strictly bounded within `[0, MaxHP]` and `[0, MaxMP]` via `Stats.ClampVitality`. Restorative actions (sleeping, celestial recovery, job changes) restore full vitality via `Character.RecoverVitality`.
- **Combat Survival Floor**: In combat settlement (`battle/apply.go`, `boss/battle.go`), fallen combatants (`HP <= 0` or fallen flag) survive with an authentic minimum floor of `1 HP` via `Character.ApplyCombatSurvival`. Surviving MP is updated and clamped within `[0, MaxMP]`.
- **Fatigue Clamping**: Standard fatigue percentage is capped at `100%` (`AddTired`). Reaching `100%` triggers exhaustion (`IsExhausted() == true`), blocking adventures and fatigue-generating mini-games. Celestial wishes (`god.WishRefresh`, `ReduceTired(150)`) bypass the standard 0% floor to provide an authentic negative fatigue buffer against subsequent combat. Resting at home clears fatigue to 0 (`ResetTired`).

### 2.6 Player Account Lifecycle & Ban Invariant
- **Account State**: Each `Player` record tracks `created_at`, `updated_at`, `last_ip`, and an optional `banned_at` timestamp.
- **Login Tracking**: Upon successful login, the player's `last_ip` and `updated_at` are refreshed.
- **Soft-Ban Enforcement**: When an account is soft-banned (`banned_at IS NOT NULL` / `IsBanned() == true`), all active Valkey sessions and Personal Access Tokens are immediately invalidated. Subsequent login attempts (`POST /sessions`) and authenticated character requests receive HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).

### 2.7 Request Throttling and Reload Sleep Penalty Reconciliation (Approved Departure)

#### 2.7.1 Legacy Reload Penalty Specification
In original Party2, inter-request intervals and rapid manual reloading were monitored to prevent CGI server overload:
- **Inter-Request Detection (`party2/lib/system.cgi:148–156`)**:
  - During `&read_user`, if the interval since the character's last access was less than `$wait_time` (`config.cgi:88`, reference default `1` second), i.e., `$m{ltime} + $wait_time > $time`, and auto-reload was not flagged (`!$in{is_auto}`):
    - Appended `"1<>"` to `$userdir/$id/reload.cgi` (recording a reload strike).
    - Terminated immediately with an error message: `更新の連打は禁止しています。最低でも $wait_time 秒は待ってください<br>※過度な更新連打は自動削除の対象となります`.
- **Strike Consumption & Character Sleep Penalty (`party2/login.cgi:37–104`)**:
  - Upon subsequent login, if `$userdir/$id/reload.cgi` existed and had non-zero size, its contents were split by `<>` to count strikes (`@lines`), and the file was immediately truncated to 0 bytes (`truncate $fh, 0`), consuming and resetting the strike record.
  - Strikes exceeding thresholds triggered severe character sleep penalties added to existing `$m{sleep}` (`$m{sleep} = $m{sleep} > 0 ? $m{sleep} + $sleep_time * 60 : $sleep_time * 60`):
    - **`> 30` strikes**: `$sleep_time = 7 * 24 * 60` minutes (7 days = 10,080 minutes / 604,800 seconds). In original source comments, account deletion (`&delete_directory`) and IP blacklisting (`&add_black_list`) were drafted but commented out, falling back to 7 days sleep.
    - **`> 25` strikes**: `$sleep_time = 3 * 24 * 60` minutes (3 days = 4,320 minutes / 259,200 seconds). IP blacklist was commented out.
    - **`> 20` strikes**: `$sleep_time = 24 * 60` minutes (24 hours = 1,440 minutes / 86,400 seconds).
    - **`> 15` strikes**: `$sleep_time = 6 * 60` minutes (6 hours = 360 minutes / 21,600 seconds).
    - **`> 10` strikes**: `$sleep_time = 60` minutes (1 hour = 60 minutes / 3,600 seconds).
    - **`<= 10` strikes**: No penalty applied; strikes were wiped on login.

#### 2.7.2 Approved Architectural Departure (#1106)
Legacy reload sleep penalties are **formally deprecated and superseded** by modern HTTP transport rate limiting and administrative soft-ban controls:
1. **Separation of Concerns & Layering Principles**:
   - Request frequency control and DoS mitigation belong exclusively to the HTTP transport/middleware layer (`internal/api/http/middleware.go` via `internal/ratelimit`).
   - Game-state sleep (`timer.CategorySleep`) is strictly reserved for in-game mechanics: HP/MP restoration via home sleep (`internal/home`) and dungeon defeat rescue penalty (`internal/rescue`).
   - Transport-level request bursts must never mutate domain character timers or lock character progression.
2. **Comparison of Throttling vs. Game Sleep**:
   - `HTTP 429 Too Many Requests`: An ephemeral, transport-level backoff signal with `X-RateLimit-*` and `Retry-After: <seconds>` headers. Traffic resumes immediately once the window lapses without altering character state.
   - `Character Sleep Penalty`: An in-game lock lasting hours or days that halts all player/party progression. Applying this to HTTP frequency violations creates severe false positives.
3. **Malicious Abuse Handling**:
   - Extreme, persistent, or malicious abuse is mitigated via administrative soft-bans (`Player.banned_at` / `POST /admin/players/{id}/ban`), revoking tokens and rejecting all requests with `403 Forbidden`, rather than poisoning gameplay sleep timers.

#### 2.7.3 Non-Interference and Testing Requirements
- **SPA & Parallel Request Non-Interference**: Modern browser and mobile clients dispatch multiple parallel asynchronous requests on page loads (e.g. character details, notifications, party status, chat). Sub-second inter-request spacing must not accumulate penalty strikes or restrict valid traffic within configured HTTP quotas (Public 10 req/min, Authenticated 60 req/min).
- **Automated Agents & PAT Non-Interference**: Programmatic clients (AI agents, automated scripts) operating via Personal Access Tokens must receive standard HTTP 429 backoff responses when exceeding quotas, without triggering character sleep locks.
- **Fail-Open Resilience**: Infrastructure limiter failures (e.g. Valkey unavailability) must fail open gracefully (`internal/api/http/ratelimit_test.go`), avoiding spurious rejections or penalty side-effects during backend incidents.

---

## 3. Data Schema & Persistence

### 3.1 Players Table
- `id VARCHAR(32) PRIMARY KEY`
- `username VARCHAR(64) NOT NULL UNIQUE`
- `password_hash VARCHAR(255) NOT NULL`
- `banned_at DATETIME(6) NULL DEFAULT NULL`
- `created_at DATETIME(6) NOT NULL`
- `updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)`
- `last_ip VARCHAR(45) NOT NULL DEFAULT ''`

### 3.2 Characters Table
- `id VARCHAR(32) PRIMARY KEY`
- `player_id VARCHAR(32) NOT NULL` (Foreign key constraint referencing `players(id)`)
- `name VARCHAR(64) NOT NULL`
- `job_id VARCHAR(32) NOT NULL`
- `gender VARCHAR(16) NOT NULL`
- `level INT NOT NULL`
- `experience INT NOT NULL`
- `money INT NOT NULL`
- `sp INT NOT NULL`
- `max_hp INT NOT NULL`, `max_mp INT NOT NULL`, `hp INT NOT NULL`, `mp INT NOT NULL`
- `attack INT NOT NULL`, `defense INT NOT NULL`, `agility INT NOT NULL`

---

## 4. State Transitions

```text
       [Register Player]
               |
               v
      Player Account Created
               |
               v
       [Login / Session]
               |
               v
     Authenticated Session
               |
               v
     [Create Character(player_id, name)]
               |
               v
      Character Linked to Player
               |
  +------------+------------+
  |                         |
  v                         v
[Own Player Request]     [Other Player Request]
  |                         |
  v                         v
Allowed (200 / 201)      Rejected (403 Forbidden)
```
