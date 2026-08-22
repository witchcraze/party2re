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

## License record

`THIRD_PARTY_LICENSES.md` is the authoritative human-readable record for
software dependencies.

For each direct dependency, record:

- package name and exact version;
- purpose;
- repository or source URL;
- license;
- relevant copyright and distribution notices;
- relevant transitive dependency licenses when they affect distribution.

Update the file in the same Issue/PR that adds or changes a dependency. Keep
`README.md` concise: it should describe the policy and link to
`THIRD_PARTY_LICENSES.md`, not duplicate the full dependency inventory.

`go.mod` and `go.sum` remain the machine-readable dependency records.
Software dependency records do not replace the separate provenance and license
records for images, fonts, or other creative assets.

The development workflow runs Go and resolves dependencies inside Docker
containers. Dependencies must not be installed into the host environment as
part of normal project setup.

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
