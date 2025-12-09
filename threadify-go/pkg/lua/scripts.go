package lua

const SetIfNotExists = `
if redis.call('exists', KEYS[1]) == 0 then
    redis.call('set', KEYS[1], ARGV[1])
    return 1
else
    return 0
end
`

const GetAndDelete = `
local value = redis.call('get', KEYS[1])
if value then
    redis.call('del', KEYS[1])
end
return value
`

const IncrementWithExpiry = `
local current = redis.call('incr', KEYS[1])
if current == 1 and ARGV[2] then
    redis.call('expire', KEYS[1], tonumber(ARGV[2]))
end
return current
`

const Upsert = `
local exists = redis.call('exists', KEYS[1])
redis.call('set', KEYS[1], ARGV[1])
if exists == 1 then
    return 'updated'
else
    return 'created'
end
`

const CompareAndSwap = `
local current = redis.call('get', KEYS[1])
if current == ARGV[1] then
    redis.call('set', KEYS[1], ARGV[2])
    return 1
else
    return 0
end
`
