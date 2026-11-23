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

The weapon (`$m{wea}`), armor (`$m{arm}`) and tool (`$m{ite}`) hands each hold at most one unit. Shields, accessories and consumables share the tool hand even though their equipment slots differ.

**Decision (#2558, 2026-11-23) — tool hand identity:** inventory ownership alone identifies the held tool, including gear with an equipment slot. If older data contains multiple tool rows or a stack, actions that require a known hand reject the ambiguous state.
