-- KEYS[1] joined, KEYS[2] expiry, KEYS[3] profile
-- ARGV[1] now, ARGV[2] expires_at, ARGV[3] ttl seconds, ARGV[4] character ID, ARGV[5] profile JSON
-- Purge entries that expired before now (one at exactly now+ttl is still listed),
-- then keep the arrival position of a listed member and refresh its expiry and profile.
-- A newcomer's arrival score follows the last one, so same-second arrivals keep their order.
local gone = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", "(" .. ARGV[1])
for _, m in ipairs(gone) do
    redis.call("ZREM", KEYS[1], m)
    redis.call("HDEL", KEYS[3], m)
end
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", "(" .. ARGV[1])
if not redis.call("ZSCORE", KEYS[1], ARGV[4]) then
    local last = redis.call("ZRANGE", KEYS[1], -1, -1, "WITHSCORES")
    redis.call("ZADD", KEYS[1], (tonumber(last[2]) or 0) + 1, ARGV[4])
end
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[4])
redis.call("HSET", KEYS[3], ARGV[4], ARGV[5])
for i = 1, 3 do
    redis.call("EXPIRE", KEYS[i], ARGV[3])
end
return 1
