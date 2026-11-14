# News & Player Notifications Design

## Overview

The Notification Feature Module (`internal/notification`) provides a system-wide news announcement broadcast catalog and an individual player notification inbox for asynchronous alerts (such as system updates, auction outcomes, guild activities, and gameplay rewards).

---

## Domain Rules & Concepts

### 1. News Announcements (`news_articles`)

Global server news and milestone broadcasts (`news.cgi`):
- **Categories**:
  - `announcement`: General server notices and admin announcements.
  - `update`: Game version releases and feature patch notes.
  - `maintenance`: Scheduled server maintenance windows.
  - `event`: In-game seasonal festivals and community events.
  - `milestone`: Major player milestones (e.g. all jobs mastered, encyclopedia completed, boss sealed).
  - `character`: Character lifecycle announcements, including joining and renaming.
- **Validation**:
  - `title`: Non-empty, maximum 200 characters.
  - `content`: Non-empty, maximum 10,000 characters.
  - `author`: Maximum 100 characters (default: "System").
- **Chronological Feed & Pagination**:
  - News entries are listed in reverse chronological order (`published_at DESC, created_at DESC`).
  - Supports limit and offset pagination (default: 20, max: 100).

#### Character joining news

Character creation publishes one joining article through the injected `character.NewsPublisher`, after saving the character, initial depot and any eligible [referral delivery](player-and-character.md#210-registration-referral-delivery) in the same SQL transaction. The publisher must propagate that transaction context without independent commits or external side effects. News becomes visible when the enclosing transaction commits; save/publication failures and outer rollback leave no joining article, referral grant or partially created character.

Joining articles use category `character`, author `System`, and plain-text title/content containing the new character's name and ID, without account identity or legacy HTML. Ordinary character saves and updates do not emit joining news. This is a global feed entry; the character's own birth entry belongs to its [trajectory](player-and-character.md#214-trajectory-軌跡).

### 2. Player Notification Inbox (`player_notifications`)

Personalized asynchronous message delivery to player accounts:
- **Recipient Identity**: Bound to `player_id` (account level) with `ON DELETE CASCADE`.
- **Categories**:
  - `system`: System maintenance notices, welcome gifts, administrative messages.
  - `auction`: Auction item sold notices, outbid alerts, winning bid confirmations.
  - `guild`: Guild invitation, promotion, or transfer alerts.
  - `adventure`: Adventure completion summaries, helper request updates.
  - `gift` / `reward`: Small medal milestone rewards, seasonal gift deliveries.
- **Validation**:
  - `player_id`: Required non-empty string.
  - `title`: Non-empty, maximum 200 characters.
  - `body`: Non-empty, maximum 2,000 characters.
  - `link`: Optional navigation target (e.g., `/auction`, `/medals`).
- **Read State Lifecycle**:
  - Defaults to unread (`is_read = false`, `read_at = null`).
  - Marking as read records timestamp `read_at`.
  - Supports single-item read, bulk mark-all-as-read, and unread count queries.
  - Strict authorization: players can only read or delete their own notifications (403 Forbidden for unauthorized access).
- **Retention & Pruning**:
  - Expired notifications can be pruned based on configurable retention thresholds (e.g. older than 30 days).

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).

## Persistence

News and player inbox records are durable MariaDB state. The [migrations](../../migrations/)
own SQL fields/indexes; the inbox is player-scoped and cascades on account deletion.
