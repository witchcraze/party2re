# King Sealing Battles Design (封印戦)

## Overview

The King Sealing Battle Feature Module (`internal/boss`) provides high-tier endgame cooperative sealing battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) using the shared Multi-Participant Core Battle engine (`internal/core/battle`).

Up to 6 players (4 for `king99`) join a system-opened sealing room (quest type 6; see [§6](#6-unseal-to-sealing-room-lifecycle)) to challenge ancient sealed kings and calamitous deities. Characters face devastating enemy skills like **Dejon** (デジョン; dimensional banishment of unconscious members with severe fatigue penalties). Victorious adventurers achieve resealing via **`@ふういん`**, gaining Hero Count (勇者カウント / `$m{hero_c}`), rare treasures, crystals (刻印晶) scaled by time since the unseal, worldwide server news broadcasts, and adding a temporary celebration crowd in the town Event Plaza (`internal/eventplaza`).

---

## Rules and decisions

- `king1`–`king10` admit up to 6 players and `king99` 4: the legacy boss NPC takes one extra slot (`p_join+1`, counted from 1).
- Legacy enters `vs_king` only by joining a sealing room (`quest.cgi:1071`). **Decision (#1199):** the Go solo operation has no counterpart and is removed (#1928).
- There is no daily attempt cap; entry is governed by fatigue (<100) and the stage's `need_join`.
- Settlement commits rewards, Dejon fatigue, records and the monster book together; news and the celebration crowd are best effort. Persistence boundary: [Boss completion recovery](../architecture/transient-run-state.md#boss-completion-recovery).

## Authentic King Stages

Each stage's name, leader, capacity and gate come from lines 3–7 of `stage/king1.cgi`–`king10.cgi` and `king99.cgi`; all use speed 12. In `quest.cgi:947–952`, `hp_N_o` requires Max HP ≥ N and `hp_N_u` Max HP < N. Enemy stats and weighted treasures come from each CGI's `@bosses`/`@treasures`; repeated treasure entries are weights, not a unique list.

---

## Battle Mechanics

The CGI exposes `＠ふういん` after enemies are defeated; Go currently combines resolution and settlement in one operation (#1864).

### 1. Entry & Fatigue Cost
- Each participating character incurs **+20% Tired** when joining the room (`quest.cgi:add_member`, `$m{tired} += 20`, written immediately and independently of the battle outcome). Watching charges nothing. The charge commits with the join, exactly once per successful join; a rejected join charges nothing; battle settlement does not charge it again.
- Go currently charges it at the start of the sealing battle. [#1927](https://github.com/witchcraze/party2re/issues/1927) moves it to the join and removes the settlement charge in the same change, so there is no period with a double charge or no charge.
- Characters with `tired >= 100` or `hp <= 0` cannot enter.

### 3. Boss Skills & Clones
- **`king99` Player Clones**:
  - Clones are created **at unseal time** from every member of the unsealing party (`@partys`, NPC companions included) and stored in the room (`stage/king99.cgi` is loaded by `make_vs_king`). Later joiners do not change them; the room leader is the clone of the unsealing party's leader. If a king99 room already exists, a new unseal does not replace its clones.
  - Each clone takes the member's battle state at that treasure creation (after the per-advance status reset): 50× Max HP and Max MP, 2× AT/DF/AG, icon, Job/SP and OldJob/SP; EXP = (Level + JobLevel) × 30, gold = ⌊Level × 0.5⌋ × 30 (`quest.cgi:1177–1178`), 20 crystals.
  - Go currently builds clones from the sealing battle participants ([#1927](https://github.com/witchcraze/party2re/issues/1927)).
  - Clones possess the **`大防御`** defensive trait.

### 4. Dejon (デジョン) Banishment
- Dejon banishes a fallen character for the rest of the battle (no revival) and adds **+30% fatigue**. Legacy `vs_king.cgi:dejon` applies it at banishment; Go settlement applies it upon battle conclusion.

### 5. Victory, Resealing (`@ふういん`), & Rewards
Upon defeating all enemy boss participants:
1. **Hero Count**: All party members receive **+1 Hero Count** (`characters.hero_count` / `$m{hero_c}`).
2. **Crystals (刻印晶)**:
   - Base crystals: Sum of `GetCrystal` for defeated enemies; in `king99`, 20 per defeated clone (Go currently uses 20 × the number of participants).
   - **Unseal-Clock Multiplier** (per defeated enemy, `_battle.cgi:145–173`): elapsed time since the [unseal clock](#6-unseal-to-sealing-room-lifecycle), not since the battle or room start:
     - $\le 10$ minutes (600s): **3.0×**
     - $\le 30$ minutes (1800s): **2.0×**
     - $\le 60$ minutes (3600s): **1.5×** (truncated)
     - $> 60$ minutes: **1.0×**
   - Go currently measures from the room creation (party) or request start (solo); [#1201](https://github.com/witchcraze/party2re/issues/1201) persists the clock and applies it.
   - Directly credited to each member's crystal balance (`characters.crystal`).
3. **Experience & Gold**:
   - For `king1`..`king10`: Sum of boss definitions.
   - For `king99`: Sum over defeated clones of the values fixed at unseal ([§3](#3-boss-skills--clones)). Go currently computes them from the participants' levels.
4. **Treasure**: a random stage treasure plus the JST weekday orb (`_npc_action.cgi:541–543`). Legacy loads the stage's `@treasures` before the battle acts (`lib/vs_king.cgi:19–23`), so job78's treasure retry (`lib/_skill.cgi:6200`) regenerates sealing chests from the same table; that retry is unreachable while the battle resolves in one request (#1862). A treasure found with the Depot full is lost, even with a matching stack (`_npc_action.cgi:73–75`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
5. **Server News**: A worldwide announcement is broadcast: `"勇者○○が○○を封印する"`.
6. **Victory Celebration**: Adds a temporary crowd to Event Plaza (`_win_vs_king.cgi`). [Event Plaza design §3](eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps) owns generation, presence retention, merchant-selection order and runtime boundaries; there is no independent fixed two-hour banquet window.
7. **Room after sealing**: The room becomes watch-only and is deleted when its last member exits ([§6](#6-unseal-to-sealing-room-lifecycle)). Go currently deletes the lobby at settlement.

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
  system-owned sealing room with its initial enemy roster. A stage never has two rooms
  (legacy skips an existing quest directory, line 168). The king99 roster is the clone
  snapshot of the unsealing party ([§3](#3-boss-skills--clones)), so settlement passes the
  participants' battle state to this step; the other stages use their catalog bosses.
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
| open | trigger opens the room | `＠ふういん` after all enemies are defeated → sealed; deletion below |
| sealed | `＠ふういん` (`vs_king.cgi:fuuin`): Hero Count, news, chest, celebration | deletion below |

Deletion (any state):

- **Exit check** (`_battle.cgi:1331`, only when a member exits with `＠にげる`): the room is
  deleted if no players remain **and** the leader boss is defeated. Defeated NPCs are dropped
  from the room roster (`_battle.cgi:523`), so a defeated leader counts as HP ≤ 0. This
  applies before sealing too: once the leader boss falls, the last exit deletes the room even
  while other enemies remain. After sealing every enemy is defeated, so the last exit
  always deletes it.
- Otherwise the room survives with no players and keeps its enemy state (remaining enemies'
  HP and statuses) for later joiners. Players removed by Dejon leave without the exit check,
  so a room emptied that way is never deleted by it.
- **Idle deletion**: 30 minutes without room activity (`quest.cgi:86–90`, all room types).

The room's enemy state belongs to that room's encounter. It is not the withdrawn shared
world-boss HP ([transient-boss-hp.md](../architecture/transient-boss-hp.md)).

**Validation matrix for the implementation Issues:** before/after unseal, 6/7 and 4/5
capacity, repeated unseal (clock restart, no duplicate room, king99 clones kept), king99
clones from the unsealing party unaffected by joiners, join fatigue charged once (none on
rejection or at settlement), last exit with leader alive/defeated before sealing and after
sealing, Dejon emptying, sealing, party crawl and normal-run triggers, clock origin, and
rollback of a failed settlement.

**Current gaps:**

- No sealing room is opened, no unseal news is published, the party lobby has no
  system-owned kind, king99 clones come from the participants and join fatigue is charged
  at battle start ([#1927](https://github.com/witchcraze/party2re/issues/1927)).
- The clock is not persisted and the multiplier uses room or request start ([#1201](https://github.com/witchcraze/party2re/issues/1201)).
- The solo entry exists ([#1928](https://github.com/witchcraze/party2re/issues/1928)).
- The battle resolves at start, keeps no per-room enemy state, has no leader-defeat exit check and needs a character leader ([#1864](https://github.com/witchcraze/party2re/issues/1864)).
