package data

import (
	"context"
	"strconv"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// appArchiveRepo 对应 app/api/lists/open/AppArchiveLists.php。
type appArchiveRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAppArchiveRepo(db *gorm.DB, c *conf.Data) *appArchiveRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &appArchiveRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere()：
//
//	state = 1
//	AND app_id = ?
//	[AND id = ?]              -- 仅当 !empty($params['id'])
//	[AND collection_id = ?]   -- 仅当 !empty($params['collection_id'])
//
// ⚠️ `!empty()` 语义：0 与 ” 都算"未传"。
func (r *appArchiveRepo) base(ctx context.Context, q biz.AppArchiveQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"app_archive").
		Where("state = ?", 1).
		Where("app_id = ?", q.AppID)
	if q.ID != 0 {
		tx = tx.Where("id = ?", q.ID)
	}
	if q.CollectionID != "" {
		tx = tx.Where("collection_id = ?", q.CollectionID)
	}
	return tx
}

// ListArchives 逐字对应：
//
//	AppArchive::where($this->queryWhere())->order('id','desc')
//	    ->limit($this->limitOffset,$this->limitLength)->select()->toArray();
//
// ⚠️ **没有 field()** -> `SELECT *`。这里用 `Find(&[]map[string]any)` 承接任意列，
//
//	不需要手写 18 个字段。
//
// ⚠️ AppArchive 模型若开软删除需加过滤 —— x_app_archive **没有 delete_time 列**，
//
//	所以无论模型怎样都不会有该过滤。
func (r *appArchiveRepo) ListArchives(
	ctx context.Context, q biz.AppArchiveQuery, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.base(ctx, q).
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// ⚠️ 清单第 1 条：datetime 列必须格式化，否则输出 RFC3339
	for _, m := range rows {
		biz.FormatTimeValuesInMap(m)
	}
	return rows, nil
}

// CountArchives 逐字对应 `AppArchive::where($this->queryWhere())->count()`。
func (r *appArchiveRepo) CountArchives(ctx context.Context, q biz.AppArchiveQuery) (int64, error) {
	var n int64
	err := r.base(ctx, q).Count(&n).Error
	return n, err
}

// ListSaleInfos 对应逐行的：
//
//	MarketListSales::where('app_id','=',$item['app_id'])
//	    ->where('archive_id','=',$item['id'])->find();
//
// ⚠️ 原文是 **N+1**（每行一次 find()）。这里一次查出所有候选行，再按 archive_id
//
//	取**每组的第一行** —— 与逐行 find() 的结果一致（find() 也是取第一行），
//	只把查询次数从 N 降到 1。**输出形态不变**。
//
// ⚠️ `find()` 没有 order，取的是 MySQL 的自然顺序（一般是主键升序）。
//
//	这里显式按 `id ASC` 取第一行，与该顺序一致。
//
// ⚠️ x_market_list_sales **没有 delete_time 列** -> 不加软删除过滤。
func (r *appArchiveRepo) ListSaleInfos(
	ctx context.Context, appID int64, archiveIDs []int64,
) (map[int64]map[string]any, error) {
	if len(archiveIDs) == 0 {
		return map[int64]map[string]any{}, nil
	}
	var rows []map[string]any
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_list_sales").
		Where("app_id = ?", appID).
		Where("archive_id IN ?", archiveIDs).
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]map[string]any, len(rows))
	for _, m := range rows {
		biz.FormatTimeValuesInMap(m)
		aid, ok := toI64(m["archive_id"])
		if !ok {
			continue
		}
		if _, exists := out[aid]; exists {
			continue // 只保留第一行（等价于 find()）
		}
		out[aid] = m
	}
	return out, nil
}

// ListAppInfo 逐字对应 `(new AppLists())->listsWithQuery([['id','in',$ids]])`。
//
// ⚠️ 7 列（比 AppLists::lists() 多 state）；**不处理 logo**。
// ⚠️ 分页用**当前请求的** offset/limit（原文 `$this->limitOffset/limitLength`）。
func (r *appArchiveRepo) ListAppInfo(
	ctx context.Context, appIDs []int64, offset, limit int,
) ([]biz.AppInfoRow, error) {
	var rows []struct {
		ID      int64  `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		State   int32  `gorm:"column:state"`
		Logo    string `gorm:"column:logo"`
		Type    int32  `gorm:"column:type"`
		FeeRate string `gorm:"column:fee_rate"`
		Flag    string `gorm:"column:flag"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Select("id", "name", "state", "logo", "type", "fee_rate", "flag").
		Where("state = ?", 1).
		Where("type = ?", 2).
		Where("id IN ?", appIDs).
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.AppInfoRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.AppInfoRow{
			ID: x.ID, Name: x.Name, State: x.State, Logo: x.Logo,
			Type: x.Type, FeeRate: x.FeeRate, Flag: x.Flag,
		})
	}
	return out, nil
}

// toI64 把 map 里的值转 int64（archive_id 可能是 int64 / []byte）
func toI64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int32:
		return int64(x), true
	case int:
		return int64(x), true
	case uint64:
		return int64(x), true
	case []byte:
		n, err := strconv.ParseInt(string(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
