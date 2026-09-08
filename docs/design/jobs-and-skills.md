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
- `MinLevel`: Minimum character level required to change to this job.
- `RequiredGender`: Optional gender requirement (`male`, `female`, or empty for any).

Job definitions are loaded from data-driven JSON (`internal/core/job/data/jobs.json`).

### Job Changes & History
- A normal job change requires character level 20 or higher, plus the target
  job's level, gender, and mastered-job prerequisites.
- A change halves Max HP, Max MP, Attack, Defense, and Agility using integer
  truncation, clamping each result to 10. Current HP/MP are restored to the
  new maxima, level becomes 1, experience becomes 0, and the job-change count
  increases by one.
- The current job and SP are retained as the previous job and previous SP.
  Returning to that previous job restores its previous SP; a mastered job
  restores the SP retained in its mastery record; an unvisited job starts at
  zero SP.
- Every job change records a transition in the character's job history (`FromJobID` -> `ToJobID`).

### Job Mastery
- A job becomes **Mastered** when the character's SP reaches the required SP
  of that job's final skill. Level 99 is not a mastery condition.
- Mastered jobs and their retained mastery SP are tracked persistently on the
  character's job record.
- Special jobs may require one catalog item (or a weapon item) when changing
  into them. The item is consumed atomically with the character and job
  update. Re-entering a mastered job does not consume the item; a mastered
  player of the leisure job may enter the Sage or Gambler jobs without the
  item.

### Job Memory Exchange (おもいだす)
- With item `item-168` (Memory Fragment), a character may temporarily replace
  the current/previous pair with two mastered jobs and their retained SP.
- The original pair is stored as job memory and restored by the next exchange.
  Memory exchange does not apply the normal level/stat penalty or increment
  the job-change count.

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

### Legacy Action Reconciliation

| Legacy action | Reconstruction contract | Notes |
| --- | --- | --- |
| `てんしょく` | `POST /characters/{id}/change-job` / `Service.ChangeJob` | JSON transport replaces the text command. |
| `おもいだす` | `POST /characters/{id}/exchange-job` / `Service.ExchangeJob` | Uses `item-168`, two mastered jobs, and a persisted temporary snapshot. |
| `よびおこす` | `POST /characters/{id}/recall-future`, `POST /characters/{id}/future-memories` / `Service.RecallFutureMemory`, `Service.SaveFutureMemory` | Uses `item-207`, slot capacity governed by `over_future`, restores status snapshot and consumes slot. |

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
