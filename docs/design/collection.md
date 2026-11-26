# Monster Book & Collection Design

## Rules

- **Pitfall:** only items up to No. 141 count toward completion. Higher-numbered items stay visible without counting (`lib/collection.cgi:45–49,81–87`).
- Legacy equipment columns use values calculated when the tables load, including random rolls and some strengths based on the viewer's stats (`lib/_data.cgi:399–407,440–452,511–516`). The Home view uses the same nominal values as the shops so repeated observations show stable values.
- A monster's first-defeat stats and rewards are an immutable snapshot; later defeats only increment the count. **Decision:** records predating the snapshot stay empty instead of fabricating stats.
- PvP opponents never enter the monster book.

## Completion and induction

Hall-of-fame induction is mandatory; completion is marked only after induction succeeds, so a failed induction is retried on the next defeat or discovery, once and without duplicates. News is best effort and published only on the first completion. Characters who completed before this rule and lost their induction are repaired through the collection service's recovery operation, not ad hoc SQL.

Discovery and induction failures propagate out of the enclosing acquisition or battle settlement and roll it back as a whole; adding a discovery caller must keep this.

Hand items are not registered by every Go acquisition path: combat stored-tool exchange (`job50`/`job86`, matching `party2/lib/_skill.cgi:10457–10461,10505–10509`) and direct-to-hand treasure drops ([#2879](https://github.com/witchcraze/party2re/issues/2879), gap vs `party2/lib/_npc_action.cgi:70–71`) do not record discovery, and `home_collection` is read-only (unlike `collection.cgi:37`). Listing-time `add_collection` in Flea Market and Player Store therefore actively checks held hand items (`inventory_items`).
