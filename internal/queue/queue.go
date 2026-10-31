// Package queue is a Redis-backed delay queue of delivery IDs.
//
// A single sorted set holds delivery IDs scored by the unix-millis time they
// become due. Workers atomically pop due members with a Lua script, so two
// workers never pop the same ID from Redis.
//
// Redis is NOT the source of truth: if an ID is lost (Redis restart, worker
// crash after pop), the Postgres reconciler re-enqueues any delivery whose
// next_attempt_at has passed and whose lease has expired.
package queue

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const dueKey = "webhooks:deliveries:due"

// popDue removes and returns up to ARGV[2] members with score <= ARGV[1].
var popDue = redis.NewScript(`
local ids = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])
if #ids > 0 then
  redis.call('ZREM', KEYS[1], unpack(ids))
end
return ids
`)

type Queue struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Queue { return &Queue{rdb: rdb} }

// Connect parses a redis:// URL and verifies connectivity.
func Connect(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}

// Schedule makes deliveryID due at t. Re-scheduling an existing ID updates its time.
func (q *Queue) Schedule(ctx context.Context, deliveryID string, t time.Time) error {
	return q.rdb.ZAdd(ctx, dueKey, redis.Z{Score: float64(t.UnixMilli()), Member: deliveryID}).Err()
}

// PopDue atomically claims up to limit delivery IDs that are due at or before now.
func (q *Queue) PopDue(ctx context.Context, now time.Time, limit int) ([]string, error) {
	return popDue.Run(ctx, q.rdb, []string{dueKey},
		strconv.FormatInt(now.UnixMilli(), 10), limit).StringSlice()
}
