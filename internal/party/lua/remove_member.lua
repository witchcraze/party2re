local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local charID = ARGV[1]

local remaining = {}
if type(state.members) == 'table' then
    for _, m in ipairs(state.members) do
        if m.character_id ~= charID then
            table.insert(remaining, m)
        end
    end
end
state.members = remaining

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[2])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

redis.call('DEL', KEYS[2])
redis.call('DEL', KEYS[3])

return 'OK'
