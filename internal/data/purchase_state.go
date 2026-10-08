package data

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// purchaseStateRepo 对应 app/api/lists/market/purchase/PurchaseStateLists.php。
//
// 两分支的 SQL 都是 PHP CLI fetchSql(true) 实测出来的（不是推断）：
//
// COMPLETE 分支：
//
//	SELECT `MarketPurchase`.`id`,`MarketPurchase`.`pay_way`,... ,app.name as platform_name
//	FROM `x_market_purchase` `MarketPurchase`
//	INNER JOIN `x_app_archive` `AppArchive` ON `MarketPurchase`.`archive_id`=`AppArchive`.`id`
//	INNER JOIN `x_app` `app` ON `MarketPurchase`.`app_id`=`app`.`id`
//	WHERE ( `MarketPurchase`.`archive_id` = ? AND `MarketPurchase`.`state` IN ('COMPLETE','OFFLINE')
//	        AND `MarketPurchase`.`end_time` > ? AND `MarketPurchase`.`receive_amount` > '0' )
//	  AND `MarketPurchase`.`delete_time` IS NULL
//	ORDER BY `MarketPurchase`.`id` DESC
//
// WANTED 分支：条件换成 state = 'WANTED'（没有 receive_amount 条件），
//
//	ORDER BY `unit_price` DESC,`grab_time` ASC,`create_time` ASC,`id` DESC
//
// ⚠️ 注意 `AND delete_time IS NULL` —— 这个模型有 SoftDelete（ThinkPHP 隐式加，
// GORM 不会）。第六轮就是因为漏了它出过 bug，这里必须带上。
type purchaseStateRepo struct {
	db     *gorm.DB
	prefix string
	rdb    *redis.Client
	loc    *time.Location
}

func NewPurchaseStateRepo(db *gorm.DB, c *conf.Data, rdb *redis.Client, app *conf.App) biz.PurchaseStateRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	loc := time.Local
	if app != nil && app.Timezone != "" {
		if l, err := time.LoadLocation(app.Timezone); err == nil {
			loc = l
		}
	}
	return &purchaseStateRepo{db: db, prefix: prefix, rdb: rdb, loc: loc}
}

func (r *purchaseStateRepo) from() string {
	return r.prefix + "market_purchase AS MarketPurchase" +
		" INNER JOIN " + r.prefix + "app_archive AS AppArchive" +
		" ON MarketPurchase.archive_id = AppArchive.id" +
		" INNER JOIN " + r.prefix + "app AS app" +
		" ON MarketPurchase.app_id = app.id"
}

const purchaseStateCols = "MarketPurchase.id, MarketPurchase.pay_way, MarketPurchase.archive_id, " +
	"MarketPurchase.amount, MarketPurchase.receive_amount, MarketPurchase.unit_price, " +
	"MarketPurchase.create_time, MarketPurchase.grab_time, " +
	"AppArchive.name, AppArchive.issuer, AppArchive.images, app.name AS platform_name"

// where 逐行对照 PurchaseStateLists::queryWhere。
//
// 两分支的差别（容易抄错）：
//   - COMPLETE：state IN ('COMPLETE','OFFLINE')（下线状态的求购单也算"已完成"，
//     因为部分成功的求购单会被置为 OFFLINE）+ receive_amount > 0
//   - WANTED  ：state = 'WANTED'，**没有** receive_amount 条件
//   - 两者都有 end_time > now
//
// 另：原实现里还有一段"生产环境排除测试用户 [6,7,1000] 发布的求购单"的逻辑
// （!env('APP_DEBUG') 时生效）。当前 APP_DEBUG=true，该分支不生效；
// 迁移时需要 AppFlags 才能完整复刻，本批暂未接入 —— 见 README 的待办。
func (r *purchaseStateRepo) where(archiveID int64, state string) (string, []any) {
	now := time.Now().In(r.loc).Format("2006-01-02 15:04:05")
	conds := []string{
		"MarketPurchase.archive_id = ?",
		"MarketPurchase.end_time > ?",
		"MarketPurchase.delete_time IS NULL",
	}
	args := []any{archiveID, now}

	if state == biz.MktPurchaseStateComplete {
		conds = append(conds, "MarketPurchase.state IN (?)")
		args = append(args, []string{biz.MktPurchaseStateComplete, "OFFLINE"})
		conds = append(conds, "MarketPurchase.receive_amount > ?")
		args = append(args, 0)
	} else {
		conds = append(conds, "MarketPurchase.state = ?")
		args = append(args, state)
	}
	return " WHERE " + joinAnd(conds), args
}

