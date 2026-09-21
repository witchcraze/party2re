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
