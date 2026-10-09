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
row; observation never creates that row. Viewing settings is distinct from the
active lease required by the existing other-home Sleep service. Lease/formula
parity is outside this observation slice.

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
- **House Check (`家チェック`)**:
  - Players can inspect any character's house status by name to view location, style, and remaining lease duration formatted in JST (`Y年M月D日 H時M分`).
- **Guild Points Bonus**:
  - Upon successful house construction, the character's guild is awarded `CycleDays * 10` Guild Points (`50 GP` for `town1`, `100 GP` for `town2`, `150 GP` for `town3`, `200 GP` for `town4`) via the wired `GuildPointsRegistrar` as a best-effort bonus. Characters not affiliated with a guild can construct houses normally without awarding points. If construction aborts (e.g., insufficient funds, capacity reached, or already owns a house), no guild points are awarded.
- **Player Customization**:
  - `companion_name`: Name of the resident house companion/pet (max 64 characters, default: `ペット`).
  - Character chat/display font color (`characters.color` / `＠からー`): HEX `#RRGGBB` format, default `#ffffff`.
  - Note: Fictional wallpaper `theme`, `motto`, and `visitor_count` have been purged in favor of character color and town estate tracking.

### 2. Player-to-Player Letter Mailbox (`character_letters`)

Asynchronous direct messaging between characters:
- **Sender & Recipient Verification**:
  - Letters are addressed from a sender character to a recipient character.
  - Sending to self is rejected (`ErrCannotSendToSelf`).
- **Letter Attributes**:
  - `content`: 1 to 1,000 characters. Sanitized via `internal/validation.ValidateMultiLine`: Unicode NFC normalized, permits newlines (`\n`, `\r`), and rejects C0/C1 control characters, zero-width characters (`\u200B`, `\u200C`, `\u200D`, `\uFEFF`), bidirectional text overrides (`\u202A`–`\u202E`, `\u2066`–`\u2069`), and excessive combining diacritics (Zalgo).
  - `color`: Custom font color HEX code (default: `#000000`).
  - `is_read`: Boolean status with timestamp `read_at`.
- **Folder Navigation**:
  - `inbox`: Received letters for recipient character.
  - `outbox`: Sent letters for sender character.
  - `unread_count`: Real-time query for unread received letters.
- **Authorization & Retention**:
  - Only the recipient may mark a letter as read.
  - **Independent Deletion Semantics**: The sender and recipient maintain independent deletion flags (`is_deleted_by_sender` and `is_deleted_by_recipient`). Deletion of a letter by one party does not remove the letter from the other party's view or alter the recipient's unread counter.
  - **Physical Purge Lifecycle**: When both the sender and recipient have deleted the letter, the database record is physically removed.
  - **Transactional Concurrency Guarantee**: Concurrent deletion by sender and recipient is serialized within `RunInTx` using pessimistic row-locking (`SELECT ... FOR UPDATE`), eliminating lost updates and ensuring reliable physical purging when both parties delete.

### 3. Companion Greeting Phrases & Resident Companions (`character_companion_phrases`, `home_members`)

The resident home companion/pet pool aggregates both monster pets brought home from the ranch (`internal/monster.LocationHome`) and Heaven Wish NPC companions (Ortega, Cat, Maid) stored in `home_members` (`HomePetReader` adapter):
- **Resident Companion Pool (`ListHomePets`)**:
  - Queries ranch monster pets at home plus Heaven Wish companions stored in `home_members`, returning unified `HomePet` entries exposed in `HomeView.ResidentPets`.
- **Resident Companion Requirement & Character Guard**:
  - `TalkToCompanion` and `ListCompanionPhrases` validate character existence upfront, returning `ErrCharacterNotFound` (HTTP `404 Not Found`) if the character ID does not exist.
  - If a character exists but has 0 pets or companions residing at Home (`ListHomePets`), talking or teaching phrases is rejected with `ErrNoPetsAtHome` (HTTP `422 Unprocessable Entity`), matching legacy CGI behavior ("教える相手がいません" / "しかし、誰もいなかった…").
