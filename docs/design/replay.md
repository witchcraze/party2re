# Replay recording, personal archives and visibility

## Reconstruction authority

**Decision (#1251):** Replay follows the original observable recording and personal archive boundary, not universal sharing of Go battle results.
Automatic recording of every combat mode and universal participant sharing are
not established legacy requirements.

Legacy references below are relative to `party2/` in the reference repository at
`ff97fc549932a146cc945d7edf71b44c868a840e`. Source inspection and isolated execution
of original routines establish the requirements below. They do not certify a
complete CGI installation or browser playback.

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

## Native scene and latest-slot contract

**Decision (2026-11-04):** count structured UTF-8 scene bytes instead of legacy HTML bytes, with the failure behavior closest to the original. These are targets; no production producer exists yet.
Preserve actual scenes and exit-copy order; do not silently drop failed captures.
The durable recovery below adapts persistence to the approved SQL command boundary.

### Capture and display projection

Use the [normal-run operation/activity matrix](adventure.md#normal-run-operation-discovery)
to classify the submitted operation separately from combat effects and activity.
The dispatcher reconciliation is:

| Original boundary | Native capture/copy target |
| --- | --- |
| `quest.cgi:189–199` create/join/spectate | Enrollment is not a recorded operation: the actor was outside `vs_monster` before dispatch |
| `party.cgi:20–27` eligible submission | Freeze the pre-command location/title; append one post-operation display scene after its room-log write, not one per hit, NPC response or floor |
| `_battle.cgi:640–649,672–680` comments, unknown text, whispers and silently banned skills | A no-message outcome can record even without a battle effect. A whisper can trigger capture, but its private log entry is always excluded. Unknown Gateway action IDs still reject; any legacy-text acceptance is a separate transport contract |
| `_battle.cgi:650–652`; `system.cgi:165–208` manual screenshot | Success and failure set a message; neither appends an automatic Replay scene. Manual screenshot publication remains separate |
| `_battle.cgi:1184–1227` participant exit | Freeze a copy of the accumulated prefix for this participant before the exit's log/possible scene append; remaining participants continue |
| Spectator exit, rejected operation, refresh, receipt lookup/retry, native idle cleanup | No personal copy. Refresh/rejection/retry/cleanup adds no scene; eligible spectator submissions follow the dispatcher rule without creating spectator archive ownership |

The title is the one selected before dispatch, including when advance/travel
changes floor or stage (`_skill.cgi:23`; `system.cgi:660–677`). Members and comments
are captured after the operation. Preserve these different observation times.
Each scene uses its submitting actor's perspective. The shared room archive can
therefore contain different actor views; an exit copies that immutable sequence,
not a newly rendered recipient view or one private sequence per current member.

Only the following display fields enter a versioned structured scene:

| Display fact | Allowed representation and visibility |
| --- | --- |
| Title and scenery | Captured title text/floor and clean-room background/field/overlay keys; no legacy markup, asset paths or executable content |
| Ordered member slots | Captured display name/color and clean-room appearance/effect keys. Hide a defeated member when its color differs from the capturing actor's, or it is a substitute; retain same-color fallen real members (`_battle.cgi:423`) |
| HP display | Mask HP and max HP independently above 999, and preserve the displayed integer HP percentage. Hidden numeric values must be absent from the scene, not merely hidden by the client |
| State display | Visible state/temporary/extended-state labels, displayed extended duration, and tension only when the original state display does not supersede it (`425–436`) |
| Comment window | At most three newest non-whisper room entries in their displayed newest-first order: captured speaker, color, text and displayed date. Include real operation/NPC text where written; exclude addresses and private recipients/body |

Escape text and resolve presentation keys through native allowed assets; never
store/render arbitrary HTML, CSS or source asset URLs. Exclude raw HP behind masks,
MP, attack/defense/agility, job/level, inventory, account/session data, full engine
logs and private whisper content. Internal run/command/sequence and capturing-actor
identities support deduplication; they do not extend the public display allowlist.
Use actual command/room facts, never invented speech or reconstructed historical
commands. Missing display/comment/role facts block the affected adoption; current
engine turn logs cannot certify them.

### Ordering, byte budget and personal copies

Scenes append in committed operation order; playback is oldest scene first.
Comments inside a scene remain newest first. Count the sum of the exact persisted
UTF-8 scene encodings, excluding storage envelopes, indexes and copy metadata.
Use one stable, versioned encoding; record each encoded length once rather than
re-encoding old scenes with new formatting. Before accepting a scene, suppress it
only if the accumulated length is **greater than 2,097,152**. At equality append
the whole next scene, even if it crosses the boundary. Never split/evict scenes,
silently apply a strict final-size cap, or impose 30-day pruning. Different byte
lengths from legacy HTML are the explicitly approved representation departure.

A successful participant exit selects the nonempty accepted prefix immediately
before its own possible scene. Copy only to that exiting character's latest slot,
with explicit `personal_archive` provenance and recipient metadata. A later exit
can include the earlier exit's scene; the earlier copy cannot. Empty/absent source
preserves the previous slot. Ordinary boss floors stay in the same normal run;
the other modes in the lifecycle table remain excluded. Timeout is not an exit.

Latest-slot replacement is atomic: readers see either the complete previous copy
or the complete new copy. A successful nonempty replacement removes the superseded
operation-driven copy from ordinary detail/history reads; it does not accumulate
new scene history. The existing classified match-history capability remains a
separate record kind. Preserve unclassified historical payloads and their denied
access. Shared immutable prefix storage is allowed, but its lifetime must cover
every referencing personal slot and pending copy, even after active-run disposal.

### Command commit and publication failure

Reuse the [SQL command/receipt boundary](../architecture/transient-run-state.md#one-command-one-bounded-sql-transaction).
Under the existing run serialization, freeze the display projection, byte decision
and exit prefix alongside the accepted command. Persist ordered Replay source
facts/copy intent with state, assets and receipt in that bounded transaction;
do not reconstruct them from a later run read. The exit intent precedes its
optional scene append logically, even though native SQL commits them together.
No archive publication runs for an uncommitted/unknown command outcome.

Failure to preserve required source facts before commit aborts the uncommitted
transaction. After commit, publication is optional to gameplay: failed scene/copy
materialization retains the frozen source and pending status, reports the known
command outcome with archive recovery pending, and leaves the old slot readable.
Recover only Replay work using the same command/run identity; never rerun combat,
consumption, rewards, exit effects or room activity. Resolve lost commit/publication
acknowledgements from persisted evidence, without duplicate scenes or copies.
GET may observe pending work but never performs publication or mutates the run.
Use a bounded explicit owner recovery operation; no new generic event framework,
background poller or independent authority is required.

Serialize latest replacement per recipient and preserve accepted exit order across
runs: a delayed older copy must never overwrite a newer published copy. Do not
discard source/pending work when the last participant leaves or the lobby expires.
Storage/decode failure is distinct from an empty archive; preserve the prior slot
and make failure observable rather than claiming a successful empty copy.

This is a native crash-recovery guarantee, not a claim of legacy transactional
atomicity. Legacy file open/copy errors abort the response
(`system.cgi:1507–1508,1540–1543`), without a rollback of earlier gameplay writes.
The native receipt must retain the committed outcome independently of publication.

## Current state and remaining gaps

- The scene and latest-slot storage boundary exists, but no production producer calls it; production starts still use the batch crawl. Explicit combat (#1329) and the Gateway cutover (#1330) are prerequisites for real operation facts, and solo/party capture adoption is #1826, which must reflect actual removal after exit.
- The older `battle_replays` raw-stat/turn-log records have no safe public display payload. Only rows explicitly classified as a personal archive with a recipient are readable, by that recipient's owner. Detail denies everything else, and the recent feed is empty regardless of login. A visibility flag alone must never enable public display.
- **Decision:** historical rows stay unavailable unless independent, trustworthy archive-copy evidence establishes their recipient; ownership is never guessed from combat headers or current membership.
- **Deployment:** guarded readers must replace every old reader before any classified producer is enabled. Old readers still expose data, so use a maintenance window or disable Replay routing during a rolling deployment.
- Observation, navigation and route replacement are open (#948). Do not predeclare scene schemas in OpenAPI or fabricate commands, chat, exit times, spectators or all-mode recording from batch results.
