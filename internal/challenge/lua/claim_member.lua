local current = redis.call('GET', KEYS[1])
if current and current ~= ARGV[1] and current ~= ARGV[2] then
  return redis.error_reply('ERR_ACTIVE_SESSION_EXISTS')
end
redis.call('SET', KEYS[1], ARGV[1])
return 'OK'
