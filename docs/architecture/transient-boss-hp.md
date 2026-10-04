# Withdrawn shared world-boss HP proposal (Candidate E)

**Status: withdrawn, not an implementation plan.** The Candidate E PoC from
#370/#394 was removed by #479/#584 (commit `674322a`). Its former repositories,
`boss_damage` Lua script, global HP pool, contributor tally, MVP rewards, and
last-hit rewards are not part of the reconstructed Party2 boss specification.
Do not restore them on the strength of this historical proposal.

The reference behavior is the party sealing battle in legacy `lib/vs_king.cgi`
and `data/stage/king*.cgi`: ordinary king encounters admit up to six members,
and king99 admits four. Investigation, sealing, banishment, rewards, and victory
banquets belong to that encounter flow. See [Boss design](../design/boss.md)
and the current `internal/boss` implementation. The Go implementation still
requires comparison with the legacy rules; removal of the PoC alone does not
prove full parity.

## Historical identifiers

These names explain old references and the existing documentation checks.
They describe deleted code, not live Valkey keys:

- `party2:boss:{boss:<boss_id>}:hp`
- `party2:boss:{boss:<boss_id>}:status`
- `party2:boss:{boss:<boss_id>}:contributors`
- `party2:boss:{boss:<boss_id>}:killer`
- `party2:boss:{boss:<boss_id>}:run_id`

The proposal discussed Cluster Hash Tag placement, Two-Phase Settlement,
Crash Recovery, and In-Memory Fallback Parity for a shared raid. Those contracts
and its benchmarks were specific to the removed PoC. They provide no guarantees
about current boss encounters or current runtime recovery.