- **Teaching Phrases (`ことばをおしえる`)**:
  - Up to 30 unique phrases per companion (original CGI specification).
  - Length: 1 to 120 characters per phrase (original CGI specification). Sanitized via `internal/validation.ValidateSingleLine`: Unicode NFC normalized, single-line (newlines rejected), and rejects C0/C1 control characters, zero-width characters (`\u200B`, `\u200C`, `\u200D`, `\uFEFF`), bidirectional text overrides (`\u202A`–`\u202E`, `\u2066`–`\u2069`), and excessive combining diacritics (Zalgo).
  - Owner can remove individual phrases by ID.
- **Talking (`＠はなす`)**:
  - Randomly selects one speaker pet from resident home pets and picks one of the taught phrases.
  - Returns `CompanionTalkResult` containing `PetName` (display name of the chosen pet) and `Phrase` (selected phrase, empty string if none taught).
  - The HTTP transport layer formats presentation dialogue: if `Phrase` is empty, fallback greeting (`"クエッ？（何か言いたそうにこちらを見つめている）"`) is presented in `companionTalkResponse.Dialogue`.

### 4. Remote Depot & Inventory Item Usage (`＠つかう`)

Adventurers can inspect equipment and consume location-2 items directly from their home interface:
- **Weapon & Armor Inspection**:
  - Weapons: Displays authentic power (attack), weight, and gold price (`武器名：%s / 強さ：%d / 重さ：%d / 価格：%dG`) matching legacy `@weas` (`party2/lib/_data.cgi:394-500`, `party2/lib/home.cgi:274-277`).
  - Armors/Shields/Accessories: Displays authentic defense, weight, and gold price (`防具名：%s / 強さ：%d / 重さ：%d / 価格：%dG`) matching legacy `@arms` (`party2/lib/_data.cgi:506-613`, `party2/lib/home.cgi:278-281`).
  - Stats are resolved directly from the item definition or authentic catalog nominal stats (deprecating obsolete `price / 20 + 1` and `price / 10 + 1` placeholders).
- **Consumable Usage**:
  - Stat-boosting seeds:
    - `命の木の実`, `不思議な木の実`: Increase MaxHP / MaxMP by 3–6 without restoring current HP / MP. If character has `OverLevel == true`, the stat gain is clamped to 0.
    - `力の種`, `守りの種`, `素早さの種`: Increase respective stats by 1–6. If character has `OverLevel == true`, the stat gain is clamped to 0.
    - `スキルの種`: Increases SP by 1–3 (not clamped by OverLevel).
    - `小さなメダル`: Consumed and increases `SmallMedals` counter by 1.
    - `幸せの種`: Sets experience to `Level * Level * 10` (triggers level up on next adventure: `"次のクエスト時にレベルアップ！"`).
  - Combat recovery items and combat-only items (`UsageCategory == 1`, e.g. `薬草`, `上薬草`, `特薬草`, `霊樹のしずく`, `魔法の聖水`) cannot be consumed at Home and return `ErrCannotUseHere` with authentic message `"%sは戦闘中でしか使えません"` (HTTP `400 Bad Request`). Non-usable/passive items (`UsageCategory == 0` or `3`) return `ErrCannotUseHere` with `"%sはここでは使えません"`. Recovery at home is performed exclusively via Resting & Sleeping (`＠やすむ`).
  - Deducts 1 item instance from inventory or remote depot storage upon successful usage.

### 5. Delivery Notices (`character_delivery_notices`)

Persistent ledger for incoming transfer events:
- Logs item deliveries, bank remittances, and gift notifications.
- Supports retrieval of uncleared notices and explicit bulk acknowledgment.
  Viewing never clears the ledger.

### 6. Resting & Sleeping (`sleep.cgi`, `home.cgi`)

