local until_ms = tonumber(ARGV[1])
local now_ms = tonumber(ARGV[2])
if until_ms <= now_ms then
    return 0
end
local cur = redis.call("PEXPIRETIME", KEYS[1])
if cur == -1 then
    return 0
end
if cur >= until_ms then
    if redis.call("GET", KEYS[1]) == "1" then
        return 0
    end
    return redis.call("SET", KEYS[1], "1", "PXAT", cur)
end
return redis.call("SET", KEYS[1], "1", "PXAT", until_ms)
