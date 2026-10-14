package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleRecordRepo 对应 app/api/lists/market/sales/SaleRecordLists.php。
type saleRecordRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSaleRecordRepo(db *gorm.DB, c *conf.Data) *saleRecordRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleRecordRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere()。
//
// ⚠️ `keyword` 打在 `AppArchive.name` 上（第 2 张表的列）。
func (r *saleRecordRepo) base(ctx context.Context, q biz.SaleRecordQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"sales_orders_detail AS SalesOrdersDetail").
		Where("SalesOrdersDetail.user_id = ?", q.UserID)
	if q.Keyword != "" {
		tx = tx.Where("AppArchive.name LIKE ?", "%"+q.Keyword+"%")
	}
	if q.State != "" {
		tx = tx.Where("SalesOrdersDetail.state = ?", q.State)
	}
	return tx
}

// ListRecords 逐字对应 lists() —— **4 张表**。
//
// ⚠️ ThinkPHP 的 join 会自动补前缀，并且**把驼峰表名转成下划线**：
//
//	已用 fetchSql 实测，`join('SalesOrders', ...)` 生成的是
//	`INNER JOIN `x_sales_orders` `SalesOrders``  —— 不是 `x_SalesOrders`。
//	（这一点我是先推断错了、再用真实 PHP 跑出来纠正的 —— 清单第 13 条。）
func (r *saleRecordRepo) ListRecords(
	ctx context.Context, q biz.SaleRecordQuery, orderBy string, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.base(ctx, q).
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive " +
			"ON SalesOrdersDetail.archive_id = AppArchive.id").
		Joins("INNER JOIN " + r.prefix + "market_sales AS market_sales " +
			"ON market_sales.id = SalesOrdersDetail.sale_id").
		Joins("INNER JOIN " + r.prefix + "sales_orders AS SalesOrders " +
			"ON SalesOrders.id = SalesOrdersDetail.order_id").
		Select(biz.SaleRecordSelect).
		Order(orderBy).
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		biz.FormatTimeValuesInMap(m)
	}
	return rows, nil
}

// CountRecords 逐字对应 count() —— ⚠️ **只有 2 张表**（少了 market_sales 与 SalesOrders）。
//
// 这是**刻意照搬**：count 的口径比 lists 宽，所以会出现
// "count 说 N 条、lists 只给 M 条（M < N）"的情况。见 biz 层的说明。
func (r *saleRecordRepo) CountRecords(ctx context.Context, q biz.SaleRecordQuery) (int64, error) {
	var n int64
	err := r.base(ctx, q).
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive " +
			"ON SalesOrdersDetail.archive_id = AppArchive.id").
		Count(&n).Error
	return n, err
}
