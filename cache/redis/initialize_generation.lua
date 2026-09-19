local current = redis.call('GET', KEYS[1])
if not current then
    local now = redis.call('TIME')
    current = string.format('%.0f', tonumber(now[1]) * 1000000 + tonumber(now[2]))
    redis.call('SET', KEYS[1], current, 'NX')
end
return current
