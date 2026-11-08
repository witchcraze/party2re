# Place log, action wait and NPC interaction

> **Status: planned contract.** Approved for the reconstruction on 2026-11-08 under
> [#1937](https://github.com/witchcraze/party2re/issues/1937). Nothing described here
> runs yet; each section names the issue that implements it.

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

### Place log ([#1938](https://github.com/witchcraze/party2re/issues/1938))

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
  - Facility scenes expose the viewer-filtered log, newest first.
  - A store failure fails the observation.
  - A GET never writes.

### Writers ([#1940](https://github.com/witchcraze/party2re/issues/1940), [#1941](https://github.com/witchcraze/party2re/issues/1941), [#1942](https://github.com/witchcraze/party2re/issues/1942))

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
  - Speech appends a public character row.
  - Whisper appends a private row to a named, existing recipient.
  - Both use the shared text checks.

### Action wait ([#1939](https://github.com/witchcraze/party2re/issues/1939))

- **When the wait starts**
  - A successful command whose legacy counterpart keeps a non-zero `act_time` sets a per-character wait in the existing timer family.
  - The wait defaults to 4 s and is configurable.
- **What the wait blocks**
  - While the wait is active, Gateway commands are rejected with a structured `action_wait` code and the remaining time. Context shows the remaining time so a client can draw the gauge.
  - Rescue and wake are exempt, because legacy serves them outside the `party.cgi` action path.
  - Speech is not an action and is never blocked.
- **Relation to the cooldown gate**
  - This gate is separate from the ScheduledAction cooldown gate described in [action preconditions](../design/action-preconditions.md).

### NPC interaction ([#1940](https://github.com/witchcraze/party2re/issues/1940))

- Each facility registers its NPC name, colour, words and an optional inspect text.
- **Talk**: the shared `npc_talk` command resolves the facility from the current scene. It logs a random word as an NPC row and starts the wait.
- **Inspect**: the shared `npc_inspect` command returns the facility's text, or the default text, to the actor only. It writes no row and starts no wait.
- Facility-specific NPC acts such as `ぱふぱふ` stay facility commands.

### Members ([#1943](https://github.com/witchcraze/party2re/issues/1943))

- Per-place membership uses the same place identity and the 300 s expiry. It is separate from global presence.
- **Open question**: may an owned GET refresh membership, or may only commands do so? Global presence lets only commands record activity. This must be decided before implementation.

## Alternatives rejected

- **MariaDB rolling table**
  - It would be durable.
  - It would add an insert and a delete to every command and contend on busy places, which is not worth it for display-only value.
- **Only NPC rows, or storing the action and parameters for clients to render**
  - Only NPC rows loses the legacy appearance.
  - Leaving rendering to clients makes every client repeat the formatting rules.
- **Per-facility talk and inspect transport**
  - About 35 facilities would each repeat the same command.
