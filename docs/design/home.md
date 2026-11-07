# Player Private Home & Mailbox Design

## Overview

The Player Private Home Feature Module (`internal/home`) manages character private estates (`home.cgi`), personal customization, player-to-player letter correspondence (mailbox/inbox/outbox), companion greeting customization (`ことばをおしえる`), delivery notice ledgers, and character resting/sleeping (`sleep.cgi`, `home.cgi`).

---

## Observation, visibility and navigation

`home` observes the owned actor by default. `scene_select` with
`target_kind:home` and `target_id` observes another character's home; the path
character remains the authenticated viewer and mutation actor. The Home service
checks the viewer's account before private enrichment, including direct service
calls. Public owner identity is limited to ID, name and color; it never returns
the raw Character, wallet, account ID or private notices.

Public facts contain existing home settings and resident pets. Only an exact
actor/target match adds `private` unread-letter, phrase and delivery counts.
Public reads do not fetch any of those private collections. Required home, pet
or owned enrichment failures fail the observation, without a partial response.
The existing settings reader returns defaults for a character without a home
row; observation never creates that row. A town estate lease is distinct from
private Home existence; the [Sleep contract](#6-resting--sleeping-sleepcgi-homecgi)
requires an existing host for other-home Sleep, without requiring a town lease.

Own Home offers `home_inbox` and `home_outbox`, both parented to `home`. They
always read the owned actor's mailbox, even when entered explicitly after a
public Home selection. Each uses the existing offset or timestamp/ID keyset
service, bounded to 100 letters, with explicit next-page inputs. Neither
selecting nor observing reads/clears a letter, acknowledges a notice, or wakes
the character. GET does not save selection or renew its TTL; expired selection
returns town, and a disappeared selected target remains explicitly unavailable.
Home Sleep templates carry the observed target ID; a submitted command retains
that explicit ID even if another client changes selection before submission.
Wake and Rescue retain their existing lifecycle and refresh-recovery contracts.

### Scoped legacy action reconciliation

Paths below are relative to the original `party2/` distribution. This is the
complete active registration in `lib/home.cgi:26–39,73–105`, with call paths
inspected at `138–201,237–265,379–507,591–604`; formulas/assets are not certified.

| Legacy action / call path | Current replacement or deferred owner |
| --- | --- |
| `system.cgi:418–435` `ほーむ` → `homu`, `$m{home}` | `scene_enter:home` / `scene_select:home` → public Home.GetHomeView. Unavailable selection stays explicit instead of silently falling back. |
| Public/own `ねる` → `neru` | Existing `home_sleep` → Home.Sleep, actor separate from explicit target. `home_wake` → Home.Wake replaces implicit `sleep.cgi:18–42` GET recovery, approved in #646/#1012. Sleep formulas unchanged. |
| `あいてむずかん` → `aitemuzukan` | Collection.GetItemCollection / `GET /characters/{id}/collections/items`; selected-owner reference detail remains #949. |
| `もんすたーぶっく` → `monster_book` | Collection.GetMonsterBook / `GET /characters/{id}/collections/monsters`; Home reference navigation remains #949. |
| `じょぶますたー` → `job_master` | Job.GetJobMastery / existing job mastery REST; selected-owner reference visibility remains #949. |
| `ぷろふぃーる` → `profile` | Character.GetProfile / existing profile REST; Home reference navigation remains #949. |
| `ぼうけんのきろく` → `bokennokiroku` | Adventure.GetChronicle / existing adventure chronicle REST; Home reference navigation remains #948/#949. |
| Own-only `つかう` → `thukau` | Home.ListHomeItems / UseHomeItem; item detail selection and mutation migration remain #949. |
| Own-only `てがみをかく` → `tegamiwokaku`, sent log | Home.ListOutbox / ListOutboxByCursor through `home_outbox`; SendLetter remains REST, mutation migration #949. |
| Own-only `てがみをよむ` → `tegamiwoyomu` | Home.ListInbox / ListInboxByCursor through `home_inbox`; ReadLetter / DeleteLetter remain explicit REST, mutation migration #949. |
| Own-only `からー` → `color` | Home.SetCharacterColor / existing color/settings REST; migration #949. |
| Own-only `ことばをおしえる` → `kotobawooshieru` | Home.ListCompanionPhrases / TeachCompanionPhrase; phrase details/mutations remain #949. |
| Own-only `ことばをわすれさせる` → `kotobawowasureru` | Home.ListCompanionPhrases / ForgetCompanionPhrase; phrase details/mutations remain #949. |
| Own-only `かすたむすきる` → `custom_skill` | Existing custom-skill REST; selection/mutation migration #949. |
| Conditional own `いめーじ` → `custom_image` | Existing profile/avatar presentation; Gateway migration #949, production asset mapping #654/#729. |
| `br`; commented avatar/upload registrations | Separator superseded by structured controls; commented actions are not active dispatch. |
| Home `hanasu` (`419–435`), shared system dialogue dispatch | Home.TalkToCompanion REST remains; scene dialogue/presence belongs to #949. Selection never introduces presence or log writes. |
| Own render letter/money/item notices (`45–67`) | Private counts are read-only; explicit ReadLetter / ClearDeliveryNotices remain. Unlike legacy rendering, observation never clears notices, as required by the Gateway read contract. |

### Retained REST and deferred detail ownership

No Home/Estate route is retired by this slice. #949 owns remaining Home command
and collection migration; #950 may remove only individually verified replacements.

| Retained route(s) | Reason / remaining owner |
| --- | --- |
| `GET /homes/{id}` | Public projection or authenticated `visitor_id` projection uses the same Home service. Existing clients must use the whitelisted owner and `private` counters. Full retirement remains #949/#950. |
| `POST /homes/{id}/settings`, `POST /characters/{id}/color` | Companion-name/color mutations are not migrated; #949. |
| `GET/POST /homes/{id}/companion/phrases`, `DELETE /homes/{id}/companion/phrases/{phrase_id}` | Owner-only phrase details/teaching/removal remain REST; #949. |
| `GET /homes/{id}/companion/talk` | Public random dialogue reader is retained; deterministic Home observation does not invoke it; #949. |
| `GET /homes/{id}/notices`, `POST /homes/{id}/notices/clear` | Owner-only full delivery notice ledger/acknowledgment remains REST; Home scene supplies counts only; #949. |
| `POST/GET /towns/{town_id}/houses`, `GET /houses/check` | Estate construction, public town listing and lease check are distinct from Home detail; #949. |
| `GET /characters/{id}/home/items`, `POST /characters/{id}/home/items/use` | Owner-only inventory/depot/equipment item detail and use are deferred; #949. |
| `POST /letters`, `GET /letters/inbox`, `GET /letters/outbox`, `GET /letters/unread-count`, `POST /letters/{id}/read`, `DELETE /letters/{id}` | Existing mailbox offset/keyset compatibility, send/read/delete and standalone count readers remain; #949/#950. |
| `POST/GET /characters/{id}/home/sleep`, `POST /characters/{id}/home/wake` | Existing sleep-status transport and connected Sleep/Wake commands remain; broader route retirement is #949/#950. |

---

## Domain Rules & Features

### 1. Town House Estate & Customization (`character_homes`)

Characters can construct and maintain private town houses across the 4 towns (`town1` to `town4`) adhering to original Party2 CGI specifications:
- **Town Estate Properties**:
  - `town1` (メケメケ村): 500 G, 5-day cycle, max 10 houses, styles `001`–`004`.
  - `town2` (キノコ町): 1,500 G, 10-day cycle, max 10 houses, styles `005`–`012`.
  - `town3` (スライム町): 3,000 G, 15-day cycle, max 10 houses, styles `013`–`020`.
  - `town4` (ガイア国): 5,000 G, 20-day cycle, max 10 houses, styles `021`–`028`.
- **Rules & Constraints**:
  - Global 1-house per character rule (`ErrAlreadyOwnsHouse`).
  - Town capacity limit of 10 houses (`ErrTownMaxHousesReached`).
  - Style must match town allowed range (`ErrInvalidHouseStyle`).
  - Home lease duration is tracked via `expires_at` in MariaDB and dual-leased in Valkey (`party2:timer:house:<character_id>`).
  - Protected action admission: House construction (`POST /towns/{town_id}/houses`) and character color customization (`POST /characters/{id}/color`) enforce the sleep action guard, rejecting active sleep or pending wake recovery with HTTP `409 Conflict`.
- **House Check (`家チェック`)**:
  - Players can inspect any character's house status by name to view location, style, and remaining lease duration formatted in JST (`Y年M月D日 H時M分`).
- **Guild Points Bonus**:
  - Upon successful house construction, the character's guild is awarded `CycleDays * 10` Guild Points (`50 GP` for `town1`, `100 GP` for `town2`, `150 GP` for `town3`, `200 GP` for `town4`) via the wired `GuildPointsRegistrar` within the same transaction. If reading the existing home state, persisting guild points, or saving the house fails, the transaction is rolled back, preserving money, existing house state and settings, and guild points. Characters not affiliated with a guild construct houses normally without awarding points. If construction aborts (e.g., insufficient funds, capacity reached, or already owns a house), no guild points are awarded.
- **Player Customization**:
  - `companion_name`: Name of the resident house companion/pet (max 64 characters, default: `ペット`). Changing it writes only this field and its `updated_at`, so it never alters the house town, style, expiry or wallpaper, even if a house is built concurrently.
  - Character chat/display font color (`characters.color` / `＠からー`): HEX `#RRGGBB` format, default `#ffffff`.
  - Note: Fictional wallpaper `theme`, `motto`, and `visitor_count` have been purged in favor of character color and town estate tracking.

### 2. Player-to-Player Letter Mailbox (`character_letters`)

Asynchronous direct messaging between characters:
- **Sender & Recipient Verification**:
  - Letters are addressed from a sender character to a recipient character.
  - A character may send a letter to itself, as in legacy. It appears in both of that character's boxes, and a guild callout delivers one to its sender.
- **Letter Attributes**:
  - `content`: 1 to N characters (see [Body length](#letter-settings)). Sanitized via `internal/validation.ValidateLegacyComment`: Unicode NFC normalized, removes line breaks, and rejects C0/C1 control characters, zero-width characters (`\u200B`, `\u200C`, `\u200D`, `\uFEFF`), bidirectional text overrides (`\u202A`–`\u202E`, `\u2066`–`\u2069`), and excessive combining diacritics (Zalgo).
  - `color`: Custom font color HEX code (default: `#000000`).
  - `is_read`: Boolean status with timestamp `read_at`.
- **Folder Navigation**:
  - `inbox`: Received letters for recipient character.
  - `outbox`: Sent letters for sender character.
  - `unread_count`: Real-time query for unread received letters.
- **Authorization & Retention**:
  - Only the recipient may mark a letter as read.
  - **Independent Deletion Semantics**: The sender and recipient maintain independent deletion flags (`is_deleted_by_sender` and `is_deleted_by_recipient`). Deletion of a letter by one party does not remove the letter from the other party's view or alter the recipient's unread counter.
  - **Physical Purge Lifecycle**: When both the sender and recipient have deleted the letter, including by eviction from a full box, the database record is physically removed.
  - **Transactional Concurrency Guarantee**: Concurrent deletion by sender and recipient is serialized within `RunInTx` using pessimistic row-locking (`SELECT ... FOR UPDATE`), eliminating lost updates and ensuring reliable physical purging when both parties delete.

<a id="letter-settings"></a>

#### Letter throttling, retention and body length

- **Legacy rule** (`lib/system.cgi:1055-1087`, `lib/home.cgi:379-402`): a letter is rejected when the newest letter in the recipient's box came from the same sender and `latest time + $bad_time > now`. Guild letters use the same routine. A letter from another sender therefore ends the wait. Each box keeps at most `$max_log` entries, and the recipient's box and the sender's log are trimmed separately.
- **Value provenance**: the tracked `config.cgi` and the template `config.cgi_origin` agree on `$bad_time = 30` and `$max_comment = 500`. `$max_log` is 30 in the tracked file and 999 in the template, as explained under [Teaching Phrases](#3-companion-greeting-phrases--resident-companions-character_companion_phrases-home_members). The current live values are unverified.
- **Adopted throttle** (approved in #1126):
  - The check uses sender and recipient character IDs and the newest letter the recipient has not deleted, ordered by `created_at` then `id`. The recipient deleting that letter takes it out of the check, because legacy has no per-letter deletion and the rule reads the visible box. The sender deleting their copy has no effect.
  - The wait is fixed at 30 seconds, compared in whole Unix seconds of the application clock: 29 seconds is rejected and 30 is allowed.
  - Rejection returns `ErrLetterThrottled` (HTTP `429`) and writes nothing. A guild callout stops at that recipient as described in [Guild design](guild.md#broadcast-callouts-よびかける); other guild notices are best-effort and are dropped.
  - Sends lock the sender and recipient character rows in ascending ID order, so concurrent letters cannot pass the check together.
- **Adopted retention** (approved in #1126):
  - Each inbox and outbox keeps the newest N letters. Delivery always succeeds at capacity. The oldest letters leave that box even when unread, through the owner's deletion flag, so the other party's copy remains.
  - N defaults to 30. `PARTY2_HOME_LETTER_LOG_LIMIT` overrides it. Lowering N or upgrading shortens a box on its next send; there is no startup trim.
- **Body length** (`lib/system.cgi:700-725`, approved in #1126): N defaults to 500. `PARTY2_HOME_LETTER_MAX_LENGTH` overrides it, and guild notices must also fit within it.
  - Line breaks are removed, joining the lines.
  - The limit applies to the length after legacy HTML escaping: `&`, `;` and `,` count as 5, `<` and `>` as 4, `"` as 6, and other characters as 1. Letters are stored unescaped, so this affects counting only.
  - Legacy also counts the `＠てがみをかく>宛先` command text. Go receives the recipient as a separate field, so this does not apply.
- **Approved difference** (#1126): legacy silently removes combining marks in several ranges, which also alters accented and Thai text. Go keeps them after NFC normalization and rejects excessive sequences. A kept mark counts as one character.

### 3. Companion Greeting Phrases & Resident Companions (`character_companion_phrases`, `home_members`)

External Home cards use a [separate local-member/phrase contract](player-and-character.md#213-external-character-and-home-cards-adopted-not-implemented):
visitors count toward their three displayed members, and phrase selection has no
speaker/pet gate. `ResidentPets` and `TalkToCompanion` alone do not implement it.

The resident home companion/pet pool aggregates both monster pets brought home from the ranch (`internal/monster.LocationHome`) and Heaven Wish NPC companions (Ortega, Cat, Maid) stored in `home_members` (`HomePetReader` adapter):
- **Resident Companion Pool (`ListHomePets`)**:
  - Queries ranch monster pets at home (`character_monsters` with `location = "home"`) plus Heaven Wish NPC companions stored in `home_members` (`is_npc = true`), returning unified `HomePet` entries exposed in `HomeView.ResidentPets`.
  - Both stores participate in Monster Grandpa ranch operations (`DepositToBox`, `BringToHome`, duplicate-name checks, and the 8-pet Home capacity limit; see [Monster Design](monster.md)). Depositing a `home_members` companion into the ranch box transitions it into `character_monsters` (`location = "box"`), and bringing it back sets `location = "home"` while preserving its ID, name, and icon.
- **Resident Companion Requirement & Character Guard**:
  - `TalkToCompanion` and `ListCompanionPhrases` validate character existence upfront, returning `ErrCharacterNotFound` (HTTP `404 Not Found`) if the character ID does not exist.
  - If a character exists but has 0 pets or companions residing at Home (`ListHomePets`), talking or teaching phrases is rejected with `ErrNoPetsAtHome` (HTTP `422 Unprocessable Entity`), matching legacy CGI behavior ("教える相手がいません" / "しかし、誰もいなかった…").
- **Teaching Phrases (`ことばをおしえる`)**:
  - **Legacy rule** (`lib/home.cgi:443-477`): the phrase list belongs to the character, not to one pet. A new phrase is added at the head and the list keeps at most `$max_log` entries, so the oldest entries fall off and teaching is never rejected for capacity. Duplicates are kept as separate entries.
  - **Capacity provenance**: the legacy `config.cgi` tracked until commit `0713fcc` (2023-03-09) sets `$max_log = 30`. That commit removed it from version control and added the template `config.cgi_origin`, which changes only `max_entry`, `max_login` and `max_log` to `999` among numeric limits, so `999` is treated as a distribution default. The current live value is unverified.
  - **Adopted contract** (approved in #1125):
    - Keep at most N phrases per character, newest first. At capacity, teaching succeeds and removes the oldest entries in the same transaction, serialized per character, so concurrent teaching never exceeds N.
    - N defaults to 30. `PARTY2_HOME_COMPANION_PHRASE_LIMIT` overrides it through the process or Compose environment. An unset, empty, non-integer, overflowing, zero or negative value means 30 and does not stop startup. This is an approved deployment override of the legacy configuration value.
    - Lowering N shortens a character's list on that character's next teach, as the legacy rewrite does; there is no startup trim.
    - Phrase lists are ordered by `created_at` descending, then `id` descending for ties.
    - Removal by ID removes one entry. Legacy removal by text removes only the first match, which is the newest duplicate.
  - Length: 1 to 120 characters per phrase (original CGI specification). Sanitized via `internal/validation.ValidateSingleLine`: Unicode NFC normalized, single-line (newlines rejected), and rejects C0/C1 control characters, zero-width characters (`\u200B`, `\u200C`, `\u200D`, `\uFEFF`), bidirectional text overrides (`\u202A`–`\u202E`, `\u2066`–`\u2069`), and excessive combining diacritics (Zalgo).
  - Owner can remove individual phrases by ID.
- **Talking (`＠はなす`)**:
  - Randomly selects one speaker pet from resident home pets and picks one retained phrase entry uniformly, so duplicates weigh by their entry count (`lib/home.cgi:422-439`).
  - Returns `CompanionTalkResult` containing `PetName` (display name of the chosen pet) and `Phrase` (selected phrase, empty string if none taught).
  - The HTTP transport layer formats presentation dialogue: if `Phrase` is empty, fallback greeting (`"クエッ？（何か言いたそうにこちらを見つめている）"`) is presented in `companionTalkResponse.Dialogue`.

### 4. Remote Depot & Inventory Item Usage (`＠つかう`)

Adventurers can inspect equipment and use supported category-2/4 items from
inventory or Depot. The current Go dispatcher is a whitelist, not complete
coverage of either category. The table below owns the remaining reconciliation.
- **Weapon & Armor Inspection**:
  - Weapons: Displays authentic power (attack), weight, and gold price (`武器名：%s / 強さ：%d / 重さ：%d / 価格：%dG`) matching legacy `@weas` (`party2/lib/_data.cgi:394-500`, `party2/lib/home.cgi:274-277`).
  - Armors/Shields/Accessories: Displays authentic defense, weight, and gold price (`防具名：%s / 強さ：%d / 重さ：%d / 価格：%dG`) matching legacy `@arms` (`party2/lib/_data.cgi:506-613`, `party2/lib/home.cgi:278-281`).
  - Stats are resolved directly from the item definition or authentic catalog nominal stats (deprecating obsolete `price / 20 + 1` and `price / 10 + 1` placeholders).

Home consumption enforces the sleep action guard (HTTP `409 Conflict`). Combat-only
items return `ErrCannotUseHere` with `"%sは戦闘中でしか使えません"` (HTTP `400`);
non-usable/passive items return `"%sはここでは使えません"`. Home recovery uses
[Sleep](#6-resting--sleeping-sleepcgi-homecgi), not combat recovery items.

#### Legacy dispatch and consumption contract

Evidence is relative to `party2/`, revision
`ff97fc549932a146cc945d7edf71b44c868a840e`; the inspected checkout
`3bebb8872f521410396aa090818d573173224ea1` has identical `lib/home.cgi`,
`lib/_data.cgi` and `lib/system.cgi`. [#1124](https://github.com/witchcraze/party2re/issues/1124)
records the decision to retain observable legacy behavior, including the
no-stat consumption of 113/114 and the source-dependent missing-box result.
This is an adopted specification; rows below whose boundary names an open Issue remain unimplemented.

Own Home alone registers `つかう` → `thukau` (`home.cgi:26–39,73–105`).
Without a target it lists held equipment/tool and Depot entries. By name it
prefers held weapon, armor, then tool, then the first matching Depot entry
(`237–340`). Go's explicit source/instance IDs replace that name ambiguity.
Weapon/armor paths inspect without consuming; categories 0/1/3 do not dispatch
callbacks. All **53 category-2 and 3 category-4** entries are accounted for below.

- **H2/H4 — held tool** (`284–292`): call without arguments; a nonempty `mes`
  stops consumption, otherwise clear the held tool. A callback return value is
  ignored. Other changes made before setting `mes` are not undone by legacy.
- **D2 — Depot category 2** (`298–348`): call while scanning the original
  Depot, before removing the selected row. `mes` returns without removing it;
  otherwise omit that row and preserve other rows, including self-deliveries.
- **D4 — Depot category 4** (`330–373`): retain the selected row in the first
  rewrite, refresh the full flag, close the file, call without arguments, then
  remove the first remaining row of that item number. There is no post-callback
  `mes` check. This is **not** consume-before-callback or rollback-on-no-effect.
- Depot capacity never blocks this dispatch. `send_item` (`system.cgi:818–835`)
  appends each reward directly to an existing Depot even when full, without
  inventory routing, collection registration or a self-delivery notice.
  `get_depot_c` (`1267–1291`) counts capacity; it does not reject delivery.
  Future Go implementations must retain these gameplay outcomes within the
  project's transactions and error handling, without reproducing unsafe file I/O.
- In Go a callback either rolls back with `no_effect` (nothing changes), commits
  and consumes one item (`consumed`), or, where legacy sets `mes` after mutating
  (237), commits its mutations, keeps the item and reports `no_effect` with
  `consumed=false`. Storage failures roll back all of them. Appearance changes
  (costumes, 058) live in Valkey: they are written under the item transaction's
  character lock, so they follow database commit order, and a failed use undoes
  its write only while it is still the current appearance (a newer change or a
  reset is kept). An appearance-store failure rolls back the use.

| Legacy item IDs / callback (`lib/_data.cgi` lines) | Reachability, effect and non-consumption condition | Current boundary / remaining owner |
| --- | --- | --- |
| 016–020, seeds (`683–737`) | H2/D2; maximum HP/MP +3–6 or attack/defense/speed +1–6; over-level gain 0 still consumes. No current HP/MP recovery; no capacity gate. | Existing Home effect; global stat ceilings remain separate. |
| 021, skill seed (`738–747`) | H2/D2; SP +1–3, including over-level; consumes. | Existing Home effect. |
| 022, happy seed (`748–756`) | H2/D2; EXP becomes level²×10; consumes without immediate level-up. | Existing Home/progression effect. |
| 023, small medal (`757–765`) | H2/D2; medal counter +1; consumes. | Existing Home effect. |
| 043, transformation staff (`789–799`) | H2/D2; uniform monster number 001–108, fallback appearance if its image is absent; consumes. | Missing; [APPEARANCE #1837](https://github.com/witchcraze/party2re/issues/1837). |
| 044–056; 138–140, costumes (`800–919;1543–1569`) | H2/D2; fixed appearance, gender branches for 046/055/056/140; consumes. Sleep/job change resets it; callback sets no midnight expiry. | Existing Home/Store adapter; APPEARANCE owns expiry/fallback parity. Original assets are excluded. |
| 057, fatigue elixir (`920–928`) | H2/D2; fatigue becomes 0, even if already 0; consumes. | Existing Home effect, including its current generic-name alias. |
| 058, book (`929–955`) | H2/D2; gender-specific job-008 appearance, character message and profile personality change; other profile fields survive; public news on successful profile write; consumes. Legacy always has a (possibly empty) profile file, so the write and news are the normal path. | Existing Home effect for both sources: login message and personality/news join the Home transaction; an absent profile is created, profile/storage failures roll back without consuming; appearance uses the costume adapter under the same rule (gender-specific placeholder, IMG-006), so APPEARANCE owns its expiry/reset parity. |
| 059; 262, reins (`956–964;2071–2077`) | H2/D2; enter `god` / `u_god`, respectively; consumes, no capacity gate. | [#1153](https://github.com/witchcraze/party2re/issues/1153) owns Home entry, both sources and durable realm admission. Scene selection alone grants no visit. |
| 060–065, six orbs (`965–1054`) | H2/D2; append `s/r/b/g/y/p`; existing same rune sets `mes` and keeps item. Full Depot irrelevant. | [ORBS #1839](https://github.com/witchcraze/party2re/issues/1839); Altar.OfferOrb already handles inventory, not Depot/Home dispatch. |
| 113/114, bracelets/rings (`1408–1429`) | H4/D4; local returned stats are discarded, so **no permanent HP/MP change; consumes one**. Full Depot irrelevant. | [NO-STAT #1840](https://github.com/witchcraze/party2re/issues/1840). Their separate quest equipment effect is not a Home +100 consumable. |
| 125/126, random bags (`1479–1505`) | H2/D2; one weighted random reward to Depot, including when full; then consume bag. | Existing Home effect for both sources. |
| 127–129, recipe books (`1506–1526`) | H2/D2; learn an eligible base then material; exhausted book still consumes, no Depot capacity gate. | Existing Home→Alchemy adapter; membership/two-stage RNG correction [#1181](https://github.com/witchcraze/party2re/issues/1181). Names in a recipe pool establish no other item's dispatch. |
| 141, appearance scroll (`1570–1580`) | H2/D2; uniform character number 001–040, fallback if image absent; consumes. | Existing adapter; APPEARANCE owns fallback/expiry parity. |
| 199, ancient treasure (`1809–1820`) | H2/D2; 2–5 weighted random rewards, all to Depot even when full; consumes once. | Existing Home effect for both sources. Inclusive loop bounds are significant. |
| 203, sacred mana (`1824`) | H2/D2; SP +200, including over-level; consumes once. | Missing; [MANA #1835](https://github.com/witchcraze/party2re/issues/1835). |
| 207, future fragment (`1828–1873`) | H2/D2; save job/stat/gender/over-level snapshot and refresh existing job-point records; reject with `mes` for active ordinary/temporary job memory or exhausted slots (saved count > OverFuture). Rejection keeps item; no Depot capacity gate. | Existing Home→Job adapter for both sources; [Jobs](jobs-and-skills.md) owns the snapshot, slot and SP rules. |
| 231–236;245, wallpapers (`1940–1993;2002–2010`) | H2/D2; replace own background with field 0/1/2/4/5/6/8 respectively; consumes, no gold or town lease prerequisite. | Missing; [WALLPAPER #1836](https://github.com/witchcraze/party2re/issues/1836). Store.BuyHomeWallpaper charges gold and uses a different catalog. |
| 237, veteran reward (`1994`) | H2/D2; below level 99: unchanged, keep item. Otherwise let d=EXP−98010, credit trunc(d×U[1,1.5)), upper-cap gold at 999999, set EXP=98010. d≤0 sets `mes` **but still executes those mutations and keeps item**; d>0 consumes even at gold cap. | Existing Home effect for both sources; gold has no lower clamp, so d<0 may leave it negative (approved retention of the legacy result). A kept item reports `no_effect` with its mutations committed. |
| 257, seal crystal (`2022`) | H2/D2; remove seal/refund half its cost, consume; absent seal sets `mes`, keeps item. | Existing Home effect; [Blacksmith](blacksmith.md) owns seal/refund rules. |
| 259, sealed key (`2024–2068`); 258 is the non-usable box | H4/D4; prefer held box, otherwise first Depot box. With box: remove it, deliver fixed 059 plus 2–4 weighted rewards (3–5 total), then consume key. Without box: H4 keeps key; D4 consumes key. Full Depot accepts every reward. | [KEY #1842](https://github.com/witchcraze/party2re/issues/1842); generic reward overflow/loss policies are not equivalent. |

For 125/126/199/259, each random reward selects a slot **1 through N−1**
of the callback's N-slot list, uniformly; slot 0 is unreachable and repeated
item slots retain their weight. Bag/key reward pools differ; neither a unique-ID
pool nor a uniform choice over distinct items is equivalent. Pool membership/weights
and both reward-count endpoints are verified from these source ranges in the
implementing tests, not duplicated here. Bag/treasure rewards and the bag's
consumption commit in one transaction with the selected source, so a failure or a
duplicate submission neither loses rewards nor consumes a second bag.

Boundary checks for those follow-ups cover both sources, Depot below/at/above
capacity, duplicate matching instances, missing-box behavior, 113/114 unchanged
stats, recipe exhaustion, orb duplication and future-memory rejection. Asset-based appearance/wallpaper follow-ups must use
independent assets and the [asset contract](https://github.com/witchcraze/party2re/issues/654)
and [resolution boundary](https://github.com/witchcraze/party2re/issues/729), not legacy image files.

### 5. Delivery Notices (`character_delivery_notices`)

Persistent ledger for incoming transfer events:
- Logs item deliveries, bank remittances, and gift notifications.
- Supports retrieval of uncleared notices and explicit bulk acknowledgment.
  Viewing never clears the ledger.

### 6. Resting & Sleeping (`sleep.cgi`, `home.cgi`)

True Party2 character recovery is conducted at Home (either one's own or a visited player's house):
- **Free Recovery**: No monetary fee (deprecates fictional paid Inn).
- **Visiting House Restriction**: When sleeping at another player's house (`target_home_id != character_id`), the host character must exist in the system (i.e., be a registered player). A private home is always available regardless of whether the host holds an active town lease. Legacy parity: `lib/home.cgi:1-7` checks for the existence of the user directory (created at character registration), not a paid town lease; `lib/system.cgi:418-435` checks for the private `home.cgi` file, which is also created at registration. If the host character does not exist, the request is rejected with `ErrCharacterNotFound` (HTTP `404 Not Found`). A character can always sleep in their own private home.

- **Adopted duration**: The decision in [#1123](https://github.com/witchcraze/party2re/issues/1123)
  adopts a **300-second (5-minute) default base**, configurable at server startup
  through Docker Compose's `PARTY2_HOME_SLEEP_BASE_SECONDS`. Unset/empty uses 300;
  an explicitly supplied value must be a positive integer number of seconds
  whose 3x duration fits the timer representation, otherwise startup fails.
  `config.cgi_origin:25–26` labels its value `5` in minutes;
  `lib/home.cgi:182–188` converts it to seconds and applies the 20/30 thresholds.
  This adopts the reference configuration, not a recovered active-server value;
  no separate unset-value fallback was found in the legacy source. The earlier
  1-minute examples in #459 do not establish a legacy default.
- **Population and order**: Use the distinct Character count from the
  [public sampled-presence contract](player-and-character.md#212-public-presence-and-login-message),
  including its shared UTC-second observation and read-time cleanup. After
  admission and before any Sleep gameplay save, due presence sample, pending-wake
  write, sleep timer or Job Memory change, obtain one successful count through
  the Character public service. An already-visible actor counts normally; do not
  add the actor or visited host merely because Sleep is requested. This follows
  `lib/home.cgi:182–188` before `party.cgi:20–30`'s final `write_user`
  (`lib/system.cgi:29–46`). Fix the duration for that sleep cycle; later
  population/configuration changes do not recalculate existing timers.

  | Observed characters | Multiplier | Seconds with default base |
  | --- | --- | --- |
  | 0 | 1x | 300 |
  | 19 | 1x | 300 |
  | 20 | 2x | 600 |
  | 29 | 2x | 600 |
  | 30 | 3x | 900 |

- **Count failures**: Missing counter or required query, cleanup, commit or
  context failure aborts sleep before its gameplay effects; never substitute
  zero, a stale cached count, session/PAT counts or all registered characters.
  Successful zero is valid. Legacy `get_login_member`
  (`lib/system.cgi:1387–1417`) stops on list-open failure; the SQL service's
  required observation/cleanup failures use the same stop-before-sleep policy.
  Successful presence pruning remains an observation even if a later Sleep step
  fails. A temporary transition claim may be acquired/released without starting
  sleep; retain the existing claim-loss and compensation guarantees below.
- **Implementation gap**: Go still defaults to 60 seconds, silently uses zero
  when the optional counter is absent/fails, and has no production counter or
  Compose base-duration setting. [Implementation follow-up](https://github.com/witchcraze/party2re/issues/1822)
  owns delivery and production wiring/boundary tests.
- **Action admission**: Protected action boundaries reject active sleep and
  pending waking recovery. Duration expiry alone does not clear the obligation.
  Observation, explicit Wake and emergency Rescue remain available. Failed SQL,
  timer or canceled-context reads halt admission; the Gateway reports
  `ACTION_PREFLIGHT_FAILED` without invoking mutations.
- **Persistent recovery**: Legacy `sleep.cgi` retains `$m{sleep}` until the next
  action. SQL `characters.pending_wake` preserves the equivalent obligation
  beyond timer expiry/restarts. Explicit Wake instead of implicit GET recovery
  is the approved transport departure (#646/#1012).
- **Sleep start**: Write pending recovery before establishing sleep timers and
  reverting temporary Job Memory. Failed timer/job restoration compensates
  owned changes with bounded cleanup even after request cancellation, returning
  cleanup failures as well. Failed SQL compensation can leave a wakeable pending
  obligation; recover through Wake/Rescue after storage recovers.
- **Cycle exclusion**: Sleep and Wake share a renewable ownership claim; another
  active transition returns `HOME_WAKE_IN_PROGRESS`. Existing unawakened sleep
  rejects another Sleep. Token loss stops forward work and stale compensation
  without clearing a successor's timers, quotas, job memory or SQL recovery flag.
  The claim's key/lease mechanics belong to [Valkey lifetimes](../architecture/valkey-keyspace.md).
- **Wake recovery**: Recheck active sleep and pending state under the claim.
  First lock/read the current Character and commit full HP/MP and zero fatigue
  in SQL, preserving unrelated concurrent currency/progression updates. Then
  revert temporary Job Memory and run required recovery hooks in order:
  Tavern fullness reset; Chapel blessing removal (prayer quota clears only when
  its prayer date is an earlier calendar day); ongoing Alchemy completion;
  rented costume removal; once-dungeon timer/quota reset. Unreadable hook state
  must not be replaced with defaults. Finalize pending recovery and asleep
  cleanup only after required steps succeed, rechecking claim/cycle ownership.
- **Partial failure**: Earlier SQL commits/hooks are not rolled back across
  stores. Hook/timer failure retains pending recovery and blocks ordinary
  actions, while returning an error. Observe state through GET; a later explicit
  Wake can finish recovery. The exclusion lease prevents concurrent execution,
  not a cross-store atomic transaction or automatic command replay.

Transport operations and schemas: [OpenAPI](../api/openapi.json).

## Persistence

Home estate, letters, companion phrases and delivery notices are durable
relational records; the [migrations](../../migrations/) own their fields.
Timer keys/expiry belong to [Valkey keyspace](../architecture/valkey-keyspace.md).
