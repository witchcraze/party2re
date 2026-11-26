# Party System Design

## Overview

Parties (`quest.cgi`, `party.cgi`) let up to four characters form a waiting lobby and run a stage together. Lobby fields, states, endpoints and reward formulas are owned by code and [OpenAPI](../api/openapi.json); the lobby lives in Valkey ([transient run state](../architecture/transient-run-state.md)). Sealing rooms are not party lobbies; they run on the run host ([boss §6](boss.md#6-unseal-to-sealing-room-lifecycle)).

Starting a party currently resolves every floor and inspects treasure automatically (the batch crawl). Player-selected actions, per-actor intervals, explicit advances and inspection, and individual exits belong to [Adventure's interaction contract](adventure.md#original-interaction-contract-and-reconstruction-target). A lobby leave is not the original active-combat exit, and a lobby roster is not resumable battle state.

## Rules

- Normal waiting lobbies capture each member's battle row (stats, gear, and held-tool vitality maxima) when that member creates or joins the lobby (`quest.cgi:583, 1010, 1163–1211`), while join conditions (`need_join`) and start eligibility evaluate the character's live state.
- The room speed (3/18/25 seconds) is the original per-actor action interval (`_battle.cgi:468,641–644`); the batch crawl stores but does not enforce it.
- Join conditions (`need_join`, `{key}_{val}_{u|o}` for max HP or job level) apply to the creator at creation and to each joining member; the stage's job-level gate (`$job_lv[$stage]`) also applies.
- A character belongs to at most one waiting or running party. Waiting lobbies idle out after 30 minutes (`quest.cgi`).
- Every member must be ready and alive before the leader starts.
- In the batch treasure room, an item goes to the opening character's inventory, else the Depot; with the Depot full it is lost (`vs_monster.cgi:91–137`, `_npc_action.cgi:60–80`).

## Known limits

Lobby status and readiness are reset inside the settlement transaction callback but live in Valkey, so an SQL rollback does not undo them; the compensating reset is best effort.
