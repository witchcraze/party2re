# Altar of Rebirth (復活の祭壇)

The Altar (`reborn.cgi`, `internal/altar`) awakens the phoenix (`不死鳥`) once a character has offered all six weekday orbs at Home (`_data.cgi:965–1054`), then grants one otherworld travel item per awakening.

- **Clean-room name (rule 00 §3):** legacy `ラーミァ` is a Dragon Quest name, so player-facing text uses `不死鳥` (`不死鳥@`, `伝説の不死鳥はよみがえりましょう`); the `G` orb rune and internal identifiers stay unchanged.
- **Decision:** `altar_pray` rejects an awakened character. Legacy offers `＠いのる` only while the orb string has no `G` and `＠ねがう` only while it has one (`reborn.cgi:46–61`). Home offering appends to any string without the rune, including `G`, so a `G` string with six more runes never prays again.