func joinAnd(conds []string) string {
	out := ""
	for i, c := range conds {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}

// orderBy 逐字对应 lists() 里那行三元的两个分支。
func orderByState(state string) string {
	if state == biz.MktPurchaseStateWanted {
		return "MarketPurchase.unit_price DESC, MarketPurchase.grab_time ASC, " +
			"MarketPurchase.create_time ASC, MarketPurchase.id DESC"
	}
	return "MarketPurchase.id DESC"
}

type purchaseStateScan struct {
	ID            int64      `gorm:"column:id"`
	PayWay        *int32     `gorm:"column:pay_way"`
	ArchiveID     int64      `gorm:"column:archive_id"`
	Amount        int64      `gorm:"column:amount"`
	ReceiveAmount int64      `gorm:"column:receive_amount"`
	UnitPrice     *string    `gorm:"column:unit_price"`
	CreateTime    *time.Time `gorm:"column:create_time"`
	GrabTime      *time.Time `gorm:"column:grab_time"`
	Name          string     `gorm:"column:name"`
	Issuer        string     `gorm:"column:issuer"`
	Images        *string    `gorm:"column:images"`
	PlatformName  string     `gorm:"column:platform_name"`
}

func (r *purchaseStateRepo) List(
	ctx context.Context, archiveID int64, state string, offset, limit int,
) ([]biz.PurchaseStateRow, error) {
	whereSQL, args := r.where(archiveID, state)
	sql := "SELECT " + purchaseStateCols + " FROM " + r.from() + whereSQL +
		" ORDER BY " + orderByState(state) + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []purchaseStateScan
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.PurchaseStateRow, 0, len(rows))
	for _, s := range rows {
		out = append(out, biz.PurchaseStateRow{
			ID:            s.ID,
			PayWay:        s.PayWay,
			ArchiveID:     s.ArchiveID,
			Amount:        s.Amount,
			ReceiveAmount: s.ReceiveAmount,
			UnitPrice:     s.UnitPrice,
			CreateTime:    s.CreateTime,
			GrabTime:      s.GrabTime,
			Name:          s.Name,
			Issuer:        s.Issuer,
			Images:        s.Images,
			PlatformName:  s.PlatformName,
		})
	}
	return out, nil
}

func (r *purchaseStateRepo) Count(ctx context.Context, archiveID int64, state string) (int64, error) {
	whereSQL, args := r.where(archiveID, state)
	// count 不 join app（与 lists 不同，实测确认）
	sql := "SELECT COUNT(*) FROM " + r.prefix + "market_purchase AS MarketPurchase" +
		" INNER JOIN " + r.prefix + "app_archive AS AppArchive" +
		" ON MarketPurchase.archive_id = AppArchive.id" + whereSQL
	var n int64
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// FindListArchiveID 对应 MarketListPurchase::where(['id'=>$id])->field('id,archive_id')。
func (r *purchaseStateRepo) FindListArchiveID(ctx context.Context, listID int64) (int64, bool, error) {
	var row struct {
		ArchiveID int64 `gorm:"column:archive_id"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_list_purchase").
		Select("archive_id").
		Where("id = ?", listID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.ArchiveID, true, nil
}

// StockNum 对应 PurchaseOrderStockLogic::getNum($purchaseId)。
//
// 原实现用 Cache::store('redis')->handler() 拿 phpredis 句柄直接 get，
// **没有前缀、值就是整数的十进制字符串** —— 所以 Go 能读同一把键。
// 键名来自 PurchaseOrderStockLogic::purchaseOrderNeedKey()：
//
//	"x_PurchaseOrder:Stock:lock:{purchaseId}"
func (r *purchaseStateRepo) StockNum(ctx context.Context, purchaseID int64) (int64, bool, error) {
	if r.rdb == nil {
		return 0, false, nil
	}
	key := "x_PurchaseOrder:Stock:lock:" + itoa(purchaseID)
	v, err := r.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		// 上游（PHP 的 (int) 转换）对非数字/错误一律当 0，这里保持一致：
		// 不把 Redis 异常升级成接口失败，交给调用方走兜底。
		return 0, false, nil
	}
	return v, true, nil
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
