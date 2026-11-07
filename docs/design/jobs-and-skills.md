# Jobs and Skills Design

## Purpose

This document specifies the domain definitions, availability conditions, job changes, mastery tracking, and skill invocation models.

The job-change rules were audited directly against the clean-room behavioral
references at `party2-main/party2/lib/job_change.cgi`,
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
- **OverLevel Flag Handling**: Normal cross-job transitions (`targetJobID != currentJobID`) reset
  the celestial limit break flag (`OverLevel = false`), requiring re-ascension in Heaven
  (`party2/lib/job_change.cgi:225`). However, same-job re-selection (`targetJobID == currentJobID`)
  preserves `OverLevel` status while still applying the Lv 1 reset and stat halving
  (`party2/lib/job_change.cgi:214-226`). Similarly, the celestial Dragon Noble wish
  (`WishCelestialDragon` in Heaven, `party2/lib/god.cgi:196-218`) preserves `OverLevel`.
- On departure, the outgoing job and its current SP become `OldJobID`/`OldSP`, `RecordMastery` awards first-time mastery when `SP >= master_sp`, and `RecordJobSP` refreshes the job's persisted SP (`job_change.cgi:243–305`). Target SP resolves from current `SP` when re-selecting the active job, `OldSP` when returning to `OldJobID`, `SavedSP` when re-entering any other mastered job, or `0` for unmastered jobs (`job_change.cgi:207–221, 271–273`).
- Every job change records a transition in the character's job history (`FromJobID` -> `ToJobID`).
- All state mutations, item/armor consumption, lock-guarded revalidation (e.g. rejecting concurrent job-memory activation), and completion side effects (all-job mastery news broadcasts and Hall of Fame legend inductions) execute strictly within the transaction boundary; transaction failures guarantee zero leaked side effects.

