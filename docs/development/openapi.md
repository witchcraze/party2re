# OpenAPI maintenance

The [documentation rule](../../.agents/rules/02-documentation-sync.md#5-openapi-specification-synchronization-ssot--modular-paths)
owns mandatory source/generated-file constraints.

| Location | Responsibility |
| --- | --- |
| [`docs/api/base.json`](../api/base.json) | Metadata, reusable schemas, security schemes |
| [`docs/api/paths/`](../api/paths/) | Domain-grouped path operations |
| [`docs/api/openapi.json`](../api/openapi.json) | Generated documentation bundle |
| [`internal/api/http/openapi.json`](../../internal/api/http/openapi.json) | Generated server-embedded bundle |

Edit modular sources, never the two bundles. When adding a registered route,
`make openapi-scaffold` can supply a starting operation. Replace scaffolded
responses, parameters and authentication with the actual handler/DTO contract;
route coverage alone does not prove schema completeness.

Run `make openapi-sync` after source changes, then `make openapi-check` and the
normal [verification workflow](testing.md#2-tiered-verification-strategy).
`openapi-sync` formats and bundles; only `openapi-scaffold` adds missing routes.
The [synchronizer](../../scripts/sync_openapi/) uses Go AST route discovery,
rejects duplicate path declarations, and checks route coverage and deterministic
bundle equality. File sizes follow transport scope rather than the Go source ceiling.
