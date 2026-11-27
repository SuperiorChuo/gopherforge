package cache

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const cacheIndexBatchSize = 128
const cacheIndexGrace = time.Minute
const cacheIndexExpiryReadyKey = "cache:index:expiry-ready:v1"

// Keep the existing Set format so old and new service replicas can share cache
// indexes during a rolling upgrade. The cursor belongs to Redis, not a process:
// every writer advances the same sweep, checking payload existence atomically
// with removal. Redis expiration is the clock; application clock skew cannot
// remove a live member. SSCAN's COUNT is a work hint, not a cardinality limit.
//
// The cursor is small and expiring. Under OOM, losing sweep progress is safe:
// the next write restarts the sweep; live members are still checked atomically.
const maintainCacheIndexScript = `
local ttl = redis.call('PTTL', KEYS[1])
if ttl == -2 then
    redis.call('DEL', KEYS[2])
    return 0
end
local lifetime = tonumber(ARGV[1])
if ARGV[3] == '1' and redis.call('GET', KEYS[3]) ~= '1' then
    -- Legacy permission writers use SADD without renewing index expiration.
    -- Keep their indexes persistent until the coordinated activation step.
    redis.call('PERSIST', KEYS[1])
    ttl = lifetime
elseif ttl < lifetime then
    redis.call('PEXPIRE', KEYS[1], lifetime)
    ttl = lifetime
end
local cursor = redis.call('GET', KEYS[2]) or '0'
local page = redis.call('SSCAN', KEYS[1], cursor, 'COUNT', ARGV[2])
local removed = 0
for _, key in ipairs(page[2]) do
    if redis.call('EXISTS', key) == 0 then
        removed = removed + redis.call('SREM', KEYS[1], key)
    end
end
if page[1] == '0' or redis.call('EXISTS', KEYS[1]) == 0 then
    redis.call('DEL', KEYS[2])
else
    local saved = redis.pcall('SET', KEYS[2], page[1], 'PX', ttl)
    if type(saved) == 'table' and saved.err then
        if string.sub(saved.err, 1, 3) ~= 'OOM' then
            return redis.error_reply(saved.err)
        end
        redis.call('DEL', KEYS[2])
    end
end
return removed
`

// Queue after payload/index writes in the same transaction. lifetime must exceed
// the maximum payload TTL for this index, including entries from older replicas.
// Never shorten an existing index TTL: configuration can change between writes.
func maintainCacheIndex(ctx context.Context, pipe goredis.Pipeliner, index string, lifetime time.Duration, legacyPersistent bool) {
	pipe.Eval(ctx, maintainCacheIndexScript, []string{index, index + ":gc-cursor", cacheIndexExpiryReadyKey}, lifetime.Milliseconds(), cacheIndexBatchSize, legacyPersistent)
}

// Delete payloads before removing their memberships, atomically per batch. A
// concurrent writer either precedes a batch and is invalidated, or follows it
// and remains indexed; never DEL the entire index after a non-atomic snapshot.
const invalidateCacheIndexScript = `
local keys = redis.call('SRANDMEMBER', KEYS[1], tonumber(ARGV[1]))
if #keys > 0 then
    redis.call('DEL', unpack(keys))
    redis.call('SREM', KEYS[1], unpack(keys))
    for i = 3, #KEYS do
        redis.call('SREM', KEYS[i], unpack(keys))
    end
end
local remaining = redis.call('SCARD', KEYS[1])
if remaining == 0 then
    redis.call('DEL', KEYS[2])
end
return remaining
`

func invalidateCacheIndex(ctx context.Context, client RedisClient, index string, otherIndexes ...string) error {
	indexes := append([]string{index, index + ":gc-cursor"}, otherIndexes...)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		pipe := client.TxPipeline()
		remaining := pipe.Eval(ctx, invalidateCacheIndexScript, indexes, cacheIndexBatchSize)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		n, err := remaining.Int64()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}
