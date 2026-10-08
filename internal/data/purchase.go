package data

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// purchaseFaceRepo 对应 app/api/lists/market/purchase/PurchaseFaceLists。
//
// ⚠️ 这里用**手写 SQL** 而不是 GORM 的链式 API，因为原实现用的是
// ThinkPHP 的 hasWhere()，它的 join 形态必须逐字还原。
// 下面的 SQL 是用 PHP CLI 启动 ThinkPHP、对真实数据跑 fetchSql(true)
// 直接抓出来的（不是推断）：
//
//	SELECT `MarketListPurchase`.`id`, ... , app.name as platform_name
//	FROM `x_market_list_purchase` `MarketListPurchase`
//	INNER JOIN `x_app_archive` `AppArchive` ON `MarketListPurchase`.`archive_id`=`AppArchive`.`id`
//	INNER JOIN `x_app` `app` ON `MarketListPurchase`.`app_id`=`app`.`id`
//	WHERE `AppArchive`.`state` = '1'
//	ORDER BY `MarketListPurchase`.`purchase_lists` DESC
//	LIMIT 0,3
//
// 两个关键结论（都是实测得出，不是猜的）：
//  1. hasWhere 生成的是 **INNER JOIN**，不是 EXISTS 子查询
//  2. where 为空数组时（生产测试用户分支）**整个 WHERE 子句消失**，
//     连 state=1 都不过滤
type purchaseFaceRepo struct {
	db     *gorm.DB
	prefix string
}

func NewPurchaseFaceRepo(db *gorm.DB, c *conf.Data) biz.PurchaseFaceRepo {
	// 前缀从配置读。不能从 db.Config.NamingStrategy 拿 ——
	// GORM 把它存成 schema.Namer 接口，取不到 TablePrefix 字段。
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &purchaseFaceRepo{db: db, prefix: prefix}
}

// 别名与原 SQL 完全一致。ThinkPHP 用模型名当别名，输出列名会去掉别名前缀，
// 所以下面 SELECT 出来的列名就是 id / archive_id / name / platform_name ...
const (
	aliasMLP = "MarketListPurchase"
	aliasAA  = "AppArchive"
	aliasApp = "app"
)

func (r *purchaseFaceRepo) table(name string) string { return r.prefix + name }

// purchaseFaceColumns 与原实现的 field() 列表逐字对应。
const purchaseFaceColumns = aliasMLP + ".*" // 占位，实际用下面的显式列表

func (r *purchaseFaceRepo) selectList() string {
	return strings.Join([]string{
		aliasMLP + ".id",
		aliasMLP + ".archive_id",
		aliasAA + ".name",
		aliasAA + ".images",
		aliasAA + ".issuer",
		aliasAA + ".issuer_time",
		aliasMLP + ".purchase_amount",
		aliasMLP + ".low_unit_price",
		aliasMLP + ".max_unit_price",
		aliasMLP + ".purchase_lists",
		aliasApp + ".name as platform_name",
	}, ",")
}

func (r *purchaseFaceRepo) from() string {
	return r.table("market_list_purchase") + " AS " + aliasMLP +
		" INNER JOIN " + r.table("app_archive") + " AS " + aliasAA +
		" ON " + aliasMLP + ".archive_id = " + aliasAA + ".id" +
		" INNER JOIN " + r.table("app") + " AS " + aliasApp +
		" ON " + aliasMLP + ".app_id = " + aliasApp + ".id"
}

