# Adventurer's Tavern Design

The Tavern Module (`internal/tavern`) reconstructs the legacy bar (`lib/bar.cgi`) as `冒険者の酒場`, run by the barkeep `@エレナ`; both names and the dish names in [`menu.json`](../../internal/tavern/data/menu.json) are clean-room replacements, while prices, healing and raffle tickets follow the legacy menu. The legacy personalised full course is tracked in #2088.

## Standing Delivery Order

A delivery reservation is a standing order; reserving needs no funds. Each solo or party adventure completion resets fullness and then settles the order once per participant. There is no standalone settlement request: the legacy `でりばりー` action only stores the order, and settlement happens solely at battle end. Settlement neither sets fullness nor awards raffle tickets, and an unaffordable meal is skipped while the order stays active.

Transport operations and schemas: [OpenAPI](../api/openapi.json).
