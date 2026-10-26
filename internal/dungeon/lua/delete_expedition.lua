-- State and rewards share the owner's hash slot; the identity check and delete are indivisible.
if redis.call('HGET', KEYS[1], 'character_id') ~= ARGV[1]
    or redis.call('HGET', KEYS[1], 'expedition_id') ~= ARGV[2] then
    return 0
end
return redis.call('DEL', KEYS[1], KEYS[2])
