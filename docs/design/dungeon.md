# Dungeon Exploration and Maps

Dungeon behavior is defined by legacy `lib/vs_dungeon.cgi` and the selected
`map/<dungeon>/<map>.cgi`, including its event subroutines. Map symbols alone
do not describe every event. Current Go is in `internal/dungeon`.

## Legacy movement and map events

The leader performs the initial advance. Afterwards cardinal movement is
available only when no living enemy-side members remain, including ordinary
unopened Dungeon chests. Movement advances the shared round and dispatches the
selected map's `event_<symbol>`. Both action registration and the movement guard
enforce this restriction (`lib/vs_dungeon.cgi:37–48,74–80`). Combat actions
remain available, and `lib/_battle.cgi:1184–1220` permits escape with current
vitality and earned stock.
A wall hit invokes the selected map's wall event without consuming a round.
The base wall event leaves coordinates unchanged; map8 overrides it with a warp.

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
The initial leader advance changes round 0 to 1 without moving or dispatching
the start tile. Thus map0's limit of 20 permits 19 subsequent successful moves,
not 20 moves after initialization (`vs_dungeon.cgi:57–103`).

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

Ordinary Dungeon treasure blocks movement. `add_treasure` creates positive-HP,
NPC-colored chest members (`lib/_npc_action.cgi:498–552`); `read_member` restores
the saved event flags and places living opposing members in `@enemys`
(`lib/_battle.cgi:453–493`). The initial flags are `S` (`lib/quest.cgi:639`);
map0 `event_2` appends `2` before generating treasure (`map/0/1.cgi:16`),
without setting `$event` to `宝`.
Thus the `$event ne '宝'` guard's exception does not apply to this normal path.
Nor does that exception register a movement action: with chests present,
`add_battle_action` registers inspection instead of directions/map observation
(`vs_dungeon.cgi:37–48`). Each request rebuilds actions before execution
(`party.cgi:19–24`, `lib/_skill.cgi:22–23,286`), and the dispatcher rejects an
unregistered action (`lib/_battle.cgi:640`). A direct movement call also rejects
the normal chest state (`vs_dungeon.cgi:77–80`). This is not merely a hidden UI
choice. Opening a chest sets its HP to zero; dead NPC members are omitted from
the next saved/reloaded roster (`_npc_action.cgi:58`, `_battle.cgi:518–535`).
Movement can resume only after no living enemy-side members remain and the
round limit permits it. Escape is a separate action, not permission to move
past unopened chests. This contract preserves the legacy behavior; allowing
ordinary chest abandonment by movement would require an approved departure.

## Current implementation and remaining reconciliation

Catalog dungeons dispatch generic `T/X/D/B/E` events in `expedition.go`, uses
Valkey working state/rewards (including `consumed_events` so each `T` tile event
grants treasure at most once per expedition and subsequent visits resolve as
normal movement), and finalizes durable records in MariaDB. Unresolved battles
retain enemy HP/MP/status, allied ailments, CMP and elemental field duration
across requests and block every movement direction. Explicitly cured ailments
and depleted CMP remain cleared/zero on the next batch.
The leader can resume the shared resolver against that enemy or escape; a new
encounter is not generated on resume. This remains batch combat, rather than
legacy per-character action dispatch: per-batch tension, stat-buff and action
scheduling internals are not a full legacy lifetime snapshot.
That generic event model does not reproduce legacy map-owned events; native
maps (below) replace it map by map.
Its turn-limit wipeout, trap bases and treasure generation
need comparison with the behavior above; they are implementation differences,
not canonical rules inferred from the Go catalog.

