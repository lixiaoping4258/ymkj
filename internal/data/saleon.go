package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleOnRepo 对应 app/api/lists/market/sales/SaleOnCollectionLists.php。
type saleOnRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSaleOnRepo(db *gorm.DB, c *conf.Data) *saleOnRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleOnRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere()：
//
//	[['MarketSales.archive_id','=',$archiveID], ['MarketSales.state','=','ON_SALE']]
func (r *saleOnRepo) base(ctx context.Context, archiveID int64, state string) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"market_sales AS MarketSales").
		Where("MarketSales.archive_id = ?", archiveID).
		Where("MarketSales.state = ?", state)
}

// archiveJoin 是 `hasWhere('archive', ...)` 生成的 INNER JOIN：
//
//	INNER JOIN x_app_archive AppArchive ON AppArchive.id = MarketSales.archive_id
func (r *saleOnRepo) archiveJoin(ctx context.Context, archiveID int64, state string) *gorm.DB {
	return r.base(ctx, archiveID, state).
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive ON AppArchive.id = MarketSales.archive_id")
}

// ListOnSale 逐字对应 lists()（**两个 JOIN**）。
//
// ⚠️ `join('app', ...)` 由 ThinkPHP 补前缀 -> `x_app`（清单第 2 条）。
// ⚠️ `order('id desc')` 原文无限定名；四张表里只有 market_sales / app_archive / app
//
//	都可能有 id，MySQL 取左表。显式写 MarketSales.id DESC 固定行为。
//
// ⚠️ MarketSales **无软删除**。
func (r *saleOnRepo) ListByState(
	ctx context.Context, archiveID int64, state string, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.archiveJoin(ctx, archiveID, state).
		Joins("INNER JOIN " + r.prefix + "app AS app ON MarketSales.app_id = app.id").
		Select(biz.SaleOnSelect).
		Order("MarketSales.id DESC").
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

// CountOnSale 逐字对应 count() —— 🔴 **只有 hasWhere('archive') 一个 JOIN**。
//
// 与 lists() 的两个 JOIN 不一致（清单第 3 条），照搬。
func (r *saleOnRepo) CountByState(ctx context.Context, archiveID int64, state string) (int64, error) {
	var n int64
	err := r.archiveJoin(ctx, archiveID, state).Count(&n).Error
	return n, err
}
