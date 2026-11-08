local cur = redis.call("GET", KEYS[1])
if cur == false then
    return 0
end
if string.match(cur, "^%s*$") ~= nil then
    return redis.error_reply("ERR casino character room mapping is invalid")
end
if cur == ARGV[1] then
    return redis.call("SET", KEYS[1], ARGV[1], "EX", tonumber(ARGV[2]))
end
return 0
