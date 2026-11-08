-- KEYS[1] log; ARGV[1] limit, ARGV[2] ttl seconds, ARGV[3..] rows (oldest first)
for i = 3, #ARGV do
    redis.call("LPUSH", KEYS[1], ARGV[i])
end
redis.call("LTRIM", KEYS[1], 0, tonumber(ARGV[1]) - 1)
redis.call("EXPIRE", KEYS[1], tonumber(ARGV[2]))
return 1
