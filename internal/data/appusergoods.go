package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// appUserGoodsRepo 对应 app/api/lists/open/AppUserGoodsLists.php。
type appUserGoodsRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAppUserGoodsRepo(db *gorm.DB, c *conf.Data) *appUserGoodsRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &appUserGoodsRepo{db: db, prefix: prefix}
}

// base 构造 queryWhere() 的条件（表别名固定为 MarketSales）。
//
// ⚠️ scene 的 default 与 case 1 都是 OFFLINE —— 见 biz 层说明。
// ⚠️ `!empty()` 语义：0 / ” 视为未传。
func (r *appUserGoodsRepo) base(ctx context.Context, q biz.AppUserGoodsQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"market_sales AS MarketSales").
		Where("MarketSales.user_id = ?", q.UserID)

	switch q.Scene {
	case 2:
		// 销售中
		tx = tx.Where("MarketSales.state = ?", biz.SalesStateOnSale).
			Where("MarketSales.sale_lock <> ?", biz.SalesLockLock)
	case 3:
		// 兑换中
		tx = tx.Where("MarketSales.state = ?", biz.SalesStateOnSale).
			Where("MarketSales.sale_lock = ?", biz.SalesLockLock)
	default:
		// scene 未传 / 1 / 其它 -> 待上架
		tx = tx.Where("MarketSales.state = ?", biz.SalesStateOffline)
	}

	if q.AppID != 0 {
		tx = tx.Where("MarketSales.app_id = ?", q.AppID)
	}
	if q.AppAuthID != 0 {
		tx = tx.Where("app_auth_id = ?", q.AppAuthID)
	}
	return tx
}

// archiveJoin 是 `hasWhere('archive', ...)` 生成的 INNER JOIN：
//
//	INNER JOIN `x_app_archive` `AppArchive` ON `AppArchive`.`id` = `MarketSales`.`archive_id`
//
// 关联定义见 MarketSales::archive() = hasOne(AppArchive::class,'id','archive_id')。
func (r *appUserGoodsRepo) archiveJoin(ctx context.Context, q biz.AppUserGoodsQuery) *gorm.DB {
	return r.base(ctx, q).
		Joins("INNER JOIN " + r.prefix + "app_archive AS AppArchive ON AppArchive.id = MarketSales.archive_id")
}

// ListGoods 逐字对应 lists()（**四张表**）：
//
//	MarketSales::hasWhere('archive', $where, <12 字段>)
//	    ->join('app','MarketSales.app_id=app.id')
//	    ->join('app_auth','MarketSales.app_auth_id=app_auth.id')
//	    ->order('id desc')->limit(...)->select()->toArray();
//
// ⚠️ `join('app')` / `join('app_auth')` 会被 ThinkPHP 补前缀 -> `x_app` / `x_app_auth`
//
//	（已用 fetchSql 验证过该行为，见 README 清单第 2 条）。
//
// ⚠️ `order('id desc')` 原文无限定名；四张表都有 id，MySQL 取左表
//
//	（实测同结构查询不报 ambiguous）。显式写 MarketSales.id DESC 固定该行为。
//
// ⚠️ MarketSales **没有软删除**（模型未 use SoftDelete，表也无 delete_time）。
// ⚠️ x_app_auth 是**物理 JOIN**，不走模型 -> **不加 delete_time 过滤**
//
//	（原文这里是 `join()` 不是模型关联，所以软删除不生效）。
func (r *appUserGoodsRepo) ListGoods(
	ctx context.Context, q biz.AppUserGoodsQuery, offset, limit int,
) ([]map[string]any, error) {
	var rows []map[string]any
	err := r.archiveJoin(ctx, q).
		Joins("INNER JOIN " + r.prefix + "app AS app ON MarketSales.app_id = app.id").
		Joins("INNER JOIN " + r.prefix + "app_auth AS app_auth ON MarketSales.app_auth_id = app_auth.id").
		Select(biz.AppUserGoodsSelect).
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

// CountGoods 逐字对应 count()：
//
//	MarketSales::hasWhere('archive', $this->queryWhere())->count();
//
// 🔴 **只有 `hasWhere('archive')` 一个 JOIN**，没有 `app` / `app_auth` 两个 join
//
//	—— 又一处 lists/count 不一致（清单第 3 条）。照搬。
func (r *appUserGoodsRepo) CountGoods(ctx context.Context, q biz.AppUserGoodsQuery) (int64, error) {
	var n int64
	err := r.archiveJoin(ctx, q).Count(&n).Error
	return n, err
}

// FindAppAuthID 逐字对应 `AppAuth::getAppAuthInfo($uid, $app_id, $app_uid)`：
//
//	return self::where('app_id','=',$app_id)->where('uid','=',$uid)
//	           ->where('app_uid','=',$app_uid)->find()->toArray();
//
// ⚠️ 用的是**模型** -> **软删除生效**（AppAuth 有 use SoftDelete）。
// ⚠️ 原文没接返回值：查不到时 `->find()` 返回 null，`->toArray()` 会**抛错**
//
//	（Call to a member function toArray() on null）。但调用处写的是
//	`if ($appAuthInfo) {...}` —— 若真抛错就不会走到判断。
//	Go 侧按"查不到 -> 不追加条件"实现（这是唯一不炸的可能行为）。
func (r *appUserGoodsRepo) FindAppAuthID(
	ctx context.Context, userID uint32, appID int64, appUID string,
) (int64, bool, error) {
	var row struct {
		ID int64 `gorm:"column:id"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth").
		Select("id").
		Where("app_id = ?", appID).
		Where("uid = ?", userID).
		Where("app_uid = ?", appUID).
		Where("delete_time IS NULL").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.ID, true, nil
}
