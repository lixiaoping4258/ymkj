package biz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// stale-while-revalidate 的核心价值是**缓存过期瞬间只有一个请求回源**，
// 其余立刻拿到旧值。这几条用例把四条分支都钉死，尤其是
// 「没抢到锁时用旧值顶着」和「冷启动不写缓存」这两条最容易写错的。

type fakeLocker struct {
	// 是否允许抢到锁
	acquire bool
	// TryLock 是否报错
	failTry bool
	locked  atomic.Int32
	unlocks atomic.Int32
	// 记录抢锁时用的 ttl
	gotTTL time.Duration
}

func (f *fakeLocker) TryLock(_ context.Context, key string, ttl time.Duration) (string, bool, error) {
	f.gotTTL = ttl
	if f.failTry {
		return "", false, errors.New("redis down")
	}
	if !f.acquire {
		return "", false, nil
	}
	f.locked.Add(1)
	return "tok", true, nil
}

func (f *fakeLocker) Unlock(_ context.Context, key, token string) error {
	f.unlocks.Add(1)
	return nil
}

func newStaleCache(cache Cache, locker Locker) *StaleCache {
	return NewStaleCache(cache, locker, log.NewStdLogger(io.Discard))
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func opts() StaleOptions {
	return StaleOptions{
		Key:         "k",
		LockKey:     "lock:k",
		LogicalTTL:  5 * time.Second,
		PhysicalTTL: 15 * time.Second,
		LockTTL:     5 * time.Second,
	}
}

// 逻辑未过期：直接返回缓存，不抢锁、不回源
func TestStaleCache_FreshReturnsCached(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: true}
	sc := newStaleCache(cache, locker)

	body, _ := json.Marshal(stalePayload{Value: raw(`{"a":1}`), ExpireAt: time.Now().Add(time.Minute).Unix()})
	cache.m["k"] = string(body)

	var called int32
	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		atomic.AddInt32(&called, 1)
		return raw(`{"a":2}`), nil
	})
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("应返回缓存值，实际 %s", got)
	}
	if called != 0 {
		t.Fatal("逻辑未过期时不应回源")
	}
	if locker.locked.Load() != 0 {
		t.Fatal("逻辑未过期时不应抢锁")
	}
}

// 逻辑过期 + 抢到锁：回源、写缓存（带新的 expire_at）、解锁
func TestStaleCache_ExpiredWithLockRebuilds(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: true}
	sc := newStaleCache(cache, locker)

	stale, _ := json.Marshal(stalePayload{Value: raw(`{"a":1}`), ExpireAt: time.Now().Add(-time.Second).Unix()})
	cache.m["k"] = string(stale)

	before := time.Now().Unix()
	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		return raw(`{"a":2}`), nil
	})
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if string(got) != `{"a":2}` {
		t.Fatalf("应返回新构建的值，实际 %s", got)
	}
	if locker.unlocks.Load() != 1 {
		t.Fatalf("应解锁一次，实际 %d", locker.unlocks.Load())
	}
	// 缓存里应写入新值 + 新的逻辑过期时间（约 now+5）
	s, ok := cache.m["k"].(string)
	if !ok {
		t.Fatal("应当写入缓存")
	}
	var p stalePayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatalf("缓存内容无法解析: %v", err)
	}
	if string(p.Value) != `{"a":2}` {
		t.Fatalf("缓存里的值应为新值，实际 %s", p.Value)
	}
	if p.ExpireAt < before+4 || p.ExpireAt > before+6 {
		t.Fatalf("逻辑过期时间应约为 now+5s，实际 %d (before=%d)", p.ExpireAt, before)
	}
}

// 逻辑过期 + 没抢到锁 + 有旧值：**用旧值顶着，绝不回源**
func TestStaleCache_ExpiredNoLockServesStale(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: false}
	sc := newStaleCache(cache, locker)

	stale, _ := json.Marshal(stalePayload{Value: raw(`{"old":true}`), ExpireAt: time.Now().Add(-time.Second).Unix()})
	cache.m["k"] = string(stale)

	var called int32
	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		atomic.AddInt32(&called, 1)
		return raw(`{"new":true}`), nil
	})
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if string(got) != `{"old":true}` {
		t.Fatalf("没抢到锁时应返回旧值，实际 %s", got)
	}
	if called != 0 {
		t.Fatal("没抢到锁时不应回源 —— 这是这套模式的核心")
	}
}

// 逻辑过期 + 没抢到锁 + 无旧值（冷启动）：只能直查，且**不写缓存**
func TestStaleCache_ExpiredNoLockColdStart(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: false}
	sc := newStaleCache(cache, locker)

	var called int32
	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		atomic.AddInt32(&called, 1)
		return raw(`{"cold":true}`), nil
	})
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if string(got) != `{"cold":true}` {
		t.Fatalf("冷启动应直查，实际 %s", got)
	}
	if called != 1 {
		t.Fatalf("冷启动应回源一次，实际 %d", called)
	}
	if _, exists := cache.m["k"]; exists {
		t.Fatal("冷启动兜底路径不应写缓存（让抢到锁的请求去写）")
	}
}

// 锁服务故障：降级为直接回源，而不是让只读接口 500
func TestStaleCache_LockErrorDegrades(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{failTry: true}
	sc := newStaleCache(cache, locker)

	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		return raw(`{"fallback":true}`), nil
	})
	if err != nil {
		t.Fatalf("锁故障不应让接口失败: %v", err)
	}
	if string(got) != `{"fallback":true}` {
		t.Fatalf("应降级直查，实际 %s", got)
	}
}

// 空结果不写缓存：避免把"查不到"缓存成结果
func TestStaleCache_EmptyResultNotCached(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: true}
	sc := newStaleCache(cache, locker)

	_, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if _, exists := cache.m["k"]; exists {
		t.Fatal("空结果不应写缓存")
	}
}

// 脏缓存不应让接口失败
func TestStaleCache_CorruptPayloadIgnored(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: true}
	sc := newStaleCache(cache, locker)
	cache.m["k"] = "not-json"

	got, err := sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		return raw(`{"ok":true}`), nil
	})
	if err != nil {
		t.Fatalf("脏缓存不应让接口失败: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("应回源，实际 %s", got)
	}
}

// 锁的 TTL 要透传给 Locker（原实现是 5000ms，配错会导致锁过早释放）
func TestStaleCache_LockTTLPassthrough(t *testing.T) {
	cache := newFakeTokenCacheForWL()
	locker := &fakeLocker{acquire: true}
	sc := newStaleCache(cache, locker)

	_, _ = sc.Serve(context.Background(), opts(), func(context.Context) (json.RawMessage, error) {
		return raw(`1`), nil
	})
	if locker.gotTTL != 5*time.Second {
		t.Fatalf("锁 TTL 应为 5s，实际 %v", locker.gotTTL)
	}
}
