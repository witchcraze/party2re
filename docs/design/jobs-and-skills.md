# Jobs and Skills Design

## Purpose

This document specifies the domain definitions, availability conditions, job changes, mastery tracking, and skill invocation models.

The job-change rules were audited directly against the clean-room behavioral
references at `/home/witchcraze/dev/party2/party2/lib/job_change.cgi`,
`job_master.cgi`, `_data.cgi`, and `_skill.cgi`. The implementation does not
copy those files; the audit establishes the level gate, transition formula,
special-job item list, final-skill SP thresholds, and temporary job-memory
state described below.

## Job System

### Job Definition Model
Each job in the game is defined with:
- `ID`: Unique identifier (e.g. `job-01`)
- `Name`: Display name (e.g. `見習い`, `戦士`)
- `Growth Rates`: Stat multipliers applied per level-up:
  - `HPGrowth`, `MPGrowth`, `AttackGrowth`, `DefenseGrowth`, `AgilityGrowth`
  - Special: Onion Knight (`job-49` / たまねぎ剣士) gains dynamic stat growth equal to `int(SP * 0.02)` across all stats on level up (`party2/lib/_data.cgi:142`).
- `CMPTier`: Combat MP tier index (`0`..`5`) into `@c_mp_rate` (0.0, 0.5, 0.8, 1.0, 1.5, 2.0).
- `RequiredGender`: Optional gender requirement (`male`, `female`, or empty for any).
- `RequiredJobIDs`: Optional list of prerequisite job IDs required to change into this job.

Job definitions are loaded from data-driven JSON (`internal/core/job/data/jobs.json`).

### Combat MP (CMP) Formula
CMP represents maximum MP in combat, calculated from the character's level and the tier rates of both the current job and previous job (`party2/lib/_battle.cgi:1708`):
```text
tier_rates = [0.0, 0.5, 0.8, 1.0, 1.5, 2.0]
cmp = int(min(level, 99) * (tier_rates[current_job_tier] + tier_rates[old_job_tier]))
```

### Job Changes & Prerequisites
- All job changes require character level 20 or higher (`party2/lib/job_change.cgi:147`).
- Target job requirements are evaluated via clean-room `ValidateRequirements`:
  - **Prerequisite Tree** (`_is_need_job`): For jobs with `RequiredJobIDs`, either `CurrentJobID` or `OldJobID` must match one of the required IDs (`party2/lib/_data.cgi:303-309`).
  - **Gender Restrictions**: Male-only (jobs 13, 15, 17, 19, 47) and Female-only (jobs 14, 16, 18, 20, 48).
  - **Milestone Gating**:
    - Berserker (`job-21` / バーサーカー): requires `MonsterKills > 200` (`kill_m`).
    - Dark Knight (`job-22` / 暗黒騎士): requires `PvPWins > 50` (`kill_p`).
    - Demon Lord (`job-35` / 魔王): requires `MaoCount >= 1` (`mao_c`) and `item-029`.
    - Hero (`job-34` / 勇者): requires `HeroCount >= 5` (`hero_c`) and `item-028`.
    - Gambler (`job-46` / ギャンブラー): requires `CasinoWins >= 10` (`cas_c`) and `item-039`.
    - Majin (`job-52` / 魔人): requires `MonsterKills > 1000` (`kill_m`).
    - Gladiator (`job-74` / 剣闘士): requires `PvPWins > 30` (`kill_p`) when coming from base fighter jobs.
    - Dual Blader (`job-77` / 双剣士): requires `MonsterKills > 1500` (`kill_m`).
    - Treasure Hunter (`job-78` / トレジャーハンター): requires `JobLevel >= 50` (`job_lv`).
    - Onion Knight (`job-49` / たまねぎ剣士): requires `SP >= 300` when not having visited the job.
    - Dragon Noble (`job-70` / 天竜人): requires having previously held `job-70`.
    - Fire Fighter (`job-84` / 炎闘士): requires having equipped `armor-29` (炎の鎧) in the body slot in addition to prerequisite jobs (1, 4, 25, 30). Pre-validated before mutation and unequipped/consumed atomically within the job change transaction (`job_change.cgi:181-188`).
  - **Item Possession & Consumption**: Special jobs require possessing an item from inventory unless re-entering current, old, or mastered job, or entering Sage from Leisure job (`job-08` / 遊び人). Entering Gambler from Leisure job still requires possessing `item-039` (イカサマのサイコロ), but consumption is exempt (`job_change.cgi:164-179`). Item possession is pre-validated before committing any mutations.
- A change halves Max HP, Max MP, Attack, Defense, and Agility using integer
  truncation, clamping each result to 10. Current HP/MP are restored to the
  new maxima, level becomes 1, experience becomes 0, and the job-change count
  increases by one.
- The current job and SP are retained as the previous job and previous SP.
  Returning to that previous job restores its previous SP; a mastered job
  restores the SP retained in its mastery record; an unvisited job starts at
  zero SP.
- Every job change records a transition in the character's job history (`FromJobID` -> `ToJobID`).
- All state mutations, item/armor consumption, and completion side effects (all-job mastery news broadcasts and Hall of Fame legend inductions) execute strictly within the transaction boundary; transaction failures guarantee zero leaked news or legend side effects.

### Job Mastery
- A job becomes **Mastered** when the character's SP reaches the required SP
  of that job's final skill. Level 99 is not a mastery condition.
- Mastered jobs and their retained mastery SP are tracked persistently on the
  character's job record.

### Job Memory Exchange (おもいだす)
- With item `item-168` (Memory Fragment), a character may temporarily replace
  the current/previous pair with two mastered jobs and their retained SP.
