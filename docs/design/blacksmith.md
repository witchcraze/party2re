# Blacksmith (鍛冶屋): Weapon Seals, Naming, and Dedicated Storage

The Blacksmith (`party2/lib/blacksmith.cgi`) engraves weapon seals for crystals,
names equipped weapons and armor, and keeps up to three weapons with their seals
and names. Code and tests in `internal/blacksmith` own the seal catalog,
validation and storage rules; this document keeps decisions and non-obvious
legacy findings.

## 1. Weapon Seals (こくいん)

- A weapon holds one seal. Applying another replaces it and charges the new
  seal's full crystal cost; there is no existing-seal rejection
  (`blacksmith.cgi:110–140`).
- Eligibility comes only from each seal's weapon ID list (`_data.cgi:2209–2223`):
  neither weapon type nor an existing seal adds a rejection.
- Item 257 (水晶の原石) with no seal engraved prints its failure message and is
  not consumed (`_data.cgi:2022`, `home.cgi:288–290`).

## 2. Equipment Naming (なづける)

**Decision (#2060).** Following legacy, there is no reset to the catalog name.
An empty name, and the Perl-false name `0`, rename nothing: legacy shows the
naming prompt (`blacksmith.cgi:59,79,99–104`), which the Gateway replaces with the
scene observation, so `blacksmith_name` rejects them as invalid names. Names are
stored as given, without trimming. A custom name disappears only when the item
leaves the slot (§3).

## 3. Dedicated Weapon Storage (専用預かり所)

- Depositing clears `wea_seal` and `wea_name` on the character, matching legacy
  `write_user` cleanup after unequipping (`system.cgi:80–83`); withdrawing
  restores them.
- Withdrawal checks only the main-hand slot and does not reject a full
  inventory (`blacksmith.cgi:192–237`). Withdrawing while a weapon is held is a
  logged reply that changes nothing (`blacksmith.cgi:206–209`).

## 4. Gateway Commands

Commands and observation follow the
[shared Gateway contract](../architecture/client-agent-api.md#input-and-execution-boundary).
Each success logs the legacy `＠verb>target` row and the smith's reply. The four
additions to `@actions`/`%actions` (`blacksmith.cgi:24–31`) map as follows:

| Legacy dispatch | Gateway action | Mapping decision |
| --- | --- | --- |
| `こくいん` → `kokuin` | `blacksmith_seal` → `ApplySeal` | Seal ID replaces the seal-name text |
| `なづける` → `nazukeru` | `blacksmith_name` → `NameEquipment` | Explicit `target`; empty name rejected (§2) |
| `あずける` → `azukeru` | `blacksmith_deposit` → `DepositWeapon` | The equipped main-hand weapon is the target |
| `ひきだす` → `hikidasu` | `blacksmith_withdraw` → `WithdrawWeapon` | Owned `slot` replaces legacy effective-name selection |

The shared `system.cgi` actions, including `はなす` and NPC `しらべる`, follow the
[navigation reconciliation](../architecture/client-agent-api.md#scoped-legacy-navigation-reconciliation).
