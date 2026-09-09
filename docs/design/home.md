# Player Private Home & Mailbox Design

## Overview

The Player Private Home Feature Module (`internal/home`) manages character private estates (`home.cgi`), personal customization, player-to-player letter correspondence (mailbox/inbox/outbox), companion greeting customization (`ことばをおしえる`), delivery notice ledgers, and character resting/sleeping (`sleep.cgi`, `home.cgi`).

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
  - `content`: 1 to 1,000 characters.
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

### 3. Companion Greeting Phrases (`character_companion_phrases`)

The resident home companion/pet can be trained with customized greetings:
- **Teaching Phrases (`ことばをおしえる`)**:
  - Up to 30 unique phrases per companion (original CGI specification).
  - Length: 1 to 120 characters per phrase (original CGI specification).
- **Forgetting Phrases (`ことばをわすれさせる`)**:
  - Owner can remove individual phrases by ID.
- **Talking (`＠はなす`)**:
  - Interacting with the companion randomly picks one of the taught phrases.
  - If no phrases are taught, a cute default greeting is returned.

### 4. Remote Depot & Inventory Item Usage (`＠つかう`)

Adventurers can inspect and consume items directly from their home interface:
- **Weapon & Armor Inspection**:
  - Weapons: Displays power and gold price (`武器名：%s / 強さ：%d / 価格：%dG`).
  - Armors/Shields/Accessories: Displays defense and gold price (`防具名：%s / 強さ：%d / 価格：%dG`).
- **Consumable Usage**:
  - Stat-boosting seeds (MaxHP, MaxMP, Attack, Defense, Agility, SP, Small Medals).
  - Recovery items (薬草, 上薬草, 特薬草, 世界樹のしずく, 魔法の聖水, 祈りの指輪, エルフの飲み薬).
  - Deducts 1 item instance from inventory or remote depot storage.

### 5. Delivery Notices (`character_delivery_notices`)

Persistent ledger for incoming transfer events:
- Logs item deliveries, bank remittances, and gift notifications.
- Supports retrieval of uncleared notices and bulk clearing upon viewing.

### 6. Resting & Sleeping (`sleep.cgi`, `home.cgi`)

True Party2 character recovery is conducted at Home (either one's own or a visited player's house):
- **Free Recovery**: No monetary fee (deprecates fictional paid Inn).
- **Concurrency Scaling**: Sleep duration scales by online concurrent player count:
  - `< 20` players: 1x base duration (60 seconds)
  - `>= 20` players: 2x base duration (120 seconds)
  - `>= 30` players: 3x base duration (180 seconds)
- **Action Locking**: While sleeping, `party2:timer:sleep:<character_id>` locks character actions with HTTP `409 Conflict` (`"お休み中「Zzz...」 目覚めるまで X分YY秒"`).
- **Awakening**:
  - HP and MP fully restored.
  - Tiredness (疲労度) reset to 0.
  - Temporary Job Memory reverted.
  - Tavern fullness state reset (`tavern.ResetFullness`).
  - Chapel prayers and active blessings cleared (`chapel.ClearBlessing`).

---

## HTTP REST Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/towns/{town_id}/houses` | Character Session | Build or extend a house in a specified town |
| `GET` | `/towns/{town_id}/houses` | Public | List active houses in a specified town |
| `GET` | `/houses/check` | Public | Check a character's house status by query `?target=...` |
| `POST` | `/characters/{id}/color` | Owner Session | Update character font color (`#RRGGBB`) |
| `GET` | `/characters/{id}/home/items` | Owner Session | List inspectable and usable items in inventory & depot |
| `POST` | `/characters/{id}/home/items/use` | Owner Session | Inspect equipment or consume seeds/herbs from home |
| `GET` | `/homes/{id}` | Optional | Get aggregated home view for character `id` (with optional `?visitor_id=...`) |
| `POST` | `/homes/{id}/settings` | Owner Session | Update home settings (companion name) |
| `POST` | `/homes/{id}/companion/phrases` | Owner Session | Teach a new greeting phrase to the home companion (max 120 chars) |
| `DELETE` | `/homes/{id}/companion/phrases/{phrase_id}` | Owner Session | Forget a taught companion phrase |
| `GET` | `/homes/{id}/companion/talk` | Public | Talk to the home companion to hear a random greeting |
| `GET` | `/homes/{id}/notices` | Owner Session | List delivery notices for character |
| `POST` | `/homes/{id}/notices/clear` | Owner Session | Clear/acknowledge all delivery notices |
| `POST` | `/characters/{id}/home/sleep` | Owner Session | Start sleeping at home (or target player's home) |
| `GET` | `/characters/{id}/home/sleep` | Owner Session | Check current sleep timer and status |
| `POST` | `/characters/{id}/home/wake` | Owner Session | Wake up with full HP/MP/tired recovery and reset hooks |
| `POST` | `/letters` | Sender Session | Send a new letter to a recipient character |
| `GET` | `/letters/inbox` | Recipient Session | List received letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/outbox` | Sender Session | List sent letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/unread-count` | Recipient Session | Get unread letter count for character |
| `POST` | `/letters/{id}/read` | Recipient Session | Mark a letter as read |
| `DELETE` | `/letters/{id}` | Sender or Recipient Session | Delete a letter from sender's outbox or recipient's inbox |

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
