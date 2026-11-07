if redis.call("GET", KEYS[1]) ~= ARGV[1] then
    return 0
end
if ARGV[2] == "" then
    redis.call("DEL", KEYS[1])
else
    redis.call("SET", KEYS[1], ARGV[2], "EX", tonumber(ARGV[3]))
end
return 1
