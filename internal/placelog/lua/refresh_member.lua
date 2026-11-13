-- KEYS[1] joined, KEYS[2] expiry, KEYS[3] profile, KEYS[4] token
-- ARGV[1] now, ARGV[2] expires_at, ARGV[3] physical ttl seconds, ARGV[4] character ID, ARGV[5] profile JSON, ARGV[6] membership token
-- Purge a bounded batch of entries that expired before now (one at exactly now+ttl is still listed),
-- then keep the arrival position of a still-live listed member and refresh its expiry, profile and token.
-- A newcomer or returning expired member is placed after the last entry so same-second arrivals keep their order.
local now = tonumber(ARGV[1])
local gone = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", "(" .. ARGV[1], "LIMIT", 0, 32)
if #gone > 0 then
    redis.call("ZREM", KEYS[1], unpack(gone))
    redis.call("ZREM", KEYS[2], unpack(gone))
    redis.call("HDEL", KEYS[3], unpack(gone))
    redis.call("HDEL", KEYS[4], unpack(gone))
end
local prev_exp = redis.call("ZSCORE", KEYS[2], ARGV[4])
local live = prev_exp and tonumber(prev_exp) >= now and redis.call("ZSCORE", KEYS[1], ARGV[4])
if not live then
    local last = redis.call("ZRANGE", KEYS[1], -1, -1, "WITHSCORES")
    redis.call("ZADD", KEYS[1], (tonumber(last[2]) or 0) + 1, ARGV[4])
end
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[4])
redis.call("HSET", KEYS[3], ARGV[4], ARGV[5])
redis.call("HSET", KEYS[4], ARGV[4], ARGV[6])
for i = 1, 4 do
    redis.call("EXPIRE", KEYS[i], ARGV[3])
end
return 1
