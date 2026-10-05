# Scheduled Work Lifecycle

Scheduled work moves from Pending to Processing, then Completed or Failed. It remains unfinished throughout Pending and Processing, including after its intended execution time has passed. Reaching that time makes work eligible for processing; it does not establish that feature settlement has completed.

An actor's unfinished-work query returns that actor's Pending and Processing records. Completed and Failed records are omitted, even if an index entry remains. A missing payload is omitted because the work may have been removed between index and payload reads. Invalid records or unavailable storage produce an error rather than an apparently successful empty list.

The query changes no records or timers and establishes no exclusive actor lock. Feature services remain responsible for validating and settling their own actions. Scheduling contains no feature-specific eligibility rules.

The implementation uses the existing actor Set index and one bulk payload read: two Valkey commands for a nonempty index, with work proportional to its number of entries. It introduces neither a Character snapshot cache nor an action-availability cache.

## Scheduling Registration & Failure Semantics

When scheduling an action (`Schedule`):
1. The action payload is written (`SET party2:scheduled:action:<id>`).
2. If `ActorID` is non-empty, the action ID is registered in the actor set index (`SADD party2:scheduled:actor:<actor_id>`).
3. The action ID is enqueued into the pending sorted set (`ZADD party2:scheduled:pending`).

If actor-index registration (`SADD`) or pending queue enqueueing (`ZADD`) fails:
- Registration immediately halts without proceeding to subsequent steps.
- Already-written partial entries (payload and actor index) are cleaned up on a best-effort basis (`DEL` / `SREM`).
- The underlying storage error is returned to the caller, guaranteeing that un-indexed or un-enqueued actions are never reported as successfully scheduled.
- Subsequent retries safely overwrite and recreate the entries through idempotent `SET`, `SADD`, and `ZADD` operations.

## Due Action Fetching & Error Semantics

When fetching due actions (`FetchDue`):
1. Candidate action IDs due for execution are retrieved from the pending queue (`ZRANGEBYSCORE party2:scheduled:pending`).
2. For each action ID, the payload is retrieved (`GET party2:scheduled:action:<id>`).
3. If the payload is genuinely absent (`valkey.IsValkeyNil`), it is treated as a stale queue entry and cleaned up from the pending queue (`ZREM`). Any cleanup error is propagated to the caller.
4. If reading the payload encounters a transient network/storage failure or context cancellation, the queued entry is preserved in `party2:scheduled:pending` and the read error is returned immediately to the worker, ensuring unfinished work is neither discarded nor permanently stranded.
5. If the payload is malformed JSON or violates domain invariants (`Validate()`), the invalid entry is cleaned up from the pending queue (and deleted from storage if malformed) to prevent repeated processing failures.

## Action Cancellation & Failure Semantics

When cancelling an actor's scheduled actions (`CancelByActorID` / `ClearActiveActions`):
1. The actor's indexed action IDs are retrieved from the actor set index (`SMEMBERS party2:scheduled:actor:<actor_id>`). If the index is absent or empty, cancellation succeeds immediately with 0 actions and `cleared=false` (safe/idle).
2. For each action ID, cancellation removes the action from the pending queue (`ZREM party2:scheduled:pending`), deletes the action payload (`DEL party2:scheduled:action:<id>`), and deletes the lock (`DEL party2:scheduled:lock:<id>`).
3. If all actions are successfully removed, the actor set index is deleted (`DEL party2:scheduled:actor:<actor_id>`).

If removing any action from the pending queue or deleting action/lock payloads fails, or if deleting the final actor set index fails:
- Cancellation immediately halts without executing dependent destructive cleanup commands.
- The actor index (`party2:scheduled:actor:<actor_id>`) is preserved so that unfinished work remains discoverable via `FindPendingByActorID` and is not falsely classified as safe.
- The storage error is propagated to callers along with the count of successfully cancelled actions.
- Callers such as `ClearActiveActions` return `cleared=false, err` to prevent callers like `EmergencyRescue` from recording unearned penalties or falsely assuming a clean slate when work remains.
- Subsequent retries safely resume and complete cancellation via idempotent `ZREM` and `DEL` operations.



