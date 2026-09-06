local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
if state.party and state.party.status == 'disbanded' then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local memberObj = cjson.decode(ARGV[1])
local charID = ARGV[2]
local maxMembers = 4
if state.party and state.party.max_members and tonumber(state.party.max_members) > 0 then
    maxMembers = tonumber(state.party.max_members)
end

if type(state.members) ~= 'table' then
    state.members = {}
end

local found = false
for i, m in ipairs(state.members) do
    if m.character_id == charID then
        state.members[i] = memberObj
        found = true
        break
    end
end

if not found then
    if #state.members >= maxMembers then
        return redis.error_reply('ERR_PARTY_FULL')
    end
    table.insert(state.members, memberObj)
end

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[4])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)
redis.call('SET', KEYS[2], ARGV[3], 'EX', lobbyTTL)

if ARGV[6] == '1' then
    local readyTTL = tonumber(ARGV[5])
    redis.call('SET', KEYS[3], '1', 'EX', readyTTL)
else
    redis.call('DEL', KEYS[3])
end

return 'OK'
