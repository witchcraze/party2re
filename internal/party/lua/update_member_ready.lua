local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local charID = ARGV[1]
local ready = (ARGV[2] == '1')

local found = false
if type(state.members) == 'table' then
    for i, m in ipairs(state.members) do
        if m.character_id == charID then
            state.members[i].ready_state = ready
            found = true
            break
        end
    end
end

if not found then
    return redis.error_reply('ERR_CHAR_NOT_IN_PARTY')
end

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[3])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

if ready then
    local readyTTL = tonumber(ARGV[4])
    redis.call('SET', KEYS[2], '1', 'EX', readyTTL)
else
    redis.call('DEL', KEYS[2])
end

return 'OK'
