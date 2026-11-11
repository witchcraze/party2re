# Player and Character Specification

## 1. Domain Overview

A Player is the account (authentication, sessions); a Character is the in-game persona. Every Character belongs to exactly one Player for its whole life, and only the owning Player's session may read or mutate it. Player, character and API shapes are owned by the code, migrations and [OpenAPI](../api/openapi.json).

## 2. Invariants and Rules

### 2.4 Currency & Wallet Ceiling

- Wallet gold is capped at 999,999 (legacy `system.cgi:71`); every credit path clamps to it. Larger sums belong in the [Bank](bank.md). Small medals cap at 999,999,999.

### 2.5 Vitality & Fatigue Invariants

- Fatigue has no upper or lower bound, matching legacy additions (`lib/_battle.cgi:1211–1213`, `lib/quest.cgi:1073`, `lib/_casino.cgi:217`, `lib/vs_king.cgi:88`) and subtraction (`lib/god.cgi:57`): it can exceed 100% and a refresh wish can push it below 0%, buffering later fatigue. At 100% or more, adventures and fatigue-generating mini-games are blocked. Resting at home sets it to 0.

### 2.6 Player Account Lifecycle & Ban Invariant
- **Soft ban** invalidates all sessions and personal access tokens immediately; later logins and character requests are rejected.
- **Registration boundary**: Account BAN does not prevent a different account from registering at the same IP. [Optional IP registration denial](admin.md#33-optional-ip-registration-denial-planned), including independent release and explicit BAN-time selection, is adopted but deferred; `last_ip` alone is not a registration prohibition.

Character activity is independent of account login tracking. The adopted
[inactivity retention contract](player-deletion.md#4-inactivity-retention) expires
individual characters, preserves their account and siblings, and requires a
durable gameplay clock before automatic deletion can be enabled.

### 2.7 Request Throttling and Reload Sleep Penalty Reconciliation (Approved Departure)

#### 2.7.1 Legacy Reload Penalty Specification

- `party2/lib/system.cgi:148–156`: during `&read_user`, when `$m{ltime} + $wait_time > $time` (`config.cgi:88`, reference default 1 second) and the request is not an auto-reload, append a strike to `reload.cgi` and stop with “更新の連打は禁止しています…”.
- `party2/login.cgi:37–104`: on the next login, count and truncate the strikes, then add sleep: more than 30 strikes 7 days, more than 25 3 days, more than 20 24 hours, more than 15 6 hours, more than 10 1 hour; 10 or fewer nothing. Account deletion and IP blacklisting were drafted but commented out.

#### 2.7.2 Approved Architectural Departure (#1106)

**Decision (#1106):** reload sleep penalties are superseded by HTTP rate limiting (429 with retry headers) and administrative soft bans. Request frequency belongs to the transport layer and never mutates character timers: game sleep is reserved for home sleep and rescue penalties, and a multi-hour gameplay lock on parallel SPA or agent requests would be a severe false positive. Limiter infrastructure failures never impose gameplay sleep either.

### 2.8 Registration Population Limit & Capacity Reconciliation (#1105)

#### 2.8.1 Legacy Population Limit Specification

- `config.cgi_origin:46` sets `$max_entry = 999`. `new_entry.cgi:133–138` rejects registration with “現在定員のため、新規登録は受け付けておりません” when the flat-file counter reaches it, and `224–234` increments the counter under an exclusive lock.
- Self-deletion and admin deletion decrement the counter (`delete.cgi:56`, `admin.cgi:115`). **Pitfall:** exile has the decrement commented out (`lib/exile.cgi:177`), so exiled players kept their slot until directory cleanup.
- `login.cgi:190–250` recalibrates the counter while rebuilding the ranking, after purging inactive accounts.

#### 2.8.2 Reconciled Architecture & Adoption Policy (#1105)

**Decision (#1105):** the limit applies to Player accounts at registration only; character creation neither consumes nor checks it. `PARTY2_MAX_PLAYERS` defaults to the legacy 999, and 0 or negative disables the limit. Only non-banned accounts count, so, unlike the commented-out legacy decrement, banned accounts free their slot; account deletion frees it too. At the limit, registration is rejected with the legacy message, atomically under concurrent registrations.

### 2.9 Transactional Character Creation & Joining News

The character, its initial Depot, any eligible [referral delivery](#210-registration-referral-delivery) and the [joining news](notifications.md#character-joining-news) commit in one transaction. A failure in any of them rolls back the creation. After an unknown commit outcome, clients must not replay the creation automatically.

Legacy `party2/new_entry.cgi:155–206` routes `mode=new_entry` through `new_entry` → `create_user`, saving the character before writing birth memory and joining news. Account registration and character creation are separate in Party2re: only character creation emits joining news. The same-transaction news boundary strengthens failure atomicity over the legacy file writes. Birth memory and trajectory history remain separate work in [#773](https://github.com/witchcraze/party2re/issues/773); joining news does not implement or replace them.

### 2.10 Registration Referral Delivery

An optional `referrer_character_id` on `POST /characters` identifies an existing
character. Each successful referred character creation grants that character one
separate `item-023` instance (quantity 1) in its existing depot and one delivery
notice identifying the joining character and referral reason. This is a redeemable
item, not a direct SmallMedals credit. Account registration grants nothing;
additional characters and referral to another character on the same account are
permitted. The newly generated character is not its own existing referrer.

Omitted/blank referral IDs preserve ordinary creation. A nonexistent referrer or
missing recipient depot is a delivery no-op. Other lookup, item or notice errors
fail the enclosing creation transaction. Delivery appends even at/above nominal
capacity without increasing capacity, stacking the reward, or changing ordinary
deposit/transfer limits. The [Depot contract](depot.md#capacity-formula) owns this
overflow distinction.

The creation/name uniqueness constraint is the event boundary: concurrent retries
of the same creation/name commit at most once, while distinct committed creations
each grant once. A failed or rolled-back creation leaves no reward or notice;
there is no separate reward retry endpoint. Successful creation participates in
an enclosing transaction's final commit/rollback, including joining news.

Legacy evidence: `party2/new_entry.cgi:33–39,66–70,214–217` carries referral `yid`
from the registration form and delivers kind 3/no.23 after creation;
`party2/lib/system.cgi:818–835` requires an existing depot and appends the item and
referral notice without a capacity check. Mapping registration onto every
character creation and retaining unconditional delivery were approved in
[#1102](https://github.com/witchcraze/party2re/issues/1102). Explicit character IDs
replace the legacy hex-encoded name.

### 2.11 Character Name Uniqueness

Creation and Naming Hall rename share the `characters.name` unique constraint.
Inputs retain the existing NFC normalization and whitespace rejection; stored
names compare exactly using `utf8mb4_nopad_bin`, without case/accent folding or
trailing-space padding. Thus `Hero`, `hero` and `Héro` are distinct, while NFC-
equivalent new inputs contend for one name. Existing stored strings are preserved
without retroactive normalization. Name lookups use the same column collation.

The SQL constraint arbitrates concurrent creation/creation, creation/rename and
rename/rename. Its conflict maps to `character.ErrNameAlreadyTaken` and HTTP 409
on both endpoints. Other SQL errors remain errors. A losing creation leaves no
character, depot, joining article or referral delivery; a losing rename retains
its name and money and publishes no rename article. Rename's availability lookup
is an early check, not the concurrency guarantee.

The [name-uniqueness migration](../../migrations/110_character_name_uniqueness.sql)
checks existing duplicate groups under that exact target collation before any
schema change and prints their character IDs. A conflict stops migration and
retains all rows; operators must resolve it explicitly before rerunning. It never
deletes, merges or automatically renames characters. See the
[schema preparation procedure](../development/development-environment.md#2-prepare-database-schema).

Legacy `new_entry.cgi:130–131,155–159` derives the registration directory from
exact name bytes and rejects an existing identity. Naming Hall's `なまえ` →
`namae` (`lib/name_change.cgi:34–41,95–143`) targets that same identity. SQL
uniqueness makes this rule atomic across both operations; existing input NFC
normalization remains the reconstruction's validation contract.

### 2.12 Public Presence and Login Message

The decision in [#1104](https://github.com/witchcraze/party2re/issues/1104)
preserves **sampled presence**, independently of account authentication and the
[continuous retention clock](player-deletion.md#4-inactivity-retention).
Presence and its message belong to a Character, identified by stable character
ID. Multiple sessions/PATs for one character count once; different characters of
one Player count separately. This is recent recorded game activity, not a live
connection or a claim that the character can currently act.

**Legacy evidence.** `login.cgi:20,25–30,106–109,126–138` validates a 60-character
message, saves it on the character and prepends a public snapshot on game entry.
`lib/system.cgi:29–46` appends another snapshot only when strictly more than
`login_time * 60` seconds have passed since the previous sampled save; intervening
saves refresh `ltime`, not this sampling clock. `get_login_member` (`1387–1417`)
discards expired records and later duplicates, rewriting the log with only the
first remaining record per name. `index.cgi:17–25` displays that list and count.
The adopted window is **900 seconds**, using `config.cgi_origin:51–52`'s 15-minute
reference default; the active legacy configuration remains unavailable.

**Recording and expiry.** Share authenticated save-equivalent events and the
server's injected UTC clock with retention, but keep separate presence records:

- The first game save records a sample. Later saves replace it only when
  `now > sampled_at + 900s`; equality does not refresh it. Explicit game entry
  also publishes a new entry snapshot, without resetting that sampling clock.
- An entry/sample is visible through `recorded_at + 900s`, including equality.
  Prefer a valid entry snapshot to a valid sample, matching legacy prepend versus
  append order. List/count observation discards expired and duplicate snapshots,
  including a newer sample hidden by a still-valid entry. Keep the sampling clock
  independently: discarding its snapshot must not permit early resampling. One
  latest entry and one latest sample per character bound the stored snapshots.
  Display fields come from the chosen snapshot, not a refreshed profile. Entry
  rows sort newest first, followed by samples oldest first; publication order
  resolves equal timestamps.
- Game entry, commands and unchanged game-screen refresh participate at their
  reconciled save boundaries. Save-before-rejection events count; auth failures,
  rolled-back saves, public/owned GET polling, account login/token management,
  incoming transfers and background settlement do not. Explicit owned commands
  replace legacy saved screen refreshes; GET never renews presence or retention.
- Session and PAT gameplay follow the same rule. Logout/token expiry does not
  remove a valid snapshot. Sleeping/dead characters remain eligible. Deleted
  characters and banned accounts are excluded immediately; unban alone records
  no activity. No record means offline; an inactivity rollout baseline must not
  create public presence.
- Use UTC epoch seconds at recording and observation boundaries, monotonically
  advancing stored clocks under concurrency. List and count use one observation
  instant and the same distinct-character predicate. Home consumes this count
  under its [sleep duration, acquisition and failure contract](home.md#5-resting--sleeping-sleepcgi-homecgi),
  without redefining presence.

**Message and safety.** Default to `""`; an explicit empty string clears it. Besides entry, Home item 058 sets it inside its own transaction without publishing a snapshot ([Home dispatch](home.md#legacy-dispatch-and-consumption-contract)).
Accept 0–60 Unicode code points, preserving spaces, punctuation and ordinary
combining marks without trimming, normalization or HTML-entity storage. A combined
glyph/emoji may consume multiple code points. Reject invalid UTF-8/unpaired JSON
surrogates, C0/C1 controls (including CR/LF/tab), Unicode line/paragraph separators,
and the existing shared checks for invisible formatting, bidi overrides and
excessive combining marks. Do not silently repair invalid input. Permitting
spaces/legacy forbidden punctuation and retaining safe combining marks, while
rejecting rather than stripping newlines, is the explicitly approved departure
in #1104; its implementation prioritizes safe storage and output.

Validate at the service boundary, use SQL
parameters, serialize with the shared JSON writer, and render through escaped
text nodes/framework text binding. HTML, script, attribute and URL contexts need
their own safe encoders/validators; a single universal escaping function is not
a security boundary. Recheck stored public text and image URL safety before
output; malformed data fails the observation instead of leaking unsafe/partial
content. Never interpolate user text into executable markup or SQL.

**Persistence and delivery.** MariaDB owns the character's message and sampling
clock (`characters`) and at most one entry and one sample snapshot per character
(`character_presence_snapshots`); Valkey is unnecessary. A gameplay save, its retention event and any due sample commit together, as do entry message changes and entry publication. Publication copies
display fields; an avatar outside the [profile avatar contract](character-customization.md#51-profile-attributes-character_profiles)
is published as an empty icon rather than blocking presence. Observation rechecks
each stored public field and fails as a whole on unsafe data. Initial deployment
publishes no fabricated activity; restart preserves eligible records. Character
deletion removes these records. The [API boundary](../architecture/client-agent-api.md#public-presence-and-message-boundary)
defines the planned whitelist and owned update path; the
[API/save-boundary integration](https://github.com/witchcraze/party2re/issues/1804)
owns runtime delivery and save-boundary wiring.

---

### 2.13 External Character and Home Cards (Adopted, Not Implemented)

The decision in [#1108](https://github.com/witchcraze/party2re/issues/1108)
retains both external cards, including wallet/experience, last game update and
Home visitors. Public share pages with social-preview metadata replace legacy
`document.write`; the same cards remain embeddable in blogs through iframes.
The [delivery contract](../architecture/client-agent-api.md#external-share-and-embed-boundary)
owns transport and rendering. Existing public profile GET behavior is unchanged.

| Card | Public facts |
| --- | --- |
| Character | Character ID/name, gender, level, job ID/name, safe icon, equipped weapon/armor/carried item catalog names, base attack/defense/agility, SP, maximum HP/MP, experience and wallet gold. Equipment bonuses are not added to the base stats. |
| Home | Owner ID/name, safe background, and the first **at most three local members** in the Home roster's order: resident NPCs/pets and visiting characters, with display name, validated color and safe icon. This is not the pet-only list or the global online list. |
| Both | Current saved login message (empty uses the game title), online indicator, and last successful game-save date/time when offline. No profile comment substitution. |

Legacy `my.cgi:14–65` and `my2.cgi:14–61` dispatch directly to `run`, with no
action table; `player.cgi:41–50` supplies both snippets and a referral link.
`my.cgi:28–51` reads raw character stats, separately from equipment names.
`my2.cgi:27–51` chooses one taught phrase uniformly, independently of a speaker
or whether any member remains, and reads the first three stored local members
without renewing/pruning their presence. With no taught phrase, omit the phrase;
do not fabricate a greeting or require a pet. Local visitor lifecycle comes from
`lib/system.cgi:475–533,964–985` and Home's owning observation contract, not
from card requests. Existing `TalkToCompanion` has different speaker/pet gates.

Cards deliberately retain a different online test from the §2.12 list:
`now < sampled_save_at + 900s`; equality is offline, and a forced entry snapshot
alone does not extend this clock. Read the clock even if list pruning removed
its snapshot; no sample means offline. Message and card facts are current saved
values, not the chosen list snapshot. Offline last-update presentation uses the
successful gameplay-save clock (legacy `ldate`, `system.cgi:29–46`), not account
login, profile edits or the sampled clock. Do not disclose any other activity
timestamps or invent historical dates during rollout; show an unavailable date
when no actual save is known. The 900-second reference window is shared with §2.12; account/session validity is never an online signal.

Character and Home delivery remain separate work in [#1819](https://github.com/witchcraze/party2re/issues/1819)
and [#1820](https://github.com/witchcraze/party2re/issues/1820). The latter needs
the missing local-visitor source owned by Home's migration scope; the pet-only
adapter is insufficient.

These are intentionally public gameplay facts, including visitors and wallet
gold, not permission to serialize Character/Profile/Home aggregates. Exclude
Player IDs/usernames, IP/host, tokens, credentials, private biography, letters,
notices, inventory contents, bank balances and timers. Apply §2.12's shared text
and image-output safety without changing existing message input policy.
