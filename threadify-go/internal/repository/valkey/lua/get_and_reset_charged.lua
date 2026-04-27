local balance = redis.call('GET', KEYS[1])
local charged = redis.call('GET', KEYS[2])
redis.call('SET', KEYS[2], '0')
return {balance, charged}
