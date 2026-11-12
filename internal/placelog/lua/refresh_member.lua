-- KEYS[1] joined, KEYS[2] expiry, KEYS[3] profile
-- ARGV[1] now, ARGV[2] expires_at, ARGV[3] ttl seconds, ARGV[4] character ID, ARGV[5] profile JSON
-- Purge entries that expired before now (one at exactly now+ttl is still listed),
-- then keep the arrival time of a listed member and refresh its expiry and profile.
local gone = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", "(" .. ARGV[1])
for _, m in ipairs(gone) do
    redis.call("ZREM", KEYS[1], m)
    redis.call("HDEL", KEYS[3], m)
end
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", "(" .. ARGV[1])
redis.call("ZADD", KEYS[1], "NX", ARGV[1], ARGV[4])
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[4])
redis.call("HSET", KEYS[3], ARGV[4], ARGV[5])
for i = 1, 3 do
    redis.call("EXPIRE", KEYS[i], ARGV[3])
end
return 1
