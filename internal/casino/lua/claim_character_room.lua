local cur = redis.call("GET", KEYS[1])
if cur ~= false and string.match(cur, "^%s*$") ~= nil then
    return redis.error_reply("ERR casino character room mapping is invalid")
end
if cur == false or cur == ARGV[1] or (ARGV[2] ~= "" and cur == ARGV[2]) then
    return redis.call("SET", KEYS[1], ARGV[1], "EX", tonumber(ARGV[3]))
end
return redis.error_reply("ERR casino character already in room")
