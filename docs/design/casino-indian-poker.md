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

## Persistence & Transactions

- All coin exchanges and game wagers are processed via atomic transactions (`*sql.Tx`) with row-level locking to prevent race conditions during concurrent mini-game plays.
