package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// unregisterCheckRepo 对应 UserLogic::unregisterCheck 的三段查询。
type unregisterCheckRepo struct {
	db     *gorm.DB
	prefix string
}

func NewUnregisterCheckRepo(db *gorm.DB, c *conf.Data) biz.UnregisterCheckRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &unregisterCheckRepo{db: db, prefix: prefix}
}

// CountUnsettledOrders 对应：
//
//	MarketPurchase::where('user_id','=',$userId)
//	    ->where(function ($q) {
//	        $q->where('state','=',WANTED)->whereOr('pay_state','=',PAYING);
//	    })->count()
//
// ⚠️⚠️ 括号必须显式打出来。ThinkPHP 的闭包 `where(...)` 会生成
//
//	`AND (state = 'WANTED' OR pay_state = 'PAYING')`；
//	若不分组写成 `user_id = ? AND state = ? OR pay_state = ?`，
//	因为 AND 优先级高于 OR，实际语义会变成
//	`(user_id = ? AND state = ?) OR (pay_state = ?)` —— **会统计到别的用户的记录**。
//
// ⚠️ MarketPurchase 有软删除（第 6 轮踩过），必须显式加 delete_time IS NULL。
func (r *unregisterCheckRepo) CountUnsettledOrders(ctx context.Context, userID uint32) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_purchase").
		Where("user_id = ?", userID).
		Where(r.db.Where("state = ?", biz.MktStateWanted).
			Or("pay_state = ?", biz.MktPayPaying)).
		Where("delete_time IS NULL").
		Count(&n).Error
	return n, err
}

// CountBoundPlatformAccounts 逐字对应：
//
//	UserAccounts::where(['user_id'=>$userId,'type'=>2])->whereIn('app_id',[3,4])->count()
//
// ⚠️ `app_id IN (3,4)` 是硬编码的（3=大海盛世 / 4=陶交所，实测 x_app 里这两条）。
// ⚠️ 该表有软删除 -> 加 delete_time IS NULL（与 login 域一致）。
func (r *unregisterCheckRepo) CountBoundPlatformAccounts(ctx context.Context, userID uint32) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_accounts").
		Where("user_id = ? AND type = ?", userID, 2).
		Where("app_id IN ?", []int{3, 4}).
		Where("delete_time IS NULL").
		Count(&n).Error
	return n, err
}

// HasNonZeroWallet 对应：
//
//	$rows = UserWallet::where(['user_id'=>$userId])->field('amount,frozen_amount')->select();
//	foreach ($rows as $w) { if ($w['amount'] > 0 || $w['frozen_amount'] > 0) { ... break; } }
//
// ⚠️ 逐行判断 `> 0` 而不是 `!= 0` —— 两列都是 `decimal(10,2) unsigned`，
//
//	不可能为负，所以 `> 0` 与 `!= 0` 等价；但按原文写 `> 0` 更稳。
//
// ⚠️ 表里**没有 delete_time** 列 -> 无软删除过滤。
func (r *unregisterCheckRepo) HasNonZeroWallet(ctx context.Context, userID uint32) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_wallet").
		Where("user_id = ?", userID).
		Where("amount > 0 OR frozen_amount > 0").
		Limit(1).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
