# Items and Equipment Design

## Purpose

This document records cross-module item usage categories, legacy hand-field mappings, and non-legacy equipment metadata boundaries (`internal/core/item`, `internal/core/inventory`). Catalog definitions, stackability/consumption invariants, and pricing live in code and [Shops](shops.md).

## Usage Location Categories

Derived from legacy `@ites` (`$ites[no][3]`, `party2/lib/_data.cgi:617`), item usage is partitioned into categories `0`–`4`:

- **`0` (None):** Not actively usable via menus (materials, coins, job emblems, auto-consumed items).
- **`1` (Combat only):** Usable only via combat `＠どうぐ`.
- **`2` (Anytime / Home):** Usable at Home (`＠ほーむ`); see [Home consumption contract](home.md#legacy-dispatch-and-consumption-contract).
- **`3` (Combat passive):** Passive or automatic in-battle triggers.
- **`4` (Quest entry / Home Depot-after-action):**
  - At Home (`＠つかう`), category `4` shares the [Home consumption contract](home.md#legacy-dispatch-and-consumption-contract) (`113`/`114` no-stat consumption; `259` opens `258`).
  - At quest entry (`party2/lib/quest.cgi:1188–1189`), `item-113` and `item-114` add +100 battle MaxHP/MaxMP capped at 999 (`party2/lib/_data.cgi:1408–1429`).
  - **Decision (#2369, 2026-11-22) — `item-259` (`異封の鍵`) at quest entry:** Intentionally treated as a **no-op** (characters enter combat normally with no Depot box lookup/opening, reward delivery, or room corruption). Legacy `quest.cgi:1188` invokes `259`'s Home-oriented callback (`_data.cgi:2024–2068`) while expecting a `%p` hash return; its scalar/empty return wipes `%p` and triggers `_battle.cgi:478` room deletion (`"データが壊れています。クエストを強制終了します"`). `home.cgi:329–372` (`$is_after`) indicates category `4` was reused on `259` to defer Depot writes rather than for quest entry.

## Equipment System

- **Non-legacy metadata:** `ItemInstance` enhancement metadata (`+0` to `+10` in Go) does not establish a legacy gold/material enhancement mechanic; authentic weapon seals belong to [Blacksmith](blacksmith.md).
- **Single accessory slot:** Go exposes one accessory slot category (`SlotAccessory`); do not invent a second accessory slot.

### Legacy hand fields

Legacy characters hold one weapon (`$m{wea}`), one armor (`$m{arm}`) and one held item (`$m{ite}`); entering a field is equipping it. Go keeps exactly these fields as inventory rows, at most one quantity-1 row per `Definition.LegacyStorageKey` kind: kind 1 weapons (main-hand), kind 2 armor (body) and kind 3 every `item-NNN` entry, so shields (off-hand), accessories and consumables share the single held field. This kind differs from `Slot.Kind()`, which groups shields and accessories with armor. An occupied field never blocks another. [Depot withdrawal](depot.md#1-item-deposit--withdrawal) and [Shop single purchase](shops.md#5-purchase-routing-and-depot-auto-transfer) fill an empty field and equip gear into `equipment_slots`, routing the rest to Depot.

**Decision (#2558, 2026-11-23) — tool hand identity:** the tool hand is the inventory's only kind-3 row, read from the inventory alone. Weapon or armor rows and `equipment_slots` neither hide nor identify it, so no separate hand reference or selection is stored. Rows breaking the one-unit field (a second kind-3 row or a stack) leave the hand unknown, and actions that need it reject. Combat writes that change the hand (job50/78/86) unlink the leaving row and equip entering gear as Depot withdrawal does.
