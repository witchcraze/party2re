# Dependency and License Policy

## Before adding a dependency

Check:

1. whether the standard library can solve the requirement;
2. why the dependency is necessary;
3. its direct license;
4. relevant transitive dependency licenses;
5. maintenance and project health;
6. known security concerns;
7. compatibility with the eventual project license.

Record the reason and license information in the Issue/PR.

## Project license

Candidates are currently:

- MIT
- Apache-2.0
- AGPLv3

The final license will be selected after implementation dependencies are known.

## Assets

Images will be recreated rather than copied from the old Party2 implementation.

Creative Commons licenses are candidates for project assets, with the exact license determined per asset/source.

## No unknown provenance

Do not add code, images, fonts, or other assets when their provenance or license cannot be established.

## Dependency minimization

Dependencies are not forbidden, but each dependency should justify its maintenance and licensing cost.

Do not add a dependency simply because it provides a small convenience that can reasonably be implemented without it.

## Related documents

- [`../../AGENTS.md`](../../AGENTS.md) — mandatory dependency and license rules.
- [`agent-workflow.md`](agent-workflow.md) — where dependency checks occur in the workflow.
- [`../../README.md`](../../README.md) — current project-level license candidates.
