# Casino Design

## Overview

The Casino follows `party2/lib/casino.cgi`, `_casino.cgi`, `casino_indian.cgi`, `casino_highlow.cgi`, `casino_doppel.cgi` and `casino_slot.cgi`: one-way coin exchange, a prize exchange, the solo slot machine and multiplayer Indian poker, High & Low and Doppelganger rooms. Formulas, catalogs and transport live in code and [OpenAPI](../api/openapi.json); live rooms are Valkey state ([transient run state](../architecture/transient-run-state.md)).

## Currency

- Exchange is one way: 1 coin = 20 gold (`casino.cgi:651–665`). Coins never convert back to gold.
- Prizes are delivered to the Depot, past capacity if needed (`casino.cgi:609–623`; [delivery capacity](depot.md#delivery-capacity-by-caller)); coin deduction and delivery commit together.

## Rooms

- Leaving as a participant adds 1 fatigue (`_casino.cgi:216–218`); spectators leave freely and never act or win. Leadership passes to the next active member; the room disbands with no active members. Kick is leader-only before the game starts.
- Casino owns a 30-minute idle lifetime. Open: legacy expiry also applied a sleep penalty (`casino.cgi:178–190`), not yet reconciled (#949).
- After an error, clients observe room and account state instead of replaying a wager; repeated actions on a finished round or an already-submitted turn are rejected. Publication ordering and cross-store limits are owned by [Valkey keyspace](../architecture/valkey-keyspace.md#index-lifetimes-ttl-scored-sorted-set-with-lazy-purging).

### Observation and visibility

The anonymous lobby is a window of at most 100 active rooms with public summary facts only (no members, cards, marks, actions, pot or private timestamps). Selecting a room grants no admission: join and spectate stay explicit mutations that enforce the password and spectator allowance. Detail requires an owned character that is an actual participant or admitted spectator. Reads never start, advance, wager, settle or renew the idle lifetime.

| Game / phase | Participant | Admitted spectator |
| --- | --- | --- |
| Indian, active | Own card hidden unless folded or waiting; other cards and declarations visible | Participant cards/declarations visible |
| Highlow, active | Own card/action visible; other cards hidden except waiting members; competitive declarations hidden, continue/wait visible | Same, without an own card |
| Doppel, active | Own mark visible; other marks hidden | All marks hidden |
| Before start / after settlement (`round <= 0`) | Remaining cards/marks and declarations visible | Same |

Evidence: `lib/casino_indian.cgi:22–33`, `lib/casino_highlow.cgi:33–50`, `lib/casino_doppel.cgi:23–36`, admission `lib/casino.cgi:382–516`, dispatch `lib/_casino.cgi:10–32,100–111`. Settlement returns the room to waiting with round zero; there is no separate finished status. Go Doppel stores the selected mark in the action too, so other participants' mark actions are masked; legacy displays actions separately from hidden cards.

### Scoped legacy dispatch reconciliation

All room, slot, exchange and prize operations still run through REST; their Gateway connection is open (#949). Screenshot (`すくしょ`) and invitation (`さそう`) (`_casino.cgi:124–145`) have no replacement and are deferred to #949/#140. Known differences, reconciled under #949:

- High & Low: legacy hides “continue” at the maximum bet or with insufficient coins (`casino_highlow.cgi:10–50`); the service accepts all-in calls.
- Doppel: legacy clears declarations on start and stores mark changes in the card field, allowing reselection before all cards are selected (`casino_doppel.cgi:10–99`). Eligibility of waiting entrants follows the existing service.

## Game rules

Non-obvious rules (deck, pot and round mechanics live in code):

- Indian and High & Low fold still pay the current bet into the pot (`casino_indian.cgi:102`). Players with zero coins are ejected after settlement.
- High & Low splits the pot between the highest High and lowest Low only with more than two players; with every player folded the pot is void (“お流れ”). Low requires more than two participants.
- Doppel offers `min(participants, 7) + 1` marks. Coins are taken on a player's first selection in the round only. Matching children split the pot and one of them, chosen at random, becomes the leader; with no match the leader takes the pot and keeps leadership.
- Slot: the 200-coin bet is limited to job46 (`casino.cgi:98,109–111`); a miss adds 1 fatigue (`casino.cgi:583,588`); the Wish 5 blessing gives a 25% chance of +50% on winning spins (`casino.cgi:565–568,577–580`).
- Showdown winners increment casino wins (`casino_indian.cgi:165`, `casino_highlow.cgi:216`, `casino_doppel.cgi:120,138`), which feed the gambler ranking (`ranking.cgi:16`) and, with item039, unlock job46 at 10 wins (`_data.cgi:134`).
