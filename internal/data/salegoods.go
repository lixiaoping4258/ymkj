package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleGoodsRepo 对应 app/api/lists/market/sales/SaleGoodsLists.php。
type saleGoodsRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSaleGoodsRepo(db *gorm.DB, c *conf.Data) *saleGoodsRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleGoodsRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere()。
//
// ⚠️ LIKE 打在 `AppArchive.name` 上（不是 MarketSales 的列）。
func (r *saleGoodsRepo) base(ctx context.Context, q biz.SaleGoodsQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"market_sales AS MarketSales").
		Where("MarketSales.user_id = ?", q.UserID)
	if q.Keyword != "" {
		tx = tx.Where("AppArchive.name LIKE ?", "%"+q.Keyword+"%")
	}
	return tx
}

// joined 是 lists() 与 count() 共用的 JOIN。
//
// ⚠️ ThinkPHP 的 join 会补表前缀 -> `x_app_archive`（清单第 2 条）。
func (r *saleGoodsRepo) joined(ctx context.Context, q biz.SaleGoodsQuery) *gorm.DB {
	return r.base(ctx, q).
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive " +
			"ON MarketSales.archive_id = AppArchive.id")
}

// ListGoods 逐字对应 lists()。
//
// ⚠️ `orderBy` 来自 biz 层的白名单映射（4 种 + default），不是拼用户输入。
//
//	**其中 price_asc / price_desc 会因 `total_price` 列不存在而报错** —— 照搬。
func (r *saleGoodsRepo) ListGoods(
	ctx context.Context, q biz.SaleGoodsQuery, orderBy string, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.joined(ctx, q).
		Select(biz.SaleGoodsSelect).
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

// CountGoods 逐字对应 count() —— **同样的 JOIN**（这点原文没出错）。
func (r *saleGoodsRepo) CountGoods(ctx context.Context, q biz.SaleGoodsQuery) (int64, error) {
	var n int64
	err := r.joined(ctx, q).Count(&n).Error
	return n, err
}
