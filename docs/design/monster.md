# Monster Grandpa & Pet Companion Design (モンスターじいさん・ペット)

The ranch (`internal/monster`, legacy `lib/farm.cgi`) stores monsters tamed after battle (`internal/battle/taming.go`, legacy `lib/_battle.cgi:230-265`) and moves them between the box and the home. It also renames, gifts and releases boxed monsters. Operations and errors are owned by the code and [OpenAPI](../api/openapi.json); this document keeps the capacity decisions.

## Capacity

`BoxCapacityForCharacter` owns the box capacity (50–350 from `JobLevel` and `OverMonster`). Both capacity rules below reproduce what original players experienced rather than the values the legacy source appears to intend.

- **Home capacity equals the owner's box capacity.** `lib/farm.cgi:17` sets a global limit of 8, but the file-scoped `my ($my_monster, $max_monster) = &get_monster_c($id)` at `:22` shadows it. The bring-home check (`:207`), the `@モンジィ` home-limit line (`:28`) and the header (`:58`) all read the shadowing value, so the 8 never applied. `GET /monster/dialogue` has no character, so its home-limit line refers to the box capacity instead of a number. Heaven Wish grants write NPC companions as home members, like monsters, but skip both the count and the duplicate-name check (`lib/god.cgi:79-117`). Bringing a monster home checks both against the companions too, and `あずける` moves a companion into the box.
- **A gift is limited by the sender's capacity.** `おくる` checks the recipient (`farm.cgi:85`) with `get_monster_c` (`lib/system.cgi:1308-1321`). That routine counts the recipient's box but derives the limit from the acting player's `job_lv`/`over_monster`. A gift can therefore exceed the recipient's own capacity. The recipient then cannot tame or deposit until the box is back under their limit.
- `get_monster_c` applies `over_monster` before clamping it to 5 (`system.cgi:1313-1314`). The God wish stops at 5 (`lib/u_god.cgi:20`), so the order has no effect.
