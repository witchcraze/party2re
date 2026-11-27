# Bank System Design

## Overview

The Bank (銀行, teller `@タクシード`, `party2/lib/bank.cgi`) stores gold per character (`Character.Deposit`), not per account. It charges no fees, pays no interest and holds at most 99,999,999,999,999 G. It has no player-to-player transfer; players send gold through the [Depot](depot.md#5-mailing-items-and-money-おくる). Savings count toward the wealth ranking.

## Rules

- **Wallet clamp on withdrawal:** legacy adds the whole requested amount to the wallet and then returns anything above 999,999 G to savings (`bank.cgi:85–88`), so a withdrawal never fails because the wallet is full. The reply still names the requested amount (`bank.cgi:89`).
- **Deposit limit:** legacy checks the limit before the wallet, and answers with the logged reply `これ以上お預かりできません`, which changes nothing but still starts the action wait (`bank.cgi:51–53`). Bank therefore returns it as a success, not a rejection.

## Observation and retained routes

Selecting `bank` exposes the wallet, savings, savings limit and NPC name through character context ([context and recovery contract](../architecture/client-agent-api.md)). No Bank REST route remains.

| Legacy action | Disposition |
| --- | --- |
| `あずける` / `ひきだす` (`bank.cgi:31–34`) | `bank_deposit` / `bank_withdraw` with explicit `params.amount`. A success logs `＠あずける>NG` / `＠ひきだす>NG` and the teller's reply. An amount that is missing or over the wallet or savings was a private prompt (`$mes`), so it is a rejection and logs nothing. |
| Common `はなす`, NPC `しらべる` | `npc_talk` / `npc_inspect` ([NPC commands](../architecture/place-log.md#npc-interaction)) |
