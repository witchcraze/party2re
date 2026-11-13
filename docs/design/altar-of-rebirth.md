# Altar of Rebirth (復活の祭壇)

The Altar (`reborn.cgi`, `internal/altar`) awakens Ramia once a character has offered all six weekday orbs, then grants one otherworld travel item per awakening. `internal/altar` owns prayer, wishes and their messages; this document keeps decisions and non-obvious legacy findings.

- The altar has no offering command: orbs are offered only by using items 60–65 at own Home, from the held tool or the Depot (`_data.cgi:965–1054`).
- **Decision:** `altar_pray` rejects an awakened character. Legacy offers `＠いのる` only while the orb string has no `G` and `＠ねがう` only while it has one (`reborn.cgi:46–61`). Home offering appends to any string without the rune, including `G`, so a `G` string with six more runes never prays again.
