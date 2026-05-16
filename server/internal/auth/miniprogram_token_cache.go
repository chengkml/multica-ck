package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const miniprogramTokenCachePrefix = "mul:auth:mp:"

type MiniprogramTokenCache struct {
	rdb *redis.Client
}

func NewMiniprogramTokenCache(rdb *redis.Client) *MiniprogramTokenCache {
	if rdb == nil {
		return nil
	}
	return &MiniprogramTokenCache{rdb: rdb}
}

func miniprogramTokenCacheKey(hash string) string { return miniprogramTokenCachePrefix + hash }

func (c *MiniprogramTokenCache) Get(ctx context.Context, openidHash string) (userID string, ok bool) {
	if c == nil {
		return "", false
	}
	v, err := c.rdb.Get(ctx, miniprogramTokenCacheKey(openidHash)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("miniprogram_token_cache: get failed; falling back to DB", "error", err)
		}
		return "", false
	}
	return v, true
}

func (c *MiniprogramTokenCache) Set(ctx context.Context, openidHash, userID string, ttl time.Duration) {
	if c == nil || ttl <= 0 {
		return
	}
	if err := c.rdb.Set(ctx, miniprogramTokenCacheKey(openidHash), userID, ttl).Err(); err != nil {
		slog.Warn("miniprogram_token_cache: set failed", "error", err)
	}
}

func (c *MiniprogramTokenCache) Invalidate(ctx context.Context, openidHash string) {
	if c == nil {
		return
	}
	if err := c.rdb.Del(ctx, miniprogramTokenCacheKey(openidHash)).Err(); err != nil {
		slog.Warn("miniprogram_token_cache: invalidate failed; entry will expire on TTL", "error", err)
	}
}
