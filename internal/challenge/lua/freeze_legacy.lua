local id = redis.call('HGET', KEYS[1], 'session_id')
if not id then return 'OK' end
if id ~= ARGV[1] then return redis.error_reply('ERR_SESSION_ID_MISMATCH') end
local status = redis.call('HGET', KEYS[1], 'status')
if status ~= 'active' and status ~= 'migrating' then
  return redis.error_reply('ERR_SESSION_NOT_ACTIVE')
end
redis.call('HSET', KEYS[1], 'status', 'migrating')
return 'OK'
