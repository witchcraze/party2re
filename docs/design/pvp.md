# Colosseum PvP (Real-time 8-Player Bet & Split Color Battles)

The Colosseum PvP module (`internal/pvp`) reconstructs `party2/lib/vs_player.cgi` and `party2/lib/quest.cgi:type=4` (asynchronous snapshot Elo ratings have been removed). Room parameters, the 9 team colors, battle resolution, and transport schemas live in code, [OpenAPI](../api/openapi.json), and [PvP financial publication](../architecture/transient-run-state.md#pvp-financial-publication).

## Rules and Open Gaps

- **Lobby preparation actions (`vs_player.cgi:33–38`, `_skill.cgi:161–164,769–837`):**
  - `@きっく` (`pvp_kick`): kicked members' bets remain in `prize_pool` (`_skill.cgi:796–837`; revisiting bet refund is tracked in [#2586](https://github.com/witchcraze/party2re/issues/2586)).
  - `@さそう` (`_skill.cgi:769–791`, quest-lobby whisper) awaits the quest-lobby place log ([#2585](https://github.com/witchcraze/party2re/issues/2585)).
- **Inactivity (`quest.cgi:86–91,172–184`, `_battle.cgi:453–461`):**
  - **Non-obvious legacy behavior:** `quest.cgi:928–1079` (`add_member`) writes join messages to `$logdir/quest.cgi` rather than the room's `log.cgi`, so joining does not refresh the inactivity clock. Because GET stays read-only, inactivity cleanup runs lazily on mutating PvP accesses; periodic background cleanup is tracked in [#2343](https://github.com/witchcraze/party2re/issues/2343).
- **Rounds on the run host (`vs_player.cgi:73–216`):**
  - Rounds 1 and later run as a room library on the [run host](../architecture/transient-run-state.md#run-host-durable-command-boundary-1325). The leader's first `かいし` hands the lobby off and moves the whole bet pool into the run. After that, bets are never refunded, and the pool is lost if the last member leaves or the round limit is reached.
  - Combat frames are color-relative, as in `read_member`: every other color is the enemy, and NPCs never respond (`vs_player.cgi:219`).
  - **Non-obvious legacy behavior:** the `$round > 10` check (`vs_player.cgi:101`) runs before the tally and the round counter increments only at the end (`:181`). Round 11 is therefore played, but its result is never counted.
  - **Non-obvious legacy behavior:** the waits `かいし` gives other members and the starter's doubled `$act_time` (`vs_player.cgi:175–179`) only drive the client countdown. The action gate stays `$ms{$m}{time}` plus `$speed` (`_battle.cgi:641`).
  - **Non-obvious legacy behavior:** `give_bet` divides the pool by every member of the winning color in `@members` (`vs_player.cgi:190–208`), allied summons included. Only players receive a share because `send_money` skips names without a user file (`system.cgi:847`). The remainder is lost.
  - Gap: HTTP still resolves each round in one request until the Gateway cutover ([#2736](https://github.com/witchcraze/party2re/issues/2736)), which also deletes the round engine behind [#2532](https://github.com/witchcraze/party2re/issues/2532).
