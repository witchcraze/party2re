# Continuous Endurance Challenge Design

## Overview

The Continuous Endurance Challenge Feature Module (`internal/challenge`) implements continuous combat survival trials where characters face escalating waves of enemies back-to-back using the shared Core Battle engine (`vs_challenge.cgi`, `challenge.cgi`).

---

## Architectural Policy & Boundaries

- **Session-Based Consecutive Combat**: Characters enter a challenge stage (`0`〜`8`) and battle consecutively from Round 1 onwards until party defeat. HP is carried forward directly across rounds without inter-round healing (legacy parity with `vs_challenge.cgi` and `_battle.cgi`).
- **Authentic Stat Scaling Formula**: Enemy combat stats scale dynamically based on the current round number according to the authentic fixed legacy formula (`vs_challenge.cgi:68`):
  $$\text{Scale} = 1.0 + 0.10 \times \text{Round}$$
- **100% Defeat Reward Retention**:
  - EXP, Gold, and items accumulate in the session ledger across defeated monsters during the run.
  - On party wipe/defeat, players retain **100% of all accumulated EXP, Gold, and items** earned during the run.
  - There is no fictional "retire and cash out" mechanic or 50% defeat penalty.
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
| `8` | 真・最強王者 | 1 | Reincarnation (`need_over_lv`) | Round 30 | Stage 7 pool with Hard Mode stat scaling |

---

## Round State Machine

```text
[Start Session] -> Active (Round 1, 100% Max HP)
      |
      v
[Execute Round] -> Core Battle Engine Resolve
      |
      +---> [Victory] -> HP Carryover (no inter-round recovery)
      |                  + Accumulate Round EXP & Gold
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
- `challenge_sessions`: Active session state tracking `character_id`, `party_id`, `members_json`, `tier_id`, `current_round`, `character_current_hp`, `accumulated_exp`, `accumulated_gold`, `accumulated_items_json`, and `status`.
- `challenge_hall_of_fame`: Record-holding parties for each tier (`tier_id` PK, `highest_round`, `party_name`, `party_color`, `cleared_at`, `members_json`).

### Error Handling & Persistence Policy

- **Error Propagation**: All database writes (`SaveSession`, `SaveHallOfFame`, `FinalizeSession`) and Valkey active session mutations (`SaveActiveSession`, `AdvanceRound`) strictly propagate errors to the caller without suppression.
- **Repository Error Distinction**: Unexpected database errors during character lookups are directly propagated to the caller, while `corecharacter.ErrNotFound` is mapped to domain `ErrCharacterNotFound`.
- **Two-Phase Settlement**: On session defeat, durable state is first committed to MariaDB via `FinalizeSession`. Upon successful commit, the transient Valkey buffer is evicted (`DeleteActiveSession`) with a TTL safety net.

---

## Multi-Player Party Challenge & Hall of Fame Records

### 1. Multi-Player Party Challenge
- Party participation limits (`p_join`) range from 1 to 4 members depending on the stage.
- Party battles resolve simultaneously using `internal/core/battle.Engine` with dynamic turn ordering, HP, MP, CMP, and skill execution.
- Surviving party members carry forward their remaining HP to the next wave without inter-round healing.

### 2. Hall of Fame (`challenge_hall_of_fame`)
- Whenever a party sets a new tier high-water mark (`round > highest_round`), the run is immortalized in the Hall of Fame.
- Records include:
  - Tier ID, highest round cleared, party name, party color, and completion timestamp.
  - Member details: Character ID, Name, Job, OldJob, Stats (HP, MP, Attack, Defense, Agility).
  - Member icon: Surviving members show their active avatar; members who fell in battle are marked with a gravestone icon (`chr/099.gif`).
