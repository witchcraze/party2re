# Jobs and Skills Design

## Purpose

This document records legacy rules, decisions and pitfalls for job changes, mastery and job memory, audited against `party2/lib/job_change.cgi`, `job_master.cgi`, `_data.cgi` and `_skill.cgi`. Job and skill definitions, requirement lists, growth and combat formulas are owned by `internal/core/job`, `internal/core/skill` and their data; combat skill behavior is owned by [Battle](battle.md).

## Job changes

- A change requires level 20 (`job_change.cgi:147`) and halves maximum HP/MP and base stats (truncated, minimum 10), restores HP/MP, resets level and experience and increments the job-change count.
- **Pitfall:** entering the gambler from the leisure job still requires holding item039 but does not consume it (`job_change.cgi:164–179`). Re-entering the current, previous or a mastered job needs no item.
- **Pitfall:** a cross-job change clears the limit-break flag (`job_change.cgi:225`), but re-selecting the current job keeps it while still resetting level and halving stats (`job_change.cgi:214–226`); the Dragon Noble wish keeps it too (`god.cgi:196–218`).
- Fire fighter (job84) requires armor29 equipped in the body slot; it is consumed in the change (`job_change.cgi:181–188`).
- Target SP comes from current SP when re-selecting the active job, previous SP when returning to the previous job, the retained SP of any other mastered job, otherwise 0 (`job_change.cgi:207–221,271–273`).

## Mastery and retained SP

- A job is mastered when the character **leaves** it with SP at or above its final skill's threshold; level 99 is not a condition. Mastery is awarded once.
- Leaving any job, including re-selecting the current one (`job_change.cgi:207–212,243–305`), records its latest SP, including decreases and zero. Falling below the threshold after mastery does not revoke mastery.
- **Decision:** entering a job alone creates no record; a record needs a departure, a mastery or an explicit recorded flag. Migrated history never invents a zero SP for unknown past transitions, and an unknown historical SP makes recall unavailable instead of guessing.
- The mastery percentage is `min(100, int((mastered + 0.5 × learning) / 86 × 100))` (`job_master.cgi:26`); **pitfall:** 86 comes from `$#jobs − 1` for the 87-job catalog. Mastering the 72 completion jobs unlocks job73 and publishes news.

## Job memory (`おもいだす`) and future memory (`よびおこす`)

- Item168 starts a persistent memory that survives sleep and is reverted manually. Item243 starts a temporary memory that sleep reverts and that cannot be reverted manually (`job_change.cgi:121`). Neither halves stats or counts as a job change.
- While either memory is active, job change (`job_change.cgi:151`), future-memory save/recall (`_data.cgi:1843`, `job_change.cgi:564`) and another memory are blocked. **Pitfall:** the Wishing Well SP exchange is blocked only during a persistent memory (`sp_change.cgi:74`); the god Lv99 limit break only during a temporary one (`god.cgi:121`).
- A limit-broken character cannot start or swap memories (`job_change.cgi:361–364`), and both target jobs must match the character's gender (`job_change.cgi:388–395`).
- Saving a future memory (item207) refreshes retained SP only for jobs that already have a record (`_data.cgi:1843–1867`). Recall restores the snapshot and, for each job without a record, leaves that SP position unchanged (`job_change.cgi:648–663`). Depot capacity is not a save condition.

## Clean-room naming

Job names use generic fantasy terms rather than franchise-specific ones (for example 模倣師 instead of ものまね士 and 数理術士 instead of 算術士), as required by the [migration constraints](../../.agents/rules/00-migration-constraints.md). Job IDs stay stable to protect character history.
