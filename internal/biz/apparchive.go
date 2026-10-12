package biz

import (
	"context"
	"time"
)

// AppArchiveUsecase 对应 app/api/lists/open/AppArchiveLists.php。
type AppArchiveUsecase struct {
	repo AppArchiveRepo
}

func NewAppArchiveUsecase(repo AppArchiveRepo) *AppArchiveUsecase {
	return &AppArchiveUsecase{repo: repo}
}

// AppArchiveRepo 一级平台艺术品档案。
type AppArchiveRepo interface {
	// ListArchives 对应：
	//   AppArchive::where($this->queryWhere())->order('id','desc')
	//       ->limit(...)->select()->toArray()
	//
	// ⚠️ **没有 field()** -> `SELECT *`（x_app_archive 有 18 列）。用 map 承接。
	ListArchives(ctx context.Context, q AppArchiveQuery, offset, limit int) ([]map[string]any, error)
	// CountArchives 对应 `AppArchive::where($this->queryWhere())->count()`
	CountArchives(ctx context.Context, q AppArchiveQuery) (int64, error)
	// ListSaleInfos 对应逐行的：
	//   MarketListSales::where('app_id',...)->where('archive_id',...)->find()
	//
	// ⚠️ 原文是 **N+1**（每行一次查询）。这里按 archive_id 批量取"每组的第一行"，
	//    结果集与逐行 find() 一致（find() 也是取第一行），但把 N+1 压成 1 次查询。
	//    **返回值的形态与逐行一致**，属于纯性能优化，不改变可观测行为。
	ListSaleInfos(ctx context.Context, appID int64, archiveIDs []int64) (map[int64]map[string]any, error)
	// ListAppInfo 对应 `(new AppLists())->listsWithQuery([['id','in',...]])`
	ListAppInfo(ctx context.Context, appIDs []int64, offset, limit int) ([]AppInfoRow, error)
}

// AppArchiveQuery 对应 queryWhere() 的四个条件。
//
//	$where[] = ['state','=',1];
//	$where[] = ['app_id','=',$this->params['app_id']];
//	if (!empty($this->params['id']))            { $where[] = ['id','=',$this->params['id']]; }
//	if (!empty($this->params['collection_id'])) { $where[] = ['collection_id','=',$this->params['collection_id']]; }
//
// ⚠️ `id` 与 `collection_id` 用的是 **`!empty()`** —— 所以 `0` 与 `”` 都视为"未传"。
type AppArchiveQuery struct {
	AppID int64
	// ID 对应 `$this->params['id']`；0 表示不筛
	ID int64
	// CollectionID 对应 `$this->params['collection_id']`；空串表示不筛
	CollectionID string
}

// AppArchiveItem 是列表里的一行：`SELECT *` 的所有列 + 两个嵌套对象。
type AppArchiveItem struct {
	// Columns 是 x_app_archive 的全部列（datetime 已格式化）
	Columns map[string]any
	// OnSale 对应 `$item['on_sale']` —— 查到时是整行，查不到是 **空数组 []**
	OnSale any
	// AppInfo 对应 `$appMap[$item['app_id']] ?? []` —— ⚠️ 这里是 `?? []`，
	// 所以查不到时是 **空数组 []**（不是 null）。
	// 对比 authUserList 同类逻辑**没有** `??`，那里是 null —— 两个接口不一样。
	AppInfo any
}

