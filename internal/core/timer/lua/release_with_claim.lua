if redis.call("GET", KEYS[1]) == ARGV[1] then
    for i = 2, #KEYS do
        redis.call("DEL", KEYS[i])
    end
    return 1
else
    return 0
end