// where 逐行对照 PurchaseFaceLists::queryWhere。
//
// 几个细节都保持原样：
//   - keyword 用 LIKE '%kw%'（不对 % 做转义，与原实现一致）
//   - price_start / price_end 只在 **floatval > 0** 时才加条件（传 0 等于不筛）
//   - time_start / time_end 直接用字符串比较（DB 列是 datetime）
func (r *purchaseFaceRepo) where(q biz.PurchaseFaceQuery, noStateFilter bool) (string, []any) {
	var conds []string
	var args []any

	if !noStateFilter {
		conds = append(conds, aliasAA+".state = ?")
		args = append(args, "1")
	}
	if biz.PhpTruthy(q.Keyword) {
		conds = append(conds, aliasAA+".name LIKE ?")
		args = append(args, "%"+q.Keyword+"%")
	}
	if q.PriceStart > 0 {
		conds = append(conds, aliasMLP+".low_price >= ?")
		args = append(args, q.PriceStart)
	}
	if q.PriceEnd > 0 {
		conds = append(conds, aliasMLP+".max_price <= ?")
		args = append(args, q.PriceEnd)
	}
	if biz.PhpTruthy(q.TimeStart) {
		conds = append(conds, aliasMLP+".public_time >= ?")
		args = append(args, q.TimeStart)
	}
	if biz.PhpTruthy(q.TimeEnd) {
		conds = append(conds, aliasMLP+".public_time <= ?")
		args = append(args, q.TimeEnd)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// orderBy 逐字对应 PurchaseFaceLists::lists() 里的 switch ($sort)。
// 值只用白名单映射，不拼接用户输入。
func orderBy(sort string) string {
	switch sort {
	case "low":
		return aliasMLP + ".low_price ASC"
	case "high":
		return aliasMLP + ".max_price DESC"
	case "public":
		return aliasMLP + ".public_time DESC"
	case "done":
		return aliasMLP + ".done_time DESC"
	default:
		return aliasMLP + ".purchase_lists DESC"
	}
}

// purchaseFaceScan 的列名对应 SELECT 出来的裸列名。
type purchaseFaceScan struct {
	ID             int64      `gorm:"column:id"`
	ArchiveID      int64      `gorm:"column:archive_id"`
	Name           string     `gorm:"column:name"`
	Images         *string    `gorm:"column:images"`
	Issuer         string     `gorm:"column:issuer"`
	IssuerTime     *time.Time `gorm:"column:issuer_time"`
	PurchaseAmount int64      `gorm:"column:purchase_amount"`
	LowUnitPrice   *string    `gorm:"column:low_unit_price"`
	MaxUnitPrice   *string    `gorm:"column:max_unit_price"`
	PurchaseLists  *int64     `gorm:"column:purchase_lists"`
	PlatformName   string     `gorm:"column:platform_name"`
}

func (r *purchaseFaceRepo) List(
	ctx context.Context, q biz.PurchaseFaceQuery, offset, limit int, noStateFilter bool,
) ([]biz.PurchaseFaceRow, error) {
	whereSQL, args := r.where(q, noStateFilter)
	sql := "SELECT " + r.selectList() + " FROM " + r.from() + whereSQL +
		" ORDER BY " + orderBy(q.Sort) + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []purchaseFaceScan
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.PurchaseFaceRow, 0, len(rows))
	for _, s := range rows {
		out = append(out, biz.PurchaseFaceRow{
			ID:             s.ID,
			ArchiveID:      s.ArchiveID,
			Name:           s.Name,
			Images:         s.Images,
			Issuer:         s.Issuer,
			IssuerTime:     s.IssuerTime,
			PurchaseAmount: s.PurchaseAmount,
			LowUnitPrice:   s.LowUnitPrice,
			MaxUnitPrice:   s.MaxUnitPrice,
			PurchaseLists:  s.PurchaseLists,
			PlatformName:   s.PlatformName,
		})
	}
	return out, nil
}

func (r *purchaseFaceRepo) Count(ctx context.Context, q biz.PurchaseFaceQuery, noStateFilter bool) (int64, error) {
	whereSQL, args := r.where(q, noStateFilter)
	// 原 SQL: SELECT COUNT(*) AS think_count FROM ... [WHERE ...]
	// 注意 count 只 join archive，**不 join app**（与 lists() 不同）
	sql := "SELECT COUNT(*) FROM " + r.table("market_list_purchase") + " AS " + aliasMLP +
		" INNER JOIN " + r.table("app_archive") + " AS " + aliasAA +
		" ON " + aliasMLP + ".archive_id = " + aliasAA + ".id" + whereSQL

	var n int64
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

var _ = purchaseFaceColumns
