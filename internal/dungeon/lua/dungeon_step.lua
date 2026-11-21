local function parse_int_field(hash_key, field)
    local raw = redis.call('HGET', hash_key, field)
    if not raw or raw == '' then
        return 0
    end
    if not string.match(raw, '^%-?%d+$') then
        return nil, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
    end
    return tonumber(raw)
end

local function decode_array_field(hash_key, field, elem_type)
    local raw = redis.call('HGET', hash_key, field)
    if not raw then
        return {}, false
    end
    local trimmed = string.match(raw, '^%s*(.-)%s*$')
    if trimmed == '' or trimmed == '[]' or trimmed == 'null' then
        return {}, false
    end
    if string.sub(trimmed, 1, 1) ~= '[' then
        return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field .. ' json'
    end
    local ok, decoded = pcall(cjson.decode, trimmed)
    if not ok or type(decoded) ~= 'table' then
        return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field .. ' json'
    end
    if elem_type then
        for _, v in ipairs(decoded) do
            if type(v) ~= elem_type then
                return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field .. ' json'
            end
        end
    end
    return decoded, true
end

local member_str_fields = {'character_id', 'name', 'job_id'}
local member_int_fields = {'level', 'current_hp', 'max_hp', 'current_mp', 'max_mp', 'accumulated_exp', 'accumulated_gold'}

local function is_int_number(v)
    return type(v) == 'number' and v == v and v ~= math.huge and v ~= -math.huge and math.floor(v) == v
end

local function validate_consumed_item(c)
    if type(c) ~= 'table' then
        return false
    end
    if c['id'] ~= nil and c['id'] ~= cjson.null and type(c['id']) ~= 'string' then
        return false
    end
    if c['instance_id'] ~= nil and c['instance_id'] ~= cjson.null and type(c['instance_id']) ~= 'string' then
        return false
    end
    if c['quantity'] ~= nil and c['quantity'] ~= cjson.null and not is_int_number(c['quantity']) then
        return false
    end
    return true
end