- The original pair is stored as job memory and restored by the next exchange.
  Memory exchange does not apply the normal level/stat penalty or increment
  the job-change count.
- Reincarnated characters with OverLevel status (`OverLevel == true`) are prohibited
  from recalling or swapping job memories (`ErrJobUnavailable`, `party2/lib/job_change.cgi:361-364`).
- Both target jobs (`targetJobID` and `targetOldJobID`) must satisfy gender compatibility
  with the character's gender (`RequiredGender`, `party2/lib/job_change.cgi:388-395`).


### Future Memory & Recall (よびおこす)
- With item `item-207` (未来のカケラ / Future Fragment), a character may save a snapshot
  of their current job, previous job, level, experience, maximum HP/MP, combat stats,
  gender, and over-level flag into a persistent future memory slot.
- Slot limit is determined by `OverFuture` (`savedCount <= OverFuture`, default allows 1 saved slot).
- Characters currently using temporary job memory (`JobMemory != nil`) cannot save future memories.
- Recalling a future memory restores the character's level, experience, stats, and jobs,
  restores the SP for both jobs from their mastered SP records, recovers HP/MP to full,
  and consumes/deletes the future memory snapshot.

### All-Job Completion & Suppin Unlock
- Mastering all 72 completion jobs (`job-01` through `job-72`) marks the character as
  having completed all jobs (`AllJobsMastered = true`).
- Upon completion, a system-wide announcement is published to the news feed
  (`$mが全ての職業をマスターしました！`).
- Job `job-73` (すっぴん) is exclusively unlocked for characters that have mastered all 72 jobs.

### Job Mastery Catalog & Completion Rate (じょぶますたー)
- Legacy `job_master.cgi` allowed players to inspect character job progress across all 87 jobs and view total mastery percentage (`comp_par`).
- Exposed via `GET /characters/{id}/job-mastery` as a public inspection endpoint.
- For each job, status is determined as:
  - `mastered`: Recorded as mastered on the character (`MasteredJobs`) or active job meeting mastery SP (`SP >= master_sp`).
  - `learning`: Job has been experienced (currently active, previous job `OldJobID`, or present in `History`), but mastery SP is not yet reached.
  - `unlearned`: Job has never been experienced by the character.
- Completion percentage (`comp_par`) follows legacy `job_master.cgi:26`:
  ```text
  count = mastered_count * 1.0 + learning_count * 0.5
  comp_par = min(100, int(count / 86 * 100))
  ```
  where the denominator 86 derives from `$#jobs - 1` (87 catalog jobs minus 1).
- When all 72 completion jobs (`job-01` through `job-72`) are mastered, `all_jobs_mastered` is true and completion title `completion_title` is awarded (`ジョブマスター`).

### Legacy Action Reconciliation

| Legacy action | Reconstruction contract | Notes |
| --- | --- | --- |
| `てんしょく` | `POST /characters/{id}/change-job` / `Service.ChangeJob` | JSON transport replaces the text command. |
| `おもいだす` | `POST /characters/{id}/exchange-job` / `Service.ExchangeJob` | Uses `item-168`, two mastered jobs, and a persisted temporary snapshot. |
| `よびおこす` | `POST /characters/{id}/recall-future`, `POST /characters/{id}/future-memories` / `Service.RecallFutureMemory`, `Service.SaveFutureMemory` | Uses `item-207`, slot capacity governed by `over_future`, restores status snapshot and consumes slot. |
| `じょぶますたー` | `GET /characters/{id}/job-mastery` / `Service.GetJobMastery` | Public inspection of 87-job catalog progress, mastery percentage, and 72-job completion flag. |

### Clean-Room Naming & IP Compliance
In compliance with `.agents/rules/00-migration-constraints.md`:
- All job names in `jobs.json` use generic fantasy tropes (e.g. `黒魔術師`, `白魔術師`, `赤魔術師`, `青魔術師`, `時空術士`, `模倣師`, `道具使い`, `風水師`, `小悪魔`, `スライム騎手`, `数理術士`) rather than trademarked or franchise-specific terms (such as "魔道士" series, "ものまね士", "アイテム士", "算術士", "ミニデーモン").
- Job IDs (`starter`, `job-01` through `job-87`) remain stable across versions to maintain persistent character history and database integrity.

---

## Skill System

### Skill Definition Model
Skills represent special combat actions or abilities:
- `ID`: Unique identifier (e.g. `skill-01`)
- `Name`: Display name (e.g. `会心の一撃`, `ヒール`)
- `RequiredJobIDs`: List of job IDs allowed to use the skill (empty means any job).
- `RequiredSP`: Skill Point threshold at which this skill is learned and can be used.
- `MPCost`: Mana cost consumed upon skill execution.
- `Effect`: Combat effect produced:
  - `Kind`: Type of effect (e.g. `damage`, `heal`, `buff`).
  - `Power`: Numeric magnitude of the effect.

### Skill Availability Evaluation
Before a skill can be invoked, `CanUse` checks:
1. `Character.SP >= Skill.RequiredSP`
2. `Character.Stats.MP >= Skill.MPCost`
3. If `RequiredJobIDs` is non-empty, `Character.JobID` must match one of the allowed jobs.
4. If a required item is specified, the character's inventory must contain the item.

### Skill Execution
Upon invocation:
1. Availability is verified.
2. `MPCost` is deducted from the character's current MP (`Character.Stats.MP -= MPCost`).
3. The skill's `Effect` payload is returned for battle or field resolution.
