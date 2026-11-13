# Player Private Home & Mailbox Design

## Overview

The Home module (`internal/home`) covers the private home (`home.cgi`), town house estates, letters, companion phrases, delivery notices and resting (`sleep.cgi`). Evidence paths are under `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`. Transport operations and schemas are in [OpenAPI](../api/openapi.json); fields are in the [migrations](../../migrations/).

## Observation, visibility and navigation

`home` observes the owned actor by default; `scene_select` with `target_kind:home` observes another character's home while the authenticated character stays the viewer and mutation actor. Public owner identity is limited to ID, name and color, never the wallet, account or private notices. Only an exact actor/target match adds private unread-letter, phrase and delivery counts, and public reads never fetch those collections. Required read failures fail the observation. A character without a home row gets defaults; observation never creates the row. A town estate lease is distinct from private Home existence.

Own Home offers `home_inbox` and `home_outbox`, which always read the owned actor's mailbox. Selecting or observing never reads/clears a letter, acknowledges a notice or wakes the character. **Departure:** legacy rendering clears letter/money/item notices (`lib/home.cgi:45–67`); observation here never does, as the Gateway read contract requires. Home Sleep templates carry the observed target ID, and a submitted command keeps that ID even if selection changes before submission.

### Scoped legacy action reconciliation

