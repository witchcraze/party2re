local hpStr = redis.call('GET', KEYS[1])
if not hpStr then
    return cjson.encode({
        status = "not_found",
        actual_damage = 0,
        remaining_hp = 0,
        killer_id = ""
    })
end

local status = redis.call('GET', KEYS[2])
if status == "defeated" or status == "settled" then
    local killer = redis.call('GET', KEYS[4]) or ""
    return cjson.encode({
        status = "already_dead",
        actual_damage = 0,
        remaining_hp = 0,
        killer_id = killer
    })
end

local currentHp = tonumber(hpStr)
if currentHp <= 0 then
    local killer = redis.call('GET', KEYS[4]) or ""
    return cjson.encode({
        status = "already_dead",
        actual_damage = 0,
        remaining_hp = 0,
        killer_id = killer
    })
end

local incomingDmg = tonumber(ARGV[2])
if incomingDmg <= 0 then
    return cjson.encode({
        status = "hit",
        actual_damage = 0,
        remaining_hp = currentHp,
        killer_id = ""
    })
end

local actualDmg = math.min(currentHp, incomingDmg)
local newHp = currentHp - actualDmg

redis.call('SET', KEYS[1], tostring(newHp))
redis.call('HINCRBY', KEYS[3], ARGV[1], actualDmg)

local ttl = tonumber(ARGV[3])
if ttl and ttl > 0 then
    redis.call('EXPIRE', KEYS[1], ttl)
    redis.call('EXPIRE', KEYS[2], ttl)
    redis.call('EXPIRE', KEYS[3], ttl)
    redis.call('EXPIRE', KEYS[4], ttl)
    redis.call('EXPIRE', KEYS[5], ttl)
end

if newHp == 0 then
    redis.call('SET', KEYS[2], "defeated")
    redis.call('SET', KEYS[4], ARGV[1])
    if ttl and ttl > 0 then
        redis.call('EXPIRE', KEYS[2], ttl)
        redis.call('EXPIRE', KEYS[4], ttl)
    end
    return cjson.encode({
        status = "killed",
        actual_damage = actualDmg,
        remaining_hp = 0,
        killer_id = ARGV[1]
    })
else
    return cjson.encode({
        status = "hit",
        actual_damage = actualDmg,
        remaining_hp = newHp,
        killer_id = ""
    })
end
