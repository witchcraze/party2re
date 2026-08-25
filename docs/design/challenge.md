# Continuous Endurance Challenge Design

## Overview

The Continuous Endurance Challenge Feature Module (`internal/challenge`) implements continuous combat survival trials where characters face escalating waves of enemies back-to-back using the shared Core Battle engine (`vs_challenge.cgi`, `challenge.cgi`).

---

## Architectural Policy & Boundaries

- **Session-Based Consecutive Combat**: Characters enter a challenge tier and battle consecutively from Wave 1 onwards. HP is carried forward across rounds with partial post-round recovery.
- **Wave Scaling Formula**: Enemy combat stats scale dynamically based on the current round number:
  $$\text{Stat}_{\text{round}} = \text{BaseStat} \times (1 + \text{ScaleFactor} \times (\text{Round} - 1))$$
- **Reward Ledger & Cashout vs Defeat Risk**:
  - EXP and Gold accumulate in a temporary session ledger across victorious waves.
  - **Safe Retreat (Cashout)**: Commits 100% of accumulated EXP, Gold, and milestone items to the character.
  - **Defeat**: Awards 50% consolation EXP and Gold; temporary items are forfeited.
- **Leaderboards & Streak Records**: Tracks all-time highest streak round reached per tier with tie-breaking by earliest completion timestamp.

---

## Challenge Tiers Catalog

| Tier ID | Name | Min Level | Scale Factor | Milestone Interval | Milestone Drops |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `novice` | 初級チャレンジ (Novice Trial) | Lv 5 | $+8\%$ / round | Every 5 waves | Minor Potion, Medicinal Herb |
| `intermediate` | 中級チャレンジ (Veteran Trial) | Lv 20 | $+10\%$ / round | Every 5 waves | Standard Potion, Minor Elixir, Iron Ore |
| `master` | 上級チャレンジ (Master Trial) | Lv 40 | $+12\%$ / round | Every 5 waves | High Potion, Standard Elixir, Mithril Ore |
| `abyss` | 奈落チャレンジ (Abyss Trial) | Lv 60 | $+15\%$ / round | Every 5 waves | High Elixir, Orichalcum Ore, Philosopher's Stone |

---

## Round State Machine

```text
[Start Session] -> Active (Round 1, 100% Max HP)
      |
      v
[Execute Round] -> Core Battle Engine Resolve
      |
      +---> [Victory] -> +20% Max HP Recovery (capped at Max HP)
      |                  + Accumulate Round EXP & Gold
      |                  + Check Milestone Item Drops (every 5 rounds)
      |                  + Advance Round (CurrentRound++)
      |                  + Choice: [Execute Next Round] or [Cashout]
      |
      +---> [Defeat]  -> 50% Accumulated EXP & Gold awarded (Items forfeited)
                         Record Streak & Update Leaderboard
                         Session Terminated (Status: Defeated)
```

---

## Persistence Schema

- `character_challenge_records`: Primary key (`character_id`, `tier_id`), tracking `highest_round`, `total_attempts`, `total_victories`, and `best_cleared_at`.
- `challenge_sessions`: Active session state tracking `character_id`, `tier_id`, `current_round`, `character_current_hp`, `accumulated_exp`, `accumulated_gold`, `accumulated_items_json`, and `status`.
