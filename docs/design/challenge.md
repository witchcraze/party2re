# Endurance Challenge Design

## Overview

Endurance challenges (`vs_challenge.cgi`, `challenge.cgi`, stages 0–8) are consecutive waves in a room that runs on the shared battle engine. Stage catalogs, enemy scaling, records and transport are owned by code, [OpenAPI](../api/openapi.json) and the [run-state architecture](../architecture/transient-run-state.md#challenge-participant-exit-target). Evidence: `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

**Decision ([#1866](https://github.com/witchcraze/party2re/issues/1866), 2026-11-23):** waves use explicit player commands on the shared run host instead of resolving each round in one request.

## Rules

- **Entry** needs positive HP, fatigue below 100% and **no pending level-up** below level 99 (`level < 99 && exp >= level × level × 10`). Stages 0–7 require level ≤ 99 and stage 8 level ≥ 100 (`lib/quest.cgi:477–482,538–541,713–714,969–972`).
- HP, MP and consumed items carry across waves with no recovery between them (`vs_challenge.cgi:48–53`; `_battle.cgi:453–554`). Consumed revival items cannot be reused on the next wave.
- Rewards come from the shared `defeat` routine per kill: each living member is credited the killer's modified reward, and a fallen member earns nothing until revived (`_battle.cgi:28,207–214`). There is no defeat penalty.
- **Treasure:** at most once per actor's taken-treasure flag, at or after the stage's threshold round, a 10% roll replaces the monster spawn with treasure boxes that members open with `しらべる` (`vs_challenge.cgi:20–25,64–69`; `_npc_action.cgi:6–60,498`).

### Known gaps

- Go credits fixed per-wave totals instead of per-kill `defeat` rewards, and ends the run as defeated when every member falls; legacy has no wipe handler, so the room stays until exit or timeout ([#2650](https://github.com/witchcraze/party2re/issues/2650)).
- Go awards one treasure item to the original leader instead of per-member box inspection ([#2651](https://github.com/witchcraze/party2re/issues/2651)).

## Individual Exit

`にげる` dispatches to `lib/_battle.cgi:nigeru:1184–1222,1330–1346`.

- A participant leaves with only their own earnings, resources and consumption; the others keep theirs and continue. Spectators receive nothing. Only the last departure ends the run.
- When the leader leaves, the first remaining participant in roster order leads. The initial `すすむ` is leader-only; afterwards the original only checks that enemies are gone (`lib/vs_challenge.cgi:20–53`), so a handoff must not trap the party behind the old leader.
- Exit clears meal fullness. **Pitfall:** item106's bonus is `floor(round × U)` with `U` uniform in `[0,77)`; multiplying the round by an integer draw is not equivalent.
- **Pitfall:** the legacy round starts at 0 while Go's current round names the next round, so an exit after victories uses current round − 1; defeat accounts for the attempted round separately. Challenge has no calm-mode selection, so calm mode is never assumed.
- The random bonus is fixed once by the exit receipt; a retry never pays again.
