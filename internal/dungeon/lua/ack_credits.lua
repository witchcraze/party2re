-- KEYS[1]: run state; ARGV[1]: expedition id; ARGV[2]: JSON array of settled credit IDs.
if redis.call('HGET', KEYS[1], 'expedition_id') ~= ARGV[1] then
    return 0
end
local existing = redis.call('HGET', KEYS[1], 'credits')
if not existing or existing == '' then
    return 0
end
local settled = {}
for _, id in ipairs(cjson.decode(ARGV[2])) do
    settled[id] = true
end
local kept = {}
for _, c in ipairs(cjson.decode(existing)) do
    if not settled[c['id']] then
        table.insert(kept, c)
    end
end
if #kept == 0 then
    redis.call('HDEL', KEYS[1], 'credits')
else
    redis.call('HSET', KEYS[1], 'credits', cjson.encode(kept))
end
return 1
