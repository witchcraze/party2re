# Jobs and Skills Design

## Purpose

This document specifies the domain definitions, availability conditions, job changes, mastery tracking, and skill invocation models.

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
- A character can change to any job whose level and gender requirements are satisfied.
- Changing to the currently equipped job is disallowed.
- Every job change records a transition in the character's job history (`FromJobID` -> `ToJobID`).

### Job Mastery
- When a character reaches Level 99 in a job, the job can be marked as **Mastered**.
- Mastered jobs are tracked persistently on the character (`MasteredJobs` list).

---

## Skill System

### Skill Definition Model
Skills represent special combat actions or abilities:
- `ID`: Unique identifier (e.g. `skill-01`)
- `Name`: Display name (e.g. `会心の一撃`, `ヒール`)
- `RequiredJobIDs`: List of job IDs allowed to use the skill (empty means any job).
- `RequiredLevel`: Minimum character level required.
- `MPCost`: Mana cost consumed upon skill execution.
- `Effect`: Combat effect produced:
  - `Kind`: Type of effect (e.g. `damage`, `heal`, `buff`).
  - `Power`: Numeric magnitude of the effect.

### Skill Availability Evaluation
Before a skill can be invoked, `CanUse` checks:
1. `Character.Level >= Skill.RequiredLevel`
2. `Character.Stats.MP >= Skill.MPCost`
3. If `RequiredJobIDs` is non-empty, `Character.JobID` must match one of the allowed jobs.
4. If a required item is specified, the character's inventory must contain the item.

### Skill Execution
Upon invocation:
1. Availability is verified.
2. `MPCost` is deducted from the character's current MP (`Character.Stats.MP -= MPCost`).
3. The skill's `Effect` payload is returned for battle or field resolution.
