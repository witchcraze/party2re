# Dungeon Exploration and Maps

Dungeon behavior is defined by legacy `lib/vs_dungeon.cgi` and the selected
`map/<dungeon>/<map>.cgi`, including its event subroutines. Map symbols alone
do not describe every event. Current Go is in `internal/dungeon`.

## Legacy movement and map events

The leader performs the initial advance. Afterwards any living member may move.
Cardinal movement is available only when no living enemy-side members remain, including ordinary
unopened Dungeon chests. Movement advances the shared round and dispatches the
selected map's `event_<symbol>`. Both action registration and the movement guard
enforce this restriction (`lib/vs_dungeon.cgi:37–48,74–80`). Combat actions
and Escape remain available.
A wall hit invokes the selected map's wall event without consuming a round.

The initial leader advance changes round 0 to 1 without moving or dispatching
the start tile. Thus map0's limit of 20 permits 19 subsequent successful moves,
not 20 moves after initialization (`vs_dungeon.cgi:57–103`).

## Scouting and blocking chests

Legacy `@ちず` defaults to radius 1. The acting character's job 9, 26, 27, or
79 supplies radius 2; held item 197 adds one. The original checks the actor,
not the presence of any qualifying party member. Visible tiles use the map's
display definitions and out-of-bounds walls.

