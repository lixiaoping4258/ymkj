package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// saleDetailRepo 对应 SaleLogic::getSaleDetail 的四段查询。
type saleDetailRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSaleDetailRepo(db *gorm.DB, c *conf.Data) *saleDetailRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &saleDetailRepo{db: db, prefix: prefix}
}

// saleDetailSelect = 原文 field([...]) 的 12 列。
//
// ⚠️ `app.name platform_name` / `app.id platform_id` —— 别名是平台名/平台 ID。
// ⚠️ 两个 join 是 `app_archive` / `app`（无前缀写法，ThinkPHP 会补；见清单第 2 条）。
const saleDetailSelect = "market_sales.id AS id, market_sales.user_id AS user_id, " +
	"market_sales.sn AS sn, market_sales.price AS price, market_sales.state AS state, " +
	"market_sales.content AS content, market_sales.publish_time AS publish_time, " +
	"app_archive.collection_id AS collection_id, app_archive.name AS name, " +
	"app_archive.images AS images, app.name AS platform_name, app.id AS platform_id"

// FindSale 逐字对应 getSaleDetail 的第一段查询。
//
// ⚠️ `state IN ('ON_SALE','SELL_OUT')` —— **OFFLINE（下架）查不到**。
// ⚠️ MarketSales **无软删除**（模型未 use SoftDelete，表也没有 delete_time）。
func (r *saleDetailRepo) FindSale(ctx context.Context, id int64) (map[string]any, bool, error) {
	var rows []map[string]any
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_sales AS market_sales").
		Joins("INNER JOIN "+r.prefix+"app_archive AS app_archive ON market_sales.archive_id = app_archive.id").
		Joins("INNER JOIN "+r.prefix+"app AS app ON market_sales.app_id = app.id").
		Select(saleDetailSelect).
		Where("market_sales.id = ?", id).
		Where("market_sales.state IN ?", []string{biz.SalesStateOnSale, biz.SalesStateSellOut}).
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	// datetime 列（publish_time）格式化 —— 清单第 1 条
	biz.FormatTimeValuesInMap(rows[0])
	return rows[0], true, nil
}

// FindSaler 逐字对应 `User::where(['id'=>$uid])->field('nickname,avatar')->findOrEmpty()`。
//
// ⚠️ 用**模型**（User 有软删除）-> 加 delete_time IS NULL。
func (r *saleDetailRepo) FindSaler(
	ctx context.Context, userID int64,
) (string, string, bool, error) {
	var row struct {
		Nickname string `gorm:"column:nickname"`
		Avatar   string `gorm:"column:avatar"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user").
		Select("nickname", "avatar").
		Where("id = ?", userID).
		Where("delete_time IS NULL").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.Nickname, row.Avatar, true, nil
}

// CountSalesByState 逐字对应：
//
//	MarketSales::where(['user_id'=>.., 'state'=>['ON_SALE']])->count()
//	MarketSales::where(['user_id'=>.., 'state'=>['SELL_OUT']])->count()
//
// ⚠️ 两处都用**单元素数组**形式（`state => ['ON_SALE']`），等价于 `state = 'ON_SALE'`。
func (r *saleDetailRepo) CountSalesByState(ctx context.Context, userID int64, state string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_sales").
		Where("user_id = ?", userID).
		Where("state = ?", state).
		Count(&n).Error
	return n, err
}
