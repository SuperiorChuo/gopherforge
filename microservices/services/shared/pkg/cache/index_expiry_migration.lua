-- Run prepare BEFORE deploying/rolling back any legacy cache writers.
-- Run activate ONLY AFTER every writer supports index lifecycle maintenance.
-- This changes index metadata only; no sessions, permissions or tokens are deleted.
local marker = 'cache:index:expiry-ready:v1'
local indexes = {'user:permissions:index', 'user:roles:index', 'user:permheader:index'}
if ARGV[1] == 'prepare' then
    redis.call('DEL', marker)
    for _, index in ipairs(indexes) do
        redis.call('PERSIST', index)
    end
    return 'prepared'
elseif ARGV[1] == 'activate' then
    redis.call('SET', marker, '1')
    -- Existing payloads live for at most one hour. Never shorten a longer TTL.
    for _, index in ipairs(indexes) do
        local ttl = redis.call('PTTL', index)
        if ttl ~= -2 and ttl < 3660000 then
            redis.call('PEXPIRE', index, 3660000)
        end
    end
    return 'activated'
end
return redis.error_reply('expected prepare or activate')
