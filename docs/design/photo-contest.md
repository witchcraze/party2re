# Photo Contest, Screenshots & Gallery Design (フォトコン会場・コンテスト)

## 1. Overview

The Photo Contest and Gallery system (`internal/contest`, legacy `photo.cgi` / `contest.cgi` / `screen_shot.cgi`) provides character screenshot capture and storage, entry submissions to periodic photo contests, community voting with comments, automated round conclusion with prize distribution (Gold, Small Medals, Guild Points), voter reward bonuses, and permanent Hall of Fame (殿堂入り / Legends) archiving.

---

## 2. Facilities & NPC

- **Facility Title**: フォトコン会場 (Photo Contest Venue)
- **NPC Name**: `@ワコール`
- **Location Context**: Legacy `lib/photo.cgi` & `contest.cgi`

---

## 3. Storage & Capacity Rules

### 3.1 Photo Storage (スクリーンショット一覧)
- **Maximum Photos per Character**: Up to 20 screenshots (`MaxPhotosPerCharacter = 20`).
- **Deletion**: Players can delete old screenshots to free capacity (`DeletePhoto`).

---

## 4. Contest Lifecycle & Rules

### 4.1 Rounds & Cycles
- **Contest Cycle Duration**: 10 days (`ContestCycleDays = 10`).
- **Minimum Entry Quorum**: 5 entries (`MinEntriesForContest = 5`).
  - If a round ends with fewer than 5 entries, the round is postponed and its duration is extended by 10 days without prize distribution.

### 4.2 Entry Submission Rules (`えんとりー`)
1. **Single Entry per Round**: A character may submit at most 1 entry per contest round.
2. **Consecutive Entry Prohibition (`is_renzoku_entry_contest = 0`)**: A character with an active entry in the currently running voting round cannot submit an entry for the upcoming preparing round until the active round concludes.
3. **Title Constraints**:
   - Length: $1 \le \text{Length} \le 40$ (UTF-8 character count).
   - Prohibited Characters: Whitespace (`\s`, `\u3000`), punctuation/symbols `,`, `;`, `"`, `'`, `&`, `<`, `>`, `@`, `＠`.
4. **Duplicate Title Prevention**: No two entries within the same round may have the exact same title.

### 4.3 Voting Rules (`とうひょう`)
1. **Active Round Only**: Voting is only allowed while a contest round is in `active` status.
2. **1 Vote per Character**: A character may cast at most 1 vote per contest round (enforced via database unique constraint on `(round, voter_character_id)`).
3. **No Self-Voting**: A character cannot vote for their own entry.
4. **Optional Comment**: Voters can include an encouraging comment (up to 100 characters).

### 4.4 Prize Structure & Rewards
When a contest is settled:
- **1st Place**: 15,000 Gold + 10 Small Medals + 700 Guild Points (EXP).
- **2nd Place**: 7,000 Gold + 6 Small Medals + 300 Guild Points (EXP).
- **3rd Place**: 3,000 Gold + 3 Small Medals + 100 Guild Points (EXP).
- **Voter Bonus**: Every voter who voted for the 1st place winner receives 1 Small Medal.
- **Hall of Fame (殿堂入り)**: The 1st place entry is permanently archived into `contest_legends`.
- **System Announcement**: Broadcasts news announcements for top 3 winners.

---

## 5. Supported Operations & HTTP API

| Action | Legacy Command | HTTP Endpoint | Description |
| :--- | :--- | :--- | :--- |
| **Venue Dialogue** | `はなす` | `GET /contest/venue` | Returns `@ワコール` dialogue and overview of contest status. |
| **Current Entries** | `みる` | `GET /contest/current` | Lists entries and vote counts for active round. |
| **Past Results** | `past` | `GET /contest/past` | Returns results and top comments of last settled contest. |
| **Hall of Fame** | `legend` | `GET /contest/legends` | Lists historical round champions. |
| **List Photos** | `みる` | `GET /characters/{id}/photos` | Lists photos owned by the authenticated character. |
| **Save Photo** | `すくしょ` | `POST /characters/{id}/photos` | Captures and saves a new character photo (max 20). |
| **Delete Photo** | `けす` | `DELETE /characters/{id}/photos/{photoId}` | Deletes a photo owned by the character. |
| **Enter Contest** | `えんとりー` | `POST /characters/{id}/contest/enter` | Submits a photo into the next contest round. |
| **Vote** | `とうひょう` | `POST /characters/{id}/contest/vote` | Votes for a contest entry with optional comment. |
| **Settle Contest** | `集計` | `POST /contest/settle` | Admin endpoint to trigger round conclusion and settlement. |

---

## 6. Concurrency & Security Guarantees

- **Transactional Consistency**: All contest entries, votes, settlements, and prize distributions execute within Unit of Work database transactions (`RunInTx`).
- **IDOR Protection**: All `/characters/{id}/*` endpoints verify session token authentication and validate character ownership.
- **Anti-Exploit Controls**: Strict unique keys on `(round, voter_character_id)` prevent double voting under concurrent requests.