True Party2 character recovery is conducted at Home (either one's own or a visited player's house):
- **Free Recovery**: No monetary fee (deprecates fictional paid Inn).
- **Visiting House Restriction**: When sleeping at another player's house (`target_home_id != character_id`), the host must own an active, unexpired town house (`targetHome.IsActive(now)`). If the host character has never built a house or their lease has expired, the request is rejected with `ErrHouseNotFound` (HTTP `404 Not Found`), matching legacy `home.cgi:1-7` (`"$yhomeという家は見つかりません"`). A character can always sleep in their own private home even without a town estate.
- **Concurrency Scaling**: Sleep duration scales by online concurrent player count:
  - `< 20` players: 1x base duration (60 seconds)
  - `>= 20` players: 2x base duration (120 seconds)
  - `>= 30` players: 3x base duration (180 seconds)
- **Action Locking & HTTP Guard**: While sleeping or under emergency rescue penalty, active timer locks (`timer.CategorySleep` and `timer.CategoryAsleep`) block all character action and state-mutating endpoints across the game—including adventures (`POST /adventures`), combat/bosses/PvP/GvG/dungeons, commerce and crafting (shop buy/sell, accessory store, alchemy synthesis/claim/learn, black market, secret shop, blacksmithing, flea market listings/purchases), town facilities (bank deposits/withdrawals, job changes and memory manipulation, depot storage and transfers, plantation farming, altar offerings, chapel prayers, wishing well exchanges, photo contests, gem store trading, lottery/raffles, event plaza interactions, tavern orders and deliveries, custom skills, guild management, and medal exchanges), Home mutations (home settings updates, sending/reading/deleting letters, teaching/forgetting companion phrases, and clearing delivery notices), and Player Store operations (store construction, listings, withdrawals, purchases, trades, store renaming, wallpaper updates, and interior management)—via HTTP layer action guards (`withAuthenticatedActionCharacter` and `withAuthenticatedActionCharacterAndJSON`), rejecting requests with HTTP `409 Conflict` (`"お休み中「Zzz...」 目覚めるまで X分YY秒"` or `"お休み中「Zzz...」 目を覚ましてください"`). Explicit recovery commands (`POST /characters/{id}/home/wake`) and read-only observation queries (viewing home, listing mailbox letters, querying unread count, viewing phrases, listening to companion talk dialogue, and reading delivery notices) remain exempt from action locking. If sleep-state lookup fails (due to storage error or canceled context), the guard halts execution immediately and writes HTTP `500 Internal Server Error`, ensuring no state-mutating callbacks are invoked on read failures. Any active temporary Job Memory is reverted upon entering sleep.
- **Awakening**:
  - HP and MP fully restored.
  - Tiredness (疲労度) reset to 0.
  - **SQL Recovery Boundary**: Production Wake uses the existing Economy transaction runner to read the current Character under its row lock, restore HP/MP/tiredness, and persist it in one SQL transaction. Currency, bank deposits, progression and other unrelated fields are retained from that locked state, so concurrent committed updates are not overwritten. This transaction commits before the required recovery hooks and Valkey cleanup; those phases retain the partial-outcome contract below.
  - Temporary Job Memory reverted.
  - Tavern fullness state reset (`tavern.ResetFullness`; unreadable fullness state halts Wake and propagates the error without overwriting counters with defaults).
  - Chapel prayers and active blessings cleared (`chapel.ClearBlessing`).
  - Ongoing alchemy synthesis completed (`alchemy.CompleteOngoingSynthesis`).
  - Costume rental reset (`store.ResetCostume`).
  - Daily once-dungeon timer lock (`timer.CategoryDungeonOnce`) released and daily quota (`dungeon_once`) reset.
  - Pending recovery flag (`timer.CategoryAsleep`) released upon successful completion of all required hooks and cleanups.
  - **Hook Failure & Partial-Outcome Semantics**: If any mandatory recovery hook or timer cleanup fails during Wake, the execution halts and returns an error (HTTP 500 / `EXECUTION_FAILED` in Gateway actions). The pending recovery flag (`timer.CategoryAsleep`) is NOT released, keeping the character in pending recovery and blocking ordinary actions. Any effects applied before the point of failure (such as restored HP/MP/tiredness or earlier hooks) remain intact without cross-store rollback. Clients must observe actual state via GET queries without assuming complete rollback or automatic retry. Subsequent explicit Wake calls can complete remaining hooks and finalize recovery once transient errors are cleared.

