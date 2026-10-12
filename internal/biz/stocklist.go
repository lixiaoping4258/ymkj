package biz

import (
	"context"
	"encoding/json"
)

// StockListUsecase 对应 app/api/lists/market/purchase/StockLists.php。
type StockListUsecase struct {
	repo StockListRepo
}

func NewStockListUsecase(repo StockListRepo) *StockListUsecase {
	return &StockListUsecase{repo: repo}
}

// StockListRepo 库存商品明细。
type StockListRepo interface {
	// FindArchiveCollectionID 对应 `AppArchive::where(['id'=>$archiveId])->findOrEmpty()`
	// 只取 `collection_id`。
	FindArchiveCollectionID(ctx context.Context, archiveID int64) (int64, bool, error)
	// ListStock 对应：
	//   WarehouseDetail::where($where)->limit(...)
	//       ->order('create_time asc ,goods_id asc')->select()
	//
	// ⚠️ **没有 field()** -> `SELECT *`（x_warehouse_detail 有 24 列）。用 map 承接。
	ListStock(ctx context.Context, q StockQuery, offset, limit int) ([]map[string]any, error)
	// CountStock 对应 `WarehouseDetail::where($where)->count()`
	CountStock(ctx context.Context, q StockQuery) (int64, error)
}

// StockQuery 对应 queryWhere() 的 6 个等值条件。
//
//	$authId = $this->params['auth_id'] ?? 0;      // ⚠️ 缺失时兜底为 0
//	return [
//	    'user_id'       => $this->userId,
//	    'auth_id'       => $authId,
//	    'collection_id' => $collection_id,
//	    'is_locked'     => 1,
//	    'used'          => 0,
//	    'is_use_num'    => 0,
//	];
//
// ⚠️ 后三个是**硬编码**的：只列"已锁定、未使用、未占用数量"的库存。
// ⚠️ `auth_id` 缺失时是 **0**（不是"不筛"）—— 会查不到任何行（除非真有 auth_id=0 的数据）。
type StockQuery struct {
	UserID       uint32
	AuthID       int64
	CollectionID int64
}

// ListStock 逐字对应 `dataLists(new StockLists())`。
//
// 原文 lists()：
//
//	$archiveId = $this->params['archive_id'] ?? 0;
//	if (empty($archiveId)) { return []; }
//	$appArchive = AppArchive::where(['id'=>$archiveId])->findOrEmpty();
//	if ($appArchive->isEmpty()) { return []; }
//	$where = $this->queryWhere($appArchive->collection_id);
//	$result = WarehouseDetail::where($where)->limit(...)
//	    ->order('create_time asc ,goods_id asc')->select();
//	foreach ($result as $item) {
//	    if (!empty($item['goods_image'])) { $item['goods_image'] = json_decode($item['goods_image'], true); }
//	    else { $item['goods_image'] = []; }
//	}
//	return $result->toArray();
//
// count()：
//
//	$archiveId = $this->params['archive_id'] ?? 0;
//	if (empty($archiveId)) { return 0; }
//	$appArchive = AppArchive::where(['id'=>$archiveId])->findOrEmpty();
//	if ($appArchive->isEmpty()) { return 0; }
//	$where = $this->queryWhere($appArchive->collection_id);
//	return WarehouseDetail::where($where)->count();
//
// ⚠️ **`archive_id` 为空、或档案查不到时，接口返回的是空列表 + count=0，不是报错。**
//
//	（源码里那段注释 "原有逻辑致命缺陷：count无参数调用queryWhere…" 说明这曾经是个 bug，
//	  现在已经修好了 —— 实现的是**修好之后**的行为。）
//
// ⚠️ `goods_image`：`!empty` 才 json_decode，否则**赋成空数组 []**（不是 null）。
func (uc *StockListUsecase) List(ctx context.Context, userID uint32, archiveID, authID int64, p PageParams) (*ListData, error) {
	if archiveID == 0 {
		// `empty($archiveId)` -> 直接返回空
		return emptyStockList(p), nil
	}
	collectionID, found, err := uc.repo.FindArchiveCollectionID(ctx, archiveID)
	if err != nil {
		return nil, err
	}
	if !found {
		return emptyStockList(p), nil
	}
	q := StockQuery{UserID: userID, AuthID: authID, CollectionID: collectionID}

	count, err := uc.repo.CountStock(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListStock(ctx, q, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		s, _ := m["goods_image"].(string)
		if s != "" {
			var v any
			if json.Unmarshal([]byte(s), &v) == nil {
				m["goods_image"] = v
				continue
			}
		}
		// else 分支：**空数组**
		m["goods_image"] = []any{}
	}
	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// emptyStockList 对应 `return [];` / `return 0;` 的分支。
func emptyStockList(p PageParams) *ListData {
	return &ListData{
		Lists:    []map[string]any{},
		Count:    0,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}
}
