local revision = tonumber(ARGV[1])
local current = tonumber(redis.call('HGET', KEYS[1], 'revision') or '0')
if current > revision then return redis.error_reply('ERR_REVISION_CONFLICT') end
if current == revision then
  if redis.call('HGET', KEYS[1], 'fingerprint') ~= redis.sha1hex(ARGV[2]) or redis.call('HGET', KEYS[1], 'deadline') ~= ARGV[5] then
    return redis.error_reply('ERR_REVISION_CONFLICT')
  end
end
redis.call('HSET', KEYS[1], 'revision', ARGV[1], 'publication', ARGV[2], 'fingerprint', redis.sha1hex(ARGV[2]), 'status', ARGV[3], 'deadline', ARGV[5])
if ARGV[3] == 'active' then
  redis.call('HSET', KEYS[2], 'revision', ARGV[1], 'ledger', ARGV[4])
  redis.call('PEXPIREAT', KEYS[1], ARGV[5])
  redis.call('PEXPIREAT', KEYS[2], ARGV[5])
else
  redis.call('HDEL', KEYS[1], 'publication')
  redis.call('DEL', KEYS[2])
  redis.call('PEXPIREAT', KEYS[1], ARGV[5])
end
return 'OK'
