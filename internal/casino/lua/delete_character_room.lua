local cur = redis.call("GET", KEYS[1])
if cur == false then
    return 0
end
if string.match(cur, "^%s*$") ~= nil then
    return redis.error_reply("ERR casino character room mapping is invalid")
end
if cur == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
