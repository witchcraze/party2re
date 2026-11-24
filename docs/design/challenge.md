# Endurance Challenge Design

## Overview

Endurance challenges (`vs_challenge.cgi`, `challenge.cgi`, stages 0–8) are consecutive waves in a room on the shared run host. Stage catalogs, enemy scaling, entry gates and transport are owned by code, [OpenAPI](../api/openapi.json) and the [run-state architecture](../architecture/transient-run-state.md#challenge-participant-exit-target). Evidence: `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

**Decision ([#1866](https://github.com/witchcraze/party2re/issues/1866), 2026-11-23):** waves use explicit player commands on the shared run host instead of resolving each round in one request.

## Rules

- **The treasure threshold is redrawn every wave, and only the advancing member's flag counts.** `$tresure_round` is drawn when the stage file loads, which `set_monster` does on each `すすむ`, so its range is not a fixed floor per run; the roll then reads only the advancing member's taken-treasure flag, so a member without it can still find boxes after another has opened one (`challenge/0.cgi:8`; `vs_challenge.cgi:60–64`).
- **`$win` is the stage's world record.** Shared routines read `$win` as the arena's match count, but Challenge stores the record there at creation and raises it in `update_record` (`quest.cgi:732–740`; `vs_challenge.cgi:47,86`). So once a stage has any record, a strong kill counts as a PvP win, nothing is captured (`_battle.cgi:232–241`) and enemy summons do nothing (`_skill.cgi:9835`).
- **Stage 5's `$stage > 20` branch is unreachable.** `$stage` is the Challenge number, so the `じばく` table never loads (`challenge/5.cgi:24–65`).

### Known gaps

- While a stage has no record, an enemy summon loads the Adventure stage file with the same number (`_skill.cgi:9861`); the run host rejects that summon instead ([#2724](https://github.com/witchcraze/party2re/issues/2724)).

## Individual Exit

`にげる` is the shared `nigeru` (`_battle.cgi:1184–1222`). Legacy has no wipe handler: a fallen member can only leave or be revived (`_battle.cgi:640–656`), so an all-fallen room stays until each member leaves or idle cleanup runs, and every departure pays the exit fatigue and accessory bonus for the current round.

**Pitfall:** item106's bonus is `floor(round × U)` with `U` uniform in `[0,77)`; multiplying the round by an integer draw is not equivalent.
