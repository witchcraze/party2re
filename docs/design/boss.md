# King Sealing Battles Design (封印戦)

Sealing rooms (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) are a room library (`internal/boss`) on the shared run host ([system-opened rooms](../architecture/transient-run-state.md#system-opened-rooms)).

## Rules and decisions

- `king1`–`king10` admit up to 6 players and `king99` 4: the legacy boss NPC takes one extra slot (`p_join+1`, counted from 1).
- Repeated `@treasures` entries are draw weights, not a unique list. A treasure found with the Depot full is lost, even with a matching stack (`_npc_action.cgi:73–75`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Legacy loads the stage file on every request (`vs_king.cgi:19–23`), so job95 summons draw from the stage's `@monsters`, whose `get_crystal => int(rand(2))` is redrawn per request, and job78's treasure retry would regenerate chests from the stage table (#1862).
- Crystals: each defeated enemy's multiplier measures the time since the unseal clock, not since the battle or room start (`_battle.cgi:145–173`). Every unseal rewrites the clock (`vs_monster.cgi:224`), so a battle already in progress sees the restarted clock; with no clock the multiplier is 1×.
- `king99` clones copy every member of the unsealing party (`@partys`, NPC companions included) from their battle rows at that treasure creation, never the room's later joiners. A player's row carries EXP `lv + job_lv` and gold `int(lv × 0.5)` written at join (`quest.cgi:1177–1178`); Go actors carry no such values, so the unseal reads the character's level at the unseal (#2756).
- Dejon writes only `lib` and Tired to the removed player (`vs_king.cgi:80–98`): they keep the HP/MP they joined with, their unpaid stock outlives the room, and the exit deletion check does not run, so a room emptied by Dejon survives.
- The celebration crowd (`_win_vs_king.cgi`) and the opening of rooms after an unseal run only for the request that confirms its own commit; a lost acknowledgement or a receipt replay runs neither. [Event Plaza §3](eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps) owns the crowd.

## 6. Unseal-to-Sealing-Room Lifecycle

The trigger is every stage19 ordinary floor-11 treasure creation ([Adventure](adventure.md#combat-travel-and-encounter-replacement)); MaoCount and the unseal clock commit with that settlement.

- A stage never has two rooms: `make_vs_king` skips a stage whose quest directory exists (`vs_monster.cgi:168`), and an idle room is deleted only when the room list is read (`quest.cgi:86–90`). An idle but unlisted room therefore blocks its stage's reopening until a listing deletes it.
- The last exit deletes the room only when its leader boss is defeated, before or after sealing (`_battle.cgi:1331`). `＠にげる` writes the room log (`party.cgi:25`), so every exit, the last included, restarts the 30-minute idle clock of a room that survives.
- The room's enemy state belongs to that room. It is not the withdrawn shared world-boss HP ([transient-boss-hp.md](../architecture/transient-boss-hp.md)).

**Current gaps:**

- Joining, room commands, observations and the listing (join for an open room, watch-only for a full or sealed one, `quest.cgi:149–153`) have no transport yet (#2783).
- The king2 and king5 leaders carry NPC jobs 101 and 96, which have no callback table, so those rooms reject commands once the leader responds (#2789).
