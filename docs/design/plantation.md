# Plantation Design

The seed garden (種菜園, `@ロータス`) reconstructs legacy `lib/plantation.cgi`. Seed and fertilizer parameters live in [`internal/plantation/catalog.go`](../../internal/plantation/catalog.go). Item fertilizers whose legacy names are distinctive franchise terms use clean-room names (rule 00 §3).

## Rules

- A character has at most one plot. A crop matures at the next 00:00 JST after sowing, matching legacy's calendar-day comparison. Observing a mature plot claims nothing; only `plantation_harvest` rolls the outcome.
- At most one fertilizer per plot. An item fertilizer is consumed from the Depot first, then from the Inventory. As in legacy, the fertilizer list offers an item fertilizer only while the character holds it.
- A harvest rolls once: the wither chance first, then the yield count and each item's quality. The plot is deleted either way. Yields go to the Depot, past capacity if needed ([delivery capacity](depot.md#delivery-capacity-by-caller)), so no retry can reroll a harvest.
