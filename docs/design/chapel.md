# Chapel, Prayers & Blessings Design (礼拝堂・祈り)

## Overview

The Chapel Feature Module (`internal/chapel`) aligns faithfully with the original Party2 Perl CGI specification (`party2/lib/chapel.cgi`). It provides player characters with town church prayers and divine blessings granting battle, recruitment, and adventure modifier boosts during gameplay.

- **Location**: `礼拝堂`
- **NPC**: `@シスター`
- **Background**: `bgimg/chapel.gif`

---

## Authentic Blessing Types (祈りの種類)

In accordance with `party2/lib/chapel.cgi`, characters can select from exactly five authentic prayers:

| Number | Key | Japanese Name | Description (Perl CGI) | Game Modifier Effect |
| :---: | :---: | :--- | :--- | :--- |
| **1** | `GOLD` | お金がほしい | モンスターを倒したとき、ゴールドが増えるかも？ | 25% chance of 1.5x Gold on battle victory (`rand(4) < 1`) |
| **2** | `EXP` | 強くなりたい | モンスターを倒したとき、経験値が増えるかも？ | 25% chance of 1.5x EXP on battle victory (`rand(4) < 1`) |
| **3** | `MONSTER` | モンスターと仲良くしたい | モンスターが仲間になりやすくなるかも？ | +50% Monster recruit rate bonus (`$par += 0.5`) |
| **4** | `DROP` | 宝箱がほしい | 宝箱が増えるかも？ | +10% Item/chest drop rate bonus (`rand(5) < 1`) |
| **5** | `CASINO` | コインがほしい | コインが増えるかも？ | Casino bonus coin luck (`rand(4) < 1`) |

---

## Single Active Wish Constraint (単一の祈り制約)

In original Party2 (`party2/lib/chapel.cgi`), if a character already has an active wish (`&checkWished || -e $filePath`):
- Any subsequent attempt to pray is rejected with the authentic Sister response:
  > **「祈りに大事なのは、数でなく気持ちなのです」**
- In Party2Re, this is strictly enforced via database pessimistic locking (`FOR UPDATE`) returning `chapel.ErrAlreadyPrayed` and HTTP status `409 Conflict`.
- When an active wish is consumed or reset at the daily date change (`chapel_clean`), the character may pray again.

---

## Dialogue & Atmosphere

The Sister greets adventurers with the following authentic messages:
- `"<Name>に神のご加護があらんことを"`
- `"待つだけでは祈りは叶いません…"`
- `"<Name>が力を尽くすとき、祈りは叶うことでしょう"`

When praying:
- Confirmation message: `"<Name>は「<PrizeName>」と祈るのですね…"`

---

## Modifiers Calculation

For victory reward settlements and drop evaluations:
- **EXP Calculation**:
  $$\text{Final EXP} = \begin{cases} \lfloor \text{Base EXP} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{EXP} \land \text{Random}(0, 1) < 0.25 \\ \text{Base EXP} & \text{otherwise} \end{cases}$$
- **Gold Calculation**:
  $$\text{Final Gold} = \begin{cases} \lfloor \text{Base Gold} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{GOLD} \land \text{Random}(0, 1) < 0.25 \\ \text{Base Gold} & \text{otherwise} \end{cases}$$
- **Monster Recruitment Rate**:
  $$\text{Recruit Rate} = \text{Base Rate} + 0.50 \quad (\text{if Blessing} = \text{MONSTER})$$
- **Drop Rate Bonus**:
  $$\text{Final Drop Rate} = \text{Base Drop Rate} + 0.10 \quad (\text{if Blessing} = \text{DROP})$$

---

## Elimination of Fictional Donations

The fictional donation feature (`POST /characters/{id}/chapel/donate`, `donation_gold_total`, `ErrInvalidDonation`, `ErrInsufficientGold`) was removed in Issue #472:
- Original Party2 has **no donation mechanism** in `chapel.cgi`.
- `donation_gold_total` and its check constraint were purged via migration `060_chapel_parity.sql`.

---

## Daily Blessing Reset (chapel_clean)

In original Party2 (`party2/lib/home.cgi: &chapel_clean`), active prayers (`$m{wish}` and `$userdir/$id/wish.cgi`) are wiped clean upon daily date change (00:00 JST).

In Party2Re:
- **Scheduled Worker Integration**: The scheduled worker (`internal/scheduling.Worker`) registers the handler for `ActionType: "chapel_reset"` (`chapel.ActionTypeChapelReset`).
- **Distributed Daily Execution**: A deterministic scheduled task (`chapel_reset:YYYY-MM-DD`) is enqueued into Valkey pending sorted set (`party2:scheduled:pending`) targeted at 00:00:00 JST (`chapel.NextMidnightJST`).
- **Reset Mechanism**: When triggered, `chapel.Service.ClearAllBlessings(ctx)` executes `UPDATE character_blessings SET active_blessing = 'NONE', updated_at = ? WHERE active_blessing != 'NONE'`, resetting all characters with active blessings to unblessed state so they can pray again on the new day.
- **Auto-Rescheduling**: Upon executing the daily reset, the handler automatically schedules the next day's 00:00:00 JST action, ensuring continuous unattended daily cycles.
- **Home Wakeup Reset**: Sleeping at home (`home.cgi: &neru`, `internal/home`) also invokes `ClearBlessing(ctx, characterID)` upon waking up, in accordance with authentic resting recovery.

