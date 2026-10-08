package data

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleFaceRepo 对应 app/api/lists/market/sales/SaleFaceLists。
//
// SQL 同样是用 PHP CLI + Db::getLastSql() 实测出来的（不是推断）：
//
//	SELECT `MarketListSales`.`id`,`AppArchive`.`name`,`AppArchive`.`images`,
//	       `AppArchive`.`issuer`,`AppArchive`.`issuer_time`,
//	       `MarketListSales`.`sale_amount`,`MarketListSales`.`low_price`,
//	       `MarketListSales`.`max_price`, app.name as platform_name
//	FROM `x_market_list_sales` `MarketListSales`
//	INNER JOIN `x_app_archive` `AppArchive` ON `MarketListSales`.`archive_id`=`AppArchive`.`id`
//	INNER JOIN `x_app` `app` ON `MarketListSales`.`app_id`=`app`.`id`
//	WHERE `AppArchive`.`state` = '1'
//	LIMIT 0,2
//
// ⚠️ 注意没有 ORDER BY —— SaleFaceLists 的 switch($sort) **没有 default 分支**，
// 所以 sort 缺省时 $orderRaw 是空串，order(”) 不会生成排序子句。
// 这与 PurchaseFaceLists（有 default）行为不同，是逐字对照源码 + 实测确认的。
type saleFaceRepo struct {
	db     *gorm.DB
	prefix string
}

const (
	saleAliasMLS = "MarketListSales"
	saleAliasAA  = "AppArchive"
	saleAliasApp = "app"
)

func NewSaleFaceRepo(db *gorm.DB, c *conf.Data) biz.SaleFaceRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleFaceRepo{db: db, prefix: prefix}
}

func (r *saleFaceRepo) table(name string) string { return r.prefix + name }

func (r *saleFaceRepo) selectList() string {
	return strings.Join([]string{
		saleAliasMLS + ".id",
		saleAliasAA + ".name",
		saleAliasAA + ".images",
		saleAliasAA + ".issuer",
		saleAliasAA + ".issuer_time",
		saleAliasMLS + ".sale_amount",
		saleAliasMLS + ".low_price",
		saleAliasMLS + ".max_price",
		saleAliasApp + ".name as platform_name",
	}, ",")
}

func (r *saleFaceRepo) joinArchive() string {
	return r.table("market_list_sales") + " AS " + saleAliasMLS +
		" INNER JOIN " + r.table("app_archive") + " AS " + saleAliasAA +
		" ON " + saleAliasMLS + ".archive_id = " + saleAliasAA + ".id"
}

func (r *saleFaceRepo) from() string {
	return r.joinArchive() +
		" INNER JOIN " + r.table("app") + " AS " + saleAliasApp +
		" ON " + saleAliasMLS + ".app_id = " + saleAliasApp + ".id"
}

// where 逐行对照 SaleFaceLists::queryWhere。
// 与 PurchaseFaceLists 的条件结构完全相同，只是主表别名不同。
func (r *saleFaceRepo) where(q biz.SaleFaceQuery) (string, []any) {
	conds := []string{saleAliasAA + ".state = ?"}
	args := []any{"1"}

	if biz.PhpTruthy(q.Keyword) {
		conds = append(conds, saleAliasAA+".name LIKE ?")
		args = append(args, "%"+q.Keyword+"%")
	}
	if q.PriceStart > 0 {
		conds = append(conds, saleAliasMLS+".low_price >= ?")
		args = append(args, q.PriceStart)
	}
	if q.PriceEnd > 0 {
		conds = append(conds, saleAliasMLS+".max_price <= ?")
		args = append(args, q.PriceEnd)
	}
	if biz.PhpTruthy(q.TimeStart) {
		conds = append(conds, saleAliasMLS+".public_time >= ?")
		args = append(args, q.TimeStart)
	}
	if biz.PhpTruthy(q.TimeEnd) {
		conds = append(conds, saleAliasMLS+".public_time <= ?")
		args = append(args, q.TimeEnd)
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// orderBy 逐字对应 SaleFaceLists::lists() 的 switch。
//
// ❗没有 default 分支：未知/缺省的 sort 返回**空串**，即不加 ORDER BY 子句。
// 值只从白名单映射，不拼接用户输入。
func saleOrderBy(sort string) string {
	switch sort {
	case "low":
		return saleAliasMLS + ".low_price ASC"
	case "high":
		return saleAliasMLS + ".max_price DESC"
	case "public":
		return saleAliasMLS + ".public_time DESC"
	case "done":
		return saleAliasMLS + ".done_time DESC"
	default:
		return ""
	}
}

type saleFaceScan struct {
	ID           int64      `gorm:"column:id"`
	Name         string     `gorm:"column:name"`
	Images       *string    `gorm:"column:images"`
	Issuer       string     `gorm:"column:issuer"`
	IssuerTime   *time.Time `gorm:"column:issuer_time"`
	SaleAmount   int64      `gorm:"column:sale_amount"`
	LowPrice     *string    `gorm:"column:low_price"`
	MaxPrice     *string    `gorm:"column:max_price"`
	PlatformName string     `gorm:"column:platform_name"`
}

func (r *saleFaceRepo) List(ctx context.Context, q biz.SaleFaceQuery, offset, limit int) ([]biz.SaleFaceRow, error) {
	whereSQL, args := r.where(q)
	sql := "SELECT " + r.selectList() + " FROM " + r.from() + whereSQL
	if ob := saleOrderBy(q.Sort); ob != "" {
		sql += " ORDER BY " + ob
	}
	sql += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []saleFaceScan
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.SaleFaceRow, 0, len(rows))
	for _, s := range rows {
		out = append(out, biz.SaleFaceRow{
			ID:           s.ID,
			Name:         s.Name,
			Images:       s.Images,
			Issuer:       s.Issuer,
			IssuerTime:   s.IssuerTime,
			SaleAmount:   s.SaleAmount,
			LowPrice:     s.LowPrice,
			MaxPrice:     s.MaxPrice,
			PlatformName: s.PlatformName,
		})
	}
	return out, nil
}

func (r *saleFaceRepo) Count(ctx context.Context, q biz.SaleFaceQuery) (int64, error) {
	whereSQL, args := r.where(q)
	// count 只 join archive，不 join app（与 lists() 不同，与原 SQL 一致）
	sql := "SELECT COUNT(*) FROM " + r.joinArchive() + whereSQL
	var n int64
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
