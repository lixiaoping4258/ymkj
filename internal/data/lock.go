package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// redisLocker 对应 app/common/service/RedisLockService.php 的锁部分。
//
// ⚠️ **这里刻意不加 xtravel:go: 前缀**，与缓存的处理相反。
//
// 理由：缓存里存的是 ThinkPHP 的 tag 结构 + 序列化数据，Go 读不了，必须隔离；
// 而锁只是一个 `SET key <随机token> NX PX <ms>`，两边格式完全一致。
// 更要紧的是——**共用锁才是对的**：用户防重复点击的意义就在于
// 无论请求落到 PHP 还是 Go，第二次都必须被挡住。加了前缀等于两把锁，
// 迁移期这个保护就失效了。
//
// 键名与 PHP 端逐字一致，例如 click:user:exchange:check:61089517。
type redisLocker struct {
	cli *redis.Client
}

func NewRedisLocker(cli *redis.Client) biz.Locker {
	return &redisLocker{cli: cli}
}

// unlockLua 对应原项目的 UNLOCK_LUA：只有持有者才能释放。
//
// 不做 CAS 直接 DEL 的话，A 的锁到点自动过期、B 抢到之后，
// A 的 Unlock 会把 B 的锁删掉 —— 经典的锁误删 bug。
var unlockLua = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

func (l *redisLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if ttl <= 0 {
		ttl = 3 * time.Second
	}
	token, err := newLockToken()
	if err != nil {
		return "", false, err
	}
	// SetNX + TTL 等价于 PHP 的 $redis->set($key, $token, ['NX','PX'=>$ttlMs])
	ok, err := l.cli.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

func (l *redisLocker) Unlock(ctx context.Context, key, token string) error {
	if token == "" {
		return nil
	}
	return unlockLua.Run(ctx, l.cli, []string{key}, token).Err()
}

func newLockToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
