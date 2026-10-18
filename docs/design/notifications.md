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
- **Validation**:
  - `title`: Non-empty, maximum 200 characters.
  - `content`: Non-empty, maximum 10,000 characters.
  - `author`: Maximum 100 characters (default: "System").
- **Chronological Feed & Pagination**:
  - News entries are listed in reverse chronological order (`published_at DESC, created_at DESC`).
  - Supports limit and offset pagination (default: 20, max: 100).

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

