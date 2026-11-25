# King Sealing Battles Design (封印戦)

## Overview

The King Sealing Battle Feature Module (`internal/boss`) provides high-tier endgame cooperative sealing battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) using the shared Multi-Participant Core Battle engine (`internal/core/battle`).

---

## Rules and decisions

- `king1`–`king10` admit up to 6 players and `king99` 4: the legacy boss NPC takes one extra slot (`p_join+1`, counted from 1).
- Settlement commits rewards, Dejon fatigue, the monster book and server news together; the celebration crowd (Valkey) is best effort. Persistence boundary: [Boss completion recovery](../architecture/transient-run-state.md#boss-completion-recovery).

## Authentic King Stages

Each stage's name, leader, capacity and gate come from lines 3–7 of `stage/king1.cgi`–`king10.cgi` and `king99.cgi`; all use speed 12. In `quest.cgi:947–952`, `hp_N_o` requires Max HP ≥ N and `hp_N_u` Max HP < N. Enemy stats and weighted treasures come from each CGI's `@bosses`/`@treasures`; repeated treasure entries are weights, not a unique list.

---

## Battle Mechanics

The CGI exposes `＠ふういん` after enemies are defeated; Go currently combines resolution and settlement in one operation (#1864).

### 3. Boss Skills & Clones
- **`king99` Player Clones**: `make_vs_king` loads `stage/king99.cgi` at unseal, so the clones copy every member of the unsealing party (`@partys`, NPC companions included) from their battle rows at that treasure creation, never the room's later joiners.
  - A player's row carries EXP `lv + job_lv` and gold `int(lv × 0.5)` (`quest.cgi:1177–1178`), written when the row is created at join. Go player actors carry no such values, so the unseal reads the character's level at the unseal instead ([#2756](https://github.com/witchcraze/party2re/issues/2756)).

### 4. Dejon (デジョン) Banishment
- Dejon banishes a fallen character for the rest of the battle (no revival) and adds **+30% fatigue**. Legacy `vs_king.cgi:dejon` applies it at banishment; Go settlement applies it upon battle conclusion.

### 5. Victory, Resealing (`@ふういん`), & Rewards
- **Crystals (刻印晶)**: each defeated enemy's multiplier measures the time since the [unseal clock](#6-unseal-to-sealing-room-lifecycle), not since the battle or room start (`_battle.cgi:145–173`).
- **Treasure**: a random stage treasure plus the JST weekday orb (`_npc_action.cgi:541–543`). Legacy loads the stage's `@treasures` before the battle acts (`lib/vs_king.cgi:19–23`), so job78's treasure retry (`lib/_skill.cgi:6200`) regenerates sealing chests from the same table; that retry is unreachable while the battle resolves in one request (#1862). A treasure found with the Depot full is lost, even with a matching stack (`_npc_action.cgi:73–75`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- **Victory Celebration**: Adds a temporary crowd to Event Plaza (`_win_vs_king.cgi`). [Event Plaza design §3](eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps) owns generation, presence retention, merchant-selection order and runtime boundaries; there is no independent fixed two-hour banquet window.
- **Room after sealing**: The room becomes watch-only and is deleted when its last member exits ([§6](#6-unseal-to-sealing-room-lifecycle)). Go currently deletes the lobby at settlement.

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

- Battle settlement adds MaoCount +1 for each real participant (`mao_c`, line 218); it is
  the only counter owner.
- The unseal clock is a single SQL row (`unsealed_at`), overwritten with the settlement time
  on **every** trigger, even when all rooms already exist (legacy rewrites `king_start.cgi`
  each time, line 224). A battle already in progress therefore sees the restarted clock.
  With no row, the multiplier is 1×.

**After commit** (best effort; failure keeps the committed state, and replay never repeats
the counter). Only the request that confirms its own commit runs this step; a lost commit
acknowledgement or a receipt replay opens nothing:

- For each king stage (`king1`–`king10`, `king99`) without an existing room, open one
  system-owned sealing room with its initial enemy roster. A stage never has two rooms
  (legacy skips an existing quest directory, line 168). The king99 roster is the clone
  snapshot of the unsealing party ([§3](#3-boss-skills--clones)), so settlement passes the
  participants' battle state to this step; the other stages use their catalog bosses.
- Publish news: `<party name>(<participant names>) によって封印されし者達の封印が解かれました！`
  (line 221). Legacy quests always have a name (`quest.cgi:561`); Go entries without one
  (the batch crawl, solo normal runs) leave it empty ([#2757](https://github.com/witchcraze/party2re/issues/2757)).

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

**Current gaps:**

- The battle resolves at start against the room's initial roster, keeps no per-room enemy state and has no leader-defeat exit check; until then any room member may start it ([#1864](https://github.com/witchcraze/party2re/issues/1864)).
