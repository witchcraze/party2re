-- KEYS: stage claim, new lobby, lobbies index, [claimed lobby]
-- ARGV: expected claim, party ID, lobby JSON, lobby TTL, index score
local current = redis.call('GET', KEYS[1]) or ''
if current ~= ARGV[1] then
    return -1
end
if KEYS[4] and redis.call('EXISTS', KEYS[4]) == 1 then
    return 0
end
redis.call('SET', KEYS[2], ARGV[3], 'EX', tonumber(ARGV[4]))
redis.call('SET', KEYS[1], ARGV[2])
redis.call('ZADD', KEYS[3], tonumber(ARGV[5]), ARGV[2])
return 1
