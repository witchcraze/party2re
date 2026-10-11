# Replay recording, personal archives and visibility

## Reconstruction authority

Replay reconstruction follows the original observable recording and personal
archive boundary, not universal sharing of Go battle results. The direction was
explicitly selected in [#1251](https://github.com/witchcraze/party2re/issues/1251).
Automatic recording of every combat mode and universal participant sharing are
not established legacy requirements.

Legacy references below are relative to `party2/` in the reference repository at
`ff97fc549932a146cc945d7edf71b44c868a840e`. Source inspection and isolated execution
of original routines establish the requirements below. They do not certify a
complete CGI installation or browser playback. Investigation details and bounded
implementation follow-ups belong to #1251.

## Original recording and archive lifecycle

`party.cgi:20–27` remembers whether the actor was in `vs_monster` before executing
a submitted command. After execution it records a scene only when no `$mes`
rejection/message is set. A refresh without a command does not record. This is
operation-driven scene capture, not one replay for every resolved battle or one
scene per engine turn. The pre-action location matters when a command changes it.

`lib/quest.cgi:5–6,1047–1076,1107–1158` and the dispatcher distinguish these modes:

| Producer/use | Automatic quest replay capture | Personal replay copy |
| --- | --- | --- |
| Ordinary Adventure/Party, type 1 (`vs_monster`) | After eligible submitted operations | Exiting participant, if the quest replay is nonempty |
| Dungeon, type 2 (`vs_dungeon`) | No corresponding dispatcher capture | None through this exit path |
| Challenge, type 3 (`vs_challenge`) | No corresponding dispatcher capture | None through this exit path |
| PvP, type 4 (`vs_player`) | No corresponding dispatcher capture | None through this exit path |
| GvG, type 5 (`vs_guild`) | No corresponding dispatcher capture | None through this exit path |
| Sealing/Boss, type 6 (`vs_king`) | No corresponding dispatcher capture | None through this exit path |
| Global replay log | Reader exists; no publisher found in the supplied tracked source | Not a personal archive |

An ordinary adventure's boss encounter remains part of that type-1 adventure;
it must not be confused with the separate type-6 sealing battle. The party is
the adventure roster, not an additional universally recorded combat mode.

`lib/system.cgi:1499–1521` appends scenes to the quest replay. If the existing
file is **greater than 2,097,152 bytes**, further capture stops. A file exactly
at that boundary can receive another whole scene; this is not a strict final
size cap, rolling truncation or a 30-day retention policy. Playback follows scene
order (`html/replay.js` in the reference repository).

`lib/_battle.cgi:nigeru` (`1184–1227,1269–1278`) checks actual membership before
copying. A spectator exits without a personal copy. A type-1 participant with a
nonempty quest replay copies the accumulated scenes to **that invoking person's**
personal replay slot (`lib/system.cgi:1540–1543`). It does not simultaneously
archive to every participant. The next successful copy overwrites that person's
previous slot. Different exit times can produce different personal copies; later
party/guild membership grants no right to another person's copy. An empty or
absent quest replay leaves the previous personal copy untouched.

Live spectating, general battle results, adventure chronicles and manual
screenshots are separate contracts. In particular `lib/system.cgi:165–232`
publishes eligible **manual screenshots** to the global screenshot log; this is
not evidence of public replay publication. Do not turn that route into a producer
of public battle replays.

## Original content and access boundary

A replay scene contains the scene title, the battle member display, and at most
the three newest non-whisper log entries, including their speaker, display color,
comment and date. The original recorder calls `_sukusho_mes` (`194–208`), not the
five-entry `_replay_mes` helper. The member renderer
(`lib/_battle.cgi:409–445`) includes displayed names, HP/max HP, HP proportion,
visible states/tension and visual presentation references. It hides numeric
HP/max HP above 999 as `???` and filters defeated opponents/substitutes according
to the capturing actor's view. It does not capture the separate header's full
attack/defense/agility, job, inventory or account data.

Reconstruct these display facts as structured data with clean-room presentation;
do not copy original HTML, source or assets. A full Go initial-stat snapshot and
complete turn-log array are not substitutes for the original scene contract.
Whispers and source network addresses are excluded from captured comments.
Credentials used to authenticate a read are not replay content.

`replay.cgi:16–27,90–105` has one read entry point (`run`) and no action dispatch
table. With no requested path, or the global log path, it reads the global replay
without credentials. A personal path requires the matching owner identifier and
password. A different personal path, missing/wrong password or malformed owner
identifier is rejected. No file means a read error, not an automatically generated
public replay.

The reconstruction target replaces password-in-form transport with existing
session authentication and verified character ownership. A personal archive is
readable only by the owner of its recipient character. Being another participant,
spectator, guild member or holder of a replay UUID does not grant access to that
copy. Multiple personal copies may share immutable scene storage internally,
provided their archive ownership and captured contents remain distinct.

An explicitly established global replay may expose its captured display scenes
anonymously. The original does not define a richer participant-only stat projection
beside those scenes. No current combat mode is implicitly a global publisher;
publication requires evidence of a source/use or a separately approved departure.
Do not invent an all-combat public feed or a sharing toggle to fill that gap.

## Read and migration target

One saved publication/recipient decision must govern all read paths:

- **Detail:** verify the personal archive recipient at the service boundary;
  only a proven global replay permits anonymous access. UUIDs are identifiers,
  not credentials. Unauthorized private and nonexistent detail requests return
  the same not-found result; invalid supplied sessions retain normal auth errors.
- **Owned history:** authenticate the requested character's ownership and return
  its personal archives. Appearing as the initiator/opponent of someone else's
  copy is not ownership. The legacy personal replay slot is latest-only; the Go
  historical match index is a separate current capability, not proof that the
  original retained every personal replay.
- **Recent:** return only proven global records. Personal archive headers must
  not leak through the feed, even to logged-in participants. With no proven
  publisher/data the feed is empty; do not manufacture public records.

Apply visibility and recipient selection before pagination/limits and cursor
construction, for both existing query forms. Future Gateway observation must use
the same service policy; route migration alone does not settle replay permissions.

Existing `battle_replays` rows have neither publication provenance nor an archive
recipient. Preserve their stored data, but do not silently backfill `public` or
guess a recipient from the two combat header IDs/current party membership. Rows
without trustworthy provenance remain unavailable through ordinary read APIs
until classified. This is an explicit access restriction needed to recover the
original private boundary, not a claim that historical ownership was recovered.
Deploy guarded reads/storage before enabling reconstructed producers. Public
provenance alone must not expose a payload containing fields absent from the
verified public scene contract.

## Current Go implementation and remaining gaps

`internal/replay` currently stores a `BattleReplay` with two initial participant
stat snapshots and a `[]corebattle.TurnLog` array in MariaDB `battle_replays`.
Payloads are JSON, not compressed data. Headers carry combat identities, result,
turn count and creation time. The recorder adapters accept `pvp`, `gvg`, `boss`,
`dungeon`, `adventure` and `challenge` names, but those names do not prove legacy
recording coverage or production integration.

The composition root wires session authentication and character persistence into
Replay. Personal detail and history verify the authenticated player's ownership
of the persisted archive recipient at the service boundary; history retains the
HTTP ownership wrapper as well. Detail checks metadata and ownership before
loading the raw payload. Unauthorized personal IDs and nonexistent IDs share the
same not-found result. Supplied invalid credentials retain authentication errors.
Combat identities, current membership and combat type never grant archive access.

`battle_replays.provenance = personal_archive` with a nonempty
`archive_recipient_id` identifies an explicitly classified personal copy. Empty
or unsupported provenance is unavailable. The trusted internal recording request
can carry this pair; existing result adapters supply neither and remain
unclassified. No production combat feature calls those adapters. Personal history
selects only this classification and recipient, before limits and cursor creation;
equal timestamps use descending replay ID as the tie-breaker. This retained
archive index does not implement the original latest-only slot or approve a new
copy/retention lifecycle.

Migration `102_replay_archive_provenance.sql` adds metadata with empty defaults
and a recipient index, preserving existing payloads without deriving ownership
from combat headers. Deploy the additive migration first, then replace **all** old
reader instances with guarded readers before enabling any classified producer.
During a rolling deployment, old readers still expose data; use a maintenance
window or disable Replay routing until all old instances have drained. Do not
revert to the old readers as a rollback while Replay routes remain reachable.
Historical rows remain unavailable unless independent trustworthy archive-copy
evidence establishes their recipient; this issue supplies no reclassification or
public import endpoint.

The current raw-stat/turn-log representation has no supported global provenance
or safe public display payload. Detail denies all non-personal records and both
recent SQL forms reject every row before pagination, including rows manually
marked public/global. Recent therefore returns an empty feed regardless of login.
A verified publisher and structured scene implementation are prerequisites for
public display support; a visibility flag alone must never enable it.

The current engine turn log contains `turn`, `actor_id`, `action_name`, `target_id`,
`damage_dealt`, `healing_done`, `is_critical`, `message` and `remaining_hp`. Recorder
adapters build snapshots from characters, monsters or engine participants and
save completed results. `DeleteOlderThan` is an available storage operation,
not evidence of an active automatic retention policy or a legacy rule.

The [current OpenAPI sources](../api/paths/replays.json) describe guarded replay
detail and the empty recent feed; owned recipient history is in
[`character.json`](../api/paths/character.json). Operation-driven capture and
personal latest-slot publication remain tracked by
[#1320](https://github.com/witchcraze/party2re/issues/1320), with interactive
normal-adventure restoration owned by
[#1323](https://github.com/witchcraze/party2re/issues/1323).
The current immediate solo/party crawl omits player operations and distinct exit
times; it is an implementation gap, not approval to replace the original Replay
with a floor/battle-result digest. Restore real submissions and actor exit facts
under the [Adventure interaction contract](adventure.md#original-interaction-contract-and-reconstruction-target)
before enabling those producers. Keep the current guarded reads in place. Do not
fabricate historical commands, chat, exit times or an all-modes replay producer.
