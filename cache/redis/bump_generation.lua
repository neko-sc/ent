local now = redis.call('TIME')
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local token = string.format('%.0f', math.max(current + 1, tonumber(now[1]) * 1000000 + tonumber(now[2])))
redis.call('SET', KEYS[1], token, unpack(ARGV))
return 1
