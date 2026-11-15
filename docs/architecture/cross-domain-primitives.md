# Cross-Domain Application Runtime Primitives

`internal/economy` supplies single-character currency/inventory transactions.
Feature services own game rules; these primitives do not establish prices,
prayer costs, enhancement mechanics, or any other legacy behavior.
`internal/core/event` supplies in-process event dispatch.

## Resource contracts

`ResourceCost` supports Gold, SmallMedals, a specific inventory instance and
quantity, or an item definition and quantity. `ResourceGrant` supports Gold,
SmallMedals, and item-definition rewards. See `internal/economy/runner.go` for
the exact exported fields; depot and gem-box state are not represented here.

Amounts must be nonnegative. A cost succeeds when **balance >= cost**, not when
cost exceeds balance. The runner checks available currency/items, deducts
costs, applies grants, enforces inventory capacity, and persists the result.
Feature-specific alternate currency checks remain in the owning feature;
Blacksmith seals consume crystals, not a generic gold enhancement fee.

## TransactionRunner

```go
type TransactionRunner interface {
    ExecuteTransaction(ctx context.Context, req TransactionRequest,
        fn TransactionCallback) (*TransactionResult, error)
}
```

`TransactionRequest` identifies the character, static `Cost` or a locked-state
`CostFunc`, `Grant`, and optional `LockInventory`. A `TxContext` exposes the
locked Character and Inventory, permits additional grants via `AddGrant`, and
collects domain events via `EmitEvent`. `economy.Run[T]` is the typed callback
helper. A callback error prevents successful transaction completion.

## Transaction scopes

| Scope | Boundary and responsibilities |
|---|---|
| One character, wallet/medals/inventory | `TransactionRunner`; character Rank 2, inventory Rank 3 when needed |
| Depot, gem box, or other aggregates | Injected `TransactionProvider`; feature coordinates all required ranks |
| P2P transfer | One transaction; character IDs sorted ascending before inventory/depot |
| Shared sale/listing purchase, helper quest completion or Challenge run transition | Lock Rank 0 shared row first, then ascending characters and dependent assets |

### 4.3 Lock acquisition and ambient transactions

Repositories obtain the executor from the context. `database.RunInTx` reuses
an ambient transaction; nested calls are not independent commits or savepoints.
External compensations register on the transaction context via
[`economy.OnRollback`](../../internal/economy/runner.go) within a
[`WithRollbackScope`](../../internal/economy/runner.go): succeeding nested
scopes promote their hooks to the enclosing scope so pre-commit failures and
pre-commit context cancellations run compensations in LIFO order while the root
SQL transaction still holds its row locks, before SQL rollback releases them.
Root `tx.Commit()` errors leave commit state indeterminate after lock release
and propagate without running rollback compensation. Root transactions use `READ COMMITTED` (`sql.LevelReadCommitted`) to avoid
secondary index range gap locks and stale snapshots under MariaDB InnoDB.
Rank order applies to acquiring new locks, not merely the textual order of
updates to rows already locked. The complete hierarchy is owned by
[the database rules](../../.agents/rules/05-database-and-caching.md).

If character deletion's shared market rows change before seller locking, the
transaction runner rolls back and restarts the root callback, up to three total
attempts. Nested callbacks never retry independently. Callbacks that can delete
characters therefore keep their effects inside the SQL transaction; an exhausted
restart returns an error with no deletion committed.

Same-table multi-row acquisition uses [`economy.LockInOrder`](../../internal/economy/locking.go);
callers retain transaction ownership and domain-specific missing-row decisions.
Ordering covers the complete transaction's shared-row set, including every
character's mailbox rows in account deletion; independently sorted subsets do
not establish a transaction-wide order.

Daily guild GP decay and GvG settlement acquire guild row locks in ascending ID
order. Decay includes zero-balance guilds in this order, applying its formula and
changing timestamps only for current positive balances under lock. This includes
GP awarded while decay waits; a bulk update through the points index can acquire
locks in a conflicting order.

Depot and Gem Box serialize changes through their parent rows (`character_depots`
and `character_gem_boxes`) and read items with `FOR UPDATE` to retain the latest
committed state; Inventory reads `inventory_items` with `FOR UPDATE` under the
character lock. Locking item reads and item deletions on `depot_items`,
`inventory_items`, and `gem_box_items` (including character deletion) explicitly
force their `character_id` secondary indexes: sparse tables can otherwise select
a primary scan and lock another character's rows. `READ COMMITTED` alone does
not bound this scan to the owning character.

The runner acquires character before inventory. A callback must not first lock
a later-rank table and then cause lazy inventory acquisition through `AddGrant`;
request `LockInventory` upfront when its callback will grant inventory items.
Features consume injected contracts rather than importing SQL adapters.

Depot stores items; bank savings live on `characters.deposit`. Auction performs
direct `Send`/`Inspect`, with no server bidding/buyout. Guild uses membership
and activity GP, with no donation leveling. The removed paid Inn pilot is
historical (#411/#459); free recovery is owned by Home.

## Domain events and commit limitations

`SubscribeSync` handlers execute in the supplied transaction context; the
runner propagates their errors. `SubscribeAsync` handlers run in goroutines
without durable delivery or retry guarantees. Their errors are currently
discarded by the dispatcher, so they must not carry indispensable settlement.

For a top-level transaction, the runner calls `PublishAsync` after its
transaction function succeeds. **An ambient outer transaction is a known
limitation**: success of the inner `RunInTx` means callback completion, not outer
commit. The current runner has no outer-commit hook, so documentation must not
promise post-commit delivery in that case. See [the audit](../migration/documentation-audit.md).
The dispatcher itself cannot detect a SQL commit or provide an atomic outbox.

Auxiliary progression hooks and informational news may be best-effort when
their owning service explicitly accepts missing records during failures.
Their failure must not turn an already applied action into a retryable failure;
this does not relax required reward or milestone persistence.

Boss victory crowds are ephemeral and attempted only after a confirmed SQL
commit. The production adapter rejects an ambient transaction because its
caller can still roll back. A failed or interrupted attempt may omit the crowd;
receipt replay does not repeat the append, and `BanquetHeld` reports confirmed
publication in the current response rather than historical delivery.

## Verification

Use the existing runner, repository, and P2P stress tests for balance/capacity,
rollback, lock order, and conservation. AST checks find selected structural
violations; source review and live integration tests are still needed for
delegated and conditional paths. Do not infer complete legacy parity or event
recovery guarantees from those checks.
