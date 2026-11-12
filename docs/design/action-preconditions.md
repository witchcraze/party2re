# Action Preconditions Design

## Overview

Gateway entry eligibility uses six gates evaluated in order: dead → fatigue → sleep → cooldown → currency → location. The catalog (`internal/playercontext`) owns entry IDs, inputs and gate flags; the [Gateway contract](../architecture/client-agent-api.md#approved-navigation-and-progressive-observation-contract) owns observation, navigation and continuation. Gates are entry eligibility only: execution services recheck exact prerequisites, and continuations use membership, role and phase instead of entry gates.

**Decisions:** prayer is a free daily blessing (#986), blacksmith seals cost crystals (#987) and casino games cost coins (#988), so none of these requires wallet gold; home sleep is free, not a paid inn. Navigation (#1047, 2026-10-06) needs no living HP, gold or fatigue capacity and mutates nothing.

## Legacy gate evidence

1. **Dead Gate (`$m{hp} <= 0`)**:
   - In legacy Party2 (`lib/quest.cgi:479, 902`), characters with HP <= 0 are prohibited from creating or joining combat quests and expeditions (`adventure_start`, `challenge_start`, `dungeon_start`, `boss_fight`, `pvp_room_create`).
   - Noncombat town and facility actions (banking `lib/bank.cgi:77–103`, commerce `lib/weapon.cgi:53–85`, crafting, casino, progression, social chat `lib/system.cgi:639–676`, and recovery) do NOT require living HP in legacy dispatch.
   - Deceased characters in town can freely access noncombat facilities, deposit/withdraw, trade, or rest at `home_sleep` to restore full HP and MP (`lib/sleep.cgi:32`).
2. **Fatigue Gate (`$m{tired} >= 100`)**:
   - Exhausted characters cannot initiate stamina-draining tasks (battles, dungeon runs, boss raids, or casino gambling).
   - **Allowed**: Rest (`home_sleep`), food (`tavern_order`), banking, storage management, and shopping.
3. **Sleep Gate (`$m{sleep} > 0`)**:
   - Under `party.cgi:14`, any character with remaining sleep seconds is routed unconditionally to `sleep.cgi`.
   - Ordinary actions remain blocked while the sleep duration is active or wake recovery is pending. `home_wake` is offered only when the duration has elapsed and recovery is pending.
   - **Emergency exception**: `rescue.cgi:53-65` accepts rescue during existing sleep and adds a penalty. `rescue_request` therefore has no awake requirement.
4. **Cooldown Gate (`party2:scheduled:actor:<id>`)**:
   - Characters with Pending or Processing ScheduledActions cannot execute cooldown-gated town or expedition actions. This gate is not the legacy 4-second action wait (`$act_time`); see the action wait gate below. Passing ExecuteAt does not imply settlement; Completed/Failed records do not block. The actor Set is an index, not an exclusive lock ([Scheduled work lifecycle](../architecture/valkey-keyspace.md#scheduled-work-lifecycle)).
   - **Action wait gate** (`party2:timer:action_wait:<id>`): a successful command starts the legacy wait (`$act_time`, default 4 s). While it runs, every action is rejected, including navigation and continuations, as legacy rejects every registered `＠` action (`lib/system.cgi:637-655`). Rescue and wake are exempt because legacy serves them outside `party.cgi`, and plain speech because it is not a `＠` action. The catalog's `WaitExempt` records why an action does not start the wait:

     | Disposition | Actions | Legacy evidence |
     | --- | --- | --- |
     | Starts the wait (default) | Every facility, town and entry command | Success paths set `$npc_com` or log the input; the `act_time = 0` sites in `lib/*.cgi` are the no-target list views, which Go replaces with scene observation. |
     | `navigation` | `scene_enter`, `scene_select`, `scene_page`, `scene_back` | `いどう`/`まち` set `$mes` or `act_time = 0` (`lib/system.cgi:289,316`); list views set `act_time = 0`. |
     | `recovery` (never blocked) | `home_wake`, `rescue_request` | `sleep.cgi` and `rescue.cgi` run outside the `party.cgi` action path. |
     | `speech` (never blocked) | `place_say` | Plain input skips `action` and is written by `write_comment` (`party.cgi:20-26`). |
     | `whisper` | `place_whisper` | `sasayaki` sets `act_time = 0` (`lib/system.cgi:261`); it is still blocked while a wait runs. |
     | `activity_pacing` | Party, PvP, GvG, dungeon, challenge and casino-room continuations | Battle and room turns keep their own pacing (`lib/_battle.cgi:1175`, `lib/casino.cgi:555`), owned by the combat and room migrations (#948, #949). They are still blocked while a wait runs. |

5. **Currency gate.** Entry requires positive wallet gold; exact prices and alternate currencies stay service checks.
6. **Location gate.** Actual activity blocks new entries and ordinary navigation; continuations require owning membership/role/phase; conflicts keep only verified leave/recovery choices. In legacy Party2 only combat quests (`lib/quest.cgi:479,902`) require living HP; town actions (`lib/system.cgi:639–655`) run regardless of HP. Every facility action implicitly checks location except `rescue_request`.

The original persists facility/home selection (`party2/party.cgi:14–30`,
`lib/system.cgi:267–318,418–435`) and replaces town choices inside games
(`_casino.cgi:10–32`, `vs_dungeon.cgi:33–48`, `vs_challenge.cgi:24–30`). Public
versus own-home actions differ (`home.cgi:26–39,73–105`). These are dispatch
requirements; current Go gates and a TTL alone do not establish complete parity.
