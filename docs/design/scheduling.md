# Scheduled Work Lifecycle

Scheduled work moves from Pending to Processing, then Completed or Failed. It remains unfinished throughout Pending and Processing, including after its intended execution time has passed. Reaching that time makes work eligible for processing; it does not establish that feature settlement has completed.

An actor's unfinished-work query returns that actor's Pending and Processing records. Completed and Failed records are omitted, even if an index entry remains. A missing payload is omitted because the work may have been removed between index and payload reads. Invalid records or unavailable storage produce an error rather than an apparently successful empty list.

The query changes no records or timers and establishes no exclusive actor lock. Feature services remain responsible for validating and settling their own actions. Scheduling contains no feature-specific eligibility rules.

The implementation uses the existing actor Set index and one bulk payload read: two Valkey commands for a nonempty index, with work proportional to its number of entries. It introduces neither a Character snapshot cache nor an action-availability cache.
