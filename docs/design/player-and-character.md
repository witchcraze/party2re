# Player and Character Specification

## 1. Domain Overview

In Party2, identity and game progression are strictly separated across two domain entities:

- **Player**: The account-level entity responsible for authentication, credentials, and session management.
- **Character**: The in-game persona holding stats, progression, inventory, equipment, jobs, and feature-specific associations.

Every `Character` belongs to exactly one `Player`. A `Player` may own multiple characters over time.

---

## 2. Invariants and Rules

### 2.1 Ownership Invariant
- A `Character` cannot exist without being associated with a valid `Player` (`player_id`).
- When a `Character` is created, its `player_id` must be explicitly specified and must refer to an existing `Player`.
- A `Character`'s ownership cannot be transferred arbitrarily; `player_id` remains immutable throughout ordinary gameplay operations.

### 2.2 Authorization & Access Control
- All player-initiated operations acting upon a `Character` (such as starting an adventure, buying/selling in shops, depositing/withdrawing in depot or bank) require an authenticated session belonging to that character's owning `Player`.
- If an authenticated session attempts to inspect or mutate a `Character` owned by a different `Player`, the request is rejected with `Forbidden` (`403`).

### 2.3 Character Listing & Querying
- A `Player` can query all characters associated with their account (`FindByPlayerID`).
- Each `Character` response contains `id`, `player_id`, `name`, `job_id`, `gender`, `level`, `experience`, `money`, `sp`, and `stats`.

### 2.4 Currency & Wallet Ceiling
- `Character.Money` holds the character's active wallet gold, strictly capped at an authentic maximum ceiling of `999,999 G` (`corecharacter.MaxMoney`, reproducing legacy Party2 `system.cgi:71`).
- All currency-crediting pathways (`Character.AddMoney`, `economy.Service.AddGold`, shops, adventures, depot, battles, courier) respect this ceiling, clamping any overflowed gold to `999,999 G`.
- Wealth beyond 999,999 G must be deposited into the Bank (`Character.Deposit`), which supports large-scale gold savings up to 99兆9999億9999万9999 G (`bank.MaxDeposit`).
- `Character.SmallMedals` is capped at `999,999,999` (`corecharacter.MaxSmallMedals`).

### 2.5 Vitality & Fatigue Invariants
- **Vitality Bounds**: HP and MP are strictly bounded within `[0, MaxHP]` and `[0, MaxMP]` via `Stats.ClampVitality`. Restorative actions (sleeping, job changes) restore full vitality via `Character.RecoverVitality`.
- **Combat Survival Floor**: In combat settlement (`battle/apply.go`, `boss/battle.go`), fallen combatants (`HP <= 0` or fallen flag) survive with an authentic minimum floor of `1 HP` via `Character.ApplyCombatSurvival`. Surviving MP is updated and clamped within `[0, MaxMP]`.
- **Fatigue Range**: Fatigue percentage has no upper or lower bound, matching legacy additions (`lib/_battle.cgi:1211-1213`, `lib/quest.cgi:1073`, `lib/_casino.cgi:217`, `lib/vs_king.cgi:88`) and subtraction (`lib/god.cgi:57`). `AddTired` can exceed `100%`, and `god.WishRefresh` (`ReduceTired(150)`) can go below `0%`, buffering later fatigue. At `100%` or more, exhaustion (`IsExhausted() == true`) blocks adventures and fatigue-generating mini-games. Resting at home clears fatigue to 0 (`ResetTired`).

