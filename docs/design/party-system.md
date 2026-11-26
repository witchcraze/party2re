# Party System Design

## Overview

Parties (`quest.cgi`, `party.cgi`) let up to four characters form a waiting lobby and run a stage together. Lobby fields, states, endpoints and reward formulas are owned by code and [OpenAPI](../api/openapi.json); the lobby lives in Valkey ([transient run state](../architecture/transient-run-state.md)). Sealing rooms are not party lobbies; they run on the run host ([boss §6](boss.md#6-unseal-to-sealing-room-lifecycle)).

The lobby stands for the legacy quest room at round 0: there is no readiness or disband step, the leader's start enrolls the roster in a floor-0 normal run, and play then follows [Adventure's interaction contract](adventure.md#original-interaction-contract-and-reconstruction-target). A lobby roster is not resumable battle state. Leaving the lobby is legacy `にげる` at round 0, whose fatigue is not yet applied (#2827).

## Rules

- Normal waiting lobbies capture each member's battle row (stats, gear, and held-tool vitality maxima) when that member creates or joins the lobby (`quest.cgi:583, 1010, 1163–1211`), while join conditions (`need_join`) and start eligibility evaluate the character's live state.

## Known limits

Lobby status is reset inside the settlement transaction callback but lives in Valkey, so an SQL rollback does not undo it; the compensating reset is best effort.
