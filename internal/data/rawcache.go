package data

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// rawCache 是**不加前缀**的 biz.Cache 实现，对应原项目的 RedisLockService。
//
// 为什么要单独一个实现：项目里有两类缓存，键空间策略相反。
//
//	redisCache（带 xtravel:go: 前缀）—— 用于原项目用 ThinkPHP cache() 的地方。
//	  TP 的缓存带 la: 前缀和自己的序列化格式，Go 读不了，只能自己管一份。
//
//	rawCache（**无前缀**）—— 用于原项目用 RedisLockService 的地方。
//	  那是直接操作 phpredis，键就是原文、值就是 json_encode 的结果，
//	  所以 Go 和 PHP **可以共用同一把键**，互相受益于对方预热的缓存。
//
// ⚠️ 共用的前提是值的形状逐字一致。salesOn/salesOut 缓存的是整个信封的数据部分，
// 由 ListData.ToJSON() 组装，已与 PHP 实测比对过（第 4 轮 purchase 列表）。
type rawCache struct {
	cli *redis.Client
}

func NewRawCache(cli *redis.Client) biz.RawKeyCache {
	return &rawCache{cli: cli}
}

func (c *rawCache) Get(ctx context.Context, key string) (any, error) {
	s, err := c.cli.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (c *rawCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Second
	}
	return c.cli.Set(ctx, key, val, ttl).Err()
}