### 2.6 Player Account Lifecycle & Ban Invariant
- **Account State**: Each `Player` record tracks `created_at`, `updated_at`, `last_ip`, and an optional `banned_at` timestamp.
- **Login Tracking**: Upon successful login, the player's `last_ip` and `updated_at` are refreshed.
- **Soft-Ban Enforcement**: When an account is soft-banned (`banned_at IS NOT NULL` / `IsBanned() == true`), all active Valkey sessions and Personal Access Tokens are immediately invalidated. Subsequent login attempts (`POST /sessions`) and authenticated character requests receive HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).
- **Registration boundary**: Account BAN does not prevent a different account from registering at the same IP. [Optional IP registration denial](admin.md#33-optional-ip-registration-denial-planned), including independent release and explicit BAN-time selection, is adopted but deferred; `last_ip` alone is not a registration prohibition.

Character activity is independent of account login tracking. The adopted
[inactivity retention contract](player-deletion.md#4-inactivity-retention) expires
individual characters, preserves their account and siblings, and requires a
durable gameplay clock before automatic deletion can be enabled.

### 2.7 Request Throttling and Reload Sleep Penalty Reconciliation (Approved Departure)

#### 2.7.1 Legacy Reload Penalty Specification
In original Party2, inter-request intervals and rapid manual reloading were monitored to prevent CGI server overload:
- **Inter-Request Detection (`party2/lib/system.cgi:148–156`)**:
  - During `&read_user`, if the interval since the character's last access was less than `$wait_time` (`config.cgi:88`, reference default `1` second), i.e., `$m{ltime} + $wait_time > $time`, and auto-reload was not flagged (`!$in{is_auto}`):
    - Appended `"1<>"` to `$userdir/$id/reload.cgi` (recording a reload strike).
    - Terminated immediately with an error message: `更新の連打は禁止しています。最低でも $wait_time 秒は待ってください<br>※過度な更新連打は自動削除の対象となります`.
- **Strike Consumption & Character Sleep Penalty (`party2/login.cgi:37–104`)**:
  - Upon subsequent login, if `$userdir/$id/reload.cgi` existed and had non-zero size, its contents were split by `<>` to count strikes (`@lines`), and the file was immediately truncated to 0 bytes (`truncate $fh, 0`), consuming and resetting the strike record.
  - Strikes exceeding thresholds triggered severe character sleep penalties added to existing `$m{sleep}` (`$m{sleep} = $m{sleep} > 0 ? $m{sleep} + $sleep_time * 60 : $sleep_time * 60`):
    - **`> 30` strikes**: `$sleep_time = 7 * 24 * 60` minutes (7 days = 10,080 minutes / 604,800 seconds). In original source comments, account deletion (`&delete_directory`) and IP blacklisting (`&add_black_list`) were drafted but commented out, falling back to 7 days sleep.
    - **`> 25` strikes**: `$sleep_time = 3 * 24 * 60` minutes (3 days = 4,320 minutes / 259,200 seconds). IP blacklist was commented out.
    - **`> 20` strikes**: `$sleep_time = 24 * 60` minutes (24 hours = 1,440 minutes / 86,400 seconds).
    - **`> 15` strikes**: `$sleep_time = 6 * 60` minutes (6 hours = 360 minutes / 21,600 seconds).
    - **`> 10` strikes**: `$sleep_time = 60` minutes (1 hour = 60 minutes / 3,600 seconds).
    - **`<= 10` strikes**: No penalty applied; strikes were wiped on login.

#### 2.7.2 Approved Architectural Departure (#1106)
Legacy reload sleep penalties are **formally deprecated and superseded** by modern HTTP transport rate limiting and administrative soft-ban controls:
1. **Separation of Concerns & Layering Principles**:
   - Request frequency control and DoS mitigation belong exclusively to the HTTP transport/middleware layer (`internal/api/http/middleware.go` via `internal/ratelimit`).
   - Game-state sleep (`timer.CategorySleep`) is strictly reserved for in-game mechanics: HP/MP restoration via home sleep (`internal/home`) and dungeon defeat rescue penalty (`internal/rescue`).
   - Transport-level request bursts must never mutate domain character timers or lock character progression.
2. **Comparison of Throttling vs. Game Sleep**:
   - `HTTP 429 Too Many Requests`: An ephemeral, transport-level backoff signal with `X-RateLimit-*` and `Retry-After: <seconds>` headers. Traffic resumes immediately once the window lapses without altering character state.
   - `Character Sleep Penalty`: An in-game lock lasting hours or days that halts all player/party progression. Applying this to HTTP frequency violations creates severe false positives.
3. **Malicious Abuse Handling**:
   - Extreme, persistent, or malicious abuse is mitigated via administrative soft-bans (`Player.banned_at` / `POST /admin/players/{id}/ban`), revoking tokens and rejecting all requests with `403 Forbidden`, rather than poisoning gameplay sleep timers.

#### 2.7.3 Non-Interference and Testing Requirements
- **SPA & Parallel Request Non-Interference**: Modern browser and mobile clients dispatch multiple parallel asynchronous requests on page loads (e.g. character details, notifications, party status, chat). Sub-second inter-request spacing must not accumulate penalty strikes or restrict valid traffic within configured HTTP quotas (Public 10 req/min, Authenticated 60 req/min).
- **Automated Agents & PAT Non-Interference**: Programmatic clients (AI agents, automated scripts) operating via Personal Access Tokens must receive standard HTTP 429 backoff responses when exceeding quotas, without triggering character sleep locks.
- **Resilience and Separation during Infrastructure Failure**: When Valkey is unavailable, `ValkeyLimiter` falls back to an in-memory `MemoryLimiter`, actively enforcing rate limits in process memory. If an unrecoverable limiter error occurs, HTTP middleware fails open gracefully (`internal/api/http/ratelimit_test.go`) to prevent service disruption. Under no circumstances do rate-limiter infrastructure incidents or rate-limit rejections mutate domain character timers (`timer.CategorySleep`) or impose gameplay sleep penalties.

### 2.8 Registration Population Limit & Capacity Reconciliation (#1105)

#### 2.8.1 Legacy Population Limit Specification
In original Party2, world registration capacity was constrained by a hardcoded configuration ceiling:
- **Configuration Default (`party2/config.cgi_origin:46`)**:
  - `$max_entry = 999;` defined the maximum allowable player population on the server.
- **Capacity Counter Storage & Admission Check (`party2/new_entry.cgi:133–138`)**:
  - Maintained an aggregate flat-file counter in `$logdir/entry.cgi` formatted as `"$entry_count<>$last_addr<>"`.
  - During registration (`new_entry.cgi`), if `$entry_count >= $max_entry`, registration was immediately blocked with the authentic message:
    `"現在定員のため、新規登録は受け付けておりません"`.
- **Atomic Counter Increment (`party2/new_entry.cgi:224–234`)**:
  - On successful registration, `sub plus_entry_count` opened `$logdir/entry.cgi` with exclusive advisory locking (`flock $fh, 2`), read `$entry_count`, incremented it (`++$entry_count`), and rewrote the file.
- **Counter Decrement on Deletion (`party2/delete.cgi:56`, `party2/admin.cgi:115`)**:
  - Self-deletion called `&minus_entry_count(1)` (`delete.cgi:56`).
  - Administrative user deletion called `&minus_entry_count($count)` (`admin.cgi:115`).
- **Exile / Ban Treatment (`party2/lib/exile.cgi:177`)**:
  - In original Party2, `&minus_entry_count` was explicitly commented out in `lib/exile.cgi` (`## &minus_entry_count;`). Consequently, banned/exiled players continued to occupy a registration slot in `$entry_count` until directory cleanup.
- **Recalibration & Inactivity Purge (`party2/login.cgi:190–250`)**:
  - When rebuilding the player ranking and list (`login.cgi`), the system scanned all user directories (`$userdir`), purged inactive accounts exceeding `$auto_delete_day` (or `job_lv <= 0` and level <= 2 inactive for strictly more than 7 days), tallied surviving active user directories (`$count`), and rewrote `$logdir/entry.cgi` as `"$count<><>"`, recalibrating any drift.

#### 2.8.2 Reconciled Architecture & Adoption Policy (#1105)
In Party2re's relational domain model, identity and game progression are separated into distinct `Player` (account) and `Character` (in-game persona) entities. The registration population limit is reconciled with this architecture as follows:

1. **Unit of Population Limit (Account-Level Enforcement)**:
   - The registration capacity ceiling applies strictly to **Player Accounts** (`players` table).
   - The registration gateway `POST /players` (`player.Service.Register`) is the sole enforcement boundary for server population limits.
   - Character creation (`POST /characters`) operates independently of this server-wide limit and neither consumes nor evaluates registration population capacity.

2. **Configuration Source & Unlimited Option**:
   - The limit is governed by the environment variable `PARTY2_MAX_PLAYERS` (configurable via `compose.yaml` / Docker Compose / process environment).
   - **Authentic Legacy Default**: Defaults to `999` to faithfully preserve the original game's reference capacity (`$max_entry = 999`).
   - **Unlimited Capacity Flag**: Setting `PARTY2_MAX_PLAYERS=0` (or negative) disables the population limit entirely, allowing unlimited account registration for modern high-capacity or cloud-native deployments.

3. **Active Population & Soft-Ban Reconciliation**:
   - The population count evaluates **active accounts only**: `SELECT COUNT(*) FROM players WHERE banned_at IS NULL`.
   - **Soft-Ban Slot Recovery**: In contrast to the original CGI's commented-out decrement, soft-banned accounts (`banned_at IS NOT NULL`) do not consume population quota, ensuring that malicious or banned accounts cannot permanently starve legitimate players of registration slots.
   - **Account Deletion Slot Recovery**: Hard deletion of a player account (`DELETE /players/me` or admin `DELETE /players/{id}`) removes the row from `players`, automatically decrementing the active count.

4. **Admission Boundary & Rejection Contract**:
   - **Threshold Condition**: When `PARTY2_MAX_PLAYERS > 0` and the current active account count satisfies `active_count >= max_players`, registration requests are rejected.
   - **HTTP Transport Response**: `POST /players` returns `HTTP 409 Conflict` (domain error `ErrRegistrationLimitReached`, JSON error code `"registration_limit_reached"`) with the authentic legacy message:
     `"現在定員のため、新規登録は受け付けておりません"`.

5. **Concurrency Invariant & Verification Requirements**:
   - **Atomic Admission Under Race Conditions**: Registration transactions must prevent race conditions where concurrent requests observe `active_count == max_players - 1` and simultaneously insert multiple records, exceeding `max_players`.
   - Enforcement must execute within a database transaction boundary (e.g. pessimistic locking on a counter row, advisory lock, or serialized lock check).
   - **Required Stress Verification**: Implementation must include a paired concurrency stress test (`testutil.RunConcurrentStressTest` / `testutil.RunRace`) verifying that when capacity has room for $M$ registrants, exactly $M$ concurrent registrations succeed and all remaining concurrent attempts fail with `409 Conflict`, with final `COUNT(*) <= max_players`.

#### 2.8.3 Implementation & Verification
Runtime enforcement of this specification is implemented across `cmd/party2/config.go`, `internal/player`, and `internal/api/http`, with verified transaction locking and paired concurrency stress tests (`internal/player/registration_concurrency_integration_test.go`, `internal/player/application_test.go`). When active account capacity (`WHERE banned_at IS NULL`) reaches `PARTY2_MAX_PLAYERS`, `POST /players` atomically rejects with `409 Conflict` (`registration_limit_reached`).

### 2.9 Transactional Character Creation & Joining News

Character creation (`POST /characters`, `character.Service.CreateWithOptions`) coordinates persistence through an ambient database transaction (`s.runInTx` via injected `TransactionProvider`):
1. **Atomic Persistence**: The character (`characters`), initial storage (`character_depots`, capacity = 5), eligible [referral delivery](#210-registration-referral-delivery), and [joining news](notifications.md#character-joining-news) share this single transaction boundary. An existing referrer character is locked before saving the new character; its depot is locked before appending the reward.
2. **Rollback Guarantee**: A character, depot, referral item/notice or news persistence error is propagated and rolls back the transaction. Creation is not reported as successful with missing required writes, nor as failed after a separately committed character. A commit error can leave its outcome unknown after a connection failure; clients must not automatically replay a creation POST.
3. **Ambient Transaction Propagation**: When called within an existing ambient transaction, character creation participates in the caller's transaction without premature commit, ensuring full rollback if subsequent enclosing operations fail.

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
  under its [sleep duration, acquisition and failure contract](home.md#6-resting--sleeping-sleepcgi-homecgi),
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

[Shared validation](../../internal/validation/validation.go)'s `ValidateVerbatimLine`
implements this contract over the same per-character safety predicate as the
trimming/normalizing wrappers, whose policies are unchanged; do not duplicate
sanitizers or silently change other text fields. Validate at the service boundary, use SQL
parameters, serialize with the shared JSON writer, and render through escaped
text nodes/framework text binding. HTML, script, attribute and URL contexts need
their own safe encoders/validators; a single universal escaping function is not
a security boundary. Recheck stored public text and image URL safety before
output; malformed data fails the observation instead of leaking unsafe/partial
content. Never interpolate user text into executable markup or SQL.

**Persistence and delivery.** MariaDB owns the character's message and sampling
clock (`characters`) and at most one entry and one sample snapshot per character
(`character_presence_snapshots`); Valkey is unnecessary. The Character
[presence service](../../internal/character/presence.go) records entry and
save-equivalent samples, observes the list/count and reads the current message
and clock for cards. Its writes lock the owning character and join the caller's
transaction, so a gameplay save, its retention event and any due sample commit
together, as do entry message changes and entry publication. Publication copies
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

---

## 3. Data Schema & Persistence

### 3.1 Players Table
- `id VARCHAR(32) PRIMARY KEY`
- `username VARCHAR(64) NOT NULL UNIQUE`
- `password_hash VARCHAR(255) NOT NULL`
- `banned_at DATETIME(6) NULL DEFAULT NULL`
- `created_at DATETIME(6) NOT NULL`
- `updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)`
- `last_ip VARCHAR(45) NOT NULL DEFAULT ''`

### 3.2 Characters Table
- `id VARCHAR(32) PRIMARY KEY`
- `player_id VARCHAR(32) NOT NULL` (Foreign key constraint referencing `players(id)`)
- `name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL` (unique index `uq_characters_name`)
- `job_id VARCHAR(32) NOT NULL`
- `gender VARCHAR(16) NOT NULL`
- `level INT NOT NULL`
- `experience INT NOT NULL`
- `money INT NOT NULL`
- `sp INT NOT NULL`
- `max_hp INT NOT NULL`, `max_mp INT NOT NULL`, `hp INT NOT NULL`, `mp INT NOT NULL`
- `attack INT NOT NULL`, `defense INT NOT NULL`, `agility INT NOT NULL`

---

## 4. State Transitions

```text
       [Register Player]
               |
               v
      Player Account Created
               |
               v
       [Login / Session]
               |
               v
     Authenticated Session
               |
               v
     [Create Character(player_id, name)]
               |
               v
      Character Linked to Player
               |
  +------------+------------+
  |                         |
  v                         v
[Own Player Request]     [Other Player Request]
  |                         |
  v                         v
Allowed (200 / 201)      Rejected (403 Forbidden)
```
