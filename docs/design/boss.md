# King Sealing Battles Design (封印戦)

## Overview

The King Sealing Battle Feature Module (`internal/boss`) provides high-tier endgame cooperative sealing battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) using the shared Multi-Participant Core Battle engine (`internal/core/battle`).

Up to 6 players (4 for `king99`) form a party in the Multiplayer Party System (`internal/party`) under quest mode "封印戦" (quest type 6) to challenge ancient sealed kings and calamitous deities. Characters face devastating enemy skills like **Dejon** (デジョン; dimensional banishment of unconscious members with severe fatigue penalties). Victorious adventurers achieve resealing via **`@ふういん`**, gaining Hero Count (勇者カウント / `$m{hero_c}`), rare treasures, clear-time scaled crystals (刻印晶), worldwide server news broadcasts, and hosting a celebratory banquet in the town Event Plaza (`internal/eventplaza`).

---

## Architectural Policy & Boundaries

- **Multi-Participant Battle Engine**: Encounters are resolved through `corebattle.PartyBattleResolver` (`corebattle.Engine{}`), handling party vs. multi-enemy boss formations, agility turn-order, MP/CMP skills, and revive/banishment rules.
- **Stage Catalog**: The Go catalog represents 11 legacy stages (`stage/king1.cgi` through `stage/king10.cgi`, and dynamic clone stage `king99.cgi`). The original definitions remain authoritative; catalog presence alone does not certify all mechanics.
- **Capacity Parity**: `king1` through `king10` support parties of up to **6 players** (`MaxMembers: 6`), whereas `king99` retains a **4-player limit** (`MaxMembers: 4`).
- **No Fictional Daily Limits**: Legacy Party2 has no daily attempt caps (the previously fabricated 3-entry solo raid limit has been completely removed). Entrance is governed solely by character fatigue (`tired < 100`) and stage-specific `need_join` conditions (e.g. `hp_400_o`).
- **Transactional Consistency**: Post-battle state transitions (HeroCount increment, entry fatigue +20%, dejon fatigue +30%, loot awards, crystal rewards, news broadcast, banquet hooks, party disbandment) are committed within a single database transaction.

---

## Authentic King Stages

The following names, leaders, capacities, and gates were checked against lines
3–7 of each original stage CGI. All stages use speed 12. In `quest.cgi:947–952`,
`hp_N_o` requires Max HP ≥ N, while `hp_N_u` requires Max HP < N.

| Stage ID | Stage Name | Leader | Max Members | Participation Gate |
|---|---|---|---|---|
| **king1** | @全てを無に還す者@ | 破壊神 | 6 | `hp_400_o` |
| **king2** | @全てを憎む者@ | 暗黒竜 | 6 | `hp_400_o` |
| **king3** | @全てを破壊する者@ | 悪魔の書 | 6 | `hp_300_o` |
| **king4** | @全てを呪う者@ | 暗黒の盾 | 6 | `hp_300_o` |
| **king5** | @全てを支配する者@ | ドールマスター | 6 | `hp_200_o` |
| **king6** | @二重世界@ | 闇のクリスタル | 6 | `hp_200_o` |
| **king7** | @全てを爆発させる者@ | ボマー | 6 | `hp_200_o` |
| **king8** | @闇でおおいつくす者@ | 魔人のツボ | 6 | `hp_400_u` |
| **king9** | @悪の城@ | 悪の城 | 6 | `hp_200_u` |
| **king10** | @無限に増殖する者@ | スライムボックス | 6 | `hp_100_u` |
| **king99** | @罪と罰@ | Party leader / player clones | 4 | `hp_200_o` |

Detailed enemy stats and weighted treasures belong to each original CGI's
`@bosses`/`@treasures`, represented by `internal/boss/catalog.go`. Repeated
treasure entries are weights; do not replace them with a unique-item list.

---

## Battle Mechanics

The following describes the current Go battle/settlement behavior. Verify the
original battle routines before treating an implementation formula as legacy
parity. The CGI exposes `＠ふういん` after enemies are defeated; Go currently
combines resolution and settlement in the service operation.

### 1. Entry & Fatigue Cost
- Each participating character incurs **+20% Tired** upon entering the sealing battle (`$m{tired} += 20`).
- Characters with `tired >= 100` or `hp <= 0` cannot enter.

