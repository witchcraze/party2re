local op, id, charPrefix, lobbyPrefix = ARGV[1], ARGV[2], ARGV[3], ARGV[4]
local ttl, now, idleAt, value = tonumber(ARGV[5]), tonumber(ARGV[6]), tonumber(ARGV[7]), ARGV[8]
local data = redis.call('GET', KEYS[1])
local state

if data and op == 'save' then
    return 'OK'
elseif data then
    state = cjson.decode(data)
    state.members = state.members or {}
elseif op == 'save' then
    state = cjson.decode(value)
elseif op == 'delete' or op == 'timeout' then
    redis.call('ZREM', KEYS[2], id)
    return op == 'delete' and 'OK' or false
else
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local function ownedKeys()
    local keys = {}
    for _, m in ipairs(state.members) do
        local key = charPrefix .. m.character_id
        if redis.call('GET', key) == id then
            table.insert(keys, key)
        end
    end
    return keys
end

local function hasOtherLobby(key)
    local owner = redis.call('GET', key)
    return owner and owner ~= id and redis.call('EXISTS', lobbyPrefix .. owner) == 1
end

if op == 'delete' then
    local keys = ownedKeys()
    local leaderKey = charPrefix .. state.party.leader_character_id
    if redis.call('GET', leaderKey) == id then
        table.insert(keys, leaderKey)
    end
    redis.call('ZREM', KEYS[2], id)
    redis.call('DEL', KEYS[1])
    for _, key in ipairs(keys) do
        redis.call('DEL', key)
    end
    return 'OK'
end

-- Index mutations go first so a failed publication leaves the room intact.
local function publish(removedKey, newMemberKey)
    local keys = {}
    -- A newly frozen roster already excludes members claimed by another live lobby.
    for _, m in ipairs(state.members) do
        local key = charPrefix .. m.character_id
        local owner = redis.call('GET', key)
        if not owner or owner == id or key == newMemberKey or op == 'save'
            or (op == 'timeout' and state.sleep_until_ms) then
            table.insert(keys, key)
        end
    end
    local clock = redis.call('TIME')
    local expires = clock[1] * 1000 + math.floor(clock[2] / 1000) + ttl
    local updated = cjson.encode(state)
    if state.party.status == 'completed' then
        redis.call('ZREM', KEYS[2], id)
    else
        redis.call('ZADD', KEYS[2], state.idle_at_ms, id)
    end
    redis.call('SET', KEYS[1], updated, 'PXAT', expires)
    for _, key in ipairs(keys) do
        redis.call('SET', key, id, 'PXAT', expires)
    end
    if removedKey then
        redis.call('DEL', removedKey)
    end
    return updated
end

if op == 'timeout' then
    if not state.idle_at_ms then
        -- Old lobbies used a sliding 30-minute TTL as the activity clock.
        local oldExpiry = redis.call('PEXPIRETIME', KEYS[1])
        if oldExpiry == -2 then
            redis.call('ZREM', KEYS[2], id)
            return false
        end
        state.idle_at_ms = oldExpiry > 0 and oldExpiry or idleAt
        publish()
    end
    if state.party.status ~= 'recruiting' then
        return redis.error_reply('ERR_PARTY_NOT_RECRUITING')
    end
    if state.idle_at_ms >= now then
        return false
    end
    local members = {}
    for _, member in ipairs(state.members) do
        if not hasOtherLobby(charPrefix .. member.character_id) then
            table.insert(members, member)
        end
    end
    local rosterChanged = #members ~= #state.members
    state.members = members
    if not state.sleep_until_ms then
        state.sleep_until_ms = tonumber(value)
        return publish()
    end
    local updated = cjson.encode(state)
    if rosterChanged then
        -- Keep excluded members out of later retries even after they leave the other lobby.
        redis.call('SET', KEYS[1], updated, 'KEEPTTL')
    end
    return updated
end

if state.sleep_until_ms then
    return redis.error_reply('ERR_PARTY_NOT_RECRUITING')
end

local removedKey, newMemberKey
if op == 'save' then
    for _, member in ipairs(state.members) do
        if hasOtherLobby(charPrefix .. member.character_id) then
            return redis.error_reply('ERR_ALREADY_IN_PARTY')
        end
    end
elseif op == 'add' then
    local member = cjson.decode(value)
    local key = charPrefix .. member.character_id
    if hasOtherLobby(key) then
        return redis.error_reply('ERR_ALREADY_IN_PARTY')
    end
    newMemberKey = key
    local found = false
    for i, existing in ipairs(state.members) do
        if existing.character_id == member.character_id then
            state.members[i] = member
            found = true
            break
        end
    end
    if not found then
        local maxMembers = tonumber(state.party.max_members)
        if not maxMembers or maxMembers <= 0 then
            maxMembers = 4
        end
        if #state.members >= maxMembers then
            return redis.error_reply('ERR_PARTY_FULL')
        end
        table.insert(state.members, member)
    end
elseif op == 'remove' then
    local remaining = {}
    for _, member in ipairs(state.members) do
        if member.character_id ~= value then
            table.insert(remaining, member)
        end
    end
    state.members = remaining
    if state.party.leader_character_id == value and #remaining > 0 then
        remaining[1].is_leader = true
        state.party.leader_character_id = remaining[1].character_id
    end
    local key = charPrefix .. value
    if redis.call('GET', key) == id then
        removedKey = key
    end
elseif op == 'update' then
    state.party = cjson.decode(value)
end

if op ~= 'add' then
    state.idle_at_ms = idleAt
elseif not state.idle_at_ms then
    local oldExpiry = redis.call('PEXPIRETIME', KEYS[1])
    state.idle_at_ms = oldExpiry > 0 and oldExpiry or idleAt
end
publish(removedKey, newMemberKey)
return 'OK'
