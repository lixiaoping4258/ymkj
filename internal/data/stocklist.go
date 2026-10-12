package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// stockListRepo 对应 app/api/lists/market/purchase/StockLists.php。
type stockListRepo struct {
	db     *gorm.DB
	prefix string
}

func NewStockListRepo(db *gorm.DB, c *conf.Data) *stockListRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &stockListRepo{db: db, prefix: prefix}
}

// FindArchiveCollectionID 对应 `AppArchive::where(['id'=>$archiveId])->findOrEmpty()`，
// 只取 collection_id。
//
// ⚠️ 只按 id 查，**不筛 state**（与 SaleLogic::getArchive 不同）。
// ⚠️ x_app_archive **没有 delete_time** -> 不加软删除过滤。
func (r *stockListRepo) FindArchiveCollectionID(
	ctx context.Context, archiveID int64,
) (int64, bool, error) {
	var row struct {
		CollectionID int64 `gorm:"column:collection_id"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_archive").
		Select("collection_id").
		Where("id = ?", archiveID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.CollectionID, true, nil
}

// base 构造 queryWhere() 的 6 个等值条件。
func (r *stockListRepo) base(ctx context.Context, q biz.StockQuery) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"warehouse_detail").
		Where("user_id = ?", q.UserID).
		Where("auth_id = ?", q.AuthID).
		Where("collection_id = ?", q.CollectionID).
		Where("is_locked = ?", 1).
		Where("used = ?", 0).
		Where("is_use_num = ?", 0)
}

// ListStock 逐字对应：
//
//	WarehouseDetail::where($where)->limit($offset,$length)
//	    ->order('create_time asc ,goods_id asc')->select();
//
// ⚠️ **没有 field()** -> `SELECT *`（24 列）。用 map 扫描承接任意列。
// ⚠️ 排序是**两个字段都是 ASC**：`create_time ASC, goods_id ASC`。
// ⚠️ WarehouseDetail **extends Model**（不是 BaseModel）且**没有 use SoftDelete**
//
//	-> 不加 delete_time 过滤（表里也没有该列）。
func (r *stockListRepo) ListStock(
	ctx context.Context, q biz.StockQuery, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.base(ctx, q).
		Order("create_time ASC").
		Order("goods_id ASC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		// datetime 列（create_time / unlock_time / update_time）格式化 —— 清单第 1 条
		biz.FormatTimeValuesInMap(m)
	}
	return rows, nil
}

// CountStock 逐字对应 `WarehouseDetail::where($where)->count()`。
func (r *stockListRepo) CountStock(ctx context.Context, q biz.StockQuery) (int64, error) {
	var n int64
	err := r.base(ctx, q).Count(&n).Error
	return n, err
}
