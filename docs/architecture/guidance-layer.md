# Architecture Guidance Layer & Repository Intelligence (.arch/)

## 1. Executive Summary

The **Guidance Layer** (`.arch/`) provides an AI-friendly, token-efficient semantic navigation index into the Party2Re codebase. It bridges high-level architectural understanding and low-level source code verification while strictly maintaining **Source Code & Comments as the absolute Ground Truth**.

```text
+-------------------------------------------------------------+
|                 Guidance Layer (.arch/)                     |
|  - system.architecture.json (Topology & Domain Map)        |
|  - modules/*.json (Granular Lock & Transaction Coordinates) |
+-------------------------------------------------------------+
                              |
                     source_ref (path#Symbol)
                              v
+-------------------------------------------------------------+
|                 Ground Truth (Production Go Code)           |
|  - Consumer interfaces, RunInTx methods, Go docstrings      |
+-------------------------------------------------------------+
```

---

## 2. Module Selection Criteria

To prevent documentation rot and keep maintenance overhead negligible, detailed module definitions (`.arch/modules/<module>.json`) are **not** created for every Go package. Instead, modules are evaluated against four objective criteria:

| Axis | Condition | Leverage |
| :--- | :--- | :--- |
| **C1: Transaction Depth** | Spans 2+ distinct entity/table mutations within a single `RunInTx` boundary. | **Required** |
| **C2: Lock Hierarchy** | Implements deterministic `SELECT ... FOR UPDATE` pessimistic locking (e.g., ascending ID sorting or multi-table order). | **High** |
| **C3: Shared / Escrow State** | Handles multi-player transactions, escrow balances, player-to-player transfers, or guild states. | **High** |
| **C4: Async Scheduling** | Integrates with Valkey delayed job queues and background execution workers. | **Medium** |

---

## 3. Target Tier Inventory

Based on the selection criteria above, all packages in `internal/` are triaged into three distinct tiers:

### Tier 1: High-Leverage Priority Targets (Core Guidance Scope)
*Modules meeting at least 2 criteria (C1, C2, C3). These represent the primary deadlock and concurrency race risks in Party2Re.*

| Module | Package Path | Primary Transaction Flows | Lock Hierarchy Order | Definition & Visual HTML |
| :--- | :--- | :--- | :--- | :--- |
| **Tavern** | `internal/tavern` | `OrderMeal`, `ClaimDelivery` | `characters(2) -> tavern_character_status(8)` | [tavern.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/tavern.html) |
| **Delivery** | `internal/delivery` | `AcceptQuest`, `CompleteDelivery`, `SendParcel`, `ClaimParcel` | `characters(2) -> inventory_items(3) -> delivery_parcels(8)` | [delivery.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/delivery.html) |
| **Bank** | `internal/bank` | `Deposit`, `Withdraw`, `Transfer` | `characters(2) -> bank_accounts(6, p1<p2) -> bank_transfers(8)` | [bank.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/bank.html) |
| **Auction** | `internal/auction` | `CreateListing`, `PlaceBid`, `Buyout`, `CancelListing` | `auction_listings(8) -> characters(2, bidder) -> characters(2, refund)` | [auction.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/auction.html) |
| **Guild** | `internal/guild` | `CreateGuild`, `Donate` | `characters(2) -> guilds(7) -> guild_members(7)` | [guild.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/guild.html) |
| **Shop** | `internal/shop` | `Purchase`, `Sell` | `characters(2) -> inventory_items(3)` | [shop.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/shop.html) |
| **Blacksmith** | `internal/blacksmith` | `Enhance` | `characters(2) -> inventory_items(3)` | [blacksmith.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/blacksmith.html) |
| **Adventure** | `internal/adventure` | `StartStage`, `Complete` (Worker) | `characters(2) -> adventures(8) -> inventory_items(3)` | [adventure.html](file:///home/witchcraze/dev/party2re/docs/architecture/modules/adventure.html) |

### Tier 2: On-Demand Targets (Secondary Scope)
*Modules with moderate complexity or single-resource mutations. Guidance files are authored on-demand only when substantial refactoring or cross-feature coupling occurs.*
- `alchemy`, `dungeon`, `casino`, `medal`, `depot`, `activity`, `farm`, `park`, `home`, `collection`, `chapel`, `secretshop`, `blackmarket`, `eventplaza`, `pvp`, `gvg`, `boss`, `rescue`.

### Tier 3: Out-of-Scope (Excluded from Module-Level JSON)
*Stateless utilities, infrastructure adapters, and self-contained domain entities where Go interfaces and docstrings provide 100% sufficient clarity.*
- **Utilities**: `id`, `pagination`, `validation`, `logging`, `ratelimit`, `valkey`, `architecture`
- **Core Entities**: `core/player`, `core/character`, `core/job`, `core/item`, `core/skill`, `core/progression`, `core/battle` (represented as high-level nodes in `system.architecture.json`, but require no `.arch/modules/` detail files).

---

## 4. Visual Documentation Hierarchy (HTML Delivery)

The Guidance Layer produces a two-level visual documentation suite:
1. **System Overview & Topology** (`docs/architecture/system-overview.html`):
   - High-level component topology, layer boundaries, and vertical slices.
2. **Module Deep Dives** (`docs/architecture/modules/<module>.html`):
   - Granular transaction boundaries, pessimistic lock sequence tables, and direct source links.

---

## 5. Automated Mechanical Verification (Zero-Token Overhead)

All `.arch` definitions are continuously protected against syntax errors, geometry violations, and symbol renaming via automated gates:

1. **Archify CLI Validation** (`make arch-validate` / `scripts/verify.sh` step `[4/7]`):
   - Enforces Showcase profile rules (<= 12 nodes on top-level topology).
   - Validates layout coordinates, label clearances, and Git revision bounds.
2. **Go AST Symbol Linter** (`internal/architecture/arch_test.go`):
   - Parses all `.arch/modules/*.json` files in **~0.02s** during standard `go test ./...`.
   - Verifies that every interface name, struct method, and exported type linked via `source_ref` (`path#Symbol`) strictly exists in the Go source code.
3. **Automated HTML Generation** (`make arch-build` / `cmd/arch-build`):
   - Compiles overview topology and standalone module HTML diagrams in sub-second execution.

---

## 5. Continuous Governance & Evolution Process (Issue-Driven Architecture)

The Guidance Layer is designed as a living, evolvable system rather than a static artifact. New tiers, module re-classifications, and artifact types can be proposed and adopted through the standard GitHub Issue workflow:

### A. Tier Promotion & Demotion Process
- **When to Promote (Tier 2/3 -> Tier 1)**: When an existing module is refactored to introduce multi-resource transactions (`RunInTx`), deterministic row-locking hierarchies, or multi-player escrow state.
- **When to Demote (Tier 1 -> Tier 2/3)**: When a feature is simplified, decoupled, or deprecated.
- **Procedure**: Submit an Architecture Issue (`.github/ISSUE_TEMPLATE/architecture.md`) describing the transaction changes and requesting module definition creation/archival.

### B. Introducing New Artifact Types & Schemas
- Future Guidance extensions (e.g. `shared_tables/*.json` for reverse Fan-in indexing, `workers/*.json` for delayed job state machines) follow the same RFC process:
  1. Define schema and invariants.
  2. Extend AST / validator checks in `internal/architecture/arch_test.go`.
  3. Submit via an Architecture PR.
