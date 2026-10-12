package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// GoodsLookUsecase 对应 app/api/logic/warehouse/GoodsLogic.php::goodsLook。
type GoodsLookUsecase struct {
	repo   GoodsLookRepo
	cache  RawCache // ⚠️ 裸共享缓存（RedisLockService），**不走 TP 的 cache()**
	appRep GoodsLookArchiveRepo
}

func NewGoodsLookUsecase(
	repo GoodsLookRepo, cache RawCache, appRep GoodsLookArchiveRepo,
) *GoodsLookUsecase {
	return &GoodsLookUsecase{repo: repo, cache: cache, appRep: appRep}
}

// GoodsLookArchiveRepo 档案列表。
type GoodsLookArchiveRepo interface {
	// ListArchives 对应：
	//
	//	AppArchive::where(["state"=>1] [+ "id"=>$archiveId])
	//	    ->field("app_id,images as goods_image,name as goods_name,
	//	            collection_id,id as archive_id")->select();
	ListArchives(ctx context.Context, archiveID string, hasArchiveID bool) ([]map[string]any, error)
}

// GoodsLookRepo 分组统计可导入数量。
type GoodsLookRepo interface {
	// CountByCollection 对应：
	//
	//	WarehouseDetail::where(['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0])
	//	    ->whereIn('collection_id', $collectionIds)
	//	    ->field('collection_id, COUNT(*) as num')->group('collection_id')
	//	    ->select()->toArray();
	//
	// 返回 collection_id -> num
	CountByCollection(ctx context.Context, userID uint32, collectionIDs []int64) (map[int64]int64, error)
}

// GoodsLookCacheTTL 对应 `RedisLockService::set($cacheKey, json_encode($data), 5)`。
const GoodsLookCacheTTL = 5 * time.Second

// GoodsLookCacheKey 对应 `"user:stock:goodsLook:$userId"`。
//
// ⚠️ 走 RedisLockService（裸 phpredis、无前缀、plain JSON）
//
//	-> **与 PHP 共用同一把键，值是可直接解析的 JSON**（清单第 15 条）。
func GoodsLookCacheKey(userID uint32) string {
	return fmt.Sprintf("user:stock:goodsLook:%d", userID)
}

// Get 逐字对应 GoodsLogic::goodsLook($userId, $params)。
//
// 原文：
//
//	$cacheKey = "user:stock:goodsLook:$userId";
//	$ret = RedisLockService::get($cacheKey);
//	if ($ret) { return json_decode($ret, true); }
//
//	$whereAppArchive = ["state" => 1];
//	if (isset($params['archive_id'])) { $whereAppArchive['id'] = trim($params['archive_id']); }
//	$res = AppArchive::where($whereAppArchive)
//	    ->field("app_id,images as goods_image,name as goods_name,collection_id,id as archive_id")
//	    ->select();
//	if ($res->isEmpty()) { return []; }
//	$data = $res->toArray();
//
//	$numMap = [];
//	$collectionIds = array_unique(array_column($data, 'collection_id'));
//	if (!empty($collectionIds)) {
//	    $numRows = WarehouseDetail::where(['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0])
//	        ->whereIn('collection_id', $collectionIds)
//	        ->field('collection_id, COUNT(*) as num')->group('collection_id')->select()->toArray();
//	    $numMap = array_column($numRows, 'num', 'collection_id');
//	}
//	foreach ($data as $k => $v) {
//	    $data[$k]['num'] = $numMap[$v['collection_id']] ?? 0;
//	    if ($data[$k]['goods_image']) { $data[$k]['goods_image'] = json_decode($v['goods_image'], true); }
//	}
//	RedisLockService::set($cacheKey, json_encode($data), 5);
//	return $data;
//
// ⚠️⚠️ 三处容易做错：
//
//  1. **`goods_image` 的假值分支是"保持原值"**，不是赋空数组：
//     if ($data[$k]['goods_image']) { json_decode(...) }
//     没有 else！所以空串仍是 `""`、NULL 仍是 `null`。
//     **这与同域的 StockLists 不同**（那边是 `else { [] }`）——清单第 13 条。
//
//  2. **`isset($params['archive_id'])`** 用的是 isset 而不是 !empty ——
//     传了空串也会**加上 `id = ”` 的条件**（会查不到任何行）。
//     Go 侧用 `hasArchiveID` 显式区分"没传"与"传了空串"。
//
//  3. **返回值是顶层数组**（不是 `{lists,count,...}`）——原实现直接返回 `$data`。
//
// ⚠️ 缓存是 **RedisLockService**（裸 phpredis、无前缀、plain JSON）
//
//	-> 与 PHP 共用，Go 读到的是可直接 json.Unmarshal 的数组。
func (uc *GoodsLookUsecase) Get(
	ctx context.Context, userID uint32, archiveID string, hasArchiveID bool,
) (json.RawMessage, error) {
	cacheKey := GoodsLookCacheKey(userID)
	if raw, ok, err := uc.cache.Get(ctx, cacheKey); err != nil {
		// 缓存读失败不阻断（原文也没接 RedisLockService 的返回值）
		_ = raw
	} else if ok && len(raw) > 0 {
		return raw, nil
	}

	data, err := uc.appRep.ListArchives(ctx, archiveID, hasArchiveID)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		// `$res->isEmpty()` -> 返回空数组（**且不写缓存** —— 原文在这之前就 return 了）
		return json.RawMessage("[]"), nil
	}

	// array_unique(array_column($data,'collection_id'))
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(data))
	for _, row := range data {
		if cid, ok := toInt64Any(row["collection_id"]); ok && !seen[cid] {
			seen[cid] = true
			ids = append(ids, cid)
		}
	}
	numMap := map[int64]int64{}
	if len(ids) > 0 {
		numMap, err = uc.repo.CountByCollection(ctx, userID, ids)
		if err != nil {
			return nil, err
		}
	}

	out := make([]map[string]any, 0, len(data))
	for _, row := range data {
		cid, _ := toInt64Any(row["collection_id"])
		row["num"] = numMap[cid] // `?? 0`：map 取不到时 Go 的零值就是 0 ✔
		// ⚠️ 只有真值才 json_decode；假值**保持原值**（没有 else 分支）
		if img, ok := row["goods_image"].(string); ok && img != "" {
			var v any
			if json.Unmarshal([]byte(img), &v) == nil {
				row["goods_image"] = v
			}
		}
		out = append(out, row)
	}

	body, merr := MarshalNoHTMLEscape(out)
	if merr != nil {
		return nil, merr
	}
	// RedisLockService::set($cacheKey, json_encode($data), 5)
	_ = uc.cache.Set(ctx, cacheKey, body, GoodsLookCacheTTL)
	return body, nil
}
