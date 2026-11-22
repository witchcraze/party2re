# Colosseum PvP (Real-time 8-Player Bet & Split Color Battles)

The Colosseum PvP module (`internal/pvp`) reconstructs `party2/lib/vs_player.cgi` and `party2/lib/quest.cgi:type=4` (asynchronous snapshot Elo ratings have been removed). Room parameters, the 9 team colors, battle resolution, and transport schemas live in code, [OpenAPI](../api/openapi.json), and [PvP financial publication](../architecture/transient-run-state.md#pvp-financial-publication).

## Rules, Departures, and Open Gaps

- **Live-room departure (`@にげる`, `_battle.cgi:1184–1248,1330–1375`):**
  - Pre-start (`recruiting`, round 0) refunds only the departing participant's bet and promotes leadership if members remain; once started (`in_progress` or `completed`), bets are never refunded (`prize_pool` closes as forfeited escrow when the last participant exits).
- **Lobby preparation actions (`vs_player.cgi:33–38`, `_skill.cgi:161–164,769–837`):**
  - `@きっく` (`pvp_kick`, leader-only before match start) removes the target and imposes a 60-second `ActionWait` without bet refund or `@にげる` exit side-effects (fatigue, vitality copyback, item bonuses, or bar delivery); the kicked member's bet remains in `prize_pool` as in `_skill.cgi:796–837` (revisiting bet refund is tracked in [#2586](https://github.com/witchcraze/party2re/issues/2586)).
  - `@しらべる` (`vs_player.cgi:34,36` → `system.cgi:322`) maps to `player_inspect` during pre-start `recruiting` (`round == 0`).
  - `@さそう` (`_skill.cgi:769–791`, quest-lobby whisper) awaits the quest-lobby place log ([#2585](https://github.com/witchcraze/party2re/issues/2585)).
- **Recovery and inactivity (`quest.cgi:86–91,172–184`, `_battle.cgi:453–461`):**
  - Missing live state clears membership without refunding bets or inferring an inactivity penalty.
  - Genuine inactivity (`now > last_activity + 1800s`) forfeits remaining escrow without refund and sets a 3600-second sleep penalty while preserving longer sleep. **Non-obvious legacy behavior:** `quest.cgi:928–1079` (`add_member`) writes join messages to `$logdir/quest.cgi` rather than the room's `log.cgi`, so joining does not refresh the inactivity clock (only creation, team selection, kick, start, round advance, and non-final departures refresh it). Because GET stays read-only, cleanup runs lazily on mutating PvP accesses; periodic background cleanup is tracked in [#2343](https://github.com/witchcraze/party2re/issues/2343).
- **Settlement and combat parity:**
  - Mid-match departures leaving one team award the round win without combat; integer-division remainders and 10-round undecided draws (`vs_player.cgi:101–104,148–151,208`) close remaining `prize_pool` as forfeited escrow without refund.
  - Match victory (`give_bet`, `vs_player.cgi:186–216`) awards prize gold only; `characters.pvp_wins` (`kill_p`) is credited per surviving character who inflicts a non-revived stronger-opponent combat defeat (`_battle.cgi:231–232`, `battle.IsStrongEnemy`) and flushed atomically at settlement or inactivity closure.
  - Open: restoring per-actor player-command combat instead of single-request round resolution is tracked in [#1867](https://github.com/witchcraze/party2re/issues/1867), and non-final round retry identity in [#2532](https://github.com/witchcraze/party2re/issues/2532).