---

## HTTP REST Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/towns/{town_id}/houses` | Character Session | Build or extend a house in a specified town |
| `GET` | `/towns/{town_id}/houses` | Public | List active houses in a specified town |
| `GET` | `/houses/check` | Public | Check a character's house status by query `?target=...` |
| `POST` | `/characters/{id}/color` | Owner Session | Update character font color (`#RRGGBB`) |
| `POST` | `/characters/{id}/home/sleep` | Owner Session | Start sleeping at home (or visited player's active town house) |
| `GET` | `/characters/{id}/home/sleep` | Owner Session | Check current sleep timer and status |
| `GET` | `/characters/{id}/home/items` | Owner Session | List inspectable and usable items in inventory & depot |
| `POST` | `/characters/{id}/home/items/use` | Owner Session | Inspect equipment or consume seeds/medals/fatigue items from home |
| `GET` | `/homes/{id}` | Public / Viewer Session | Get public projection; `?visitor_id=...` requires its owner session and service verification |
| `POST` | `/homes/{id}/settings` | Owner Action Session | Update home settings (companion name) |
| `POST` | `/homes/{id}/companion/phrases` | Owner Action Session | Teach a new greeting phrase to the home companion (max 120 chars) |
| `DELETE` | `/homes/{id}/companion/phrases/{phrase_id}` | Owner Action Session | Forget a taught companion phrase |
| `GET` | `/homes/{id}/companion/talk` | Public | Talk to the home companion to hear a random greeting |
| `GET` | `/homes/{id}/notices` | Owner Session | List delivery notices for character |
| `POST` | `/homes/{id}/notices/clear` | Owner Action Session | Clear/acknowledge all delivery notices |
| `POST` | `/characters/{id}/home/wake` | Owner Session | Wake up with full HP/MP/tired recovery and reset hooks |
| `POST` | `/letters` | Sender Action Session | Send a new letter to a recipient character |
| `GET` | `/letters/inbox` | Recipient Session | List received letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/outbox` | Sender Session | List sent letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/unread-count` | Recipient Session | Get unread letter count for character |
| `POST` | `/letters/{id}/read` | Recipient Action Session | Mark a letter as read |
| `DELETE` | `/letters/{id}` | Sender or Recipient Action Session | Delete a letter from sender's outbox or recipient's inbox |

---

## Persistence

Data is persisted in MariaDB via `migrations/034_player_home_and_mailbox.sql`, `migrations/036_player_mailbox_independent_deletion.sql`, `migrations/062_character_tired.sql`, and `migrations/063_home_estate_parity.sql`:
- `character_homes`: (character_id PRIMARY KEY, town_id, house_style, expires_at, companion_name, bgimg, updated_at)
- `character_letters`: (id PRIMARY KEY, sender_character_id, sender_name, recipient_character_id, recipient_name, content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at)
- `character_companion_phrases`: (id PRIMARY KEY, character_id, phrase, created_at)
- `character_delivery_notices`: (id PRIMARY KEY, character_id, notice_type, message, is_cleared, created_at)
- `characters.tired`: (INT NOT NULL DEFAULT 0)
- `characters.color`: (VARCHAR(7) NOT NULL DEFAULT '#ffffff')
- Valkey keys:
  - `party2:timer:house:<character_id>`: House duration lease timer
  - `party2:timer:sleep:<character_id>`: Sleep timer
  - `party2:timer:asleep:<character_id>`: Sleep state indicator
