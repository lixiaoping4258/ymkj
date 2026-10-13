package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleOrderListRepo 对应 app/api/lists/market/sales/SaleOrderLists.php。
type saleOrderListRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSaleOrderListRepo(db *gorm.DB, c *conf.Data) *saleOrderListRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleOrderListRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere()。
//
// ⚠️ `order_state` 那一列**不存在** —— 若传了值，这里会照原样拼进 SQL，
//
//	MySQL 报 `Unknown column 'order_state'`。这是**刻意照搬**（见 biz 层说明）。
//
// ⚠️ `time_start` / `time_end` 用整数与 datetime 列比较 —— 行为已在 biz 层说明。
func (r *saleOrderListRepo) base(ctx context.Context, q biz.SaleOrderQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"sales_orders AS SalesOrders").
		Where("SalesOrders.user_id = ?", q.UserID)

	if q.PayState != "" {
		tx = tx.Where("SalesOrders.pay_state = ?", q.PayState)
	}
	if q.OrderState != "" {
		// 🔴 表里没有这一列 —— 与 PHP 一样会报 Unknown column（照搬）
		tx = tx.Where("SalesOrders.order_state = ?", q.OrderState)
	}
	if q.Keyword != "" {
		tx = tx.Where("SalesOrders.order_no LIKE ?", "%"+q.Keyword+"%")
	}
	if q.HasTimeStart {
		// 🔴 与 datetime 列比较整数 —— 恒为真（照搬原实现）
		tx = tx.Where("SalesOrders.create_time >= ?", q.TimeStart)
	}
	if q.HasTimeEnd {
		// 🔴 同上，恒为假（照搬原实现）
		tx = tx.Where("SalesOrders.create_time <= ?", q.TimeEnd+86400)
	}
	return tx
}

// joined 是 lists() 与 count() **共用**的三个 JOIN（原文两处写法一致）。
//
// ⚠️ ThinkPHP 的 join 会补表前缀 -> `x_sales_orders_detail` / `x_app_archive`
//
//	（清单第 2 条，已用 fetchSql 验证过该行为）。
//
// ⚠️ SalesOrders **extends Model、无 SoftDelete** -> 不加 delete_time 过滤。
func (r *saleOrderListRepo) joined(ctx context.Context, q biz.SaleOrderQuery) *gorm.DB {
	return r.base(ctx, q).
		Joins("INNER JOIN " + r.prefix + "sales_orders_detail AS SalesOrdersDetail " +
			"ON SalesOrdersDetail.order_id = SalesOrders.id").
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive " +
			"ON SalesOrdersDetail.archive_id = AppArchive.id")
}

// ListOrders 逐字对应 lists()。
//
// ⚠️ `orderBy` 由 biz 层从**白名单**映射得到（4 个 sort 值 + default），
//
//	不是把用户输入直接拼进 ORDER BY —— 这一点比原实现更稳，
//	但**可观测行为完全一致**（同样只有那 4 种排序）。
func (r *saleOrderListRepo) ListOrders(
	ctx context.Context, q biz.SaleOrderQuery, orderBy string, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.joined(ctx, q).
		Select(biz.SaleOrderSelect).
		Order(orderBy).
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		// datetime 列（pay_expire_time）格式化 —— 清单第 1 条
		biz.FormatTimeValuesInMap(m)
	}
	return rows, nil
}

// CountOrders 逐字对应 count() —— **同样的三个 JOIN**（与 lists 一致，这点原文没出错）。
func (r *saleOrderListRepo) CountOrders(ctx context.Context, q biz.SaleOrderQuery) (int64, error) {
	var n int64
	err := r.joined(ctx, q).Count(&n).Error
	return n, err
}
