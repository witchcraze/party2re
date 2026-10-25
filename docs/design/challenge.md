# Continuous Endurance Challenge Design

## Overview

The Continuous Endurance Challenge Feature Module (`internal/challenge`) implements continuous combat survival trials where characters face escalating waves of enemies back-to-back using the shared Core Battle engine (`vs_challenge.cgi`, `challenge.cgi`).

---

## Architectural Policy & Boundaries

- **Session-Based Consecutive Combat**: Characters enter a challenge stage (`0`〜`8`) and battle consecutive rounds until defeat or individual departure. Each participant carries HP, MP and consumed-item state directly across rounds without inter-round recovery (legacy parity with `vs_challenge.cgi` and `_battle.cgi`).
- **Authentic Stat Scaling Formula**: Each non-treasure wave spawns `int(rand(2.5 + len(party) * 0.6)) + 1` enemies drawn randomly from the stage's monster pool (`vs_challenge.cgi:68`, `_npc_action.cgi:add_monster`). Enemy combat stats scale dynamically by `(0.9 + rand(0.3) + (len(party) - 2) * 0.1) * Scale` and truncate via `int(...)`, while EXP and Gold rewards remain unscaled:
  $$\text{Scale} = 1.0 + 0.10 \times \text{Round}$$
- **100% Defeat Reward Retention**:
  - Each living participant earns the round's EXP and Gold; a fallen member retains prior earnings but earns no new rewards until revived (`_battle.cgi:208–214`). Leader death does not stop surviving allies.
  - On party wipe/defeat, all participants retain **100% of their earned EXP and Gold**. The current shared treasure ledger pays its designated leader recipient once; the [individual-exit rules](#individual-exit) distinguish that ledger from unreconciled per-player treasure interactions.
  - There is no 50% defeat penalty. Voluntary departure follows the [individual-exit rules](#individual-exit); the current service has no exit command.
- **Authentic Treasure Room Trigger**:
  - When no treasure has been awarded yet in the run and $\text{Round} \ge \text{TreasureThreshold}$ (drawn in `10..20` inclusive on Stage `0` per `challenge/0.cgi`, or fixed at `TreasureRound` on other stages), a 10% roll (`rand < 0.10`) enters the treasure branch instead of spawning monsters (`vs_challenge.cgi:64–69`), awarding an item from the stage's pool with no combat.
- **Leaderboards & Hall of Fame**: Tracks all-time highest round reached per stage with tie-breaking by earliest completion timestamp.

---

## Individual Exit

The original `lib/_skill.cgi:set_action` exposes `にげる`, dispatched to
`lib/_battle.cgi:nigeru:1184–1222,1330–1346` (reference revision
`ff97fc549932a146cc945d7edf71b44c868a840e`). These are reconstruction requirements;
the participant exit service settles these effects under the run transition boundary.

- A participant leaves and receives only their own earned EXP/Gold, remaining
  HP/MP and consumed-item settlement. Other participants keep their resources,
  earnings and current encounter/round. Departed characters never fight or earn
  another round's rewards in that run. Spectator departure grants no rewards.
- If the leader leaves, the first remaining participant in roster order becomes
  leader. Only the last participant's departure ends the run. The former leader
  can enter another run without changing the continuing run's identity.
- The initial `すすむ` is leader-only; after that the original checks only that
  enemies are gone (`lib/vs_challenge.cgi:20–53`). Leader handoff must preserve
  progression rather than trapping the party behind the original leader's ID.
- Exit fatigue is `3 * legacy round + 1`; calm mode halves that amount with
  truncation. The original clears meal fullness and persists combat resources.
  Equipped item 105 adds `20 * legacy round` EXP; item 106 adds
  `floor(legacy round * U)`, with `U` uniform in `[0,77)`.
  Multiplying the round by an integer draw in `[0,77)` is not equivalent.
  The legacy initial round is zero; after victories Go's `CurrentRound` names
  the next round, so an exit
  after victories uses `CurrentRound - 1`. Defeat must account for the attempted
  round separately. Current Challenge has no calm-mode selection; that absence
  does not authorize pretending an unrecorded mode was selected.
- Use existing shared combat persistence and Tavern fullness ownership for these
  effects. Equipment eligibility must account for cumulative combat consumption;
  an exit receipt fixes any random bonus once. Fullness and all required durable
  effects belong to the same settlement transaction, without resetting meal
  history. Any post-commit meal delivery uses the existing
  [Tavern arrival contract](tavern.md#3-delivery-reservation-system-reservedelivery--claimdelivery)
  and cannot replay successful exit effects.
- The existing shared treasure ledger is not a per-player entitlement model.
  Each recorded award keeps one designated recipient and is delivered once;
  leadership changes do not retarget already-earned treasure. An award after
  handoff names its recipient when earned. Full individual treasure inspection
  remains unreconciled; [#1207](https://github.com/witchcraze/party2re/issues/1207)
  owns the encounter/treasure-branch correction, not that entire lifecycle.

The approved [participant-exit storage contract](../architecture/transient-run-state.md#challenge-participant-exit-target)
defines identity, receipts and recovery. The [Gateway contract](../architecture/client-agent-api.md#3-command-pillar-post-apiv1charactersidactions)
exposes caller-only exit with explicit run identity and revision, delegating all
settlement and publication recovery to this boundary.

---

## Authentic Stages Catalog (`0`〜`8`)

| Stage ID | Name | Max Participants (`p_join`) | Join Constraints (`need_join`) | Treasure Floor Threshold | Base Monster Pool / Enemy |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `0` | 最弱逆襲 | 1 | `hp_400_u` (Max HP < 400) | Round 10–20 (`int(rand(11)+10)`) | スライム |
| `1` | 怒羊牧場 | 2 | `hp_400_u` (Max HP < 400) | Round 10 | ラースシープ |
| `2` | 恐怖肝試 | 2 | None | Round 10 | ゴースト, メイジゴースト, ミイラ男, マミー, ガイコツ剣士, ブラッドハンド, 死霊 |
| `3` | 幻覚猛毒 | 3 | None | Round 10 | オバケキノコ, ドクキノコ, マージマタンゴ |
| `4` | 即死博打 | 4 | None | Round 10 | イタズラ妖精, ギャンブル妖精, ブルーストーン, あやしい影, ミミック, パンドラボックス |
| `5` | 花火大会 | 4 | None | Round 20 | 爆弾岩 |
| `6` | 鉄壁要塞 | 3 | None | Round 20 | レッドストーン, ブルーストーン, イエローストーン, グリーンストーン, パープルストーン, シルバーストーン, ブラックストーン, メタルスライム, ハグレメタル |
| `7` | 最強王者 | 1 | None | Round 30 | 人面樹, 亡霊剣士, デビルシェル, ゴーレム, 闇の魔術士, ギガンテス, ひくいどり, ベヒーモス, キングスライム, 死霊の騎士, 竜王, 片翼の天使, ディアボロス, ボマー |
| `8` | 真・最強王者 | 1 | Celestial OverLevel (`need_over_lv`; no level reset) | Round 30 | Stage 7 pool with Hard Mode stat scaling |

---

## Round State Machine

```text
[Start Session] -> Active (Round 1, 100% Max HP)
      |
      v
[Execute Round]
      |
      +---> [Treasure Branch] (no treasure yet, Round >= Threshold, rand < 0.10)
      |            -> Award Item from Stage Pool (no combat, 0 EXP/Gold)
      |            -> Update Hall of Fame & Advance Round (CurrentRound++)
      |
      +---> [Monster Combat Branch] -> Core Battle Engine Resolve
                   |
                   +---> [Victory] -> HP/MP/Consumption Carryover (no inter-round recovery)
                   |                  + Accumulate unscaled EXP & Gold for living members
                   |                  + Update Hall of Fame & Advance Round (CurrentRound++)
                   |
                   +---> [Defeat]  -> Award 100% Accumulated EXP, Gold, and Items
                                      Record High Score & Update Leaderboard / Hall of Fame
                                      Session Terminated (Status: Defeated)
```

---

- `character_challenge_records`: Primary key (`character_id`, `tier_id`), tracking `highest_round`, `total_attempts`, `total_victories`, and `best_cleared_at`.
- `challenge_sessions`: Immutable admission evidence and terminal status; live rounds and provisional rewards reside in Valkey. Run coordination, participant claims/settlement outcomes and actor-owned command receipts have separate durable tables.
- `challenge_hall_of_fame`: Record-holding parties for each tier (`tier_id` PK, `highest_round`, `party_name`, `party_color`, `cleared_at`, `members_json`).

### Error Handling & Persistence Policy

- **Error Propagation**: All database writes (`SaveSession`, `SaveHallOfFame`, `FinalizeSession`) and Valkey publication (`PublishRun`) strictly propagate errors to the caller without suppression.
- **Repository Error Distinction**: Unexpected database errors during character lookups are directly propagated to the caller, while `corecharacter.ErrNotFound` is mapped to domain `ErrCharacterNotFound`.
- **Settlement and Publication**: Participant exit pays only the caller; defeat pays only remaining unsettled participants. SQL commits resources, consumption, earnings, records, exit effects and the receipt/publication intent together. Publication failure remains observable pending recovery; an identical actor/run/revision retry returns its receipt without paying or drawing a bonus again. See the [settlement contract](../architecture/transient-run-state.md#challenge-finalization).

---

## Multi-Player Party Challenge & Hall of Fame Records

### 1. Multi-Player Party Challenge
- Party participation limits (`p_join`) range from 1 to 4 members depending on the stage.
- Party battles resolve simultaneously using `internal/core/battle.Engine` with dynamic turn ordering, HP, MP, CMP, and skill execution.
- Surviving party members carry forward remaining HP/MP and consumed-item state to the next wave without inter-round recovery (`vs_challenge.cgi:48–53`, `_battle.cgi:453–554`). Consumed revival items cannot be rebound on the next wave.

### 2. Hall of Fame (`challenge_hall_of_fame`)
- Whenever a party sets a new tier high-water mark (`round > highest_round`), the run is immortalized in the Hall of Fame.
- Records include:
  - Tier ID, highest round cleared, party name, party color, and completion timestamp.
  - Member details: Character ID, Name, Job, OldJob, Stats (HP, MP, Attack, Defense, Agility).
  - Member icon: Surviving members show their active avatar; members who fell in battle are marked with a gravestone icon (`chr/099.gif`).
