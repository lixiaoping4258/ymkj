package data

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// idGenerator 对应 extend/xlu/Id.php 的 gen()。
//
// ⚠️ 键名必须与 PHP **完全一致**：
//
//	la:id:gen     自增序号（raw int）
//	la:id:prefix  前缀 = 秒级时间戳（raw int）
//
// 前缀 `la:` 来自 ThinkPHP 的缓存 store 配置（config/cache.php 的 redis 段），
// 不是 Go 侧的 xtravel:go: 隔离前缀 —— 这两个键**是要共用的**，绝不能加隔离前缀。
//
// 实测确认它们是裸整数字符串（未被 TP 序列化），所以可以直接 GET/INCR/SET。
type idGenerator struct {
	cli *redis.Client
}

const (
	idGenKey    = "la:id:gen"
	idPrefixKey = "la:id:prefix"
	// idGenMaxSuffix 原实现里的阈值：suffix >= 99995 时换新的时间前缀
	idGenMaxSuffix = 99995
)

// NewIDGenerator 对应 Id::gen 所需的 Redis 客户端。
func NewIDGenerator(cli *redis.Client) biz.IDGenerator {
	return &idGenerator{cli: cli}
}

// Gen 逐行对应 Id::gen()。
//
// 与原实现的对应关系：
//
//	if (!$redisCache->has($key)) { $redisCache->delete($prefixKey); }
//	    -> Exists(id:gen)==0 时 Del(id:prefix)
//	$prefix = $redisCache->get($prefixKey);
//	if (!$prefix) { $prefix = time()+$offset; $redisCache->set($prefixKey, $prefix); }
//	    -> GET；空则用当前秒级时间戳 + offset 并 SET
//	$suffix = $redisCache->inc($key);
//	    -> INCR（Redis 保证原子；**不要自己加锁**，加锁反而会和 PHP 的 INCR 打架）
//	if ($suffix >= 99995) { set(key,0); set(prefixKey, time()+offset); }
//	    -> 同样的重置
//	return intval(sprintf('%s%05d', $prefix, $suffix));
//	    -> 字符串拼接后转 int64
func (g *idGenerator) Gen(ctx context.Context, offset int64) (int64, error) {
	// if (!$redisCache->has($key)) { $redisCache->delete($prefixKey); }
	n, err := g.cli.Exists(ctx, idGenKey).Result()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		if err := g.cli.Del(ctx, idPrefixKey).Err(); err != nil {
			return 0, err
		}
	}

	// $prefix = get($prefixKey);  if (!$prefix) { ... }
	prefix, err := g.cli.Get(ctx, idPrefixKey).Int64()
	if err == redis.Nil || prefix == 0 {
		prefix = time.Now().Unix() + offset
		if err := g.cli.Set(ctx, idPrefixKey, prefix, 0).Err(); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	// $suffix = inc($key)
	suffix, err := g.cli.Incr(ctx, idGenKey).Result()
	if err != nil {
		return 0, err
	}

	// if ($suffix >= 99995) { set(key,0); set(prefixKey, time()+$offset); }
	if suffix >= idGenMaxSuffix {
		prefix = time.Now().Unix() + offset
		pipe := g.cli.TxPipeline()
		pipe.Set(ctx, idGenKey, 0, 0)
		pipe.Set(ctx, idPrefixKey, prefix, 0)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
	}

	// return intval(sprintf('%s%05d', $prefix, $suffix))
	// 注意 %05d 是**零填充到 5 位**；超过 5 位时按实际位数拼接（PHP 行为一致）
	return strconv.ParseInt(fmt.Sprintf("%d%05d", prefix, suffix), 10, 64)
}
