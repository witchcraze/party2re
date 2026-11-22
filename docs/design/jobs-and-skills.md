# Jobs and Skills Design

## Purpose

This document records legacy rules, decisions and pitfalls for job changes and mastery, audited against `party2/lib/job_change.cgi`, `job_master.cgi`, `_data.cgi` and `_skill.cgi`. Job and skill definitions, requirement lists, growth and combat formulas are owned by `internal/core/job`, `internal/core/skill` and their data; combat skill behavior is owned by [Battle](battle.md).

## Job changes

- **Pitfall:** entering the gambler from the leisure job still requires holding item039 but does not consume it (`job_change.cgi:164–179`). Re-entering the current or previous job needs no item, whereas a mastered job outside current/previous still requires its prerequisites, item or equipped armor (`_data.cgi:303–309`, `job_change.cgi:158–188`).
- **Pitfall:** a cross-job change clears the limit-break flag (`job_change.cgi:225`), but re-selecting the current job keeps it while still resetting level and halving stats (`job_change.cgi:214–226`); the Dragon Noble wish keeps it too (`god.cgi:196–218`).

## Mastery and retained SP

- A job is mastered when the character **leaves** it with SP at or above its final skill's threshold; level 99 is not a condition. Mastery is awarded once.
- Leaving any job, including re-selecting the current one (`job_change.cgi:207–212,243–305`), records its latest SP, including decreases and zero. Falling below the threshold after mastery does not revoke mastery.
- **Decision:** entering a job alone creates no record; a record needs a departure, a mastery or an explicit recorded flag. Migrated history never invents a zero SP for unknown past transitions, and an unknown historical SP makes recall unavailable instead of guessing.
- The mastery percentage is `min(100, int((mastered + 0.5 × learning) / 86 × 100))` (`job_master.cgi:26`); **pitfall:** 86 comes from `$#jobs − 1` for the 87-job catalog. Mastering the 72 completion jobs unlocks job73 and publishes news.

## Clean-room naming

Job names and the job temple use generic fantasy terms rather than franchise-specific ones (for example 模倣師 instead of ものまね士, 数理術士 instead of 算術士 and 転職の神殿 for the legacy temple), as required by the [migration constraints](../../.agents/rules/00-migration-constraints.md). Job IDs stay stable to protect character history.
