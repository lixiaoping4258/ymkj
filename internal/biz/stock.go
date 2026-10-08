package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/market/PurchaseController::lookAll
//   app/api/logic/warehouse/GoodsLogic::lookAll

// StockLookAll 是 lookAll 的返回结构 {state, num}。
type StockLookAll struct {
	State int32 // 1 正常；0 表示有释放任务正在处理中
	Num   int64 // 可释放的库存数量
}

// StockRepo 库存数据访问。
type StockRepo interface {
	// CountUnlockedStock 对应
	// WarehouseDetail::where(['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0])->count()
	CountUnlockedStock(ctx context.Context, userID uint64) (int64, error)
	// UnlockInProgress 对应 cache(GoodsLogicUnlock::getCacheKey($userId)) 是否有值。
	//
	// ⚠️ 这个标记由 PHP 侧的释放任务用 `cache()` 写入，
	// 而 ThinkPHP 的缓存带自己的前缀（la:）和序列化格式，
	// Go 侧用的是隔离键空间（xtravel:go:），**迁移期读不到它**。
	// 所以迁移期内 state 会恒为 1（不会因为释放任务进行中而变成 0）。
	// 影响面：仅是前端"是否显示处理中"的提示，num 是正确的。
	// 等释放任务也迁到 Go 之后，两边共用 Go 的缓存，这个标记自然就通了。
	UnlockInProgress(ctx context.Context, userID uint64) (bool, error)
}

const stockLookAllTTL = 10 * time.Second

// StockUsecase 库存用例。
type StockUsecase struct {
	repo  StockRepo
	cache Cache
	log   *log.Helper
}

func NewStockUsecase(repo StockRepo, cache Cache, logger log.Logger) *StockUsecase {
	return &StockUsecase{repo: repo, cache: cache, log: log.NewHelper(logger)}
}

// LookAll 对应 GoodsLogic::lookAll($userId)。
//
// 原实现：
//
//	$cacheKey = "user:stock:lookAll:$userId";
//	if ($ret = RedisLockService::get($cacheKey)) { return json_decode($ret, true); }
//	$data['state'] = 1;
//	$key = GoodsLogicUnlock::getCacheKey($userId);      // warehouse:unlock:goods:{userId}
//	$state = cache($key);
//	$where = ['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0];
//	$data['num'] = WarehouseDetail::where($where)->count();
//	if ($state) { $data['state'] = 0; }                 // 正有释放任务处理中
//	RedisLockService::set($cacheKey, json_encode($data), 10);
//	return $data;
//
// 注意 `if ($state)` 是 PHP 的真值判断：空串/0/空数组都算"没有任务"。
func (uc *StockUsecase) LookAll(ctx context.Context, userID uint64) (*StockLookAll, error) {
	cacheKey := fmt.Sprintf("user:stock:lookAll:%d", userID)

	if v, err := uc.cache.Get(ctx, cacheKey); err != nil {
		uc.log.WithContext(ctx).Warnf("读取库存汇总缓存失败: %v", err)
	} else if m, ok := v.(map[string]any); ok {
		return &StockLookAll{
			State: int32(toInt64(m["state"])),
			Num:   toInt64(m["num"]),
		}, nil
	}

	num, err := uc.repo.CountUnlockedStock(ctx, userID)
	if err != nil {
		return nil, err
	}
	inProgress, err := uc.repo.UnlockInProgress(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &StockLookAll{State: 1, Num: num}
	if inProgress {
		out.State = 0
	}

	// 与原实现一样缓存整个 {state, num}，10 秒
	if err := uc.cache.Set(ctx, cacheKey,
		map[string]any{"state": out.State, "num": out.Num}, stockLookAllTTL); err != nil {
		uc.log.WithContext(ctx).Warnf("写入库存汇总缓存失败: %v", err)
	}
	return out, nil
}

// toInt64 把 JSON 解码出来的数字（float64）或原生整数转成 int64。
func toInt64(v any) int64 {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	default:
		return 0
	}
}
