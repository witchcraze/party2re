local session_key = KEYS[1]
local rewards_key = KEYS[2]

local expected_session_id = ARGV[1]
local surviving_hp = ARGV[2]
local exp_delta = tonumber(ARGV[3]) or 0
local gold_delta = tonumber(ARGV[4]) or 0
local reward_item = ARGV[5]
local now_timestamp = ARGV[6]
local ttl_seconds = tonumber(ARGV[7]) or 7200

-- 1. Check existence of active challenge session key
if redis.call('EXISTS', session_key) == 0 then
    return redis.error_reply("ERR_SESSION_NOT_FOUND: challenge session not found")
end

-- 2. Verify expected session ID
local stored_id = redis.call('HGET', session_key, 'session_id')
if stored_id ~= expected_session_id then
    return redis.error_reply("ERR_SESSION_ID_MISMATCH: challenge session id mismatch")
end

-- 3. Verify status is active
local status = redis.call('HGET', session_key, 'status')
if status ~= 'active' then
    return redis.error_reply("ERR_SESSION_NOT_ACTIVE: challenge session is not active")
end

-- 4. Advance round counter and update surviving HP
local new_round = redis.call('HINCRBY', session_key, 'current_round', 1)
redis.call('HMSET', session_key,
    'character_current_hp', surviving_hp,
    'updated_at', now_timestamp
)

-- 5. Update rewards hash
local total_exp = redis.call('HINCRBY', rewards_key, 'exp', exp_delta)
local total_gold = redis.call('HINCRBY', rewards_key, 'gold', gold_delta)

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

-- 6. Refresh sliding TTL (2 hours)
redis.call('EXPIRE', session_key, ttl_seconds)
redis.call('EXPIRE', rewards_key, ttl_seconds)

-- 7. Fetch static identifiers for full session reconstruction
local tier_id = redis.call('HGET', session_key, 'tier_id') or ''
local created_at = redis.call('HGET', session_key, 'created_at') or ''

return {
    tostring(new_round),
    surviving_hp,
    tostring(total_exp),
    tostring(total_gold),
    new_items_str,
    now_timestamp,
    tier_id,
    created_at
}
