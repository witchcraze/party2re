# Player Private Home & Mailbox Design

## Overview

The Player Private Home Feature Module (`internal/home`) manages character private estates (`home.cgi`), personal customization, player-to-player letter correspondence (mailbox/inbox/outbox), companion greeting customization (`ことばをおしえる`), and delivery notice ledgers.

---

## Domain Rules & Features

### 1. Home Estate & Customization (`character_homes`)

Each character possesses a private home estate accessible by the owner and visiting adventurers:
- **Customizable Attributes**:
  - `theme`: Background style or HEX color code (e.g. `#123456`, max 16 characters). Default: `#ffffff`.
  - `motto`: Welcome greeting or owner statement (max 255 characters).
  - `companion_name`: Name of the resident house companion/pet (max 64 characters, default: `ペット`).
- **Visitor Tracking**:
  - Viewing another character's home automatically increments `visitor_count` and records `last_visited_at`.
  - Owner visits do not increment visitor counts.

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
  - **Independent Deletion Semantics**: The sender and recipient maintain independent deletion flags (`is_deleted_by_sender` and `is_deleted_by_recipient`). Deletion of a letter by one party (e.g., sender deleting sent history or recipient clearing inbox) does not remove the letter from the other party's view or alter the recipient's unread counter.
  - **Physical Purge Lifecycle**: When both the sender and recipient have deleted the letter, the database record is physically removed.

### 3. Companion Greeting Phrases (`character_companion_phrases`)

The resident home companion/pet can be trained with customized greetings:
- **Teaching Phrases (`ことばをおしえる`)**:
  - Up to 10 unique phrases per companion.
  - Length: 1 to 200 characters per phrase.
- **Forgetting Phrases (`ことばをわすれさせる`)**:
  - Owner can remove individual phrases by ID.
- **Talking (`＠はなす`)**:
  - Interacting with the companion randomly picks one of the taught phrases.
  - If no phrases are taught, a cute default greeting is returned.

### 4. Delivery Notices (`character_delivery_notices`)

Persistent ledger for incoming transfer events:
- Logs item deliveries, bank remittances, and gift notifications.
- Supports retrieval of uncleared notices and bulk clearing upon viewing.

---

## HTTP REST Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `GET` | `/homes/{id}` | Optional | Get aggregated home view for character `id` (with optional `?visitor_id=...`) |
| `POST` | `/homes/{id}/settings` | Owner Session | Update home customization settings (theme, motto, companion name) |
| `POST` | `/homes/{id}/companion/phrases` | Owner Session | Teach a new greeting phrase to the home companion |
| `DELETE` | `/homes/{id}/companion/phrases/{phrase_id}` | Owner Session | Forget a taught companion phrase |
| `GET` | `/homes/{id}/companion/talk` | Public | Talk to the home companion to hear a random greeting |
| `GET` | `/homes/{id}/notices` | Owner Session | List delivery notices for character |
| `POST` | `/homes/{id}/notices/clear` | Owner Session | Clear/acknowledge all delivery notices |
| `POST` | `/letters` | Sender Session | Send a new letter to a recipient character |
| `GET` | `/letters/inbox` | Recipient Session | List received letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/outbox` | Sender Session | List sent letters (`?character_id=...&limit=...&offset=...`) |
| `GET` | `/letters/unread-count` | Recipient Session | Get unread letter count for character |
| `POST` | `/letters/{id}/read` | Recipient Session | Mark a letter as read |
| `DELETE` | `/letters/{id}` | Sender or Recipient Session | Delete a letter from sender's outbox or recipient's inbox |

---

## Persistence

Data is persisted in MariaDB via `migrations/034_player_home_and_mailbox.sql` and `migrations/036_player_mailbox_independent_deletion.sql`:
- `character_homes`: (character_id PRIMARY KEY, theme, motto, companion_name, visitor_count, last_visited_at, updated_at)
- `character_letters`: (id PRIMARY KEY, sender_character_id, sender_name, recipient_character_id, recipient_name, content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at)
- `character_companion_phrases`: (id PRIMARY KEY, character_id, phrase, created_at)
- `character_delivery_notices`: (id PRIMARY KEY, character_id, notice_type, message, is_cleared, created_at)
