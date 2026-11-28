# Jobs and Skills Design

This document records enduring design decisions for jobs and mastery (`internal/core/job`, `internal/job`); definitions, requirements, growth, and formulas live in code and tests, and combat skill behavior lives in [Battle](battle.md).

## Decisions

- **Migrated job history and unrecorded SP:** Entering a job alone creates no record; a record requires a departure, a mastery, or an explicit recorded flag. Migrated history never invents `0` SP for unknown past transitions, and an unknown historical SP makes recall unavailable instead of guessing.
- **Stable job IDs:** Clean-room display names follow [migration constraints](../../.agents/rules/00-migration-constraints.md), while job IDs stay stable to protect character history.
