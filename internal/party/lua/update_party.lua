local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local partyObj = cjson.decode(ARGV[1])
state.party = partyObj

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[2])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

local status = ARGV[3]
if status == 'disbanded' or status == 'completed' then
    redis.call('ZREM', KEYS[2], ARGV[4])
end

return 'OK'
