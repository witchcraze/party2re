# Place log, action wait and NPC interaction

> **Status:** approved on 2026-11-08 under
> [#1937](https://github.com/witchcraze/party2re/issues/1937). Sections that are
> not implemented yet name their open issue.

Legacy Party2 is a conversation screen. Every place has a short shared log. A player's
command and the NPC's reply are both written to it, and a short wait follows each
successful command. NPC `はなす`/`しらべる`, command feedback, whisper and the member list
all build on these mechanisms. This contract rebuilds them with modern storage and
transport. It keeps the legacy appearance and the timing rules.

## Legacy behaviour

References are to `party2/` in the local legacy copy. Values come from the tracked
configuration `0713fcc^:party2/config.cgi`.

| Mechanism | Legacy source | Rule |
| --- | --- | --- |
| Request flow | `party.cgi:13-28` | Load the current place and refresh its members. Run the `＠verb>target` action. Write the log unless the action set the private message `$mes`. |
| Log rows | `lib/system.cgi:660-676` | Prepend the actor's raw input line, then the NPC's `$npc_com` line when set. Cap the log at `$max_log` = 30. A row stores time, speaker name, colour, text and an optional recipient `$to_name`. |
| Visibility | `lib/system.cgi:618-633` | A row with a recipient is a whisper. Only the sender and the recipient see it. |
| Action wait | `lib/system.cgi:159,637-655` | While the wait is running, every registered `＠` action is rejected (`まだ行動することはできません`). An action that did not set `$mes` sets the wait to `$act_time` = 4 s. List and prompt views set `$act_time = 0` or use `$mes`. |
| Talk | `lib/system.cgi:240-246` | A random line from the facility's `@words` becomes `$npc_com`, so it is logged and starts the wait. A facility without words answers privately with `返事がない、ただのしかばねのようだ…`. |
| NPC inspect | `lib/system.cgi:322-328,413`, facility `shiraberu_npc` overrides | The text is returned privately through `$mes`. The default text is `しかし何も見つからなかった…`. |
| Speech / whisper | `party.cgi:20-26`, `lib/system.cgi:251-263` | Plain input is logged as speech. `ささやき` checks that the recipient exists, sets `$to_name` and sets `act_time = 0`. |
| Members | `lib/system.cgi:470-520,960-975` | Each request refreshes the place's member list. An entry expires after `$limit_member_time` = 300 s. Moving or logging out removes it. |

Logs exist per place. That covers about 45 fixed facilities and towns, and also the
per-user home and store, per-guild and per-quest/casino-room places. Battle and room
logs stay with their own features.

## Contract

### Place log

- **Storage**
  - A place is identified by kind and ID. A fixed facility uses its scene destination. Owner-scoped places use the owner ID.
  - Each place has one Valkey Master list ([keyspace](valkey-keyspace.md)). An append prepends the new rows and trims the list to the cap in one atomic step.
  - The cap defaults to 30 and the idle TTL to 7 days. Both are configurable. Every append refreshes the TTL.
  - Memory therefore grows with the number of recently active places, not with the number of players.
- **Accepted departure: durability**
  - Logs are display context. They carry no gameplay, asset or progression authority.
  - Losing them on a Valkey failure or restart is accepted, and they have no MariaDB copy.
- **Row content**
  - Each row holds the time, the speaker kind (character or NPC), the speaker's name and colour as they were when the row was written, the text, and an optional private recipient.
  - Private rows count toward the shared cap, as in legacy.
- **Observation**
  - Facility scenes with a legacy log, the town hub and the four towns expose `scene.place_log`, viewer-filtered and newest first. It is `[]` for other scenes. Rows carry no character IDs; `private` marks a whisper the viewer sent or received.
  - A store failure fails the observation.
  - A GET never writes.
  - Owner-scoped places (home, store, guild) are observed once those scenes connect.

### Writers

Command echoes (the actor and NPC rows of facility commands) are open in [#1941](https://github.com/witchcraze/party2re/issues/1941).


- **When rows are written**
  - Only a successful, effectful command writes.
  - Observation, navigation, list and prompt views (legacy `$mes`/`act_time=0`) and rejections never write.
- **The actor row**
  - The actor row is rendered on the server in the legacy look, for example `＠あずける>100G`.
  - The catalog declares the legacy verb once per action.
  - The command adapter supplies the target display text from the resolved result. It uses names and amounts, never internal IDs.
  - The rendered text is stored as written and is never re-rendered.
- **The NPC row**
  - The command's NPC reply is appended together with the actor row, in one append after the command commits.
  - If the append fails, the server records the failure and the command outcome does not change. This is an accepted departure: legacy wrote the log in the same request.
- **Speech and whisper**
  - `place_say` appends the text as a public character row in any place with a log. It never checks or starts the wait.
  - `place_whisper` takes a recipient name and appends `＠ささやき>name text` as a private row. An unknown name is rejected with the legacy `nameというプレイヤーが存在しません`. Like other `＠` actions it is rejected during the wait, but it does not start one (`act_time = 0`).
  - Text uses the legacy comment checks: line breaks removed, 500 characters after HTML escaping (`$max_comment`). The recipient name is not counted, as for letters. Empty speech is rejected, as legacy wrote nothing for empty input; a whisper without text logs the bare `＠ささやき>name`, as in legacy.
  - The row is the command's only effect, so an append failure is an unknown outcome (`500 EXECUTION_FAILED`), unlike the best-effort NPC row.

### Action wait

- **When the wait starts**
  - A successful command whose legacy counterpart keeps a non-zero `act_time` sets a per-character wait in the existing timer family.
  - The wait defaults to 4 s and is configurable.
- **What the wait blocks**
  - While the wait is active, Gateway commands are rejected with `409 ACTION_WAIT` and the context shows `character.action_wait_ms`, so a client can draw the gauge. The per-action dispositions are listed in [action preconditions](../design/action-preconditions.md).
  - A wait-starting command claims the wait atomically (`SET NX EX` with a token) before it runs, so concurrent commands cannot all pass; the loser gets `ACTION_WAIT`. As in legacy (`$m{wt} = $time + $act_time`, with `$time` taken at request start), the wait counts from the request start.
  - A rejected or unknown outcome releases its own claim by token, so it never keeps the wait. A failed release is logged and the claim expires with its TTL. A failed timer read or claim fails the preflight.
  - The remaining time is read in whole seconds, so `action_wait_ms` and the gate for non-starting actions may open up to about a second early. Legacy counted whole seconds too (`int($m{wt} - $time)`). Wait-starting commands are decided by the claim key itself, so they are not affected.
  - Legacy reads and writes the user file without a lock (`read_user`/`write_user`), so concurrent legacy requests could both pass. That is a legacy defect, not a rule, and the claim intentionally closes it.
  - Rescue and wake are exempt, because legacy serves them outside the `party.cgi` action path.
  - Speech is not a `＠` action and is never blocked.
- **Relation to the cooldown gate**
  - This gate is separate from the ScheduledAction cooldown gate described in [action preconditions](../design/action-preconditions.md).

### NPC interaction

- The HTTP layer keeps one NPC registry keyed by scene destination: name, words (with `{name}` for the actor) and an optional inspect text. The NPC colour is the legacy `$npc_color` `#FF69B4`. Facilities in the registry offer both commands; the facility comes from the current scene, and the scene's qualification is rechecked.
- **Talk**: `npc_talk` logs the actor row `＠はなす` and a random word as the NPC row, and starts the wait. A facility without words would answer privately with `返事がない、ただのしかばねのようだ…`.
- **Inspect**: `npc_inspect` returns the facility's `shiraberu_npc` text, or the default `しかし何も見つからなかった…`, to the actor only (`private: true`). It writes no row and starts no wait (`WaitPrivate`).
  The park (town hub) is the exception: its `shiraberu_npc` swaps `@words` and calls `&hanasu` (`lib/park.cgi`), so the line is a logged `$npc_com` with the actor row `＠しらべる>@町娘`, and it starts the wait.
- Legacy word lines that embed live facts (Gem Store and Accessory item recommendations, Depot capacity) are omitted until those facts are wired.
- Facility-specific NPC acts stay facility commands in the `npc` category, such as SecretShop `secretshop_puffpuff`. It logs `＠ぱふぱふ` and its reply as rows private to the actor and starts the wait.

### Members

- **Storage**
  - Members are kept per place in Valkey Master ([keyspace](valkey-keyspace.md)), under the log's place identity. They are separate from global presence, and their loss is accepted as for the log.
  - An entry holds the character's name, colour, avatar URL and aura effect as of its latest refresh. It expires 300 s after that refresh (configurable). It is still listed at exactly 300 s, because legacy drops a row only when `now - 300 > last`.
  - A character is listed in one place at a time.
- **Refresh** (decided 2026-11-13 in [#1943](https://github.com/witchcraze/party2re/issues/1943))
  - Every owned Gateway command refreshes the actor in its current place before it runs, whether the command is then accepted or rejected. Legacy `read_member` likewise runs before the action. A refresh failure fails the preflight.
  - `presence_refresh` stands in for the legacy page reload, including auto-reload. Its only effect is this refresh. A reload is not a `＠` action, so the wait never blocks it and it starts no wait, and it stays available while sleeping, dead or in an activity. [#1804](https://github.com/witchcraze/party2re/issues/1804) adds the saved-sample effect to the same command.
  - GET never refreshes, as for global presence.
- **Leave**
  - A successful `scene_enter` or `scene_back` removes the actor, as legacy moves call `leave_member`. The actor appears in the new place with its next command.
  - Logout (`DELETE /sessions`) removes every character of the account, because it replaces the per-character `ろぐあうと`.
  - A failed removal is logged and the entry expires.
- **Observation**
  - The scenes with a place log expose `scene.members`: the facility NPC first when the facility has one, then characters in arrival order. Rows carry no character IDs.
  - A store failure fails the observation.

## Alternatives rejected

- **MariaDB rolling table**
  - It would be durable.
  - It would add an insert and a delete to every command and contend on busy places, which is not worth it for display-only value.
- **Only NPC rows, or storing the action and parameters for clients to render**
  - Only NPC rows loses the legacy appearance.
  - Leaving rendering to clients makes every client repeat the formatting rules.
- **Per-facility talk and inspect transport**
  - About 35 facilities would each repeat the same command.
