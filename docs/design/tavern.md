# Adventurer's Tavern Design

The Tavern Module (`internal/tavern`) reconstructs `lib/bar.cgi` as `冒険者の酒場`, run by `@エレナ`; both names and the dish names in [`menu.json`](../../internal/tavern/data/menu.json) are clean-room replacements, while prices, healing and raffle tickets follow the legacy menu.

## Standing Delivery Order

`でりばりー` only saves a standing order without requiring funds; there is no standalone settlement endpoint. In legacy `sub nigeru` (`party2/lib/_battle.cgi:1214, 1348–1375`), every participant exit—including round-0 Party waiting-lobby leave—clears fullness first and then settles any standing order once per participant without setting fullness or awarding raffle tickets/guild points, leaving an unaffordable order active.

Known gaps: normal, boss, challenge, and dungeon exits reset fullness without settling standing deliveries ([#2856](https://github.com/witchcraze/party2re/issues/2856)), and PvP/GvG exits settle deliveries without returning the outcome to HTTP ([#2857](https://github.com/witchcraze/party2re/issues/2857)).
