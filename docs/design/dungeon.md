# Dungeon Exploration and Maps

Dungeon behavior is defined by legacy `lib/vs_dungeon.cgi` and the selected
`map/<dungeon>/<map>.cgi`, including its event subroutines. Map symbols alone
do not describe every event. Current Go is in `internal/dungeon`.

## Legacy movement and map events

The leader performs the initial advance. Afterwards cardinal movement is
available once enemies are defeated. Movement advances the shared round and
dispatches the selected map's `event_<symbol>`; active enemies ordinarily block
movement. A wall hit invokes the wall event without advancing coordinates.

| Symbol | Legacy behavior |
|---|---|
| `0` | Passage; the base event may spawn a monster with `rand(2) <= 1` |
| `1` | Wall; additional impassable symbols come from the selected map's `@wall` |
| `S` | Start tile |
| `B` | Base boss event, guarded by its event flag |
| `I` | Example hidden passage displayed as a wall (`map/0/1.cgi`, `map/1/1.cgi`) |
| `2`, `3`, etc. | Map-owned events, such as one-time treasure triggers |
| Other letters/numbers | Map-specific keys, doors, stairs, bosses, hazards, or routes |

For example `map/1/1.cgi` has key `K` and conditional door `D`; `map/2/1.cgi`
uses `F` to choose a next map and `7` for its floor boss. It is incorrect to
universally interpret `D` as stairs or `I` as an impassable wall.

Each map supplies `$max_round` (examples range from 20 to 200); there is no
universal 25–35-turn floor budget. At the limit, further movement is blocked
and the legacy message requests escape/disbandment. Do not equate that guard
with automatic wipeout and zero rewards without checking the exit flow.

## Scouting, party traps, and treasure

Legacy `@ちず` defaults to radius 1. The acting character's job 9, 26, 27, or
79 supplies radius 2; held item 197 adds one. The original checks the actor,
not the presence of any qualifying party member. Visible tiles use the map's
display definitions and out-of-bounds walls.

`_trap_d` damages each party member by `int(base × (0.9 + rand(0.3)))`; each
map supplies the base damage. It does not establish a universal 15%-MaxHP trap.
`_add_treasure` starts from `$#partys`, and acting Job 78 adds `1 + int(rand(2))`.
Inspect map event flags and the shared treasure/exit routines for actual reward
delivery and replay prevention; do not invent a floor-scaled 100G chest rule.

## Current implementation and remaining reconciliation

Go currently dispatches generic `T/X/D/B/E` events in `expedition.go`, uses
Valkey working state/rewards, and finalizes durable records in MariaDB.
That generic event model does not reproduce all legacy map-owned events.
Its turn-limit wipeout, trap bases, treasure generation, and party scouting
need comparison with the behavior above; they are implementation differences,
not canonical rules inferred from the Go catalog.

Active state is no longer mastered by a SQL `dungeon_active_expeditions` table.
See [run-state architecture](../architecture/transient-run-state.md),
[current API](../api/paths/dungeon.json), and
[documented differences](../migration/documentation-audit.md).