local function validate_member(m)
    if type(m) ~= 'table' then
        return false
    end
    for _, f in ipairs(member_str_fields) do
        local v = m[f]
        if v ~= nil and v ~= cjson.null and type(v) ~= 'string' then
            return false
        end
    end
    for _, f in ipairs(member_int_fields) do
        local v = m[f]
        if v ~= nil and v ~= cjson.null and not is_int_number(v) then
            return false
        end
    end
    local item_ids = m['item_ids']
    if item_ids ~= nil and item_ids ~= cjson.null then
        if type(item_ids) ~= 'table' or (#item_ids == 0 and next(item_ids) ~= nil) then
            return false
        end
        for _, id in ipairs(item_ids) do
            if type(id) ~= 'string' then
                return false
            end
        end
    end
    local consumed = m['consumed_items']
    if consumed ~= nil and consumed ~= cjson.null then
        if type(consumed) ~= 'table' or (#consumed == 0 and next(consumed) ~= nil) then
            return false
        end
        for _, c in ipairs(consumed) do
            if not validate_consumed_item(c) then
                return false
            end
        end
    end
    return true
end

local function decode_object_arg(raw, name, elem_kind)
    if not raw then
        return {}, false
    end
    local trimmed = string.match(raw, '^%s*(.-)%s*$')
    if trimmed == '' or trimmed == '{}' then
        return {}, false
    end
    if string.sub(trimmed, 1, 1) ~= '{' then
        return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
    end
    local ok, decoded = pcall(cjson.decode, trimmed)
    if not ok or type(decoded) ~= 'table' then
        return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
    end
    for k, v in pairs(decoded) do
        if type(k) ~= 'string' then
            return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
        end
        if elem_kind == 'int' then
            if not is_int_number(v) then
                return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
            end
        elseif elem_kind == 'consumed_items' then
            if type(v) ~= 'table' or (#v == 0 and next(v) ~= nil) then
                return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
            end
            for _, c in ipairs(v) do
                if not validate_consumed_item(c) then
                    return nil, false, 'ERR_EXPEDITION_CORRUPTED: invalid ' .. name .. ' json'
                end
            end
        end
    end
    if next(decoded) == nil then
        return {}, false
    end
    return decoded, true
end

local function validate_optional_timestamp(hash_key, field)
    local raw = redis.call('HGET', hash_key, field)
    if not raw or raw == '' then
        return nil
    end
    local yr_s, mon_s, day_s, hr_s, min_s, sec_s, rest = string.match(raw, '^(%d%d%d%d)%-(%d%d)%-(%d%d)T(%d%d):(%d%d):(%d%d)(.*)$')
    if not yr_s then
        return 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
    end
    local yr, mon, day = tonumber(yr_s), tonumber(mon_s), tonumber(day_s)
    local hr, min, sec = tonumber(hr_s), tonumber(min_s), tonumber(sec_s)
    if mon < 1 or mon > 12 or hr > 23 or min > 59 or sec > 59 then
        return 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
    end
    local days_in_month = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
    if mon == 2 and yr % 4 == 0 and (yr % 100 ~= 0 or yr % 400 == 0) then
        days_in_month[2] = 29
    end
    if day < 1 or day > days_in_month[mon] then
        return 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
    end
    if string.sub(rest, 1, 1) == '.' then
        local frac
        frac, rest = string.match(rest, '^(%.%d%d?%d?%d?%d?%d?%d?%d?%d?)(.*)$')
        if not frac then
            return 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
        end
    end
    if rest == 'Z' then
        return nil
    end
    local tz_h, tz_m = string.match(rest, '^[+-](%d%d):(%d%d)$')
    if not tz_h or tonumber(tz_h) > 23 or tonumber(tz_m) > 59 then
        return 'ERR_EXPEDITION_CORRUPTED: invalid ' .. field
    end
    return nil
end

local state_key = KEYS[1]
local rewards_key = KEYS[2]

local expected_expedition_id = ARGV[1]
local new_x = ARGV[2]
local new_y = ARGV[3]
local hp_delta = tonumber(ARGV[4]) or 0
local round_delta = tonumber(ARGV[5]) or 0
local exp_delta = tonumber(ARGV[6]) or 0
local gold_delta = tonumber(ARGV[7]) or 0
local reward_item = ARGV[8]
local now_timestamp = ARGV[9]
local ttl_seconds = tonumber(ARGV[10]) or 7200
local event_key = ARGV[11] or ''
local member_hp_deltas_str = ARGV[12] or '{}'
local member_mp_deltas_str = ARGV[13] or '{}'
local consumed_items_str = ARGV[14] or '{}'
local member_exp_deltas_str = ARGV[15] or '{}'
local member_gold_deltas_str = ARGV[16] or '{}'

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

local encounter = redis.call('HGET', state_key, 'encounter') or ''
if encounter ~= '' and ARGV[18] ~= 'true' then
    return redis.error_reply('ERR_COMBAT_ACTIVE')
end
local revision, rev_err = parse_int_field(state_key, 'revision')
if rev_err then
    return redis.error_reply(rev_err)
end
if ARGV[17] ~= '' and tonumber(ARGV[17]) ~= revision then
    return redis.error_reply('ERR_EXPEDITION_STATE_CHANGED')
end
revision = revision + 1
encounter = ARGV[19] or ''

-- 4. Calculate new HP and round
local current_hp, hp_err = parse_int_field(state_key, 'current_hp')
if hp_err then
    return redis.error_reply(hp_err)
end
local round, round_err = parse_int_field(state_key, 'round')
if round_err then
    return redis.error_reply(round_err)
end
round = round + round_delta

current_hp = current_hp + hp_delta

local _, variant_err = parse_int_field(state_key, 'map_variant')
if variant_err then
    return redis.error_reply(variant_err)
end
local _, limit_err = parse_int_field(state_key, 'round_limit')
if limit_err then
    return redis.error_reply(limit_err)
end
local started_err = validate_optional_timestamp(state_key, 'started_at')
if started_err then
    return redis.error_reply(started_err)
end

local members, has_members, members_err = decode_array_field(state_key, 'members', 'table')
if members_err then
    return redis.error_reply(members_err)
end
if has_members then
    for _, m in ipairs(members) do
        if not validate_member(m) then
            return redis.error_reply('ERR_EXPEDITION_CORRUPTED: invalid members json')
        end
    end
end
local members_str = '[]'

local deltas, has_deltas, hp_deltas_err = decode_object_arg(member_hp_deltas_str, 'member_hp_deltas', 'int')
if hp_deltas_err then
    return redis.error_reply(hp_deltas_err)
end
local decoded_mp, has_mp_deltas, mp_deltas_err = decode_object_arg(member_mp_deltas_str, 'member_mp_deltas', 'int')
if mp_deltas_err then
    return redis.error_reply(mp_deltas_err)
end
local decoded_items, has_consumed_items, consumed_err = decode_object_arg(consumed_items_str, 'consumed_items', 'consumed_items')
if consumed_err then
    return redis.error_reply(consumed_err)
end
local decoded_exp, has_exp_deltas, exp_deltas_err = decode_object_arg(member_exp_deltas_str, 'member_exp_deltas', 'int')
if exp_deltas_err then
    return redis.error_reply(exp_deltas_err)
end
local decoded_gold, has_gold_deltas, gold_deltas_err = decode_object_arg(member_gold_deltas_str, 'member_gold_deltas', 'int')
if gold_deltas_err then
    return redis.error_reply(gold_deltas_err)
end

if has_members then
    if has_deltas then
        local all_dead = true
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and deltas[cid] then
                local m_hp = (tonumber(m['current_hp']) or 0) + deltas[cid]
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

    if has_mp_deltas then
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and decoded_mp[cid] then
                local m_mp = (tonumber(m['current_mp']) or 0) + decoded_mp[cid]
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

    if has_consumed_items then
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and decoded_items[cid] then
                local existing = m['consumed_items'] or {}
                for _, item in ipairs(decoded_items[cid]) do
                    table.insert(existing, item)
                end
                m['consumed_items'] = existing
            end
        end
    end
end

if current_hp < 0 then
    current_hp = 0
end

-- An all-fallen party stays exploring until Escape; only exits finish a run.
local final_status = 'exploring'
if ARGV[20] and ARGV[20] ~= '' then
    final_status = ARGV[20]
end

-- 5. Check and record event consumption
local consumed_events, _, events_err = decode_array_field(state_key, 'consumed_events', 'string')
if events_err then
    return redis.error_reply(events_err)
end

-- Validate existing rewards fields before mutating state or rewards.
local _, exp_err = parse_int_field(rewards_key, 'exp')
if exp_err then
    return redis.error_reply(exp_err)
end
local _, gold_err = parse_int_field(rewards_key, 'gold')
if gold_err then
    return redis.error_reply(gold_err)
end
local items, _, items_err = decode_array_field(rewards_key, 'items', 'string')
if items_err then
    return redis.error_reply(items_err)
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
    else
        -- An accepted one-time event never regenerates its encounter.
        encounter = ''
    end
end

local new_events_str = '[]'
if #consumed_events > 0 then
    new_events_str = cjson.encode(consumed_events)
end

-- Apply member rewards only if event was consumed (one-time event idempotency)
if has_members and event_consumed == 1 then
    if has_exp_deltas then
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and decoded_exp[cid] and decoded_exp[cid] > 0 then
                m['accumulated_exp'] = (tonumber(m['accumulated_exp']) or 0) + decoded_exp[cid]
            end
        end
    elseif #members == 1 and exp_delta > 0 then
        members[1]['accumulated_exp'] = (tonumber(members[1]['accumulated_exp']) or 0) + exp_delta
    end

    if has_gold_deltas then
        for _, m in ipairs(members) do
            local cid = m['character_id']
            if cid and decoded_gold[cid] and decoded_gold[cid] > 0 then
                m['accumulated_gold'] = (tonumber(m['accumulated_gold']) or 0) + decoded_gold[cid]
            end
        end
    elseif #members == 1 and gold_delta > 0 then
        members[1]['accumulated_gold'] = (tonumber(members[1]['accumulated_gold']) or 0) + gold_delta
    end
end
if has_members then
    members_str = cjson.encode(members)
end

-- 6. Update state hash
redis.call('HMSET', state_key,
    'revision', tostring(revision),
    'encounter', encounter,
    'pos_x', new_x,
    'pos_y', new_y,
    'current_hp', tostring(current_hp),
    'round', tostring(round),
    'status', final_status,
    'consumed_events', new_events_str,
    'updated_at', now_timestamp,
    'members', members_str
)

-- A floor change carries the round and switches the limit with the layout.
local map_variant = tonumber(ARGV[21]) or 0
if map_variant > 0 then
    redis.call('HSET', state_key, 'map_variant', tostring(map_variant), 'round_limit', ARGV[22])
end

-- The map's own run state is replaced whole; the revision check above guards it.
if ARGV[24] == 'true' then
    redis.call('HSET', state_key, 'map_state', ARGV[23])
end

-- 7. Update rewards hash
if event_consumed == 0 then
    exp_delta = 0
    gold_delta = 0
    reward_item = ''
end
local total_exp = redis.call('HINCRBY', rewards_key, 'exp', exp_delta)
local total_gold = redis.call('HINCRBY', rewards_key, 'gold', gold_delta)

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
    new_x,
    new_y,
    tostring(current_hp),
    tostring(round),
    tostring(total_exp),
    tostring(total_gold),
    new_items_str,
    now_timestamp,
    dungeon_id,
    started_at,
    redis.call('HGET', state_key, 'party_id') or '',
    redis.call('HGET', state_key, 'party_name') or '',
    members_str,
    new_events_str,
    tostring(event_consumed),
    encounter,
    tostring(revision),
    redis.call('HGET', state_key, 'map_variant') or '',
    redis.call('HGET', state_key, 'round_limit') or '',
    redis.call('HGET', state_key, 'map_state') or ''
}