Active registration: `lib/home.cgi:26–39,73–105` (call paths `138–201,237–265,379–507,591–604`). **Decision (#646/#1012):** explicit `home_wake` replaces the implicit recovery on GET in `sleep.cgi:18–42`. The other actions (reference views, item use, letters, color, phrases, custom skill, image, talk) still run through REST; their Gateway migration is open (#949). Commented avatar/upload registrations are not active dispatch.

### Retained REST and deferred detail ownership

Home and estate REST routes remain until their replacements are verified: command migration is #949, route removal #950.

## Domain rules

### 1. Town house estate

Four towns have different prices, lease cycles and style ranges, with at most ten houses per town and one house per character; house construction awards the guild `cycle days × 10` points in the same transaction. House construction and color changes are rejected during sleep or pending wake.

### 2. Player-to-player letters

A character may send a letter to itself, as in legacy; it appears in both of its boxes. Sender and recipient delete independently; the row is purged only when both have deleted it, including eviction from a full box.

Each box's copy belongs to its owner, not to the counterpart's identity (legacy stores the recipient's copy and the sender's log in separate per-player directories, and self-deletion removes only the deleting player's directory). Deleting a character or account hides only that character's copies; the survivor keeps theirs, including read state and the name/color/body/time snapshots, until they delete it. Character IDs on a letter are therefore historical references, not foreign keys; the row is purged once neither side retains a copy.

<a id="letter-settings"></a>

#### Letter throttling, retention and body length

- **Legacy rule** (`lib/system.cgi:1055–1087`, `lib/home.cgi:379–402`): a letter is rejected when the newest letter in the recipient's box came from the same sender and `latest time + $bad_time > now`; guild letters use the same routine, and a letter from another sender ends the wait. Each box keeps at most `$max_log` entries; the recipient's box and the sender's log are trimmed separately.
- **Value provenance:** tracked `config.cgi` and template `config.cgi_origin` agree on `$bad_time = 30` and `$max_comment = 500`; `$max_log` is 30 in the tracked file and 999 in the template (see [phrases](#3-companion-greeting-phrases--resident-companions-character_companion_phrases-home_members)). Live values are unverified.
- **Decision (#1126) — throttle:** compare sender/recipient character IDs against the newest letter the recipient has not deleted (by `created_at`, then `id`); the recipient deleting it removes it from the check, the sender deleting their copy does not. The wait is a fixed 30 seconds in whole seconds (29 rejected, 30 allowed). Rejection writes nothing. A guild callout stops at that recipient ([Guild](guild.md#broadcast-callouts-よびかける)); other guild notices are dropped best-effort.
- **Decision (#1126) — retention:** each box keeps the newest N (default 30, `PARTY2_HOME_LETTER_LOG_LIMIT`). Delivery always succeeds; the oldest letters leave that box even when unread, through the owner's deletion flag. Lowering N shortens a box on its next send.
- **Decision (#1126) — body length** (`lib/system.cgi:700–725`): default 500 (`PARTY2_HOME_LETTER_MAX_LENGTH`), also for guild notices. Line breaks are removed. The limit counts the length after legacy HTML escaping (`&`, `;`, `,` count 5; `<`, `>` 4; `"` 6). Legacy also counted the `＠てがみをかく>宛先` text; Go takes the recipient separately.
- **Departure (#1126):** legacy silently strips combining marks in several ranges, which alters accented and Thai text. Go keeps them after NFC normalization and rejects excessive sequences.

### 3. Companion Greeting Phrases & Resident Companions (`character_companion_phrases`, `home_members`)

Resident companions combine ranch monsters at home and Heaven Wish NPC companions; both share the ranch operations and home capacity ([Monster](monster.md)). Talking or teaching with no resident companion is rejected, as in legacy (“教える相手がいません”). External Home cards use a [separate member/phrase contract](player-and-character.md#213-external-character-and-home-cards-adopted-not-implemented).

- **Legacy rule** (`lib/home.cgi:443–477`): the phrase list belongs to the character, not a pet. A new phrase goes to the head, duplicates are separate entries, and the list keeps at most `$max_log`, so teaching is never rejected for capacity. Removal by text removes only the first match (the newest duplicate).
- **Capacity provenance:** the `config.cgi` tracked until commit `0713fcc` (2023-03-09) sets `$max_log = 30`. That commit added the template `config.cgi_origin`, which changes only `max_entry`, `max_login` and `max_log` to 999, so 999 is treated as a distribution default. The live value is unverified.
- **Decision (#1125):** keep at most N phrases per character (default 30, `PARTY2_HOME_COMPANION_PHRASE_LIMIT`; an invalid value means 30 and does not stop startup). At capacity, teaching succeeds and removes the oldest in the same serialized transaction. Lowering N shortens the list on the next teach; there is no startup trim.
- **Talking** (`lib/home.cgi:422–439`) picks a random resident speaker and a uniformly random retained entry, so duplicates weigh by count.

### 4. Remote Depot & Inventory Item Usage (`＠つかう`)

Equipment is inspected without consumption. Home recovery uses [Sleep](#5-resting--sleeping-sleepcgi-homecgi), not combat recovery items.

#### Legacy dispatch and consumption contract

**Decision (#1124):** retain observable legacy behavior, including the no-stat consumption of 113/114 and the source-dependent missing-box result. Own Home alone registers `つかう` (`home.cgi:26–39,73–105`). By name it prefers held weapon, armor, then tool, then the first matching Depot entry (`237–340`); Go uses explicit source and instance IDs instead. Categories 0/1/3 do not dispatch callbacks. All 53 category-2 and 3 category-4 entries are accounted for.

- **H2/H4 — held tool** (`284–292`): call without arguments; a nonempty `mes` stops consumption, otherwise clear the held tool. The return value is ignored, and changes made before setting `mes` are not undone.
- **D2 — Depot category 2** (`298–348`): call while scanning the Depot, before removing the row. `mes` returns without removing it.
- **D4 — Depot category 4** (`330–373`): **pitfall** — keep the row in the first rewrite, call without arguments, then remove the first remaining row of that item number, with no post-callback `mes` check. This is not consume-before-callback or rollback-on-no-effect.
- Depot capacity never blocks dispatch: `send_item` (`system.cgi:818–835`) appends rewards to an existing Depot even when full, without inventory routing, collection registration or a self-delivery notice; `get_depot_c` (`1267–1291`) only counts.
- Go outcomes: `no_effect` rolls back; `consumed` commits and consumes one; where legacy sets `mes` after mutating (item237), the mutations commit, the item is kept and the result is `no_effect`. Appearance writes are ordered by the item transaction's character lock and undone on failure only while still current.
- **Pitfall — random rewards** (items125/126/199/259): each reward selects slot **1 through N−1** of the N-slot list; slot 0 is unreachable and repeated slots keep their weight. Rewards and consumption commit together.
- **Item237** (`_data.cgi:1994`): below level 99 nothing happens and the item is kept. Otherwise d=EXP−98010, credit trunc(d×U[1,1.5)) capped at 999999 gold, set EXP=98010; d≤0 sets `mes` but still executes those mutations and keeps the item. **Decision:** gold has no lower clamp, so d<0 may leave it negative.
- **Item058** (`_data.cgi:929–955`): legacy always has a profile file, so the profile write and public news are the normal path.

Open items (`lib/_data.cgi` lines):

| Items | Legacy behavior | Owner |
| --- | --- | --- |
| 043 transformation staff (`789–799`) | Uniform monster appearance 001–108, fallback if the image is absent; consumes | #1837 |
| 060–065 orbs (`965–1054`) | Append rune `s/r/b/g/y/p`; an existing rune sets `mes` and keeps the item | #1839 |
| 113/114 bracelets/rings (`1408–1429`) | Returned stats are discarded: **no permanent HP/MP change; consumes one** | #1840 |
| 203 sacred mana (`1824`) | SP +200, including over-level; consumes | #1835 |
| 231–236, 245 wallpapers (`1940–1993;2002–2010`) | Replace own background with field 0/1/2/4/5/6/8; consumes, no gold or lease prerequisite (unlike the Store's paid wallpaper) | #1836 |
| 259 sealed key (`2024–2068`) | Prefer held box, otherwise first Depot box. With a box: remove it, deliver 059 plus 2–4 weighted rewards, then consume the key. Without a box: held key kept, Depot key consumed. Full Depot accepts every reward | #1842 |

Appearance and wallpaper items use newly authored assets ([asset contract](https://github.com/witchcraze/party2re/issues/654), [resolution](https://github.com/witchcraze/party2re/issues/729)), not legacy images.

### 5. Resting & Sleeping (`sleep.cgi`, `home.cgi`)

Recovery is free and happens at one's own or a visited home. Sleeping at another home requires only that the host character exists: `lib/home.cgi:1–7` checks the user directory and `lib/system.cgi:418–435` the private `home.cgi`, both created at registration, not a town lease.

- **Decision (#1123) — duration:** 300-second default base, configurable at startup through `PARTY2_HOME_SLEEP_BASE_SECONDS` (unset/empty is 300; any other value must be a positive integer whose 3× fits the timer, otherwise startup fails). `config.cgi_origin:25–26` gives 5 minutes; `lib/home.cgi:182–188` converts it to seconds and applies the 20/30 thresholds: fewer than 20 observed characters ×1, 20–29 ×2, 30 or more ×3. This adopts the reference configuration, not a recovered live value.
- **Population** uses the distinct character count from the [public presence contract](player-and-character.md#212-public-presence-and-login-message), taken once after admission and before any Sleep effect. The actor or host is not added just because Sleep is requested. This follows `lib/home.cgi:182–188` before `party.cgi:20–30`'s final `write_user` (`lib/system.cgi:29–46`). The duration is fixed for that cycle. A failed count aborts Sleep; never substitute zero, a cached count or all registered characters (legacy `get_login_member`, `lib/system.cgi:1387–1417`, also stops on failure).
- **Open (#1822):** Go still defaults to 60 seconds, treats a missing or failing counter as zero, and lacks the production counter and the startup setting.
- **Persistent recovery:** legacy `sleep.cgi` keeps `$m{sleep}` until the next action; `characters.pending_wake` keeps the equivalent obligation beyond timer expiry and restarts. Protected actions reject active sleep and pending wake; duration expiry alone does not clear the obligation. Observation, Wake and Rescue stay available.
- **Wake order:** commit full HP/MP and zero fatigue first, then revert temporary job memory and run recovery hooks in order: Tavern fullness reset; Chapel blessing removal (the prayer quota clears only when its date is an earlier calendar day); ongoing Alchemy completion; rented costume removal; once-dungeon reset. Pending recovery is finalized only after every required step succeeds. Earlier steps are not rolled back across stores; a failure keeps pending recovery so a later Wake can finish. Sleep and Wake share an ownership claim ([Valkey keyspace](../architecture/valkey-keyspace.md)).
