# Job Mastery & Character Rebirth Design

## Overview

The Job Mastery and Rebirth (ジョブマスター・転生) system provides end-game progression. Characters who reach maximum level (`Level 99`) master their active job and can undergo reincarnation/rebirth to restart at `Level 1` with permanent bonus stats while retaining all mastered jobs and achievements.

## Domain Model

### Job Mastery
- **Requirement**: Reaching `Level 99` in a job.
- **State**: Tracked in `CharacterJob.MasteredJobs` list.
- **Persistence**: Preserved across job changes, level resets, and rebirths.

### Character Rebirth (Reincarnation)
- **Requirement**: Character must be `Level 99` (`MinLevelForRebirth`).
- **Reset Invariants**:
  - `Level` is reset to `1`.
  - `Experience` is reset to `0`.
  - `RebirthCount` is incremented by `1`.
  - Mastered jobs and job history are fully preserved.
- **Permanent Stat Bonus**:
  - Each rebirth grants `+5` permanent bonus points (`PermanentBonusPerRebirth`) to initial base stats (HP, MP, Attack, Defense, Agility).
  - Base stats at Level 1 after rebirth:
    - `MaxHP`: `30 + (RebirthCount * 10)`
    - `MaxMP`: `10 + (RebirthCount * 5)`
    - `Attack`: `10 + (RebirthCount * 5)`
    - `Defense`: `10 + (RebirthCount * 5)`
    - `Agility`: `10 + (RebirthCount * 5)`
