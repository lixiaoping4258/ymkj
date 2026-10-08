package data

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// redisCache 用 Redis 实现 biz.Cache。
//
// ⚠️ 键前缀隔离：原项目用 ThinkPHP 的 cache()，同一个逻辑键
// （比如 trade:market:periods）在 Redis 里存的是 TP 自己的序列化格式。
// 如果 Go 和 PHP 共用同一个键，会互相读出对方看不懂的数据。
//
// 迁移期两套系统并行，所以这里默认加 xtravel:go: 前缀，
// 键空间完全隔离，互不污染。等 PHP 下线后再考虑合并。
type redisCache struct {
	cli    *redis.Client
	prefix string
}

// NewRedisCache 只用 cli 一个参数，是为了让 wire 能自动装配 ——
// 一旦多一个 string 参数，wire 不知道那个 string 从哪来。
// 前缀通过环境变量 REDIS_CACHE_PREFIX 覆盖。
func NewRedisCache(cli *redis.Client) biz.Cache {
	prefix := os.Getenv("REDIS_CACHE_PREFIX")
	if prefix == "" {
		prefix = "xtravel:go:"
	}
	return &redisCache{cli: cli, prefix: prefix}
}

func (c *redisCache) k(key string) string { return c.prefix + key }

func (c *redisCache) Get(ctx context.Context, key string) (any, error) {
	b, err := c.cli.Get(ctx, c.k(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil // 未命中，与 PHP cache() 返回 null 对齐
	}
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		// 脏数据：当作未命中，顺手删掉，避免每次都走到这里
		_ = c.cli.Del(ctx, c.k(key)).Err()
		return nil, nil
	}
	return v, nil
}

func (c *redisCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if val == nil {
		// PHP 里 cache($k, null) 等同于没存。保持一致。
		return nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return c.cli.Set(ctx, c.k(key), b, ttl).Err()
}