### 2. Defensive Traits (TMP Abilities)
Bosses and clones feature specialized passive defensive abilities processed by `internal/core/battle`:
- **`大防御`**: Reduces all incoming damage (physical & magic) to **10%** (0.1× damage).
- **`攻軽減`**: Reduces incoming physical damage to **25%** (0.25× damage).
- **`攻無効`**: Nullifies all physical damage (0 damage).
- **`魔無効`**: Nullifies all magic attacks (0 damage) and logs: `"<user> の <action>！ <target> は魔法をうけつけない！"`.

### 3. Boss Skills & Clones
- **Authentic Job Skills**: Normal bosses (`king1`..`king10`) are assigned skills mapped to their `Job` and `OldJob` based on their `SP` and `OldSP` thresholds in addition to `dejon`.
- **`king99` Player Clones**:
  - Clones possess 50× HP and MP, and 2× offensive/defensive stats of the participating players.
  - Inherit all combat skills from the player's active Job and OldJob.
  - Clones possess the **`大防御`** defensive trait.

### 4. Dejon (デジョン) Banishment
- Bosses execute the **Dejon** skill (`ActionKindDejon = "dejon"`).
- Target: Any opposing character whose HP is reduced to `0` or below.
- Effect: The unconscious character is cast into another dimension and permanently removed from combat. They cannot be revived for the remainder of the battle.
- Penalty: An additional **+30% Tired** is applied. Legacy `vs_king.cgi:dejon` applies it at banishment; Go settlement applies it upon battle conclusion.

### 5. Victory, Resealing (`@ふういん`), & Rewards
Upon defeating all enemy boss participants:
1. **Hero Count**: All party members receive **+1 Hero Count** (`characters.hero_count` / `$m{hero_c}`).
2. **Crystals (刻印晶)**:
   - Base crystals: Sum of `GetCrystal` for defeated bosses in normal stages; $\text{len(allies)} \times 20$ in `king99`.
   - **Clear-Time Multiplier**:
     - $\le 10$ minutes (600s): **3.0×**
     - $\le 30$ minutes (1800s): **2.0×**
     - $\le 60$ minutes (3600s): **1.5×**
     - $> 60$ minutes: **1.0×**
   - Directly credited to each member's crystal balance (`characters.crystal`).
3. **Experience & Gold**:
   - For `king1`..`king10`: Sum of boss definitions.
   - For `king99`: Scaling formula based on party levels:
     $$\text{EXP} = \sum (\text{Level} + \text{JobLevel}) \times 30$$
     $$\text{Gold} = \sum \lfloor \text{Level} \times 0.5 \rfloor \times 30$$
4. **Treasure Drop & Weekday Orbs**:
   - A random item from the stage's `treasure_item_ids` augmented with the current weekday orb (evaluated in Japan Standard Time / JST matching authentic legacy Party2 `_npc_action.cgi:541-543`):
     - Monday: `item-060` (月光の玉)
     - Tuesday: `item-061` (火炎の玉)
     - Wednesday: `item-062` (水流の玉)
     - Thursday: `item-063` (木霊の玉)
     - Friday: `item-064` (黄金の玉)
     - Saturday: `item-065` (白銀の玉)
     - Sunday: Random choice among `item-060`..`item-065`.
   - Reward delivery uses `coreinventory.Inventory` capacity enforcement; overflow items route to `depot.Depot` storage (Rank 5 lock); items overflowing a full depot are treated as lost drops (`_npc_action.cgi:74-75`).
5. **Server News**: A worldwide announcement is broadcast: `"勇者○○が○○を封印する"`.
6. **Celebration Banquet**: Triggers a 2-hour victory banquet in Event Plaza (`_win_vs_king.cgi`).
7. **Lobby Disbandment**: The temporary staging party is deleted upon conclusion.

---

## 6. Demon King Unsealing (`UnsealDemonKing`)

In legacy Party2 (`lib/vs_monster.cgi:105`), stage **19** invokes `make_vs_king`, unsealing the ancient kings and triggering the sealing battle era:
- **Mao Count**: All participating non-NPC characters gain **+1 Mao Count** (`characters.mao_count` / `$m{mao_c}`).
- **Worldwide News**: Broadcasts `"<heroes>によって封印されし者達の封印が解かれました！"`.
- **Rank 2 Locking**: Character rows are locked in ascending lexicographical order within a single transaction.

Current `internal/adventure/unseal.go` also accepts stage 20. That extra trigger
is a [documented implementation difference](../migration/documentation-audit.md),
not a canonical legacy rule. Rank 2 locking describes the Go persistence boundary.
