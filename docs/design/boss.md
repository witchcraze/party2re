# King Sealing Battles Design (封印戦)

## Overview

The King Sealing Battle Feature Module (`internal/boss`) provides high-tier endgame cooperative sealing battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) using the shared Multi-Participant Core Battle engine (`internal/core/battle`).

Up to 6 players (4 for `king99`) join a system-opened sealing room (quest type 6; see [§6](#6-unseal-to-sealing-room-lifecycle)) to challenge ancient sealed kings and calamitous deities. Characters face devastating enemy skills like **Dejon** (デジョン; dimensional banishment of unconscious members with severe fatigue penalties). Victorious adventurers achieve resealing via **`@ふういん`**, gaining Hero Count (勇者カウント / `$m{hero_c}`), rare treasures, crystals (刻印晶) scaled by time since the unseal, worldwide server news broadcasts, and adding a temporary celebration crowd in the town Event Plaza (`internal/eventplaza`).

---

## Architectural Policy & Boundaries

- **Multi-Participant Battle Engine**: Encounters are resolved through `corebattle.PartyBattleResolver` (`corebattle.Engine{}`), handling party vs. multi-enemy boss formations, agility turn-order, MP/CMP skills, and revive/banishment rules.
- **Stage Catalog**: The Go catalog represents 11 legacy stages (`stage/king1.cgi` through `stage/king10.cgi`, and dynamic clone stage `king99.cgi`). The original definitions remain authoritative; catalog presence alone does not certify all mechanics.
- **Capacity Parity**: `king1` through `king10` admit up to **6 players** (`MaxMembers: 6`), whereas `king99` admits **4** (`MaxMembers: 4`). In legacy the boss NPC takes one extra slot (`p_join+1`, counted from 1), so the player capacity equals `p_join`.
- **No Solo Entry**: Legacy enters `vs_king` only by joining a sealing room (`quest.cgi:1071`). The Go solo operation `fightBoss` (`ChallengeBoss`) has no legacy counterpart. #1199 decided to remove it ([#1928](https://github.com/witchcraze/party2re/issues/1928)).
- **No Fictional Daily Limits**: Legacy Party2 has no daily attempt caps (the previously fabricated 3-entry solo raid limit has been completely removed). Entrance is governed solely by character fatigue (`tired < 100`) and stage-specific `need_join` conditions (e.g. `hp_400_o`).
- **Transactional Consistency**: Solo and party settlement run as one SQL Unit of Work: rewards (HeroCount, EXP, gold, crystals, participant-specific loot instances), entry fatigue +20%, dejon fatigue +30%, participant state, boss challenge records, and mandatory Monster Book / Hall of Fame records, including any triggered completion. Required failures roll back all of these and propagate. Party sealing also commits a room receipt; cleanup failures after commit retain rewards and retry only room/index deletion. See [Boss completion recovery](../architecture/transient-run-state.md#boss-completion-recovery) for the persistence boundary. News and celebration hooks remain best-effort: their failures do not abort settlement, the Valkey crowd append cannot roll back with SQL, and no separate banquet record is saved.

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
- Each participating character incurs **+20% Tired** when joining the room (`quest.cgi:add_member`, `$m{tired} += 20`). Go currently charges it at the start of the sealing battle; [#1927](https://github.com/witchcraze/party2re/issues/1927) moves it to the join.
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
   - **Unseal-Clock Multiplier** (per defeated enemy, `_battle.cgi:145–173`): elapsed time since the [unseal clock](#6-unseal-to-sealing-room-lifecycle), not since the battle or room start:
     - $\le 10$ minutes (600s): **3.0×**
     - $\le 30$ minutes (1800s): **2.0×**
     - $\le 60$ minutes (3600s): **1.5×** (truncated)
     - $> 60$ minutes: **1.0×**
   - Go currently measures from the room creation (party) or request start (solo); [#1201](https://github.com/witchcraze/party2re/issues/1201) persists the clock and applies it.
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
   - Legacy loads the stage's `@treasures` before the battle acts (`lib/vs_king.cgi:19–23`), so Job 78 `トレハンのいじ` (`lib/_skill.cgi:6200`) regenerates sealing chests from the same table plus the weekday orb. The legacy comment records this loading as a later fix inferred from related behavior. The retry is not reachable while the sealing battle resolves in one request ([#1862](https://github.com/witchcraze/party2re/issues/1862)).
   - Reward delivery uses `coreinventory.Inventory` capacity enforcement; overflow items route to `depot.Depot` storage (Rank 5 lock); items overflowing a full depot are treated as lost drops (`_npc_action.cgi:74-75`).
5. **Server News**: A worldwide announcement is broadcast: `"勇者○○が○○を封印する"`.
6. **Victory Celebration**: Adds a temporary crowd to Event Plaza (`_win_vs_king.cgi`). [Event Plaza design §3](eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps) owns generation, presence retention, merchant-selection order and runtime boundaries; there is no independent fixed two-hour banquet window.
7. **Room after sealing**: The room becomes watch-only and is deleted when its last member leaves ([§6](#6-unseal-to-sealing-room-lifecycle)). Go currently deletes the lobby at settlement.

---

## 6. Unseal-to-Sealing-Room Lifecycle

This section is the native contract decided in #1199. It follows the legacy behavior;
the remaining implementation gaps are listed at the end.

**Trigger.** Every stage **19** ordinary floor-11 treasure creation, outside the clear
predicate (`lib/vs_monster.cgi:91–110`, `make_vs_king` at `159–225`). This applies to
normal runs ([event](adventure.md#combat-travel-and-encounter-replacement)) and to solo
and party batch crawls (`DungeonCrawlResult.UnsealDemonKing`). Boss victory, chest
inspection, exit and receipt replay are not triggers. Each genuine treasure recreation is
a new trigger. Nothing else is inferred (stage 20, habitat, monster defeat).

**In the settlement transaction** (commit or roll back together with rewards):

- Battle settlement adds MaoCount +1 for each real participant (`mao_c`, line 218). It is
  the only counter owner; never also call `boss.Service.UnsealDemonKing`, which writes the
  counter too and has no production caller.
- The unseal clock is a single SQL row (`unsealed_at`), overwritten with the settlement time
  on **every** trigger, even when all rooms already exist (legacy rewrites `king_start.cgi`
  each time, line 224). A battle already in progress therefore sees the restarted clock.
  With no row, the multiplier is 1×.

**After commit** (best effort; failure keeps the committed state, and replay never repeats
the counter):

- For each king stage (`king1`–`king10`, `king99`) without an existing room, open one
  system-owned sealing room. A stage never has two rooms (legacy skips an existing quest
  directory, line 168).
- Publish news: `<party name>(<participant names>) によって封印されし者達の封印が解かれました！`
  (line 221).

**Room.** A party-lobby kind owned by the system. The boss NPC is the leader, so there is
no character leader. Name = stage name, no password, speed 12, `need_join` and capacity
from the stage ([table](#authentic-king-stages)). Ordinary parties keep their 4-player
limit and cannot use king stages.

**Join** (`quest.cgi:928–1080`): allowed while the room is unsealed (battle round 1), even
mid-battle, and below capacity. Requires `need_join` (Max HP), HP > 0, Tired < 100 and no
other active room. Joining adds +20 Tired. The listing offers join for an open room and
watch-only for a full or sealed one (`quest.cgi:149–153`).

**Lifecycle:**

| State | Enters when | Leaves when |
|---|---|---|
| open | trigger opens the room | `＠ふういん` after all enemies are defeated → sealed; 30 minutes without activity → deleted (`quest.cgi:86–90`, all room types) |
| open, empty | last member leaves or is banished by Dejon | the room keeps its enemy state (`_battle.cgi:1331`) until joined or idle-deleted |
| sealed | `＠ふういん` (`vs_king.cgi:fuuin`): Hero Count, news, chest, celebration | last member leaves → deleted; idle deletion |

The empty-room state belongs to that room's encounter. It is not the withdrawn shared
world-boss HP ([transient-boss-hp.md](../architecture/transient-boss-hp.md)).

**Validation matrix for the implementation Issues:** before/after unseal, 6/7 and 4/5
capacity, repeated unseal (clock restart, no duplicate room), sealing, party crawl and
normal-run triggers, clock origin, and rollback of a failed settlement.

**Current gaps:**

- No sealing room is opened, no unseal news is published and the party lobby has no
  system-owned kind ([#1927](https://github.com/witchcraze/party2re/issues/1927)).
- The clock is not persisted and the multiplier uses room or request start ([#1201](https://github.com/witchcraze/party2re/issues/1201)).
- The solo entry exists ([#1928](https://github.com/witchcraze/party2re/issues/1928)).
- The battle resolves at start, keeps no per-room state and needs a character leader ([#1864](https://github.com/witchcraze/party2re/issues/1864)).
