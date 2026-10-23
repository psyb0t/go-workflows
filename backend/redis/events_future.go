package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/psyb0t/ctxerrors"

	redis "github.com/redis/go-redis/v9"
)

func scheduleFutureEvents(ctx context.Context, rb *redisBackend) error {
	now := time.Now().UnixMilli()
	nowStr := strconv.FormatInt(now, 10)
	if _, err := futureEventsCmd.Run(ctx, rb.rdb, []string{
		rb.keys.futureEventsKey(),
	}, nowStr, rb.keys.prefix).Result(); err != nil && !errors.Is(err, redis.Nil) {
		return ctxerrors.Wrap(err, "checking future events")
	}

	return nil
}
