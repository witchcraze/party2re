# Continuous Endurance Challenge Design

## Overview

The Continuous Endurance Challenge Feature Module (`internal/challenge`) implements continuous combat survival trials where characters face escalating waves of enemies back-to-back using the shared Core Battle engine (`vs_challenge.cgi`, `challenge.cgi`).

---

## Architectural Policy & Boundaries

- **Session-Based Consecutive Combat**: Characters enter a challenge stage (`0`〜`8`) and battle consecutively from Round 1 onwards until party defeat. Each participant carries HP, MP and consumed-item state directly across rounds without inter-round recovery (legacy parity with `vs_challenge.cgi` and `_battle.cgi`).
- **Authentic Stat Scaling Formula**: Enemy combat stats scale dynamically based on the current round number according to the authentic fixed legacy formula (`vs_challenge.cgi:68`):
  $$\text{Scale} = 1.0 + 0.10 \times \text{Round}$$
- **100% Defeat Reward Retention**:
  - Each living participant earns the round's EXP and Gold; a fallen member retains prior earnings but earns no new rewards until revived (`_battle.cgi:208–214`). Leader death does not stop surviving allies.
  - On party wipe/defeat, all participants retain **100% of their earned EXP and Gold**. The current shared treasure ledger pays its designated leader recipient once; per-player treasure interactions remain part of [#1207](https://github.com/witchcraze/party2re/issues/1207).
  - Voluntary exit (`_battle.cgi:nigeru`, HP/MP/item settlement at lines 1208–1222) remains tracked in [#1208](https://github.com/witchcraze/party2re/issues/1208); its eventual command must use the same terminal settlement contract. There is no 50% defeat penalty.
- **Authentic Treasure Room Trigger**:
  - For rounds $\ge \text{TreasureRound}$ (10, 20, or 30 depending on the stage), there is a 10% chance per floor (`rand < 0.10`) to encounter a treasure room / receive treasure from the stage's item pool if no treasure has been awarded yet in that run (`vs_challenge.cgi:64`).
- **Leaderboards & Hall of Fame**: Tracks all-time highest round reached per stage with tie-breaking by earliest completion timestamp.

---

## Authentic Stages Catalog (`0`〜`8`)

| Stage ID | Name | Max Participants (`p_join`) | Join Constraints (`need_join`) | Treasure Floor Threshold | Base Monster Pool / Enemy |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `0` | 最弱逆襲 | 1 | `hp_400_u` (Max HP < 400) | Round 10 | スライム |
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
[Execute Round] -> Core Battle Engine Resolve
      |
      +---> [Victory] -> HP/MP/Consumption Carryover (no inter-round recovery)
      |                  + Accumulate EXP & Gold for living members
      |                  + Check Treasure Room Trigger (if Round >= TreasureRound and rand < 0.10)
      |                  + Advance Round (CurrentRound++)
      |                  + [Execute Next Round]
      |
      +---> [Defeat]  -> Award 100% Accumulated EXP, Gold, and Items
                         Record High Score & Update Leaderboard / Hall of Fame
                         Session Terminated (Status: Defeated)
```

---

- `character_challenge_records`: Primary key (`character_id`, `tier_id`), tracking `highest_round`, `total_attempts`, `total_victories`, and `best_cleared_at`.
- `challenge_sessions`: Durable start identity and terminal snapshot; live rounds and provisional rewards reside in Valkey. The session ID also guards final settlement.
- `challenge_hall_of_fame`: Record-holding parties for each tier (`tier_id` PK, `highest_round`, `party_name`, `party_color`, `cleared_at`, `members_json`).

### Error Handling & Persistence Policy

- **Error Propagation**: All database writes (`SaveSession`, `SaveHallOfFame`, `FinalizeSession`) and Valkey active session mutations (`SaveActiveSession`, `AdvanceRound`) strictly propagate errors to the caller without suppression.
- **Repository Error Distinction**: Unexpected database errors during character lookups are directly propagated to the caller, while `corecharacter.ErrNotFound` is mapped to domain `ErrCharacterNotFound`.
- **Two-Phase Settlement**: On session defeat, `FinalizeSession` claims the existing active SQL session and commits the terminal snapshot, all participants' resources/consumptions, earned rewards and records together. Duplicate claims return `ErrSessionNotActive` without another payout. Valkey cleanup follows commit and is best-effort; see the [settlement contract](../architecture/transient-run-state.md#challenge-finalization) for failure and retry behavior.

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
