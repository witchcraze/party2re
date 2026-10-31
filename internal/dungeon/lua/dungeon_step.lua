local state_key = KEYS[1]
local rewards_key = KEYS[2]

local expected_expedition_id = ARGV[1]
local new_floor = ARGV[2]
local new_x = ARGV[3]
local new_y = ARGV[4]
local hp_delta = tonumber(ARGV[5]) or 0
local turns_delta = tonumber(ARGV[6]) or 0
local exp_delta = tonumber(ARGV[7]) or 0
local gold_delta = tonumber(ARGV[8]) or 0
local medals_delta = tonumber(ARGV[9]) or 0
local reward_item = ARGV[10]
local now_timestamp = ARGV[11]
local ttl_seconds = tonumber(ARGV[12]) or 7200
local event_key = ARGV[13] or ''
local member_hp_deltas_str = ARGV[14] or '{}'
local member_mp_deltas_str = ARGV[15] or '{}'
local consumed_items_str = ARGV[16] or '{}'
local member_exp_deltas_str = ARGV[17] or '{}'
local member_gold_deltas_str = ARGV[18] or '{}'

-- 1. Check existence of active expedition state key
if redis.call('EXISTS', state_key) == 0 then
    return redis.error_reply("ERR_EXPEDITION_NOT_FOUND: active expedition not found")
end

-- 2. Verify expected expedition ID
local stored_id = redis.call('HGET', state_key, 'expedition_id')
if stored_id ~= expected_expedition_id then
    return redis.error_reply("ERR_EXPEDITION_ID_MISMATCH: expedition id mismatch")
end

-- 3. Verify status is active exploring
local status = redis.call('HGET', state_key, 'status')
if status ~= 'exploring' then
    return redis.error_reply("ERR_EXPEDITION_NOT_ACTIVE: expedition is not in exploring status")
end

-- 4. Calculate new HP and turns budget
local current_hp = tonumber(redis.call('HGET', state_key, 'current_hp') or '0')
local turns_remaining = tonumber(redis.call('HGET', state_key, 'turns_remaining') or '0')

current_hp = current_hp + hp_delta
turns_remaining = turns_remaining + turns_delta

local members_str = redis.call('HGET', state_key, 'members') or '[]'
local members = {}
local has_members = false
if members_str ~= '' and members_str ~= '[]' then
    local ok, decoded = pcall(cjson.decode, members_str)
    if ok and type(decoded) == 'table' then
        members = decoded
        has_members = true
    end
end

if has_members then
    local deltas = {}
    local has_deltas = false
    if member_hp_deltas_str ~= '' and member_hp_deltas_str ~= '{}' then
        local ok, decoded_deltas = pcall(cjson.decode, member_hp_deltas_str)
        if ok and type(decoded_deltas) == 'table' then
            deltas = decoded_deltas
            has_deltas = true
        end
    end

    if has_deltas then
        local all_dead = true
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and deltas[cid] then
                local delta = tonumber(deltas[cid]) or 0
                local m_hp = (tonumber(m['current_hp']) or 0) + delta
                if m_hp < 0 then
                    m_hp = 0
                end
                m['current_hp'] = m_hp
            end
            if (tonumber(m['current_hp']) or 0) > 0 then
                all_dead = false
            end
        end
        if #members > 0 and all_dead then
            current_hp = 0
        end
    elseif #members == 1 and hp_delta ~= 0 then
        local m_hp = (tonumber(members[1]['current_hp']) or 0) + hp_delta
        if m_hp < 0 then
            m_hp = 0
        end
        members[1]['current_hp'] = m_hp
    end

    if member_mp_deltas_str ~= '' and member_mp_deltas_str ~= '{}' then
        local ok, decoded_mp = pcall(cjson.decode, member_mp_deltas_str)
        if ok and type(decoded_mp) == 'table' then
            for _, m in ipairs(members) do
                local cid = m['character_id']
                if cid and decoded_mp[cid] then
                    local delta = tonumber(decoded_mp[cid]) or 0
                    local m_mp = (tonumber(m['current_mp']) or 0) + delta
                    if m_mp < 0 then
                        m_mp = 0
                    end
                    local max_mp = tonumber(m['max_mp']) or 0
                    if max_mp > 0 and m_mp > max_mp then
                        m_mp = max_mp
                    end
                    m['current_mp'] = m_mp
                end
            end
        end
    end

    if consumed_items_str ~= '' and consumed_items_str ~= '{}' then
        local ok, decoded_items = pcall(cjson.decode, consumed_items_str)
        if ok and type(decoded_items) == 'table' then
            for _, m in ipairs(members) do
                local cid = m['character_id']
                if cid and decoded_items[cid] and type(decoded_items[cid]) == 'table' then
                    local existing = m['consumed_items'] or {}
                    for _, item in ipairs(decoded_items[cid]) do
                        table.insert(existing, item)
                    end
                    m['consumed_items'] = existing
                end
            end
        end
    end

    if member_exp_deltas_str ~= '' and member_exp_deltas_str ~= '{}' then
        local ok, decoded_exp = pcall(cjson.decode, member_exp_deltas_str)
        if ok and type(decoded_exp) == 'table' then
            for _, m in ipairs(members) do
                local cid = m['character_id']
                if cid and decoded_exp[cid] then
                    local delta = tonumber(decoded_exp[cid]) or 0
                    if delta > 0 then
                        m['accumulated_exp'] = (tonumber(m['accumulated_exp']) or 0) + delta
                    end
                end
            end
        end
    elseif #members == 1 and exp_delta > 0 then
        members[1]['accumulated_exp'] = (tonumber(members[1]['accumulated_exp']) or 0) + exp_delta
    end

    if member_gold_deltas_str ~= '' and member_gold_deltas_str ~= '{}' then
        local ok, decoded_gold = pcall(cjson.decode, member_gold_deltas_str)
        if ok and type(decoded_gold) == 'table' then
            for _, m in ipairs(members) do
                local cid = m['character_id']
                if cid and decoded_gold[cid] then
                    local delta = tonumber(decoded_gold[cid]) or 0
                    if delta > 0 then
                        m['accumulated_gold'] = (tonumber(m['accumulated_gold']) or 0) + delta
                    end
                end
            end
        end
    elseif #members == 1 and gold_delta > 0 then
        members[1]['accumulated_gold'] = (tonumber(members[1]['accumulated_gold']) or 0) + gold_delta
    end
    members_str = cjson.encode(members)
