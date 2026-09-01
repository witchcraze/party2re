# Monster Grandpa & Pet Companion Design (モンスターじいさん・ペット)

## 1. Overview

The Monster system (`internal/monster`, legacy `farm.cgi` / `monster.cgi`) provides monster storage, pet companionship at player home estates, monster renaming, peer gifting, and wild releases. Defeated monsters encountered during adventures can be captured/tamed and sent to the Monster Grandpa facility (`@モンジィ`).

---

## 2. Facilities & NPC

- **Facility Title**: モンスターじいさん (Monster Grandpa)
- **NPC Name**: `@モンジィ`
- **Location Context**: Legacy `farm.cgi` (farm town facility)

---

## 3. Storage & Capacity Rules

### 3.1 Box Capacity (モンスター預かり所)
- **Base Capacity**: 50 monsters.
- **Limit Break Expansion (`OverMonster`)**:
  $$\text{BoxCapacity} = 50 + (50 \times \text{OverMonster}) \quad (0 \le \text{OverMonster} \le 5)$$
  - Tier 0: 50 monsters
  - Tier 1: 100 monsters
  - Tier 2: 150 monsters
  - Tier 3: 200 monsters
  - Tier 4: 250 monsters
  - Tier 5: 300 monsters

### 3.2 Home Pet Capacity (家のペット)
- **Maximum Pets per Home**: Up to 8 monsters (`MaxHomePets = 8`).
- **Uniqueness Constraint**: Within a single player home, no two pets may share the exact same `custom_name`. If a duplicate name is brought home, the system rejects the operation and prompts the player to rename the monster (`なづける`).

---

## 4. Supported Operations

| Action | Legacy Command | HTTP Endpoint | Description |
| :--- | :--- | :--- | :--- |
| **Dialogue** | `はなす` | `GET /monster/dialogue` | Retrieves `@モンジィ` dialogue and tips. |
| **List & Summary** | `みる` | `GET /characters/{id}/monsters?location=box\|home` | Lists stored monsters/pets with capacity counts. |
| **Tame / Capture** | `モンスターゲット` | `POST /characters/{id}/monsters/tame` | Adds a captured monster to Grandpa box (fails if box full). |
| **Bring to Home** | `つれてく` | `POST /characters/{id}/monsters/{instance_id}/bring-home` | Moves a monster from box to home pets (max 8, unique name check). |
| **Deposit to Box** | `あずける` | `POST /characters/{id}/monsters/{instance_id}/deposit` | Moves a pet from home back to Grandpa box. |
| **Rename** | `なづける` | `POST /characters/{id}/monsters/{instance_id}/rename` | Renames a monster instance (max 8 characters, sanitization rules). |
| **Send Monster** | `おくる` | `POST /characters/{id}/monsters/{instance_id}/send` | Gifts a monster to another player character's box. |
| **Release** | `わかれる` | `POST /characters/{id}/monsters/{instance_id}/release` | Releases a monster back into the wild. |

---

## 5. Monster Naming Constraints

Custom monster pet nicknames must satisfy:
1. Length: $1 \le \text{Length} \le 8$ (UTF-8 character count).
2. Prohibited Characters:
   - Whitespace (ASCII spaces and fullwidth Japanese space `\u3000`).
   - Special characters: `,`, `;`, `"`, `'`, `&`, `<`, `>`, `@`.

---

## 6. Concurrency & Transaction Guarantees

All monster mutations run within Unit of Work database transactions (`RunInTx`) with deterministic pessimistic locking:
- `characters` locked with `FOR UPDATE` (sorted in ascending character ID order during transfers).
- `character_monsters` rows locked with `FOR UPDATE`.
- Zero IDOR: All operations verify session authentication and character ownership.
