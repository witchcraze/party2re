# Endurance Challenge Design

## Overview

Endurance challenges (`vs_challenge.cgi`, `challenge.cgi`, stages 0–8) are consecutive waves fought until defeat or individual departure. Stage catalogs, enemy scaling, records and transport are owned by code, [OpenAPI](../api/openapi.json) and the [run-state architecture](../architecture/transient-run-state.md#challenge-participant-exit-target). Evidence: `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

## Rules

- **Entry** needs positive HP, fatigue below 100% and **no pending level-up** below level 99 (`level < 99 && exp >= level × level × 10`). Stages 0–7 require level ≤ 99 and stage 8 level ≥ 100 (`lib/quest.cgi:477–482,538–541,713–714,969–972`).
- HP, MP and consumed items carry across waves with no recovery between them (`vs_challenge.cgi:48–53`; `_battle.cgi:453–554`). Consumed revival items cannot be reused on the next wave.
- Each wave spawns `int(rand(2.5 + party × 0.6)) + 1` enemies (`vs_challenge.cgi:68`); combat stats scale by round while EXP and gold do not.
- Living members earn each round's rewards; a fallen member keeps prior earnings but earns nothing new until revived (`_battle.cgi:208–214`). Leader death does not stop the others. A party wipe keeps **100%** of earnings; there is no defeat penalty.
- **Treasure:** once per run, at or after the stage's threshold round (stage 0 draws 10–20, `challenge/0.cgi`), a 10% roll replaces the monster spawn with a treasure (`vs_challenge.cgi:64–69`).
- A new high-water mark records the party in the hall of fame; members who fell are shown with a gravestone.

## Individual Exit

`にげる` dispatches to `lib/_battle.cgi:nigeru:1184–1222,1330–1346`.

- A participant leaves with only their own earnings, resources and consumption; the others keep theirs and continue. Spectators receive nothing. Only the last departure ends the run.
- When the leader leaves, the first remaining participant in roster order leads. The initial `すすむ` is leader-only; afterwards the original only checks that enemies are gone (`lib/vs_challenge.cgi:20–53`), so a handoff must not trap the party behind the old leader.
- Exit fatigue is `3 × round + 1`, halved (truncated) in calm mode; exit clears meal fullness. Equipped item105 adds `20 × round` EXP; item106 adds `floor(round × U)` with `U` uniform in `[0,77)`. **Pitfall:** multiplying the round by an integer draw in `[0,77)` is not equivalent.
- **Pitfall:** the legacy round starts at 0 while Go's current round names the next round, so an exit after victories uses current round − 1; defeat accounts for the attempted round separately. Challenge has no calm-mode selection, so calm mode is never assumed.
- The random bonus is fixed once by the exit receipt; a retry never pays again. Each recorded treasure award keeps one designated recipient chosen when earned; leadership changes do not retarget it, and full per-player treasure inspection is not reconciled.