Current map observation uses the acting member's saved job and nonempty run
inventory. An empty or omitted saved `item_ids` cannot distinguish no items
from a run created without inventory discovery, so observation requires a fresh
read of that actor's inventory. A missing reader or failed read returns no map;
observation never fills or rewrites the saved roster. Older solo buffers also
require the actor's character read. Canonical
`job-09/job-26/job-27/job-79` and `item-197` identities and compatible aliases
are recognized. A party member's bonus does not expand another actor's view.
Required-source or invalid-geometry failures return no partial map.
Context includes actor-visible cells and actual legal directions, never the full
floor grid or private event/reward buffers. Member discovery uses the canonical
run reference; no lobby lifetime grants access. Saved enemies suppress movement,
and terminal buffers preserve their frozen outcome for leader-only settlement
retry through Fight or Escape. Observation performs neither operation.
See the [context contract](../architecture/client-agent-api.md#approved-navigation-and-progressive-observation-contract).

Active state is no longer mastered by a SQL `dungeon_active_expeditions` table.
See [run-state architecture](../architecture/transient-run-state.md),
[current API](../api/paths/dungeon.json), and
[documented differences](../migration/documentation-audit.md).

## Native map contract

This is the design decision approved in
[#1202](https://github.com/witchcraze/party2re/issues/1202); map0 implements it
(see [Map0 implementation](#map0-implementation)). It extends the existing Dungeon owner and ports, without a
script engine, legacy runtime or a second run store. The existing `dungeon-01`
through `dungeon-03` catalog remains available; adding native maps does not
authorize deletion, reinterpretation of saved runs, or automatic catalog cutover.
Native map geometry and presentation must be independently authored, preserving
verified dimensions and event behavior without copying legacy grids or assets.

### Native schema and transition boundary

The following are semantic requirements, not fixed Go types or SQL DDL. Add only
the facts needed by the map being implemented; later maps extend this contract
in their own units.

| Owner | Required facts and invariants |
|---|---|
| Map definition (`internal/dungeon`) | Stable native catalog/map identity, dimensions, start, round limit, cells and their map-owned event identities. Keep movement eligibility, visible appearance and event behavior separate: a displayed wall may be traversable, and `D` does not globally mean stairs. |
| Live expedition (existing Valkey buffer) | Selected map identity, position, current round, existing run/revision/member/encounter facts, and typed event state. Distinguish one-time triggers, keys/doors, reversible switches, selected routes and counters; do not encode them as a legacy flag string. Shared cross-map events explicitly reuse a state identity rather than resetting on every map change. |
| Accepted event transition | Save position, round, event state and resulting encounter/reward facts together through the existing revision-checked step contract. A rejected limit move changes none of them and consumes no encounter RNG. Reload and stale competing requests must not regenerate an accepted one-time event. |
| Pending encounter | Distinguish hostile combat from unopened treasure for resolution, while both block ordinary Dungeon movement whenever living enemy-side members remain. Treasure generation is distinct from actor-authorized inspection and delivery; it does not activate the isolated `$event == '宝'` exception. Discovery must withhold movement choices and direct movement calls must reject the same state. Persist any accepted random choice needed to resume without redrawing it. |
| Movement availability | Derive exhausted movement from current round and the selected map's limit, independently of terminal status. An exhausted expedition remains active, retaining vitality and stock; Escape and required combat/inspection continuations remain available. A genuine defeat remains a distinct outcome. |
| Terminal settlement | Reuse frozen outcomes and the SQL run-ID settlement claim. A map boss trigger does not by itself authorize the current generic boss-victory clear bonus, medals, or forced expedition completion. Exit parity beyond the movement-limit guard remains separately scoped below. |

For map0, initialize the round to 1 as part of accepted start, representing the
leader's initial advance through the existing Start use case. Do not add a new
public advance command solely to reproduce two legacy requests. Map changes,
where introduced later, must specify their own position/round/event carry rules;
neither generic floor increment nor a universal budget reset is implied.

### Observation and API ownership

Map observations expose only the actor-authorized projection: visible cells,
current position/round/limit, movement availability and verified continuations.
They must not expose hidden traversability, undiscovered event identifiers,
future routes or the full private event-state map. GET/refresh performs no event
dispatch, RNG draw, reward delivery or exploration mutation. For map0, `I` is
displayed as a wall when in view, including after traversal; being the current
position displays the party marker. There is no source-backed permanent reveal
flag (`map/0/1.cgi:14–16`, `vs_dungeon.cgi:114–129`). Map5's lever-controlled
visibility is a different map rule.

[The modular Dungeon OpenAPI](../api/paths/dungeon.json) owns current transport
schemas. Runtime follow-ups update it and regenerate artifacts when their
public contract changes. Native maps reuse the existing routes.
Current actor scouting and Gateway observations are described above. Native
map reads must extend the selected map's projection rather than infer event
types from catalog letters. Existing leader-only commands stay
in place; participant commands and individual exit need a separately approved
contract, not an incidental authorization change in the map implementation.
The [Dungeon Guidance index](../../.arch/modules/dungeon.json) continues to
describe implemented ports/locks only. Update its verified symbol references
when implementation changes those boundaries; do not index proposed symbols.

### Legacy action reconciliation

These are all Dungeon additions to the shared battle action dispatch, plus entry
and exit. General battle skills remain with the shared battle owner. References
are relative to `party2/` at legacy revision
`ff97fc549932a146cc945d7edf71b44c868a840e`; the inspected files are unchanged at
checkout revision `3bebb8872f521410396aa090818d573173224ea1`.

| Legacy action / call path | Native use case / transport and scope |
|---|---|
| `ダンジョン` → `quest.dungeon` / `get_dungeon_data` (`quest.cgi:198,623–703`); join selects `vs_dungeon` (`1051–1053`) | Existing `StartExpedition` / `StartPartyExpedition`, POST `.../dungeons/start` / `party-start`; map0 adds independently authored native definitions and selected-map persistence. Preserve consented roster admission. |
| `すすむ` → `susumu` (`vs_dungeon.cgi:33–36,57–67`) | Map0 Start initializes round 1; separate action is superseded by atomic start, retaining leader admission. |
| `にし` → `nishi` (`39–40,73`) | `Move(west)`, existing POST `.../dungeons/move`; map-owned dispatch follows admission. |
| `きた` → `kita` (`39,41,70`) | `Move(north)`, same route. |
| `みなみ` → `minami` (`39,42,71`) | `Move(south)`, same route. |
| `ひがし` → `higashi` (`39,43,72`) | `Move(east)`, same route. |
| `ちず` → `chizu` (`39,44,114–129`) | `ObserveExpedition`, GET `/api/v1/characters/{id}/context` and command refresh; actor-only visibility. The standalone map GET is retired. |
| Treasure `しらべる` → `shiraberu(target)` (`45–48`; `_npc_action.cgi:6–84`) | Deferred native treasure-inspection contract, including per-actor eligibility and item delivery. Generic `T` automatic rewards do not implement it. No connected native inspect endpoint is claimed. |
| `add_battle_action2` → map16 `さいくつ` → `saikutu` (`map/16/1.cgi:40–87`, `__1.cgi:36–64`) | Deferred map16 explicit mining use case; no current command replacement. |
| Shared `にげる` → `nigeru` (`_battle.cgi:1184–1222,1459–1473`) | Existing `Escape`, POST `.../dungeons/escape`; map0 verifies availability after exhaustion and stock preservation. Per-character departure, fatigue and exit equipment effects remain unverified/deferred; whole-party Escape is not equivalent. |

## Map events and restoration units

The inventory below records event families and bounded follow-up scopes, not
full map or combat parity. It covers the map-local handlers (in addition to the
base passage/wall/start/boss events); each follow-up must inspect every reachable
variant, selected enemy/treasure data and enclosing dispatch before coding.
Numeric and letter identifiers here are evidence pointers, not native enums.
Legacy grids, dialogue and monster/asset names are not implementation inputs.

### Map0 implementation

`native-map-0` adds six independently authored 5×5 layouts beside the
unchanged catalog (`internal/dungeon/native_map.go`); the legacy has six
initial variants, two without a hidden passage (`map/0/1.cgi`–`6.cgi`). Start
picks and saves one layout, places the party on its start and sets round 1
without a tile event. A successful move advances the round and dispatches the
destination; at round 20 every move rejects with no state, RNG or settlement
change, while the run stays active for Escape and keeps its stock. Walls and
bounds reject without spending a round. Resumed combat batches spend no round.
The run stores the remaining budget (`turns_remaining = limit − round`);
observation reports `round`/`round_limit`.

- Passage: draw `r = 2 × U[0,1)` and spawn iff `r <= 1`; start, hidden
  passage and consumed one-time cells draw nothing.
- Treasure: once per expedition, creates one unopened chest per living member,
  plus `1 + rand(2)` when the acting leader is job 78. Chests block all
  movement (choices withheld, direct calls rejected) until every chest is
  opened; they are not combat, grant no reward and are never auto-opened.
- Boss: once per expedition, uses the saved combat continuation; victory grants
  the monster's rewards only, with no clear bonus, medals or completion.
- Hidden passage: traversable, always shown as a wall in view; observation
  lists all four directions while movement is allowed, so it reveals nothing.

Remaining map0 differences: chest contents and inspection (#1762), encounter
group size/stat scaling and the legacy stage data choice (one native enemy per
encounter), the wish-4 extra chest and the per-character exit gaps under
[Remaining limits](#remaining-limits). Start admission is the
shared combat admission (HP, fatigue and activity checks); legacy dungeon 0 has no
job-level gate, so the native map has no level or tier requirement. Native maps
keep the actor-only scouting above.

### Map1 key and door unit

[Map1 implementation #1761](https://github.com/witchcraze/party2re/issues/1761) is separate and builds on the map0 transition contract.
Independently author 9×9 native layouts; legacy has seven initial variants and
a round limit of 50 (`map/1/1.cgi` through `7.cgi`). Its `2` and `3` treasure
events have independent once flags; `I` retains the hidden-wall presentation.
Test `K` acquisition once, opening `D` with the key or acting job 9, persistence
of opened-door state, and repeat entry without new rewards/consumption. The
legacy key is an event fact and is not consumed on opening. Without either
qualification, the event undoes the horizontal entry while spending the move's
round (`map/1/1.cgi:18–38`); distinguish this from a pre-movement wall rejection.
Verify every variant's entry direction before defining the native return
position. This event does not advance a floor.

### Map13 channel puzzle and routes

Decided in [#1774](https://github.com/witchcraze/party2re/issues/1774); this is
a target specification, not current behavior. Evidence is static, relative to
`party2/` at the legacy revision above (`map/13/` files abbreviated). Implementation
units: puzzle/route [#1959](https://github.com/witchcraze/party2re/issues/1959),
normal destination [#1960](https://github.com/witchcraze/party2re/issues/1960),
EX destination and start enablement [#1961](https://github.com/witchcraze/party2re/issues/1961).

**Main floor** (`1.cgi`). Only one initial variant exists: entry skips `_` files (`quest.cgi:673–703`).
The floor is 9×9 with round limit 80. The start's only exit is the altar `A`, so the first
move always reaches it. Native layouts are independently authored under the
[layout constraints](#map13-layout-constraints) below. Passages use the base spawn with the floor pool. The map overrides base `event_B`
(`1.cgi:59`), so this floor has no boss or treasure.

| Event | Rule |
|---|---|
| Channels `B/R/Y/W` (`1.cgi:59–62`) | Entering toggles that channel; re-entry toggles it back. No RNG and no once flag. |
| Reroll `C` (`1.cgi:63`) | Always replaces the target with a uniform `int(rand(15))` draw over the 15 nonempty channel combinations (`1.cgi:21–38`). Before activation the draw is superseded by the altar's first draw, because the last digit run is the target (`1.cgi:48–49`). |
| Altar `A` (`1.cgi:47–58`) | The first visit draws the target and activates the puzzle. In the same step, a match selects the route and transfers; a mismatch discloses the target color. The branches that remove `A` or return on it are unreachable, because a match leaves the floor and no path returns. |

Disclosure persists for every member, matching the shared party log: the run
keeps the last disclosed target and whether a reroll happened since. Observation
shows channel states and the altar marker for cells in view (`1.cgi:41–46`). It never
shows an undisclosed target or route odds.

**Route** (`_is_EX`, `1.cgi:68–83`). Let k be the number of active channels at the match
(the target's size, 1–4) and n the allied roster (`@partys`, including fallen and allied
NPC members, `_battle.cgi:482–490`). The route is EX if n ≥ 5 or k = 4. Otherwise, for
n ≥ 2 it is EX iff `k >= rand(5)`, so P = k/5; a solo party takes the normal route. Players
are capped at 4 (`quest.cgi:21`), so n ≥ 5 needs allied NPCs, which native rosters lack.

**Carry.** The selected map is a saved header field (`_battle.cgi:464,521`) loaded
at the start of the next request (`_skill.cgi:23`, `vs_dungeon.cgi:23–28`). The altar step
spends its round and keeps the coordinate, which is a passage on both destinations.
The round continues, the limit stays 80, and event state is one run-scoped record.
Channels and target freeze after the transfer. Native state is typed: channels, target,
activation, disclosure, selected destination and per-event once facts. Accepted draws
are stored with their revision-checked step and never redrawn on reload or retry.

#### Map13 layout constraints

These are the movement costs measured on the legacy floors (`1.cgi:8–19`, `_1.cgi:8–19`,
`_EX.cgi:8–19`). The cells themselves are not reproduced. A native layout may use any
cells, walls and passage count, but it must keep these 9×9 bounds, the adjacency and the
shortest move counts, because they set the 80-round budget and the event order.
Distances count successful moves. "Avoiding" means the path enters no event cell other
than its two endpoints.

- Main floor: the start has exactly one open neighbour, the altar. Every other main-floor
  cell is reachable only through the altar, so each event's distance from the start is
  1 + its distance from the altar. Every pair of main-floor events is connected by an
  avoiding path with the shortest lengths below. Unrestricted shortest paths equal these,
  except B–Y, which is 4 through the altar. Re-toggling a channel costs two moves
  (step off, step back).

  | Avoiding | A | B | C | R | W | Y |
  |---|---|---|---|---|---|---|
  | A | 0 | 2 | 2 | 6 | 6 | 2 |
  | B | 2 | 0 | 4 | 4 | 8 | 6 |
  | C | 2 | 4 | 0 | 4 | 4 | 4 |
  | R | 6 | 4 | 4 | 0 | 4 | 8 |
  | W | 6 | 8 | 4 | 4 | 0 | 4 |
  | Y | 2 | 6 | 4 | 8 | 4 | 0 |

- Destinations (both routes): the arrival is a passage at the altar's coordinate,
  with exactly one open neighbour. The floor has exactly 11 reachable cells with these
  adjacencies and no others:
  - a corridor: arrival – first cell (alternate boss, or a passage on EX) – recovery – boss – center treasure;
  - center – west, center – east, center – north;
  - north – top;
  - west corner adjacent to both west and north; east corner adjacent to both east and north.

  Shortest distances from the arrival: first cell 1, recovery 2, boss 3, center 4,
  west/east/north 5, corners/top 6. No treasure is reachable without passing the
  recovery, the boss and the center treasure cell.

**Destinations** (`_1.cgi`, `_EX.cgi`). From arrival, a corridor runs north: alternate
boss `Z` (normal) or a passage (EX), then recovery `Q`, then boss `V`. Above it lie a treasure
row of three, another row of three and a top cell. Every event is once per run.
Define L, the legacy `$len`, as the count of non-digit flags at request start (`_1.cgi:21–25`):
3 + k + destination events already consumed. So L = 3 + k at `Z`, and at `Q` it is 4 + k (normal) or 3 + k (EX).

| Event | Normal route (k = 1–3) | EX route |
|---|---|---|
| Alternate boss | First single boss iff `rand(10) > L − 1`, so P = (8 − k)/10; else the second (`_1.cgi:89–94,116–168`). | None. Passages use the one-entry EX pool. |
| Recovery | Party HP and an independent MP base of `(rand(20) + 25) × L`, element "none", through the shared party-heal routines with the actor's modifiers; fallen members excluded (`_1.cgi:96–102`, `_skill.cgi:9056–9188`). | Same, with its L (`_EX.cgi:55–61`). |
| Boss | Three-member group with party scaling, then the special boss field: allied max-stat caps and status reset, lasting for the run (`_1.cgi:39–45`, `_battle.cgi:1476–1532`). | First the phantom default field (`_battle.cgi:1786–1791`), then the EX group and the special field (`_EX.cgi:39–46`). |
| Treasure | Gated by the frozen channels (`_1.cgi:47–87`): center W, west R, east B, north Y, west corner k > 1, east corner k > 2, top k > 3 (unreachable). Unavailable cells are plain passages with no flag or display. | Six ungated ordinary cells. The top cell is a fixed treasure: chests per allied player, fallen included, with no job-78/wish bonus (`_EX.cgi:47–53`, `_npc_action.cgi:555–598`). |

Contents and inspection: [#1762](https://github.com/witchcraze/party2re/issues/1762).
Groups/scaling: [#1954](https://github.com/witchcraze/party2re/issues/1954).
Rewards: [#1957](https://github.com/witchcraze/party2re/issues/1957).
Per-character combat: [#1865](https://github.com/witchcraze/party2re/issues/1865).
Start admission for dungeon 13 (menu job level 80 versus server 25, `quest.cgi:209,530–536`):
[#1955](https://github.com/witchcraze/party2re/issues/1955). Map13 adds no
map-specific action (no `add_battle_action2` override). Start, Move, observation and
Escape reuse the native use cases above. The map stays unlisted and start-rejected
until both destinations are implemented, so players never meet a partial map.

### Map2–17 follow-up scopes

Each row is a separate restoration objective; subdivide complex rows at the
listed boundary before implementation. Limits are map-owned round limits, not
promises of that many moves after initialization. Submaps do not all share the
same limit. Initial catalogs and selected data need their own verified native
content; this table does not authorize their automatic rollout.

| Map / evidence | Events and round limits | Follow-up verification boundary |
|---|---|---|
| [2 / #1763](https://github.com/witchcraze/party2re/issues/1763): `map/2/1.cgi:18–23`, `_1.cgi:18–21`, `__1.cgi:21–23`, `___1.cgi:20–23` | `F` routes, `7/8` floor bosses, `T` pitfall/damage, `2/3/4` treasure, `Q` no-op; limits 30/40/50 | First establish cross-map round/position/flag carry, then independently author the branching floors; test route RNG, pitfall return and bases 30/70/40, and no boss-as-clear assumption. |
| [3 / #1764](https://github.com/witchcraze/party2re/issues/1764): `map/3/1.cgi:18–30`, variants `2..8.cgi` | `A/C/D/E/F/G` warps, `T` confusion/encounter, `b` boss, `2/3` treasure; some variants `I`; limit 60 | Verify each warp destination and variant's hidden passages; status must reach the ensuing encounter in order. |
| [4 / #1765](https://github.com/witchcraze/party2re/issues/1765): `map/4/1.cgi:20–34`, variants `2..7.cgi` | Passage damages actor by rounded 2% MaxHP, `M` message, `H` once healing choice, `b` boss, `2/3/4` treasure; limit 70 | Verify actor-only attrition and death, no inherited passage encounter, and the actual healing target/helper behavior before implementation. |
| [5 / #1766](https://github.com/witchcraze/party2re/issues/1766): `map/5/1.cgi:18–54`, variants `2..4.cgi` | `L` lever/boss, `I` display depends on lever, `T` poison/encounter, `t` damage base 80, `H` job-43 party recovery, treasure; limit 60 | Separate lever/visibility from hazard/recovery if useful; test lever replay, status ordering, actor qualification and all members' vitality. |
| [6 / #1767](https://github.com/witchcraze/party2re/issues/1767): `map/6/1.cgi:20–43`, variants `2..5.cgi` | Exclusive `A/B`, `C/D`, `E/F` choices, message markers `5/6/7`, `X` boss, treasure; limit 30 | Test mutually exclusive branch flags, encounter pools and ordered currency/heal/trap alternatives; audit negative-money behavior rather than silently normalizing it. |
| [7 / #1768](https://github.com/witchcraze/party2re/issues/1768): `map/7/1.cgi:20–45`, variants `2..6.cgi` | `K` switch or jobs 4/25 open rock `D`, `T` paralysis/encounter, `t` damage base 150, `A/b` bosses, treasure; limit 60 | Door-style entry reversal, actor qualification, shared switch/rock state and hazard ordering. |
| [8 / #1769](https://github.com/witchcraze/party2re/issues/1769): `map/8/1.cgi:20–32`, variants `2..12.cgi` | `W` and wall event `1` random warp, walls appear as passage, `A/C/D` bosses, treasure; limit 50 | Wall collision can move position without spending a round; test visible-versus-traversable distinction and warp bounds/RNG. |
| [9 / #1770](https://github.com/witchcraze/party2re/issues/1770): `map/9/1.cgi:21–38`, variants `2..4.cgi` | Passage, treasure and `A/B/C` bosses impose party magic seal; limit 60 | Verify seal application even on consumed events, alternate boss data and save/reload of affected battle state. |
| [10 / #1771](https://github.com/witchcraze/party2re/issues/1771): `map/10/1.cgi:20–28`, `_1.cgi:20–28`, `__1.cgi:20–32` | Party attack seal on passage/boss/treasure/routes, `F` next map, `A/B/C` bosses; limit 30 | First verify route carry rules, then seal/encounter order and independently once-only events across three maps. |
| [11 / #1772](https://github.com/witchcraze/party2re/issues/1772): `map/11/1.cgi:31–61` | `2/3/4` treasure pool selection, `A..F` distinct bosses, `H` job-43 recovery; limit 30 | Verify pool bounds, distinct boss bindings, once flags and eligible party recovery. |
| [12 / #1773](https://github.com/witchcraze/party2re/issues/1773): `map/12/1.cgi:31–61` | Same event families as 11 with different treasure/boss sources; limit 30 | Separate content unit: verify its own pool bounds and boss bindings rather than certifying it with map11 tests. |
| [13 / #1774](https://github.com/witchcraze/party2re/issues/1774): `map/13/1.cgi:40–85`, `_1.cgi:39–103`, `_EX.cgi:39–62` | `B/R/Y/W` toggles, `C` reroll, `A` combination/route selection; submap boss/treasure/recovery events; limit 80 | Decided; see [Map13 channel puzzle and routes](#map13-channel-puzzle-and-routes). |
| [14 / #1775](https://github.com/witchcraze/party2re/issues/1775): `map/14/1.cgi:20–75`, `_P.cgi:22–33`, `_K.cgi:22–36`, `_T.cgi` | Random active points `A..E` and stable subroute assignment, `R` return, submap bosses/treasures; limit 200 | Split route allocation from submap content; revisit must preserve assignment and explicit cross-map once identities. Audit allocation-helper claims against actual state writes. |
| [15 / #1776](https://github.com/witchcraze/party2re/issues/1776): `map/15/1.cgi:20–63`, `_1.cgi`, `_2.cgi`, `__1.cgi` | Without-replacement event choices, route markers, encounter/status/treasure events; limits 30/50/60/90 | Split event-choice state from submap restoration; verify exhausted-choice behavior, route carry and every submap's overrides. |
| [16 / #1777](https://github.com/witchcraze/party2re/issues/1777): `map/16/1.cgi:22–87`, `_1.cgi:34–61`, `__1.cgi:25–64`, `__2.cgi:36–48` | Mining actions with depletion, route switches, periodic recovery, weekday boss progression, special field/fixed treasure; limit 150 | Separate mining/inspection command contract, progression/clock boundaries and final submaps; test actor claims, interval reset, depletion and conditional boss/route facts. |
| [17 / #1778](https://github.com/witchcraze/party2re/issues/1778): `map/17/1.cgi:62–176`, `_1.cgi:41–105`, `__1.cgi:32–60` | State-dependent encounter rate/walls, switches `3..8`, doors/key, traps/ambushes, multimap routes and conditional treasure; limit 200 | Split switch/wall state, underground key/ambush routes and secret-room rewards. Resolve differing treasure display/eligibility thresholds explicitly; test ordered route/state changes. |

## Remaining limits

This decision does not certify the whole Dungeon cluster. Native treasure
inspection/delivery is scoped in [#1762](https://github.com/witchcraze/party2re/issues/1762), covering actor claims,
extra-open item 215, treasure counts/pools and failure recovery. Legacy treasure
creation is not reward delivery (`vs_dungeon.cgi:131–135`,
`_npc_action.cgi:6–84,498–552`); its actor eligibility must not be inferred from
the current automatic `T` reward path.
Per-character combat action dispatch is scoped in
[#1865](https://github.com/witchcraze/party2re/issues/1865). Independent participant
departure and fatigue/equipment exit effects remain unscheduled gaps; whole-party
Escape and a shared movement counter do not supply them. Exact content and cross-map semantics beyond the
map0/map1 evidence above require their row's scoped investigation. Idle expiry,
Valkey-loss recovery and multi-instance guarantees retain their existing limits
in the [run-state architecture](../architecture/transient-run-state.md).