**Pitfall:** the `$event ne '宝'` exception in `_susumu` never frees unopened
chests ([treasure contract](#treasure-inspection-and-delivery)). Map0 `event_2` appends
`2` to the flags and never sets `$event` to `宝` (`map/0/1.cgi:16`). With chests
present, `add_battle_action` registers only `しらべる`, so the dispatcher also
rejects movement (`vs_dungeon.cgi:37–48,77–80`; `_battle.cgi:640`). Escape stays
a separate action; letting movement abandon chests would be a departure.

## Start admission

**Decision (#1774, #1955).** Original players saw the selection menu, so its
thresholds apply rather than the looser server formula (`quest.cgi:526–537`).
Only the leader is checked; joiners are not gated by job level
(`add_member`, `quest.cgi:928–1079`). The listing shows only maps the viewer may
start as leader, and a start below the threshold is rejected.

| Dungeon | Leader job level | Menu evidence (`quest.cgi`) |
|---|---|---|
| 0 | any | always listed |
| 1–12 (i) | ≥ 2i + 1 | `233–236` |
| 13 / 14 / 15 / 17 | ≥ 80 / 30 / 50 / 40 | `@ex_dungeon`, `209,237–241` |
| 16 | any, while the once quota is free | `get_limited_dungeons`, `1224–1228` |

Open: dungeon 16's once quota (#1955).

## Retired synthetic catalog

**Decision (#1958).** The synthetic `dungeon-01`–`dungeon-03` catalog had no legacy counterpart and was removed, together with its per-character record table. `dungeon_expedition_history` keeps old catalog rows because it is the settlement claim; history reads never resolve `dungeon_id` against the catalog.

## Native map contract

**Decision (#1202).** It extends the existing Dungeon owner and ports, without a
script engine, legacy runtime or a second run store.

### Native schema and transition boundary

Semantic requirements, not fixed Go types; each map adds only the facts it
needs. Step atomicity, revision checks, pending encounters, Escape and
settlement are owned by the [run buffer contract](../architecture/transient-run-state.md#candidate-d-in-progress-run-buffers).

- **Map definition.** Keep movement eligibility, visible appearance and event
  behavior separate: a displayed wall may be traversable, and `D` does not
  globally mean stairs.
- **Event state.** Typed per-map run state distinguishes one-time triggers,
  keys/doors, reversible switches, selected routes and counters, never a legacy
  flag string. Shared cross-floor events reuse one state identity. Accepted
  random choices are saved with the step, so reload and stale requests never
  redraw them.
- **Exhaustion.** Derived from the round and the selected map's limit. A
  rejected limit move changes nothing and draws no RNG; the run stays active
  with Escape and required combat/inspection continuations. Legacy never ends a
  quest because every ally fell (`_trap_d` sets HP to 0 without ending it).

For map0, initialize the round to 1 as part of accepted start, representing the
leader's initial advance through the existing Start use case. Do not add a new
public advance command solely to reproduce two legacy requests.

**Floor changes.** The selected map, round, position and flags are one saved
header (`_battle.cgi:464,521`); the next request loads the new map file with its
own `$max_round` (`vs_dungeon.cgi:23–28`). A map change resets none of them, so
the party keeps its coordinate, the round continues under the destination's
limit, and flags stay run-wide across floors. As with a warp, the landing cell
dispatches nothing until a later move enters it.

### Observation and API ownership

Map observations expose only the actor-authorized projection: visible cells,
current position/round/limit, movement availability and verified continuations.
They must not expose hidden traversability, undiscovered event identifiers,
future routes or the full private event-state map. GET/refresh performs no event
dispatch, RNG draw, reward delivery or exploration mutation. For map0, `I` is
displayed as a wall when in view, including after traversal; being the current
position displays the party marker. There is no source-backed permanent reveal
flag (`map/0/1.cgi:14–16`, `vs_dungeon.cgi:114–129`); map5's `I` is
[revealed by its lever](#map5-lever-temple).

Native maps reuse the existing routes ([Dungeon OpenAPI](../api/paths/dungeon.json)). Participant combat (#1865) and individual exit (#1956) need their own approved contracts, not an incidental change in a map implementation.

### Legacy action reconciliation

These are all Dungeon additions to the shared battle action dispatch, plus entry
and exit. General battle skills remain with the shared battle owner. References
are relative to `party2/` at legacy revision
`ff97fc549932a146cc945d7edf71b44c868a840e`; the inspected files are unchanged at
checkout revision `3bebb8872f521410396aa090818d573173224ea1`.

| Legacy action / call path | Native replacement and scope |
|---|---|
| `ダンジョン` → `quest.dungeon` / `get_dungeon_data` (`quest.cgi:198,623–703`); join selects `vs_dungeon` (`1051–1053`) | Existing solo/party start; preserve consented roster admission |
| `すすむ` → `susumu` (`vs_dungeon.cgi:33–36,57–67`) | Superseded by atomic start, which initializes round 1 and keeps leader admission |
| `にし`/`きた`/`みなみ`/`ひがし` (`39–43,70–73`) | Any living member moves in that direction; map-owned dispatch follows admission |
| `ちず` → `chizu` (`39,44,114–129`) | Character context observation, actor-only visibility |
| Treasure `しらべる` → `shiraberu(target)` (`45–48`) | Participant inspection under the [treasure contract](#treasure-inspection-and-delivery). Map0 [#2119](https://github.com/witchcraze/party2re/issues/2119), map3 [#2374](https://github.com/witchcraze/party2re/issues/2374). |
| `add_battle_action2` → map16 `さいくつ` (`map/16/1.cgi:40–87`, `__1.cgi:36–64`) | Open: map16 mining (#1777) |
| Shared `にげる` → `nigeru` (`_battle.cgi:1184–1222,1459–1473`) | Leader Escape settles every member as `nigeru`, at the saved round. The menu never offers calm mode (commented out, `quest.cgi:41,46,319`), so its fatigue halving does not apply. Open: per-character departure (#1956); whole-party Escape is not equivalent |

### Treasure inspection and delivery

**Decision (#1762, 2026-11-13).** Players get what the original gave them: each
member opens a chest themselves and receives its item at once. The project owner
delegated the mechanism; it is the [claim boundary](../architecture/transient-run-state.md#dungeon-treasure-claims).
Not yet implemented ([#2119](https://github.com/witchcraze/party2re/issues/2119)).

- **Creation.** The treasure step creates one chest per living roster member. An
  acting job 78 adds `1 + rand(2)`; an acting Chapel blessing 4 adds one with
  probability 1/5. Contents are drawn at creation by the shared Adventure chest
  generator from the map's stage pool and stay hidden. Reload and retries never
  redraw them. **Pitfall:** map0 picks stage 0 or 1 with equal odds on every
  request (`map/0/1.cgi:19–20`), so the treasure step's own request picks the pool.
- **Inspection.** Any living participant, not only the leader, opens one chosen
  unopened chest of the current treasure. A dead actor, a missing or opened chest,
  or no treasure rejects without change. Chests never respond.
- **Allowance.** **Finding:** legacy keeps only the map symbol of the actor's last
  opening, reset on joining (`_npc_action.cgi#shiraberu`, `quest.cgi#get_battle_line`).
  Opening at an event with that same symbol needs item 215 in the tool hand,
  consumed before delivery. This covers a second chest at the same event, and also
  a later floor's event with the same symbol unless the actor opened at another
  symbol in between. Native treasure events therefore carry an allowance key that
  is equal exactly where the legacy symbols are equal; map0's single event gives
  one chest per actor per run. **Finding:** the Dungeon check precedes the
  fixed-chest check, so fixed chests (map13 EX, map16) share this allowance.
- **Delivery.** Immediate, under the [Depot treasure rule](depot.md#delivery-capacity-by-caller).
  A tool entering the empty hand also registers the actor's equipped items in the
  Collection (`add_collection`). An empty chest (pool entry `0`) still spends the
  allowance and any item 215. Delivered items survive the later Escape;
  exit never awards unopened chests.
- **Gap.** Chests are also attackable enemies, and some skills reveal contents.
  A destroyed chest is empty. Native has neither path until [#1865](https://github.com/witchcraze/party2re/issues/1865)
  (defeat rewards: [#1957](https://github.com/witchcraze/party2re/issues/1957)),
  so chests nobody may open, such as a solo job 78's extras, leave only Escape.

## Map events and restoration units

Each section records one map's decisions and non-obvious findings. A restored
map without a section has none beyond its code and tests; unrestored maps are
tracked by their own Issues. Numeric and letter identifiers here are evidence
pointers, not native enums.

Every restored map shares these gaps: group size and scaling
([#1954](https://github.com/witchcraze/party2re/issues/1954)), applying enemy
accuracy, job skills and temporary effects in explicit combat
([#1865](https://github.com/witchcraze/party2re/issues/1865)), defeat rewards and
crystals ([#1957](https://github.com/witchcraze/party2re/issues/1957)), chest
contents and inspection ([treasure contract](#treasure-inspection-and-delivery);
per-map sources: [#2428](https://github.com/witchcraze/party2re/issues/2428),
map15 [#2451](https://github.com/witchcraze/party2re/issues/2451),
map17 [#2455](https://github.com/witchcraze/party2re/issues/2455))
and narration, owned by the Dungeon log ([#948](https://github.com/witchcraze/party2re/issues/948)).

### Map0 implementation

The three placeholder regular enemies remain until the stage 0/1 pools are
restored with groups ([#1954](https://github.com/witchcraze/party2re/issues/1954)).

### Map3 implementation

Variants 5–8 define `event_I` at line 21, but their grids have no hidden
passage cell.

Some warp destinations are themselves event cells. Variant 5's `C` and `E`
target their own cells (`map/3/5.cgi:23–25`). Variant 1's `F` lands on the
treasure at `(6,6)` without opening it (`map/3/1.cgi:12,25`). The dispatcher
evaluates the entered cell once before drawing the map (`vs_dungeon.cgi:96–101`).

Chest contents come from stages 5–8 ([#2374](https://github.com/witchcraze/party2re/issues/2374)).

### Map1 implementation

Non-obvious findings:

- **Locked-door reversal is not always horizontal.** Legacy `event_D` undoes the
  entry with `++$px` in variants 1, 2 and 5 but `--$py` in 3, 4, 6 and 7. Every
  locked door has exactly one open neighbour, so native returns the party to the
  cell it came from. The round is spent, unlike a wall hit.
- **Flags are per symbol.** Several variants repeat `K` or `3` under one flag,
  so the second cell does nothing. Native keys `K`, `D`, `2` and `3` by symbol.
- **Only the actor qualifies.** `D` opens for the acting character's job 9
  (`$m{job}`) or the picked-up key; another member's job does not count. The key
  is never consumed.

### Map2 implementation

Non-obvious findings:

- **Pitfalls can exceed the limit.** A floor-2 or floor-3 pitfall drops to a floor
  with a lower limit. A run past round 30 or 40 lands with its round above the new
  limit, so only Escape remains; the round is shown as is (e.g. 35/30).
- **Pitfalls can land in a wall.** The floor-2 pitfalls drop to the same coordinate
  on a random first floor, which is a wall in `1.cgi` (from `_2`/`_4`) and `3.cgi`
  (from `_1`/`_3`). Movement checks only the target, so the party walks out.
  A `___3` pitfall lands on `__1`'s own pitfall without triggering it.
- **`_4` is reachable only by falling.** The first floor's stairs draw `_1`–`_3`;
  only the third floor's pitfall draws `_1`–`_4`.
- **Dead event.** The second floor defines `event_2` but has no `2` cell.

### Map4 attrition maze

Non-obvious findings:

- **Only the mover pays.** `event_0` replaces the base spawn with an HP charge on
  the actor (`1.cgi:24`), so a party spreads the cost by moving in turn.
- **`H` recovers one random member, not the party.** The call passes no target:
  `&{'event_'…}` without parentheses forwards `_susumu`'s arguments, so the
  "target" is the direction name (`vs_dungeon.cgi:75,101`). `_check_party` treats
  it as unspecified: a uniform roster member, fallen included, with a fallen pick
  healing the actor instead (`_skill.cgi:9635–9638`). The pre-move reset has
  already cleared statuses and tension, and one flag covers every `H` cell.

### Map5 lever temple

Non-obvious findings in `map/5/`:

- **The lever changes only the view.** `I` is never impassable; the `L` flag
  only switches its display from wall to passage (`1.cgi:18–20`). `★` stays
  after the pull, and `B` and treasure have no marker, unlike map4.
- **`H` revives.** It sets every `@partys` member, fallen included, to full HP
  and MP (`1.cgi:26–37`). Only the acting character's job 43 qualifies; another
  entry sets no flag, so a later job-43 entry still succeeds.
- **The `T` poison lasts one battle.** It precedes the encounter, and the next
  step's `reset_status_all` clears it (`vs_dungeon.cgi:93`), as with map3's mist.

Narration also owns the repeated-lever lines, the only `L` draws.

### Map6 choice corridor

Map6 stays unlisted and start-rejected until its gold outcomes land ([#2417](https://github.com/witchcraze/party2re/issues/2417)).

**Finding.** Gold pickup and loss change the mover's own wallet at once, outside
run stock, and `write_user` caps it at 999999 (`system.cgi:71`). The intended
floor `$m{money}-=0 if $m{money} < 0` is a no-op (`1.cgi:40`, `5.cgi:44`).

**Decision (#1767).** Reproduce it: each accepted event changes the mover's
wallet immediately and exactly once, possibly below zero, as for
[item 237](home.md#4-remote-depot--inventory-item-usage-つかう). Deferring it to
settlement would drop it whenever a run idles out.

### Map13 channel puzzle and routes

Decision [#1774](https://github.com/witchcraze/party2re/issues/1774). File references
are relative to `party2/map/13/` at the legacy revision above.

- **Disclosure persists for every member,** matching the shared party log. The run
  keeps the last disclosed target and whether a reroll happened since. Observation
  never shows an undisclosed target or route odds.
- **A reroll before activation is wasted.** The altar's first draw supersedes it,
  because the last digit run is the target (`1.cgi:48–49`).
- **The altar's removal and return branches are unreachable,** because a match
  leaves the floor and no path returns (`1.cgi:47–58`).
- **The n ≥ 5 EX rule needs allied NPCs,** which native rosters lack. Players are
  capped at 4 (`quest.cgi:21`), but the roster counts allied NPC members
  (`_battle.cgi:482–490`).
- **L counts non-digit flags at request start** (`$len`, `_1.cgi:21–25`): 3 + k plus
  the destination events already consumed. It is 3 + k at the alternate boss, and
  4 + k (normal) or 3 + k (EX) at recovery.
- **The normal route's top treasure is unreachable.** Its gate k > 3 needs k = 4,
  which always routes to EX (`_1.cgi:47–87`).
- **The EX boss installs the phantom default field before its group** and the
  special boss field after it. Both last for the run (`_EX.cgi:39–46`).

### Map14 sunken routes

Decision [#1775](https://github.com/witchcraze/party2re/issues/1775). File references
are relative to `party2/map/14/` at the legacy revision above.

- Two returns put the party inside a main-floor wall. The party can walk out
  from `_P` at (5,9) or `_T` at (6,9) (`1.cgi:16`, `_P.cgi:18`, `_T.cgi:18`).
- The legacy flags share a single string and use regex checks. Checks on
  different floors avoid collisions with each other and the initial `S`
  (`1.cgi:55–75`, `_P.cgi:22–30`, `_K.cgi:22–36`, `_T.cgi:23–62`).
- Site `k` is always eligible because the allocation list omits it even though
  it has a treasure event (`_T.cgi:33,46,54`).

### Map15 fortress branches

Decision [#1776](https://github.com/witchcraze/party2re/issues/1776): reproduce the
original without departures. File references are relative to `party2/map/15/` at
the legacy revision above.

- **The final bosses install `fortress` as the run's default field** before the
  group spawns. It applies to every later battle without a temporary field
  (`__1.cgi:28`, `_2.cgi:30`, `_battle.cgi:1568–1610,1786–1791`). Native records
  it in the run state; the batch resolver has no named fields, so battles apply it
  only with explicit combat ([#1865](https://github.com/witchcraze/party2re/issues/1865)).

### Map17 tomb switches and secret room

Decision [#1778](https://github.com/witchcraze/party2re/issues/1778): reproduce the
original without departures. File references are relative to `party2/map/17/` at
the legacy revision above.

- **Passing `b` splits the run.** It is the only way into the inner area, and
  `B` then turns the party back. No route leads back to the band or its stairs,
  so `j`/`k` count for the secret room only if visited first. `b` also switches
  every later pool, boss group and chest from `__data.cgi` to `___data.cgi`
  (`1.cgi:117–133,172–177`). `_data.cgi` is never loaded; the summon skill
  returns early for stage 17 (`lib/_skill.cgi:9835–9842`).
- **Much of the map is unreachable.** The outer ring's stairs lead only to
  underground pockets without a band stair, so `h`, `i`, `H`, `I`, the bottom
  `v` corridor and the outer `9` cells are never entered. `R` never cuts the
  band: each band `9` has a parallel lane. Switches `3`–`5`, `A`/`a`/`C` and
  `P` have no cell.
