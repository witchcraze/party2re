# Town Park & Public Bulletin Board Design

## Overview

The Town Park Feature Module (`internal/park`) provides a central social gathering space (`交流広場`) where player characters can post public messages on a town bulletin board, interact with town NPCs, and receive daily fortune divinations.

---

## Domain Rules & Features

### 1. Message Board Posting (`park_posts`)

Players can submit public bulletin board messages:
- **Author Identity**: Each post is bound to a verified character owned by the authenticated player session.
- **Content Validation & Sanitization**:
  - Length constraint: 1 to 200 characters (UTF-8 runes).
  - HTML escaping: All HTML tags and special characters (`<`, `>`, `&`, `"`, `'`) are escaped (`SanitizeContent`).
- **Color & Recipient Styling**:
  - Optional custom font color (e.g. HEX `#ff0000`, max 16 characters). Default: `#000000`.
  - Optional target recipient name (max 64 characters).
- **Rate Limiting**:
  - Rapid message spamming is prevented by a rate-limit interval (minimum 3 seconds between posts per character).

### 2. Message Board Feed & Pagination

- Recent messages are ordered in reverse chronological order (`created_at DESC`).
- Supports limit and offset pagination (default limit: 20, max: 100).

---

## NPC Interactions (@町娘)

The Town Park features the resident Town Girl NPC (`@町娘`) who provides multiple social activities:

### 1. Talk (`はなす`)
The NPC provides conversational dialogs acknowledging the character name and dynamically referencing their current class/job (e.g. "職業は勇者ですね？").

### 2. Divination (`うらない`)
The NPC conducts daily fortune-telling, drawing from 20 fortunes and 27 lucky colors:
- **Fortunes (運勢)**:
  - 大吉, 吉, 中吉, 末吉, 小吉, 凶, 大凶, ハッピー, アンハッピー, オッパピー, 残念, 頑張って, 愛があります, 開き直ってください, 何か起きます
- **Lucky Colors (ラッキーカラー)**:
  - 黒, 白, 青, 赤, 空, ピンク, 紫, 緑, 灰, ブルー, 水, 肌, オレンジ, 黄, 茶, ワインレッド, 猫, 海, 土, 森, 藍, 杏子, イチゴ, オリーブ, 金, 銀, パール

### 3. Inspect (`しらべる`)
Interacting with the environment / NPC triggers playful inspection responses (e.g., looking for glasses).

---

## Persistence

Data is persisted in MariaDB via `park_posts`:
- Primary Key: UUID `id` (CHAR 32)
- Foreign Key: `character_id` referencing `characters(id)` with `ON DELETE CASCADE`.
- Indexes: `(created_at DESC)` and `(character_id, created_at DESC)`.
