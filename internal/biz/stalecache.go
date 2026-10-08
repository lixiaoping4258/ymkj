package biz

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件实现「逻辑过期 + 重建锁」的缓存模式（stale-while-revalidate）。
//
// 逐行对照原项目 app/api/controller/v1/market/PurchaseController::salesOn：
//
//	$params = get(); ksort($params);
//	$cacheKey = 'purchase:salesOn:' . md5(json_encode($params));
//	$lockKey  = 'lock:' . $cacheKey;
//	$cached   = RedisLockService::get($cacheKey);
//	$payload  = $cached ? json_decode($cached, true) : null;
//
//	if ($payload && $payload['expire_at'] > time()) {          // 逻辑未过期
//	    return json($payload['value']);
//	}
//	$token = RedisLockService::tryLock($lockKey, 5000);        // 重建锁
//	if ($token === false) {
//	    if ($payload) { return json($payload['value']); }      // 没抢到锁 -> 用旧值顶着
//	    return $this->dataLists(...);                          // 冷启动 -> 直查
//	}
//	try {
//	    $data = $this->dataLists(...);
//	    $value = $data->getData();
//	    RedisLockService::set($cacheKey, json_encode([
//	        'value' => $value, 'expire_at' => time() + 5,
//	    ]), 15);                                               // 物理 TTL 只兜底
//	    return json($value);
//	} finally {
//	    RedisLockService::unlock($lockKey, $token);
//	}
//
// 这套模式的意义：缓存过期的那一瞬间，**只有一个请求去回源**，
// 其余请求立刻拿到旧值返回。既不会有惊群（thundering herd）打垮数据库，
// 也不会有请求排队等待。
//
// 两个 TTL 的分工要理解清楚：
//   - 逻辑过期（5s）决定"数据算不算新鲜"，到点后旧值仍然可读
//   - 物理 TTL（15s）只是兜底，保证即使没人回源，脏数据也不会永久留在缓存里
//
// ⚠️ 迁移注意：**不是所有接口都用这套**。同一个控制器里
// `salesOut` 用的就是简单版（直接查缓存、过期就回源、没有锁和逻辑过期）。
// 迁移时逐字保持各自的行为，不要"顺手统一"。

// StaleCache 提供 stale-while-revalidate 的读取语义。
type StaleCache struct {
	cache  Cache
	locker Locker
	log    *log.Helper
}

func NewStaleCache(cache Cache, locker Locker, logger log.Logger) *StaleCache {
	return &StaleCache{cache: cache, locker: locker, log: log.NewHelper(logger)}
}

// StaleOptions 描述一次 stale-while-revalidate 读取。
type StaleOptions struct {
	// Key 缓存键（不含前缀，前缀由 Cache 实现加）
	Key string
	// LockKey 重建锁的键。原实现是 'lock:' . $cacheKey
	LockKey string
	// LogicalTTL 逻辑过期时长（原实现 5s）
	LogicalTTL time.Duration
	// PhysicalTTL 物理 TTL，只做兜底（原实现 15s）
	PhysicalTTL time.Duration
	// LockTTL 重建锁的持有时长（原实现 5000ms）
	LockTTL time.Duration
}

// stalePayload 是缓存里存的结构，与原实现的 JSON 形状一致。
type stalePayload struct {
	Value    json.RawMessage `json:"value"`
	ExpireAt int64           `json:"expire_at"`
}

// BuildFunc 回源构造函数。
type BuildFunc func(ctx context.Context) (json.RawMessage, error)

// Serve 按 stale-while-revalidate 语义取数据。
//
// 返回值可能是：新鲜缓存 / 陈旧缓存（没抢到锁时）/ 刚构建的新值。
func (c *StaleCache) Serve(ctx context.Context, opt StaleOptions, build BuildFunc) (json.RawMessage, error) {
	payload := c.load(ctx, opt.Key)

	// 逻辑未过期：直接用
	if payload != nil && payload.ExpireAt > time.Now().Unix() {
		return payload.Value, nil
	}

	// 该重建了。只有抢到锁的人去干活
	token, acquired, err := c.locker.TryLock(ctx, opt.LockKey, opt.LockTTL)
	if err != nil {
		// 原实现这里会让异常冒到 controller 的 catch，最终返回 fail。
		// 但"锁服务抖动"不该让整个只读接口失败 —— 降级为直接回源，
		// 最坏情况是短暂退化成无保护（可能多几次回源），比 500 好。
		c.log.WithContext(ctx).Warnf("重建锁获取失败，降级为直接回源: %v", err)
		return build(ctx)
	}

	if !acquired {
		// 没抢到锁：有旧值就用旧值顶着，**绝不排队等待**
		if payload != nil {
			return payload.Value, nil
		}
		// 冷启动兜底：没有任何旧值可顶，只能直查。
		// 注意原实现这里**不写缓存** —— 让抢到锁的那个请求去写，
		// 避免冷启动时一堆请求各写一遍。
		return build(ctx)
	}

	// 抢到锁：回源 + 写缓存，最后一定解锁
	defer func() {
		if uerr := c.locker.Unlock(ctx, opt.LockKey, token); uerr != nil {
			c.log.WithContext(ctx).Warnf("释放重建锁失败: %v", uerr)
		}
	}()

	value, err := build(ctx)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 {
		// 空结果不写缓存，避免把"查不到"缓存成结果
		return value, nil
	}

	physical := opt.PhysicalTTL
	if physical <= 0 {
		physical = opt.LogicalTTL * 3
	}
	body, merr := json.Marshal(stalePayload{
		Value:    value,
		ExpireAt: time.Now().Add(opt.LogicalTTL).Unix(),
	})
	if merr == nil {
		if serr := c.cache.Set(ctx, opt.Key, string(body), physical); serr != nil {
			c.log.WithContext(ctx).Warnf("写入 SWR 缓存失败: %v", serr)
		}
	}
	return value, nil
}

// load 读缓存并解析。任何异常都当成"没有缓存"，不影响主流程。
func (c *StaleCache) load(ctx context.Context, key string) *stalePayload {
	v, err := c.cache.Get(ctx, key)
	if err != nil {
		c.log.WithContext(ctx).Warnf("读取 SWR 缓存失败: %v", err)
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	var p stalePayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		// 脏数据：当作没有，顺手让物理 TTL 自然淘汰
		c.log.WithContext(ctx).Warnf("SWR 缓存内容无法解析，忽略: %v", err)
		return nil
	}
	if len(p.Value) == 0 {
		return nil
	}
	return &p
}
