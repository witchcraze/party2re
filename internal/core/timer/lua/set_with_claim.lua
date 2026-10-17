if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("SET", KEYS[2], "1", "EX", ARGV[2])
    return 1
else
    return 0
end
