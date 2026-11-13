-- KEYS[1] joined, KEYS[2] expiry, KEYS[3] profile, KEYS[4] token
-- ARGV[1] character ID, ARGV[2] expected membership token
-- Delete the member entry only when its stored token still matches the superseded write,
-- so delayed cleanup from an earlier Refresh or Leave cannot erase a newer replacement.
local cur = redis.call("HGET", KEYS[4], ARGV[1])
if ARGV[2] == "" then
    if cur then
        return 0
    end
elseif cur ~= ARGV[2] then
    return 0
end
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("ZREM", KEYS[2], ARGV[1])
redis.call("HDEL", KEYS[3], ARGV[1])
redis.call("HDEL", KEYS[4], ARGV[1])
return 1
