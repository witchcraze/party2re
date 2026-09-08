# Wishing Well (願いの泉) Design

## Overview

The Wishing Well (`sp_change.cgi` / `internal/wishingwell`) reproduces the authentic Party2 mechanism for permanently enhancing a character's base attributes by sacrificing Skill Points (SP) to the Goddess (`@女神`).

---

## Facility Specification

- **Location Name**: 願いの泉 (Wishing Well)
- **NPC Name**: `@女神` (Goddess)
- **Background Graphic**: `bgimg/sp_change.gif`
- **Legacy CGI Source**: `party2/lib/sp_change.cgi`

---

## SP Exchange Rates (ステータス変換レート)

Characters may sacrifice available SP to increase their stats according to authentic ratios:

| Target Stat | Japanese Name | Exchange Ratio | Formula |
| --- | --- | --- | --- |
| **Max HP** (`mhp`) | ＨＰ (たいりょく) | 1 SP → +2 Max HP | `$m{mhp} += $sp * 2` |
| **Max MP** (`mmp`) | ＭＰ (まりょく) | 1 SP → +2 Max MP | `$m{mmp} += $sp * 2` |
| **Attack** (`attack`, `at`) | 攻撃力 (こうげき) | 1 SP → +1 Attack | `$m{at} += $sp * 1` |
| **Defense** (`defense`, `df`) | 守備力 (ぼうぎょ) | 1 SP → +1 Defense | `$m{df} += $sp * 1` |
| **Agility** (`agility`, `ag`) | 素早さ (すばやさ) | 1 SP → +1 Agility | `$m{ag} += $sp * 1` |

> [!NOTE]
> Only maximum limits (`MaxHP` and `MaxMP`) are permanently increased. Current HP and MP are not modified during the exchange.

---

## Validation & Business Rules

1. **Minimum Offering**: SP to sacrifice must be at least 1 (`sp >= 1`).
2. **Affordability**: SP to sacrifice cannot exceed the character's available SP (`sp <= char.SP`).
3. **Job Memory Constraint**:
   If the character currently has an active `JobMemory` (`job_memory.cgi`), SP exchange is prohibited:
   - Dialogue / Error: `"思いだした職業のSPは使えません"`
4. **OverLevel Constraint**:
   If the character is in `OverLevel` state (`over_lv == 1`), SP exchange is permanently disabled:
   - Dialogue / Error: `"特別な強さを持った人はSPをささげてもステータスを上げられません"`
5. **Success Dialogue**:
   On successful sacrifice, the Goddess bestows the increase with the authentic message:
   `"SP <sp> のかわりに <StatName> を <StatIncrease> あたえましょう"`

---

## Architecture & Concurrency

- **Domain Logic**: Handled in `internal/core/character/sp_exchange.go` via `Character.ApplySPExchange`. Invariants protect `Character.SP` and `Character.Stats` from external corruption.
- **Service Layer**: `internal/wishingwell.Service` orchestrates validation, status reporting, and transactional execution.
- **Lock Hierarchy**: Acquires only character row lock (`FindByIDForUpdate`) conforming to Rank 2 (`RankCharacter`), preserving the deadlock-free locking hierarchy.
- **REST Endpoints**:
  - `GET /characters/{id}/wishing-well`: Returns status, current SP, eligibility flags, and NPC dialogues.
  - `POST /characters/{id}/wishing-well/exchange`: Sacrifices SP for permanent stat increase.