### Job Mastery & Recorded Job SP
- **First-Time Mastery (`RecordMastery`) vs. Departure SP Refresh (`RecordJobSP`)**:
  - A job becomes **Mastered** (`MasteredJobs`) when the character leaves it with `SP >= master_sp` (the final skill's required SP; Level 99 is not a mastery condition). `RecordMastery` triggers only on the first time a job reaches mastery so mastery notifications and 72-job completion checks never repeat.
  - Leaving any job (including same-job re-selection, `job_change.cgi:207–212, 243–305`) calls `RecordJobSP`, which records the job as experienced (`RecordedJobIDs`, persisted as `character_job_points.is_recorded = true`) and updates its latest SP (`RecordedJobSP` in `character_job_points`, plus `MasteredJobSP` in `character_job_masteries` when mastered), including decreases and zero. Decreasing SP below `master_sp` after mastery updates the retained SP without revoking `MasteredJobs`.
  - `SavedSP` is the canonical reader for retained SP across mastered job re-entry (`ChangeJob`), job memory exchange (`ExchangeJob`), future-memory recall (`RecallFutureMemory`), and mastery catalog inspection (`GetJobMastery`), reading `RecordedJobSP` first and falling back to `MasteredJobSP` for legacy mastery-only rows.
- **Record Existence & Migration Invariants**:
  - Entering a job alone does not create a departure record. Record existence (`HasRecordedJob`) requires a departure (`History.FromJobID`), a mastery record (`MasteredJobs`), or an explicit `RecordedJobIDs` flag (`is_recorded = true`).
  - Migration 103 backfilled known current/previous SP and mastery SP into `character_job_points` without inventing zero values for historical transitions. Migration 104 marked `is_recorded = true` only on rows backed by departure or mastery evidence; preservation-only rows (`is_recorded = false`) retain their SP across partial upserts but do not grant recall eligibility.
  - The battle adapter snapshots recorded jobs read-only for [in-adventure Tool168 memory](battle.md#temporary-transformation-and-job-memory) (including unknown SP for unrecorded historical transitions) and never persists to the job repository.

### Job Memory Exchange (おもいだす)
- Recalls two recorded jobs (`HasRecordedJob` with known `SavedSP`, including unmastered jobs) without level/stat halving or incrementing job-change counts; unrecorded jobs or unknown SP return `ErrJobUnavailable`. In-adventure Tool168 is a separate [previous-job-only operation](battle.md#temporary-transformation-and-job-memory).
- **Two Memory Lifecycles**:
  - **Persistent (`JobMemoryKindPersistent = "persistent"`)**: Started by consuming 1x `item-168` (記憶のカケラ); retained across `home.Sleep`; cleared only via manual revert (`ExchangeJob` without item), restoring remembered current/old jobs and SP. Existing `characters.job_memory_kind` rows default to `'persistent'`.
  - **Temporary (`JobMemoryKindTemporary = "temporary"`)**: Started by consuming 1x `item-243` (陽炎の追憶記); automatically reverted by `home.Sleep`; manual revert via `ExchangeJob` is prohibited (`ErrJobUnavailable`, `job_change.cgi:121`).
- **Facility Guard Matrix & Invariants**:
  - While either memory kind is active, normal job change (`ChangeJob`, `job_change.cgi:151`), future-memory save/recall (`SaveFutureMemory` / `RecallFutureMemory`, `_data.cgi:1843`, `job_change.cgi:564`), and starting another job memory are prohibited (`ErrJobUnavailable`).
  - Wishing Well SP exchange is prohibited only during persistent memory (`sp_change.cgi:74`); God Lv99 limit break is prohibited only during temporary memory (`god.cgi:121`).
  - Characters with `OverLevel == true` cannot start or swap job memories (`job_change.cgi:361-364`), and both target jobs must match the character's gender (`RequiredGender`, `job_change.cgi:388-395`).
  - All mutations execute under transactional row locks via `economy.ExecuteTransaction` (`job_change.cgi:361-400, 525-549`), keeping `CharacterJob.CurrentJobID` synchronized with `Character.JobID` across start, manual revert, and `home.Sleep` restoration.

### Future Memory & Recall (よびおこす)
- Consuming 1x `item-207` (未来のカケラ) saves a snapshot of current/previous jobs, level, experience, max HP/MP, combat stats, gender, and `OverLevel` into a future memory slot (`savedCount <= OverFuture`, default 1 slot).
- Saving a future memory refreshes persisted SP (`RecordJobSP`) for the current and previous jobs **only when their records already exist** (`HasRecordedJob`, `_data.cgi:1843–1867`).
- Preflight and transactional row-lock revalidation (Character Rank 2 -> Inventory Rank 3) enforce `JobMemory == nil` and `savedCount <= OverFuture`; lock-revalidation failures roll back without consuming `item-207` or creating a snapshot.
- Recalling a future memory restores the snapshot's level, experience, stats, and jobs, restores SP for each job whose record exists via `SavedSP` (leaving that SP position unchanged when no record exists, `job_change.cgi:648–663`), recovers HP/MP to full, and deletes the consumed snapshot.
- Home dispatch and Depot consumption remain tracked under the [Home reconciliation](home.md#legacy-dispatch-and-consumption-contract) and reuse the same snapshot and job-point contract.

### All-Job Completion & Suppin Unlock
- Mastering all 72 completion jobs (`job-01` through `job-72`) marks the character as
  having completed all jobs (`AllJobsMastered = true`).
- Upon completion, a system-wide announcement is published to the news feed
  (`$mが全ての職業をマスターしました！`).
- Job `job-73` (すっぴん) is exclusively unlocked for characters that have mastered all 72 jobs.

### Job Mastery Catalog & Completion Rate (じょぶますたー)
- Legacy `job_master.cgi` allowed players to inspect character job progress across all 87 jobs and view total mastery percentage (`comp_par`).
- Exposed via `GET /characters/{id}/job-mastery` as a public inspection endpoint.
- Genuine absence of a persisted `character_jobs` record initializes a read-only projection from the character's live job state without mutating storage, whereas storage or context cancellation errors during job-record loading propagate as errors (`500 Internal Server Error` over HTTP) rather than fabricating empty progress.
- For each job, status is determined as:
  - `mastered`: Recorded as mastered on the character (`MasteredJobs`) or active job meeting mastery SP (`SP >= master_sp`).
  - `learning`: Job has an explicit record (`CharacterJob.HasRecordedJob`, currently active, previous job `OldJobID`, or present in `History`), but mastery SP is not yet reached.
  - `unlearned`: Job has never been experienced by the character. Preserved SP rows with `is_recorded = false` without mastery or history evidence are not considered experienced.
- Current SP resolves with priority: active job `Character.SP`, previous job `Character.OldSP`, saved value `CharacterJob.SavedSP` (retaining known 0 SP without threshold substitution), or fallback (mastery threshold for mastered jobs without saved SP, 0 for unmastered).
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
| `おもいだす` | `POST /characters/{id}/exchange-job` / `Service.ExchangeJob` | Uses `item-168` (persistent) or `item-243` (temporary), two recorded jobs with saved SP, and a persisted job memory snapshot. |
| `よびおこす` | `POST /characters/{id}/recall-future`, `POST /characters/{id}/future-memories` / `Service.RecallFutureMemory`, `Service.SaveFutureMemory` | Uses `item-207`, slot capacity governed by `over_future` (revalidated under character lock within economy transaction boundary), restores status snapshot and consumes slot. |
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