// List 对应 `dataLists(new AppArchiveLists())`。
//
// 原文：
//
//	$lists = AppArchive::where($this->queryWhere())->order('id','desc')
//	    ->limit($this->limitOffset,$this->limitLength)->select()->toArray();
//	$app_ids = array_column($lists,'app_id');
//	$appLists = (new AppLists())->listsWithQuery([['id','in',implode(',',$app_ids)]]);
//	$appMap = MapHelper::mapWithKey($appLists,'id');
//	$result = [];
//	foreach ($lists as &$item) {
//	    $salesInfo = MarketListSales::where('app_id','=',$item['app_id'])
//	        ->where('archive_id','=',$item['id'])->find();
//	    $item['on_sale'] = $salesInfo ? $salesInfo->toArray() : [];
//	    $item['app_info'] = $appMap[$item['app_id']] ?? [];
//	    $result[] = $item;
//	}
//	return $result;
//
// ⚠️ 两个嵌套对象在**同一层**：`on_sale` 和 `app_info` 都是 `$item` 的键。
// ⚠️ `on_sale` 的 `archive_id` 匹配的是 `$item['id']`（档案主键）。
// ⚠️ `app_info` 用的是 `?? []`，而 `authUserList` 同类逻辑没有 `??`（那边是 null）。
func (uc *AppArchiveUsecase) List(ctx context.Context, q AppArchiveQuery, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountArchives(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListArchives(ctx, q, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}

	// 收集 app_id / archive_id
	seenApp := map[int64]bool{}
	appIDs := make([]int64, 0, len(rows))
	archiveIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		if aid, ok := mapInt64(r, "id"); ok {
			archiveIDs = append(archiveIDs, aid)
		}
		if pid, ok := mapInt64(r, "app_id"); ok && !seenApp[pid] {
			seenApp[pid] = true
			appIDs = append(appIDs, pid)
		}
	}

	// on_sale：批量取（等价于逐行 find()）
	sales := map[int64]map[string]any{}
	if q.AppID != 0 && len(archiveIDs) > 0 {
		sales, err = uc.repo.ListSaleInfos(ctx, q.AppID, archiveIDs)
		if err != nil {
			return nil, err
		}
	}

	// app_info
	appMap := map[int64]AppInfoRow{}
	if len(appIDs) > 0 {
		infos, err := uc.repo.ListAppInfo(ctx, appIDs, p.Offset, p.Limit)
		if err != nil {
			return nil, err
		}
		for _, a := range infos {
			appMap[a.ID] = a
		}
	}

	items := make([]AppArchiveItem, 0, len(rows))
	for _, r := range rows {
		it := AppArchiveItem{Columns: r, OnSale: []any{}, AppInfo: []any{}}
		if aid, ok := mapInt64(r, "id"); ok {
			if s, hit := sales[aid]; hit {
				it.OnSale = s
			}
		}
		if pid, ok := mapInt64(r, "app_id"); ok {
			if a, hit := appMap[pid]; hit {
				it.AppInfo = appInfoToMap(a)
			}
		}
		items = append(items, it)
	}

	return &ListData{
		Lists:    items,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// mapInt64 从 map 里取整数（GORM 的 map 扫描可能给 int64 / int32 / []byte 等）。
func mapInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	return toInt64Any(v)
}

// appInfoToMap 把 AppInfoRow 转成与 PHP `listsWithQuery` 一致的 7 键对象。
//
// ⚠️ **不处理 logo**（listsWithQuery 不处理）。
func appInfoToMap(a AppInfoRow) map[string]any {
	return map[string]any{
		"id":       a.ID,
		"name":     a.Name,
		"state":    a.State,
		"logo":     a.Logo,
		"type":     a.Type,
		"fee_rate": a.FeeRate,
		"flag":     a.Flag,
	}
}

// FormatTimeValuesInMap 把 map 里的 time.Time 就地格式化成 `Y-m-d H:i:s`。
//
// ⚠️ 这是**清单第 1 条**的通用解法：`SELECT *` 的列类型未知，
//
//	凡是 datetime 都会是 time.Time，直接 JSON 序列化会得到 RFC3339，
//	与 PHP 的 "Y-m-d H:i:s" 不一致。
func FormatTimeValuesInMap(m map[string]any) {
	for k, v := range m {
		switch t := v.(type) {
		case time.Time:
			m[k] = t.Format("2006-01-02 15:04:05")
		case *time.Time:
			if t == nil {
				m[k] = nil
			} else {
				m[k] = t.Format("2006-01-02 15:04:05")
			}
		case []byte:
			// MySQL 的 []byte 一般是字符串列；转成 string 更贴近 PHP 输出
			m[k] = string(t)
		}
	}
}
