local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local current = redis.call('INCR', key)
if current == 1 then
    redis.call('PEXPIRE', key, window_ms)
end
local ttl = redis.call('PTTL', key)
if ttl < 0 then
    redis.call('PEXPIRE', key, window_ms)
    ttl = window_ms
end

return {current, ttl}
