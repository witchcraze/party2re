if redis.call("GET", KEYS[1]) == ARGV[1] then
    local secs = tonumber(ARGV[2])
    if secs and secs > 0 then
        redis.call("SET", KEYS[2], "1", "EX", secs)
    else
        redis.call("SET", KEYS[2], "1")
    end
    return 1
else
    return 0
end
