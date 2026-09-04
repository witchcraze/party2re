# OpenAPI Specification Management & Synchronization

This document establishes the architecture, workflow, and developer/agent rules for maintaining the Party2 REST API OpenAPI 3.1 specification.

---

## 1. Architectural Architecture & Single Source of Truth (SSOT)

The Party2 HTTP REST API exposes over 185 endpoints and 200 operations. To prevent massive context-window bloat, latency, and merge conflicts associated with editing a single 16,500+ line JSON document, the OpenAPI specification is split into modular source files.

```text
docs/api/
├── base.json                 <-- SSOT: Base metadata, tags, component schemas, securitySchemes
├── paths/                    <-- SSOT: Modular path definitions by domain
│   ├── admin.json
│   ├── adventure.json
│   ├── auction.json
│   ├── auth.json
│   ├── character.json
│   ├── shop.json
│   └── ... (38 modular domain files)
└── openapi.json              <-- COMPILED ARTIFACT (DO NOT EDIT DIRECTLY)

internal/api/http/
└── openapi.json              <-- COMPILED ARTIFACT (EMBEDDED IN GO BINARY, DO NOT EDIT DIRECTLY)
```

### The Cardinal Rule

> [!CAUTION]
> **NEVER manually edit `docs/api/openapi.json` or `internal/api/http/openapi.json` directly.**
>
> These two files are **deterministically compiled build artifacts**. Any manual changes made to them will be rejected by CI (`make openapi-check`) and overwritten on the next execution of `make openapi-sync`.
>
> All additions and modifications **MUST** be performed in `docs/api/base.json` or `docs/api/paths/{module}.json`.

---

## 2. Directory & File Organization

### `docs/api/base.json`
Contains top-level OpenAPI 3.1 metadata:
- `openapi`: Specification version (`"3.1.0"`).
- `info`: API title, version, and general description.
- `servers`: Server deployment targets (e.g. `http://localhost:8080`).
- `tags`: High-level group descriptions.
- `components`: Reusable data schemas (`components.schemas`) and security schemes (`components.securitySchemes`).

### `docs/api/paths/{module}.json`
Contains path definitions grouped by functional domain. Each file typically contains 2 to 15 path definitions (under 500 lines), allowing fast targeted edits with minimal token consumption:
- Each key is a URL path pattern (e.g. `"/characters/{id}/avatar"`).
- Path items contain HTTP method operations (`"get"`, `"post"`, `"put"`, `"delete"`).
- Every operation must define:
  - `summary`: Short descriptive summary.
  - `operationId`: Unique camelCase identifier (e.g. `"uploadCharacterAvatar"`).
  - `tags`: Array of tag names for documentation grouping.
  - `parameters`: Path/query parameters.
  - `responses`: HTTP response status codes and schema references.

---

## 3. Tooling & Automation (`scripts/sync_openapi.go`)

The repository includes an automated toolchain in `scripts/sync_openapi.go` that operates with zero external runtime dependencies using pure Go standard library AST parsers:

1. **Deterministic Bundler**:
   - Reads `docs/api/base.json` and merges all files in `docs/api/paths/*.json`.
   - Validates that no two path files declare the same URL path (collision prevention).
   - Generates sorted, 2-space indented JSON written to both `docs/api/openapi.json` and `internal/api/http/openapi.json`.
2. **Go AST Route Scanner & Scaffolder**:
   - Parses `internal/api/http/handler.go` (`func (h *Handler) Router()`) using `go/parser` and `go/ast`.
   - Extracts all registered `mux.HandleFunc("METHOD /path", ...)` routes.
   - Detects any route in `handler.go` that is not yet documented in `docs/api/paths/*.json`.
   - Automatically generates boilerplate endpoint specifications into the appropriate `docs/api/paths/{module}.json` with standard status codes (`200`, `400`, `401`, `403`, `500`) and path parameters.
3. **Synchronization & Coverage Guard**:
   - Validates that 100% of routes registered in Go code are documented in the specification.
   - Checks that compiled artifacts match the source files exactly.

---

## 4. Developer & Agent Workflow

### A. Adding a New HTTP Route

1. **Register the route in `internal/api/http/handler.go`**:
   ```go
   mux.HandleFunc("POST /characters/{id}/tokens", h.handleCreateAPIToken)
   ```
2. **Run automatic scaffolding**:
   ```bash
   make openapi-sync
   # or
   make openapi-scaffold
   ```
   The tool automatically parses `handler.go` via AST, identifies the route's domain (e.g. `character` or `auth`), creates or appends the endpoint boilerplate to `docs/api/paths/{module}.json`, and bundles the compiled artifacts.
3. **Flesh out the schema in `docs/api/paths/{module}.json`**:
   - Adjust `summary`, `description`, request body schemas, and response types.
   - If a new reusable model is needed, add it to `docs/api/base.json` under `components.schemas`.
4. **Re-synchronize**:
   ```bash
   make openapi-sync
   ```

### B. Modifying an Existing Endpoint

1. Locate the endpoint in `docs/api/paths/{module}.json`.
2. Edit only the relevant lines in that modular file.
3. Run `make openapi-sync` to propagate changes to compiled artifacts.
4. Verify with `make openapi-check`.

---

## 5. Verification Commands

| Command | Purpose |
| :--- | :--- |
| `make openapi-sync` | Scaffolds missing routes, formats modular files, and compiles `openapi.json`. |
| `make openapi-scaffold` | Explicit target to scaffold missing routes and synchronize. |
| `make openapi-check` | Verifies 100% route coverage and file synchronization without writing changes. |
| `make check` | Runs full repository verification pipeline, including formatting, linting, `openapi-check`, database migrations, and tests. |