end

if current_hp < 0 then
    current_hp = 0
end

local final_status = 'exploring'
if has_members and #members > 0 then
    local all_dead = true
    for _, m in ipairs(members) do
        if (tonumber(m['current_hp']) or 0) > 0 then
            all_dead = false
            break
        end
    end
    if all_dead then
        current_hp = 0
        final_status = 'wiped_out'
    end
else
    if current_hp <= 0 then
        current_hp = 0
        final_status = 'wiped_out'
    end
end
if turns_remaining <= 0 then
    turns_remaining = 0
    final_status = 'wiped_out'
end

-- 5. Check and record event consumption
local events_str = redis.call('HGET', state_key, 'consumed_events')
local consumed_events = {}
if events_str and events_str ~= '' and events_str ~= '[]' then
    local ok, decoded = pcall(cjson.decode, events_str)
    if ok and type(decoded) == 'table' then
        consumed_events = decoded
    end
end

local event_consumed = 1
if event_key ~= '' then
    for _, k in ipairs(consumed_events) do
        if k == event_key then
            event_consumed = 0
            break
        end
    end
    if event_consumed == 1 then
        table.insert(consumed_events, event_key)
    end
end

local new_events_str = '[]'
if #consumed_events > 0 then
    new_events_str = cjson.encode(consumed_events)
end

-- 6. Update state hash
redis.call('HMSET', state_key,
    'current_floor', new_floor,
    'pos_x', new_x,
    'pos_y', new_y,
    'current_hp', tostring(current_hp),
    'turns_remaining', tostring(turns_remaining),
    'status', final_status,
    'consumed_events', new_events_str,
    'updated_at', now_timestamp,
    'members', members_str
)

-- 7. Update rewards hash
if event_consumed == 0 then
    exp_delta = 0
    gold_delta = 0
    medals_delta = 0
    reward_item = ''
end
local total_exp = redis.call('HINCRBY', rewards_key, 'exp', exp_delta)
local total_gold = redis.call('HINCRBY', rewards_key, 'gold', gold_delta)
local total_medals = redis.call('HINCRBY', rewards_key, 'medals', medals_delta)

local items_str = redis.call('HGET', rewards_key, 'items')
local items = {}
if items_str and items_str ~= '' and items_str ~= '[]' then
    local ok, decoded = pcall(cjson.decode, items_str)
    if ok and type(decoded) == 'table' then
        items = decoded
    end
end

if reward_item and reward_item ~= '' then
    table.insert(items, reward_item)
end

local new_items_str = '[]'
if #items > 0 then
    new_items_str = cjson.encode(items)
end
redis.call('HSET', rewards_key, 'items', new_items_str)

-- 8. Refresh sliding TTL (2 hours)
redis.call('EXPIRE', state_key, ttl_seconds)
redis.call('EXPIRE', rewards_key, ttl_seconds)

-- 9. Fetch static identifiers for full object reconstruction
local dungeon_id = redis.call('HGET', state_key, 'dungeon_id') or ''
local started_at = redis.call('HGET', state_key, 'started_at') or ''

return {
    final_status,
    new_floor,
    new_x,
    new_y,
    tostring(current_hp),
    tostring(turns_remaining),
    tostring(total_exp),
    tostring(total_gold),
    tostring(total_medals),
    new_items_str,
    now_timestamp,
    dungeon_id,
    started_at,
    redis.call('HGET', state_key, 'party_id') or '',
    redis.call('HGET', state_key, 'party_name') or '',
    members_str,
    new_events_str,
    tostring(event_consumed)
}
