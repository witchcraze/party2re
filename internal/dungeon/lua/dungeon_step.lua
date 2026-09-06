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

local final_status = 'exploring'
if current_hp <= 0 then
    current_hp = 0
    final_status = 'wiped_out'
elseif turns_remaining <= 0 then
    turns_remaining = 0
    final_status = 'wiped_out'
end

-- 5. Update state hash
redis.call('HMSET', state_key,
    'current_floor', new_floor,
    'pos_x', new_x,
    'pos_y', new_y,
    'current_hp', tostring(current_hp),
    'turns_remaining', tostring(turns_remaining),
    'status', final_status,
    'updated_at', now_timestamp
)

-- 6. Update rewards hash
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

-- 7. Refresh sliding TTL (2 hours)
redis.call('EXPIRE', state_key, ttl_seconds)
redis.call('EXPIRE', rewards_key, ttl_seconds)

-- 8. Fetch static identifiers for full object reconstruction
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
    started_at
}
