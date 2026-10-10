# Bank System Design

## Overview

The Bank (銀行) system provides secure gold storage managed by the bank teller NPC Taxeed (`@タクシード`, asset: `bank.gif`), matching the original Party2 Perl CGI specification (`party2/lib/bank.cgi`).

Savings are stored per character (`Character.Deposit int64`) rather than at the player account level. The bank provides zero-fee, 24/7 gold deposits and withdrawals with an upper limit of 99,999,999,999,999 G (99兆9999億9999万9999 G). Direct transfers between players are not part of the legacy bank specification and are handled via postal depot mechanics.

## Domain Model

### Character Bank Deposit (`Character.Deposit`)
- **CharacterID**: Target character identifier.
- **Money**: Active wallet gold (clamped to `MaxWallet = 999,999 G` upon withdrawal).
- **Deposit**: Bank deposit balance (`int64`, up to `MaxDeposit = 99,999,999,999,999 G`).

### NPC Receptionist: Taxeed (`@タクシード`)
- **Asset**: `bank.gif`
- **Standard Dialogues**:
  - `"ゴールドをお預かりいたします"`
  - `"24時間いつでも、手数料もございません"`
  - `"99999999999999 Gまでお預かりいたします"`
- **Interaction Messages**:
  - On Deposit: `"{amount} Gお預かりいたしました"`
  - On Withdraw: `"{amount} Gお返しいたします"`
  - On Limit Exceeded: `"これ以上お預かりできません"`

### Operations

- **Deposit (`Deposit`)**:
  - Deposits gold from character wallet (`Character.Money`) into bank savings (`Character.Deposit`).
  - Validates `amount > 0` and character wallet has sufficient funds (`Money >= amount`).
  - Enforces `Deposit + amount <= MaxDeposit` (99,999,999,999,999 G). Rejects with `ErrDepositLimitExceeded` if exceeded.
  - Updates character wallet: `money -= amount`, and bank savings: `deposit += amount`.

- **Withdrawal (`Withdraw`)**:
  - Withdraws gold from bank savings (`Character.Deposit`) into character wallet (`Character.Money`).
  - Validates `amount > 0` and bank savings has sufficient funds (`Deposit >= amount`).
  - Legacy Wallet Clamp (`999,999 G`):
    - When gold is withdrawn, wallet money is incremented by requested amount and deposit is decremented.
    - If `money > MaxWallet (999,999)`:
      - Excess gold `excess = money - MaxWallet` is automatically refunded back to `deposit`.
      - Character wallet `money` is set to `999,999`.
      - Actual gold added to wallet is `requested - excess`.
    - Formula:
      ```
      deposit -= amount
      money += amount
      if money > 999999:
          refund = money - 999999
          deposit += refund
          money = 999999
      ```

## Transaction & Concurrency Invariants

- Deposits and withdrawals operate strictly within the `characters` table using Rank 2 row locking (`SELECT ... FOR UPDATE`).
- Each standalone bank operation locks one character row. Composite operations must still follow the global lock hierarchy; a single-row repository does not certify deadlock freedom for an arbitrary caller.
- Character bank deposits are included in wealth rankings (`characters.money + characters.deposit`).

## Observation and retained routes

Selecting `bank` exposes the owned character's wallet, savings, savings limit,
NPC name and complete dialogue list through character context. GET and command
refresh use the same existing `GetState` reader; observation never deposits,
withdraws, chooses a random dialogue or updates navigation. Required read failure
prevents partial observation. See the [context and recovery contract](../architecture/client-agent-api.md).

Bank state GET and deposit/withdraw REST operations are retired after verified
Gateway/context replacement. Bank inspect and talk POSTs remain; these dialogue
commands still require legacy cooldown, presence and log reconciliation under
#947.

| Former or retained REST operation | Supported replacement / disposition |
| --- | --- |
| GET `/characters/{id}/bank` (retired) | POST `scene_enter` with `destination:bank`, then GET `/api/v1/characters/{id}/context`; GET observes without navigating and actual activity takes precedence |
| POST `/characters/{id}/bank/deposit` (retired) | POST `/api/v1/characters/{id}/actions`, `action:bank_deposit`, explicit `params.amount` |
| POST `/characters/{id}/bank/withdraw` (retired) | Same Gateway, `action:bank_withdraw`, explicit `params.amount`; result keeps actual withdrawal/refund facts |
| POST `/characters/{id}/bank/inspect` (retained) | Existing NPC metadata; action effects/parity remain unresolved |
| POST `/characters/{id}/bank/talk` (retained) | Existing random dialogue; action effects remain unresolved |

Requests to retired routes return 404. Gateway commands retain owned-actor,
sleep/pending-Wake and unfinished-work guards, strict amount inputs and known
outcome preservation with GET-only refresh recovery. Resolver links for the two
commands target the Gateway. The unused `bank_state` alias is removed; it does
not imply that a context GET selects Bank.

The local behavioral reference is `party2/party2/lib/bank.cgi`: header display
(lines 39–42) maps to wallet/savings observation; the only facility-specific
actions `あずける` and `ひきだす` (lines 30–33) map to the existing Deposit/Withdraw
services and Gateway commands. NPC name, limit and words come from lines 8–24.
`party.cgi:13–30` loads the current facility and shared action/request lifecycle.
Shared `はなす` uses a random word (`lib/system.cgi:240–246`); NPC `しらべる`
uses the default nothing-found response (`322–328,413`), while the current Go
inspect returns NPC metadata. The observation exposes existing service facts;
it does not claim parity for these commands or reproduce their side effects.
Shared navigation uses the approved ordinary selector; presence effects remain
separate. Whisper, logout and screenshot are outside Bank's responsibility.
