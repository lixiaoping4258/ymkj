package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/market/PurchaseController::purchaseInfo
//   app/api/logic/market/MarketListPurchaseLogic::getArchive

// ArchiveRow 是档案基础信息（原 getArchive 的返回）。
type ArchiveRow struct {
	ID           int64
	CollectionID string // varchar，PHP 输出字符串
	Name         string
	Images       *string // DB 里的 json 字符串
	Issuer       string
	PlatformName string
	PlatformID   int64
}

// ArchiveRepo 档案数据访问。
type ArchiveRepo interface {
	FindArchive(ctx context.Context, id int64) (*ArchiveRow, error)
	// FindSaleArchive 对应 SaleLogic::getArchive 的查询。
	// 与 FindArchive 的列**不同**：少了 collection_id 和 platform_id。
	FindSaleArchive(ctx context.Context, id int64) (*SaleArchiveRow, error)
	// FindSalesArchiveID 对应 MarketListSales::where(['id'=>$id])->field('id,archive_id')。
	FindSalesArchiveID(ctx context.Context, salesID int64) (int64, bool, error)
}

// SaleArchiveRow 对应 SaleLogic::getArchive 的返回（5 个字段）。
type SaleArchiveRow struct {
	ID           int64
	Name         string
	Images       *string
	Issuer       string
	PlatformName string
}

// RawCache 是「裸 Redis、无前缀、纯 JSON」的缓存端口。
//
// 原项目有两套缓存机制，迁移处理方式相反：
//   - ThinkPHP 的 cache()：带 la: 前缀 + TP 序列化 -> **必须隔离**，Go 读不了
//   - RedisLockService：直接操作 phpredis，无前缀、值就是 json_encode 的结果
//     -> **可以和 PHP 共用同一把键**，两边互相受益于对方的缓存
//
// getArchive 用的是后者，所以这里的实现不加前缀。
type RawCache interface {
	Get(ctx context.Context, key string) (json.RawMessage, bool, error)
	Set(ctx context.Context, key string, val json.RawMessage, ttl time.Duration) error
}

const archiveCacheTTL = 600 * time.Second

// ArchiveUsecase 档案信息用例。
type ArchiveUsecase struct {
	repo  ArchiveRepo
	cache RawCache
	// isolated 是带前缀的隔离缓存，供原项目里用 ThinkPHP cache() 的场景使用。
	// 与 cache 的区别见 RawCache 的说明。
	isolated Cache
	log      *log.Helper
}

func NewArchiveUsecase(repo ArchiveRepo, cache RawCache, isolated Cache, logger log.Logger) *ArchiveUsecase {
	return &ArchiveUsecase{repo: repo, cache: cache, isolated: isolated, log: log.NewHelper(logger)}
}

const saleArchiveCacheTTL = 3600 * time.Second

// GetSaleArchive 对应 SaleLogic::getArchive($id)。
//
// 原实现与本文件的 GetArchive **长得很像但有三处不同**，逐字照抄是关键：
//
//	$cacheKey = sprintf('archive:%s', $id);
//	$ret = cache($cacheKey);                      // ← TP 的 cache()，物理键 la:archive:{id}
//	if ($ret) { return $ret; }
//	$archive = AppArchive::alias('app_archive')
//	    ->where(['app_archive.id'=>$id, 'app_archive.state'=>1])
//	    ->field('app_archive.id,app_archive.name,app_archive.images,app_archive.issuer,app.name platform_name')
//	    ->join('app','app.id=app_archive.app_id')->findOrEmpty();   // ← 只有 5 个字段
//	if ($archive->isEmpty()) { self::setError('未找到档案信息'); return false; }
//	$ret = $archive->toArray();
//	if (is_string($ret['images'])) { $ret['images'] = json_decode($ret['images'], true); }
//	cache($cacheKey, $ret, 3600);                  // ← TTL 3600，不是 600
//	return $ret;
//
// 因为用的是 TP 的 cache()，**Go 侧读不到 PHP 写的那份**，
// 所以这里用带前缀的隔离缓存（isolated），不能像 GetArchive 那样共用裸键。
func (uc *ArchiveUsecase) GetSaleArchive(ctx context.Context, id int64) (map[string]any, error) {
	cacheKey := fmt.Sprintf("archive:%d", id)

	if v, err := uc.isolated.Get(ctx, cacheKey); err != nil {
		uc.log.WithContext(ctx).Warnf("读取秒转档案缓存失败: %v", err)
	} else if m, ok := v.(map[string]any); ok {
		return m, nil
	}

	row, err := uc.repo.FindSaleArchive(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	out := map[string]any{
		"id":            row.ID,
		"name":          row.Name,
		"images":        decodeImages(row.Images),
		"issuer":        row.Issuer,
		"platform_name": row.PlatformName,
	}
	if err := uc.isolated.Set(ctx, cacheKey, out, saleArchiveCacheTTL); err != nil {
		uc.log.WithContext(ctx).Warnf("写入秒转档案缓存失败: %v", err)
	}
	return out, nil
}

// FindSalesArchiveID 对应 MarketListSalesLogic::findSales：
// 由秒转列表 id 找到对应的 archive_id。
// 返回 (0, false, nil) 表示记录不存在。
func (uc *ArchiveUsecase) FindSalesArchiveID(ctx context.Context, salesID int64) (int64, bool, error) {
	return uc.repo.FindSalesArchiveID(ctx, salesID)
}

// GetArchive 对应 MarketListPurchaseLogic::getArchive($id)。
//
// 原实现：
//
//	$cacheKey = sprintf('archive:%s', $id);
//	if ($ret = RedisLockService::get($cacheKey)) { return json_decode($ret, true); }
//	$archive = AppArchive::alias('app_archive')
//	    ->where(['app_archive.id'=>$id, 'app_archive.state'=>1])
//	    ->field('app_archive.id,app_archive.collection_id,app_archive.name,app_archive.images,
//	            app_archive.issuer,app.name platform_name,app.id platform_id')
//	    ->join('app','app.id=app_archive.app_id')->findOrEmpty();
//	if ($archive->isEmpty()) { setError('未找到档案信息'); return false; }
//	$ret = $archive->toArray();
//	if (is_string($ret['images'])) { $ret['images'] = json_decode($ret['images'], true); }
//	RedisLockService::set($cacheKey, json_encode($ret), 600);
//	return $ret;
//
// 返回 (nil, nil) 表示查不到，由 service 层翻成 fail('未找到档案信息')。
func (uc *ArchiveUsecase) GetArchive(ctx context.Context, id int64) (map[string]any, error) {
	cacheKey := fmt.Sprintf("archive:%d", id)

	if raw, ok, err := uc.cache.Get(ctx, cacheKey); err != nil {
		uc.log.WithContext(ctx).Warnf("读取档案缓存失败: %v", err)
	} else if ok {
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			return m, nil
		}
		uc.log.WithContext(ctx).Warnf("档案缓存内容无法解析，回源: key=%s", cacheKey)
	}

	row, err := uc.repo.FindArchive(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	out := map[string]any{
		"id":            row.ID,
		"collection_id": row.CollectionID,
		"name":          row.Name,
		"images":        decodeImages(row.Images),
		"issuer":        row.Issuer,
		"platform_name": row.PlatformName,
		"platform_id":   row.PlatformID,
	}

	if b, merr := json.Marshal(out); merr == nil {
		if serr := uc.cache.Set(ctx, cacheKey, b, archiveCacheTTL); serr != nil {
			uc.log.WithContext(ctx).Warnf("写入档案缓存失败: %v", serr)
		}
	}
	return out, nil
}
