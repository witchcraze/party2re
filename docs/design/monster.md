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
- **Base Capacity**: 50 monsters (100 monsters for veteran characters with completed job changes $\text{JobLevel} \ge 100$).
- **Limit Break Expansion (`OverMonster`)**:
  $$\text{BoxCapacity} = \text{BaseCapacity} + (50 \times \text{OverMonster}) \quad (0 \le \text{OverMonster} \le 5)$$
  - Standard base ($50$):
    - Tier 0: 50 monsters
    - Tier 1: 100 monsters
    - Tier 2: 150 monsters
    - Tier 3: 200 monsters
    - Tier 4: 250 monsters
    - Tier 5: 300 monsters
  - Veteran base ($100$, $\text{JobLevel} \ge 100$):
    - Tier 0: 100 monsters
    - Tier 1: 150 monsters
    - Tier 2: 200 monsters
    - Tier 3: 250 monsters
    - Tier 4: 300 monsters
    - Tier 5: 350 monsters

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

---

## 7. Post-Battle Monster Rising & Taming (戦闘後モンスター起き上がり)

Legacy reference: `party2/lib/_battle.cgi:230-265`.

Upon victory in combat (`OutcomeWin`), each defeated enemy participant undergoes an authentic legacy recruitment roll to determine if it "rises up and looks this way" (`起き上がりこちらを見ている`).

### 7.1 Combat Strength Formula (`strong`)

$$\text{strong}(p) = \lfloor \text{MaxHP} + \text{MaxMP} + \text{Attack} + (\text{Defense} \times 0.5) + \text{Agility} \rfloor$$

An enemy is classified as strong (`is_strong`) if:
$$\text{strong}(\text{enemy}) > \text{strong}(\text{player}) \times 0.5$$

### 7.2 Recruitment Probability Formula (`$par`)

Base probability parameter:
- If weak enemy: $\text{par} = 2.0$
- If strong enemy: $\text{par} = 1.0$ (or $\text{par} = 2.0$ if player's job is Monster Tamer / 魔物使い `job-12`)

Flat bonus additions to $\text{par}$:
1. **Monster Food** (`item-077` / 魔物のエサ): $+2.0$ if present in player inventory.
2. **Chapel Blessing 3** (`BlessingMonster` / 願い3: "モンスターと仲良くしたい"): $+0.5$ if active.
3. **Dragon Ruler Equipment Set**: $+0.5$ if both Dragon God Staff (`weapon-36` / 竜神の杖) and Champion Cloak (`armor-39` / 王者のマント) are equipped.
4. **Captured Status** (`captured` / `捕縛`): $+4.0$ if the enemy was captured during combat.

Recruitment check succeeds if:
$$\text{rand}(200) < \text{par} \iff \text{Intn}(2000) < \lfloor \text{par} \times 10 \rfloor$$

### 7.3 Capacity Check & Dialogue Messages

If the roll succeeds:
- **Monster Book Registration**: Recorded via `MonsterDefeatRecorder` (`internal/collection`).
- **Box Capacity Check**: If the character's Monster Grandpa box is at maximum capacity (`ErrMonsterBoxFull`):
  > `なんと {enemyName} が起き上がりこちらを見ている。しかし、{playerName}のモンスター預かり所はいっぱいだった。{enemyName}は悲しそうに去っていった…`
- **Taming Success**:
  > `なんと {enemyName} が起き上がりこちらを見ている。{enemyName}はうれしそうに{playerName}のモンスター預かり所に向かった`

