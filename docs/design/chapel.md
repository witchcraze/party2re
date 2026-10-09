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
| **3** | `MONSTER` | モンスターと仲良くしたい | モンスターが仲間になりやすくなるかも？ | +0.25% Monster recruit rate bonus flat (`$par += 0.5` out of 200 in `rand(200) < $par`) |
| **4** | `DROP` | 宝箱がほしい | 宝箱が増えるかも？ | 20% chance of +1 treasure chest / drop item (`rand(5) < 1`) |
| **5** | `CASINO` | コインがほしい | コインが増えるかも？ | Casino bonus coin luck (`rand(4) < 1`) |

---

## Single Active Wish Constraint & Daily Quota (単一の祈り制約と祈願日次枠)

In original Party2 (`party2/lib/chapel.cgi`), if a character already has an active wish or has already prayed on the current day (`&checkWished || -e $filePath`):
- Any subsequent attempt to pray is rejected with the authentic Sister response:
  > **「祈りに大事なのは、数でなく気持ちなのです」**
- In Party2re, this is enforced by separating the active blessing effect (`active_blessing`, corresponding to `$m{wish}`) from the prayer record (`prayed_at`, corresponding to `wish.cgi`).
- Database pessimistic locking (`FOR UPDATE` on `characters` at Rank 2 and `character_blessings`) serializes concurrent prayer attempts, returning `chapel.ErrAlreadyPrayed` and HTTP status `409 Conflict`.
- Sleeping at home resets the active blessing to `NONE`. However, if the prayer occurred on the same JST calendar day, the prayer record is retained, preventing same-day re-prayer. Re-prayer becomes eligible only after waking from a sleep taken on a subsequent calendar day.

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

For victory reward settlements, dungeon exploration chests, and drop evaluations:
- **EXP Calculation** (`_battle.cgi:190-193`):
  $$\text{Final EXP} = \begin{cases} \lfloor \text{Base EXP} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{EXP} \land \text{Random}(4) = 0 \\ \text{Base EXP} & \text{otherwise} \end{cases}$$
- **Gold Calculation** (`_battle.cgi:186-189`):
  $$\text{Final Gold} = \begin{cases} \lfloor \text{Base Gold} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{GOLD} \land \text{Random}(4) = 0 \\ \text{Base Gold} & \text{otherwise} \end{cases}$$
- **Monster Recruitment Rate** (`_battle.cgi:237`):
  $$P(\text{Recruit}) = \frac{\text{par} + 0.5}{200} \quad (\text{if Blessing} = \text{MONSTER}, \text{equivalent to flat } +0.25\%)$$
- **Treasure / Drop Bonus** (`_npc_action.cgi:501, 729`):
  $$\text{Extra Chests / Drops} = \begin{cases} +1 & \text{if } \text{Blessing} = \text{DROP} \land \text{Random}(5) = 0 \\ 0 & \text{otherwise} \end{cases}$$

---

## Elimination of Fictional Donations

The fictional donation feature (`POST /characters/{id}/chapel/donate`, `donation_gold_total`, `ErrInvalidDonation`, `ErrInsufficientGold`) was removed in Issue #472:
- Original Party2 has **no donation mechanism** in `chapel.cgi`.
- `donation_gold_total` and its check constraint were purged via migration `060_chapel_parity.sql`.

---

## Resting Recovery & Blessing Lifecycle Reset (chapel_clean)

In original Party2 (`party2/lib/home.cgi: &chapel_clean`), active prayers are managed through resting at home (`&neru`):
1. **Sleep Clears Active Effect**: Upon sleeping, the active blessing (`$m{wish}`) is cleared to 0.
2. **Quota Reset on Next-Day Sleep**: The prayer record file (`wish.cgi`) is checked against the current date. If `wish.cgi`'s timestamp is on a previous calendar day (`$wishedDay ne $day || $wishedMon ne $mon || $wishedYear ne $year`), `wish.cgi` is deleted. If sleep occurs on the same calendar day as the prayer, `wish.cgi` remains, blocking re-prayer.
3. **No Automatic Midnight Wipe**: Midnight rollover alone does NOT clear active blessings or restore prayer eligibility. A character who stays awake across midnight retains their active blessing until they sleep.

In Party2re (Issue #1163):
- **Elimination of Midnight Reset Worker**: The fictional daily midnight reset scheduled action (`chapel_reset`) has been eliminated. No cron or worker scans characters at midnight. Residual `chapel_reset` actions in Valkey are handled as safe no-ops without modifying blessings or re-enqueuing.
- **Home Wakeup Reset**: Resting recovery (`internal/home: Wake`) invokes `chapel.Service.ClearBlessing(ctx, characterID)`.
  - If the prayer was made on the same JST calendar day: `active_blessing` is updated to `'NONE'`, while `prayed_at` is preserved. Same-day re-prayer remains blocked (`ErrAlreadyPrayed`).
  - If the prayer was made on an earlier calendar day: both the active blessing and the prayer record are deleted (`DELETE FROM character_blessings`), permitting the character to pray again on the new day.

