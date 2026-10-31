if redis.call("GET", KEYS[1]) == ARGV[1] then
    local secs = tonumber(ARGV[2])
    if secs and secs > 0 then
        return redis.call("EXPIRE", KEYS[1], secs)
    else
        redis.call("PERSIST", KEYS[1])
        return 1
    end
else
    return 0
end
