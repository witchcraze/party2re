# King and World Boss Battles Design

## Overview

The King and World Boss Feature Module (`internal/boss`) provides high-tier endgame raid and sealing boss challenges (`vs_king.cgi`, `stage/king1..10.cgi`) using the shared Core Battle engine (`internal/core/battle`). Players challenge legendary boss encounters across 10 progressive tiers and an ultimate world boss tier to earn massive experience, gold, rare drop loots (crystals, orbs), and first-clear milestone bonuses.

---

## Architectural Policy & Boundaries

- **Core Battle Engine Isolation**: All boss encounters are resolved through `internal/core/battle.Engine`. The battle engine operates language-agnostically without boss-specific conditionals.
- **Data-Driven Encounters**: Boss attributes (stats, level requirement, experience/gold rewards, drop items, first-clear bonuses) are defined through a data catalog.
- **Player Progression & History**: Challenge records, total boss defeats (hero count / 英雄度), highest cleared tier, first-clear timestamps, and daily attempt limits are persisted in MariaDB (`character_boss_records`, `boss_challenge_history`).
- **Transactional Consistency**: Match history recording, character progression updates (level/EXP/money), item drop inventory awards, and record adjustments are committed within a single atomic database transaction.

---

## Domain Rules & Boss Tiers

### 1. Boss Tiers & Level Requirements

| Tier | Boss Name | Title | Min Level | HP | Attack | Defense | Agility | Base EXP | Base Gold | Drop Loots |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Tier 1** | レッドストーン・ガーディアン | 封印の尖兵 | Lv 15 | 250 | 45 | 30 | 20 | 300 | 500 | Potion |
| **Tier 2** | ブルーストーン・ゴーレム | 氷結の守護神 | Lv 25 | 500 | 80 | 60 | 35 | 600 | 1,000 | High Potion |
| **Tier 3** | エメラルド・ワイバーン | 碧空の暴君 | Lv 35 | 900 | 130 | 95 | 55 | 1,000 | 1,800 | Ether |
| **Tier 4** | アメジスト・ロード | 紫電の魔将 | Lv 45 | 1,400 | 190 | 140 | 75 | 1,600 | 2,800 | High Ether |
| **Tier 5** | トパーズ・キメラ | 砂塵の獣王 | Lv 55 | 2,000 | 260 | 190 | 100 | 2,400 | 4,000 | Elixir |
| **Tier 6** | オブシディアン・ナイト | 黒曜の覇者 | Lv 65 | 2,800 | 340 | 250 | 130 | 3,400 | 5,500 | Crystal I |
| **Tier 7** | クリスタル・ドラゴン | 光彩の巨竜 | Lv 75 | 3,800 | 430 | 320 | 165 | 4,600 | 7,500 | Crystal II |
| **Tier 8** | ダークネス・ベヒモス | 深淵の殲滅者 | Lv 85 | 5,000 | 530 | 400 | 200 | 6,000 | 10,000 | Crystal III |
| **Tier 9** | アビス・ルーラー | 黄泉の帝王 | Lv 95 | 6,500 | 640 | 490 | 245 | 8,000 | 14,000 | Dark Orb |
| **Tier 10** | 全てを無に還す者 | 終焉の破壊神 | Lv 99 | 8,500 | 760 | 590 | 300 | 12,000 | 20,000 | Light Orb |
| **Tier 99** | 太古の創世神 | 天界の守護龍神 | Lv 99 | 12,000 | 920 | 720 | 360 | 25,000 | 50,000 | Rainbow Orb |

---

### 2. Challenge & Prerequisite Gates

1. **Level Gate**: A character cannot challenge a boss if `character.Level < boss.MinLevel`.
2. **Sequential Tier Unlocking**:
   - Tier 1 has no prerequisite.
   - For Tier $N > 1$, the character must have cleared Tier $N - 1$ (`HighestTierCleared >= N - 1`).
   - Tier 99 requires Tier 10 to be cleared (`HighestTierCleared >= 10`).
3. **Daily Attempt Limits**:
   - Each boss encounter permits up to `DailyEntryLimit = 3` attempts per day.
   - Daily attempts reset at `00:00:00 UTC`.

---

### 3. Rewards & Milestone Bonuses

- **Victory Rewards**:
  $$\text{EXP Awarded} = \text{Base EXP} + (\text{First Clear Bonus if Applicable})$$
  $$\text{Gold Awarded} = \text{Base Gold} + (\text{First Clear Bonus if Applicable})$$
  - Upon first clear of a tier, the character receives first-clear bonus EXP & Gold, and updates `HighestTierCleared`.
  - Increments `TotalBossDefeats` (hero count / 英雄度).
  - Rewards first drop item from `DropItemIDs` into character inventory.
- **Defeat / Draw**:
  - Consumes 1 daily challenge attempt.
  - Grants 0 EXP, 0 Gold, 0 items without corrupting character health or inventory state.

---

## Leaderboard & History

- **Boss Leaderboard**: Characters are ranked by `HighestTierCleared` descending, `TotalBossDefeats` descending, and `FirstClearedAt` ascending.
- **Challenge History**: Records turn count, outcome, rewards, drop items, and first-clear flags.
