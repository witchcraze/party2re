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
- Each `Character` response contains `id`, `player_id`, `name`, `job_id`, `gender`, `level`, `experience`, `money`, `rebirth_count`, and `stats`.

---

## 3. Data Schema & Persistence

### 3.1 Characters Table
- `id VARCHAR(32) PRIMARY KEY`
- `player_id VARCHAR(32) NOT NULL` (Foreign key constraint referencing `players(id)`)
- `name VARCHAR(64) NOT NULL`
- `job_id VARCHAR(32) NOT NULL`
- `gender VARCHAR(16) NOT NULL`
- `level INT NOT NULL`
- `experience INT NOT NULL`
- `money INT NOT NULL`
- `rebirth_count INT NOT NULL`
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
