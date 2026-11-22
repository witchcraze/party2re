# Monster Book & Collection Design

## Rules

- **Pitfall:** only items up to No. 141 count toward completion. Higher-numbered items stay visible without counting (`lib/collection.cgi:45–49,81–87`).
- Legacy equipment columns use values calculated when the tables load, including random rolls and some strengths based on the viewer's stats (`lib/_data.cgi:399–407,440–452,511–516`). The Home view uses the same nominal values as the shops so repeated observations show stable values.
- A monster's first-defeat stats and rewards are an immutable snapshot; later defeats only increment the count. **Decision:** records predating the snapshot stay empty instead of fabricating stats.
- PvP opponents never enter the monster book. Sealing battles record their bosses (king99 clones as one shadow entry) with habitat 「封印戦」.

## Completion and induction

Hall-of-fame induction is mandatory; completion is marked only after induction succeeds, so a failed induction is retried on the next defeat or discovery, once and without duplicates. News is best effort and published only on the first completion. Characters who completed before this rule and lost their induction are repaired through the collection service's recovery operation, not ad hoc SQL.

Discovery and induction failures propagate out of the enclosing acquisition or battle settlement and roll it back as a whole; adding a discovery caller must keep this. Sealing battles follow the [Boss settlement](boss.md#rules-and-decisions).
