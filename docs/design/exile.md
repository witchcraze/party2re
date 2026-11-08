# Exile Vote Board (荒らし追放騎士団)

Players run the board themselves: they petition to exile a disruptive player, and other players vote on it. A verdict puts someone to **sleep**. Nobody is deleted or banned. The game rules follow the original `party2/lib/exile.cgi`, including behaviors that look accidental. Those behaviors are kept on purpose. Possible improvements wait for a decision in [#1972](https://github.com/witchcraze/party2re/issues/1972); until then, a difference from the original is a defect.

## Rules

| Rule | Value | Legacy evidence |
|---|---|---|
| Who may petition, vote or see the board | Characters with at least 50 job changes (`JobLevel`). Others get no petition list. | `exile.cgi:72,122` |
| Votes needed for a verdict | 10 approvals or 10 oppositions. The applicant's own approval counts. | `exile.cgi:20,104,169,190` |
| Guilty | Petition removed, target sleeps **1 hour** | `exile.cgi:23,172` |
| Acquitted | Petition removed, applicant sleeps **5 days** (skipped if the applicant no longer exists) | `exile.cgi:17,192` |
| Sleep from a verdict | Replaces any remaining sleep, even a longer one | `system.cgi:861` (`regist_you_data`) |
| Withdrawal lock | 3 hours from the **last write to the whole board** | `exile.cgi:29,140-141` |
| Petition limit | Setting is 3, but each existing row is counted before the comparison, so a 4th petition is admitted. Once any applicant holds 4, nobody can file. | `exile.cgi:26,97` |
| Rename | Refused while the character is an applicant, a target or a voter on any open petition | `name_change.cgi:112,167-180` |

Account deletion, host blacklists and guild removal on a guilty verdict are commented out in the original (`exile.cgi:174-177`), so none of them happens here. Automatic `@追放騎士` petitions (`system.cgi:1027`) have no live callers, so they are not reproduced. The applicant-penalty exemption for `@` names is unreachable for the same reason.

## Commands

The board is the `exile` town facility. Its scene shows the open petitions in submission order, each with every vote, newest first.

| Legacy command | Gateway command | Behavior |
|---|---|---|
| `＠ついほう>名前＠りゆう>理由` | `exile_petition` (`target_name`, `reason`) | Checks, in order: name and reason present, job level, not yourself, target exists. It then scans the open petitions in order and refuses a target that is already petitioned, or a scan that passes the limit. Otherwise it records the petition with the applicant's approval. The original 500-character command limit applies to the escaped name and reason together. |
| `＠さんせい>名前` | `exile_approve` (`target_name`) | Adds an approval. Refused for a repeat voter or for the target. The 10th approval returns the guilty verdict. |
| `＠はんたい>名前` | `exile_oppose` (`target_name`) | Before anything else, every open petition filed by the actor is withdrawn, whatever name is given. Within the lock window this refuses the whole command. Matching another petition still records the opposition, and the 10th opposition returns an acquittal. |

A vote that neither withdraws a petition nor matches one returns `EXILE_PETITION_NOT_FOUND`. In the original, this case showed the list instead. Petitions, withdrawals and verdicts publish `exile` news. The scene and [OpenAPI](../api/openapi.json) own the response shapes.

## Identity

The original stored names in `violator.cgi`. A deleted target was dropped the next time someone voted on it, and duplicate votes were found by substring matching. Here, petitions and ballots keep IDs plus name copies taken when they were written. A deleted target removes the petition. A deleted applicant's petition and a deleted voter's ballot stay. Targets are matched by exact name, which works because involved characters cannot rename.

## Storage and concurrency

`exile_board` is a single row. Every command locks it, which serializes board changes the way the original `flock` did. Its `updated_at` is the board-wide write time that the withdrawal lock uses. After the board, each command locks the rows of the characters whose names it records, in ascending ID order. Renaming locks the character row before checking involvement, so a rename and a petition naming the same character cannot both succeed. The sentenced sleep is the shared Valkey sleep timer, which cannot join the SQL transaction. SQL is authoritative: the verdict commits an absolute deadline in `exile_sentences`, and only then is the deadline projected to the timer (`SetLockUntil`) and the row deleted. Projection holds the board lock, so it runs after every committed verdict and cannot write an older deadline over a newer one. A failed, ambiguous or interrupted projection keeps the row, and the next exile command projects the same deadline again; a deadline that has already passed is dropped. A sleep is therefore never applied without a committed verdict. **Residual risk:** if Valkey is unavailable or the process stops right after a verdict, the sentence starts late, at the next exile command, with its original deadline. A retried projection also replaces any sleep the character started in the meantime, as the original verdict would have. **Known gap:** the sleep gate only blocks new actions. The original sent any sleeping character to the sleep screen on their next request (`party.cgi:14`). Here, work already running for a sentenced character, such as an adventure in progress, is not interrupted, and this has not been reconciled.
