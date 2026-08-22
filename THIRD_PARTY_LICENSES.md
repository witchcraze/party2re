# Third-Party Software Licenses

This file is the authoritative human-readable record of software dependencies
used by Party2 Re.

Dependency versions are recorded in `go.mod` and `go.sum`. This file records
their purpose, source, license, and distribution notices. Update it in the
same Issue/PR that adds or changes a dependency.

## Direct dependencies

| Package | Version | Purpose | License | Source |
| --- | --- | --- | --- | --- |
| `github.com/go-sql-driver/mysql` | v1.9.3 | MariaDB access through `database/sql` | MPL-2.0 | https://github.com/go-sql-driver/mysql |

## Indirect dependencies

| Package | Version | Purpose | License | Source |
| --- | --- | --- | --- | --- |
| `filippo.io/edwards25519` | v1.1.0 | Indirect dependency of the MySQL driver | BSD-3-Clause | https://github.com/FiloSottile/edwards25519 |

## Distribution notices

The MySQL driver is distributed under MPL-2.0. Preserve its license and
copyright notices when distributing the application. The driver is used
without source modifications.

Creative assets such as images and fonts are tracked separately under
`docs/assets/`.
