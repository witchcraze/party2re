# Optional Environment Variables

This document lists the optional settings of the application daemon. Required settings and the base Docker Compose example are in the [README](../../README.md#実行環境変数).

Set a value under the `environment` key of the `app` service:

```yaml
services:
  app:
    environment:
      PARTY2_MAX_PLAYERS: "999"
      PARTY2_HOME_COMPANION_PHRASE_LIMIT: "30"
      PARTY2_HOME_LETTER_LOG_LIMIT: "30"
      PARTY2_HOME_LETTER_MAX_LENGTH: "500"
      PARTY2_PARK_POST_LOG_LIMIT: "30"
      PARTY2_PARK_POST_MAX_LENGTH: "500"
      PARTY2_PLACE_LOG_LIMIT: "30"
      PARTY2_PLACE_LOG_TTL_HOURS: "168"
```

| Variable | Description | Example |
| :--- | :--- | :--- |
| `PARTY2_DB_MAX_OPEN_CONNS` | Maximum open database connections (default `25`). Invalid values are ignored. | `50` |
| `PARTY2_DB_MAX_IDLE_CONNS` | Maximum idle database connections (default `25`). Invalid values are ignored. | `25` |
| `PARTY2_DB_CONN_MAX_LIFETIME` | Maximum database connection lifetime (default `5m`). Invalid values are ignored. | `10m` |
| `PARTY2_DB_CONN_MAX_IDLE_TIME` | Maximum database connection idle time (default `1m`). Invalid values are ignored. | `2m` |
| `PARTY2_VALKEY_PASSWORD` | Valkey AUTH password (default none). | `<valkey-password>` |
| `PARTY2_VALKEY_DB` | Valkey database number (default `0`). Invalid values are ignored. | `0` |
| `PARTY2_VALKEY_DIAL_TIMEOUT` | Valkey connection timeout (default `5s`). Invalid values are ignored. | `10s` |
| `PARTY2_ADDR` | Listen address, which takes precedence over the port. Falls back to the alias `ADDR`. | `:8080` |
| `PARTY2_PORT` | Listen port (default `8080`). Falls back to the alias `PORT`. | `8080` |
| `PARTY2_ADMIN_API_KEY` | Administrative API key. Falls back to the alias `ADMIN_API_KEY`. Without either, administrative endpoints return `403`. | `<admin-api-key>` |
| `PARTY2_CORS_ORIGINS` | Comma-separated allowed CORS origins. When unset, all cross-origin requests are rejected. CORS is a browser interoperability policy and does not replace token or administrative key checks. Set this when a web client is served from another origin. | `https://app.party2.game,http://localhost:3000` |
| `PARTY2_TRUSTED_PROXIES` | Comma-separated trusted reverse proxy CIDRs or IPs. When unset, `X-Forwarded-For` and `X-Real-IP` are not trusted and `RemoteAddr` identifies the client, which is safe for direct exposure. Set this behind a proxy so rate limits use the real client IP. An invalid value is a startup error. | `127.0.0.1/32,10.0.0.0/8` |
| `PARTY2_MAX_PLAYERS` | Player account limit (default `999`). `0` or a negative value means unlimited. A non-integer value is a startup error. See [registration limits](../design/player-and-character.md#28-registration-population-limit--capacity-reconciliation-1105). | `999` |
| `PARTY2_HOME_COMPANION_PHRASE_LIMIT` | Companion phrases kept per character (default `30`). The oldest phrases are removed beyond the limit. An unset, empty, non-integer, overflowing, zero or negative value uses `30`, and an invalid value logs a warning. See [Home design](../design/home.md#3-companion-greeting-phrases--resident-companions-character_companion_phrases-home_members). | `50` |
| `PARTY2_HOME_LETTER_LOG_LIMIT` | Letters kept in each inbox and outbox (default `30`). The oldest letters leave a box beyond the limit on its next send. An unset, empty, non-integer, overflowing, zero or negative value uses `30`, and an invalid value logs a warning. See [Home design](../design/home.md#letter-settings). | `50` |
| `PARTY2_HOME_LETTER_MAX_LENGTH` | Maximum letter body length, counted after legacy HTML escaping (default `500`). Guild notices must also fit. A stored body above about 64 KiB fails. An unset, empty, non-integer, overflowing, zero or negative value uses `500`, and an invalid value logs a warning. See [Home design](../design/home.md#letter-settings). | `1000` |
| `PARTY2_PARK_POST_LOG_LIMIT` | Posts kept on the park board (default `30`). The oldest posts are removed beyond the limit on the next post. An unset, empty, non-integer, overflowing, zero or negative value uses `30`, and an invalid value logs a warning. See [Town Park design](../design/town_park.md#3-board-retention). | `50` |
| `PARTY2_PARK_POST_MAX_LENGTH` | Maximum park post body length, counted after legacy HTML escaping (default `500`). A stored body above about 64 KiB after HTML escaping fails. An unset, empty, non-integer, overflowing, zero or negative value uses `500`, and an invalid value logs a warning. See [Town Park design](../design/town_park.md#1-message-board-posting-park_posts). | `200` |
| `PARTY2_PLACE_LOG_LIMIT` | Rows kept in each place log (default `30`, legacy `$max_log`). Older rows are trimmed on the next append. An unset, empty, non-integer, overflowing, zero or negative value uses `30`, and an invalid value logs a warning. See [place log](../architecture/place-log.md#place-log-1938). | `100` |
| `PARTY2_PLACE_LOG_TTL_HOURS` | Hours a place log is kept after its last append (default `168`, 7 days). Idle home, guild and room logs expire. An unset, empty, non-integer, overflowing, zero or negative value uses `168`, and an invalid value logs a warning. See [place log](../architecture/place-log.md#place-log-1938). | `72` |
