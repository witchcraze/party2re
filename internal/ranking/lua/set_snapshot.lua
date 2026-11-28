-- KEYS[1]: snapshot cache key (party2:ranking:snapshot:<category> or party2:ranking:snapshot:pair:<leader>:<dependent>)
-- ARGV[1]: encoded cachedEntry JSON
-- ARGV[2]: TTL in milliseconds (positive integer)
-- ARGV[3]: write mode ("refresh" or "backfill")
-- ARGV[4]: candidate timestamp ("2006-01-02T15:04:05.000000000Z")
-- ARGV[5]: entry kind ("single" or "pair")
local function is_valid_snapshot_data(raw)
	if type(raw) ~= 'string' or raw == '' then
		return false
	end
	local ok, decoded = pcall(cjson.decode, raw)
	return ok and type(decoded) == 'table'
end

local cur = redis.call('GET', KEYS[1])
if cur then
	local ok, existing = pcall(cjson.decode, cur)
	if ok and type(existing) == 'table' and type(existing['ts']) == 'string' and existing['ts'] ~= '' then
		local valid = false
		if ARGV[5] == 'pair' then
			local p = existing['pair']
			if type(p) == 'table' and type(p['leader']) == 'table' and type(p['dependent']) == 'table'
				and is_valid_snapshot_data(p['leader']['snapshot_data'])
				and is_valid_snapshot_data(p['dependent']['snapshot_data']) then
				valid = true
			end
		else
			local s = existing['snapshot']
			if type(s) == 'table' and is_valid_snapshot_data(s['snapshot_data']) then
				valid = true
			end
		end
		if valid then
			if ARGV[3] == 'backfill' then
				if ARGV[4] <= existing['ts'] then
					return 0
				end
			else
				if ARGV[4] < existing['ts'] then
					return 0
				end
			end
		end
	end
end

redis.call('SET', KEYS[1], ARGV[1], 'PX', tonumber(ARGV[2]))
return 1
