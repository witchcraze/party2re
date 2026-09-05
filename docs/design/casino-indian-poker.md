# Casino & Indian Poker Design

## Overview

The Casino (カジノ) system introduces mini-games and wagering mechanisms to Party2. Indian Poker (インディアンポーカー) is a blind-card bluffing and wagering game played with a standard 52-card deck against the NPC House Dealer.

## Currency & Account Model

### Casino Coins (`casino_accounts`)

- **Currency Exchange Rate**: `1 Casino Coin = 20 Gold` (standard value from reference implementation).
- **Exchange Operations**:
  - `ExchangeGoldToCoins`: Deducts gold from character wallet and credits casino coins.
  - `ExchangeCoinsToGold`: Deducts casino coins and rewards gold to character wallet.
- **Account Invariants**:
  - Coin balances are stored as non-negative integers (`coins >= 0`) per character.

## Indian Poker Rules & Mechanics

### Card Deck & Ranking

- **Deck**: Standard 52-card deck with 4 suits (♠, ♥, ♦, ♣) and 13 ranks (Ace to King).
- **Rank Hierarchy**: King (13, highest) > Queen (12) > Jack (11) > 10 > ... > 2 > Ace (1, lowest).
- **Card Visibility**:
  - Players cannot see their own card ("blind" card held up to forehead).
  - Players can see all opponents' / dealer's cards.
  - The Dealer sees the player's card, but cannot see the dealer's own card.

### Betting Structure

1. **Base Rate & Ante**:
   - The game begins with a chosen base rate ($B \in [1, 5000]$ coins).
   - Both the Player and the Dealer automatically pay an initial ante equal to $B$. Initial pot is $2B$.
2. **Rounds**:
   - Maximum 5 betting rounds (`DefaultMaxRounds`).
   - In round $R$, the required bet to stay in the hand is $R \times B$.

### Player & Dealer Actions

- **Call / Continue (`ActionCall` / つづける)**:
  - Match the current round bet.
  - If both parties call, the game advances to round $R + 1$ (or showdown if round 5 is reached).
- **Showdown (`ActionShowdown` / しょうぶ)**:
  - Match the current round bet and immediately trigger the showdown.
- **Fold (`ActionFold` / おりる)**:
  - Forfeit the hand. The opponent immediately wins the entire pot.

### Dealer AI Behavior

- The Dealer evaluates action probabilities using the player's visible card rank:
  - **High Player Card (King, Queen, Jack)**: High probability that dealer is beaten. Dealer folds with high probability or calls conservatively.
  - **Low Player Card (Ace, 2, 3)**: High probability that dealer holds a superior card. Dealer aggressively calls or triggers showdown.
  - **Mid Cards (4..10)**: Balanced play based on game round.

### Showdown & Payout Settlement

- **Player Win** (`Player Rank > Dealer Rank` or Dealer Folded):
  - Player receives the entire pot (`PayoutCoins = Pot`).
- **Dealer Win** (`Dealer Rank > Player Rank` or Player Folded):
  - Player receives 0 (`PayoutCoins = 0`).
- **Tie** (`Player Rank == Dealer Rank`):
  - Pot is returned / split (`PayoutCoins = PlayerCommittedCoins`).

## Persistence & State Management

### Poker Session Model (`casino_poker_sessions`)

Multi-round Indian Poker games are persisted in MariaDB via `casino_poker_sessions` to enable turn-by-turn interactive play:

- **Schema Attributes**:
  - `id`: Unique session identifier (`VARCHAR(64)` / UUID).
  - `character_id`: Foreign key referencing characters table with cascading deletion.
  - `status`: Game state (`in_progress`, `player_won`, `dealer_won`, `tie`, `player_folded`, `dealer_folded`).
  - `round`: Current betting round ($1 \le R \le 5$).
  - `max_rounds`: Maximum betting rounds (default 5).
  - `base_rate`: Initial ante / base unit bet.
  - `pot_coins`: Total accumulated pot.
  - `player_committed`: Total coins committed by player.
  - `dealer_committed`: Total coins committed by dealer.
  - `player_card_suit`, `player_card_rank`: Player's drawn card (Ace=1 .. King=13).
  - `dealer_card_suit`, `dealer_card_rank`: Dealer's drawn card.
  - `history_json`: Turn-by-turn action and log event history.
  - `created_at`, `updated_at`: Timestamps.

### Client View & Information Security (Anti-Cheating)

- **Card Masking (`ClientView`)**:
  - While a session is in `in_progress` status, the player's own card is strictly masked to `{suit: "?", rank: 0}` in all HTTP responses (`GET`, `POST start`, `POST action`).
  - The dealer's card is visible as intended by game design.
  - Only upon showdown, fold, or game completion is the player's true card unmasked in the client response.

## HTTP Endpoints & Session Lifecycle

1. **Start Game (`POST /characters/{id}/casino/poker`)**:
   - Accepts `{ "base_rate": <coins> }`.
   - Validates that no active session (`status = 'in_progress'`) currently exists for the character; returns `422 Unprocessable Entity` if one is already active.
   - Deducts initial ante atomically from `casino_accounts`.
   - Deals cards, creates a new session in MariaDB, and returns the session state with masked player card.

2. **Query Active Session (`GET /characters/{id}/casino/poker`)**:
   - Queries the active (`status = 'in_progress'`) session for the character.
   - Returns `404 Not Found` if no active session exists.
   - Returns session state with masked player card.

3. **Play Round Action (`POST /characters/{id}/casino/poker/action`)**:
   - Accepts `{ "action": "call" | "showdown" | "fold" }`.
   - Validates active session existence; returns `404 Not Found` if no session is active.
   - For `call` and `showdown`: Deducts the required round bet from player's `casino_accounts` balance.
   - Dealer AI makes its move based on the player's card rank.
   - Resolves round progression, dealer fold, or showdown settlement atomically.
   - Updates `casino_poker_sessions` and credits payout to `casino_accounts` on player win/tie.
   - Returns final or updated session state with unmasked cards if the game finished.

## Concurrency & Deadlock Prevention

- **Clustered Lock Ordering**:
  - To prevent MariaDB gap-lock deadlocks (`Error 1213`) under concurrent requests, transactions always acquire an exclusive row-level lock (`SELECT ... FOR UPDATE`) on the parent `casino_accounts` record via primary key `character_id` before querying or mutating `casino_poker_sessions`.
  - Session saves employ `UPDATE ... WHERE id = ?` first and fallback to `INSERT` only when no row was updated, completely preventing insert intention gap conflicts.
- **Account Balance Conservation**:
  - Bet deductions and pot payouts are executed strictly within the same database transaction as session state transitions.

