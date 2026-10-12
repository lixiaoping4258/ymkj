package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// balanceQueryRepo 对应 BalanceWalletLogic::queryBalance 的两条查询。
type balanceQueryRepo struct {
	db     *gorm.DB
	prefix string
}

func NewBalanceQueryRepo(db *gorm.DB, c *conf.Data) biz.BalanceQueryRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &balanceQueryRepo{db: db, prefix: prefix}
}

// FindPayConfigByWayID 逐字对应：
//
//	PayWay::hasWhere('payConfig',
//	        ['PayWay.id'=>$channel, 'PayWay.status'=>1],
//	        'PayConfig.id,PayConfig.pay_way')->find();
//
// ⚠️ 表名 x_dev_pay_way / x_dev_pay_config（模型 `protected $name = 'dev_pay_way'`）。
// ⚠️ INNER JOIN（hasWhere 默认）—— 没有对应 pay_config 的支付方式查不到。
// ⚠️ 条件里的 `PayWay.id` = 传入的 channel（**支付方式 ID**，不是 scene）。
func (r *balanceQueryRepo) FindPayConfigByWayID(
	ctx context.Context, channel int64,
) (int32, bool, error) {
	var row struct {
		PayWay int32 `gorm:"column:pay_way"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"dev_pay_way AS PayWay").
		Joins("INNER JOIN "+r.prefix+"dev_pay_config AS PayConfig ON PayConfig.id = PayWay.pay_config_id").
		Select("PayConfig.pay_way AS pay_way").
		Where("PayWay.id = ?", channel).
		Where("PayWay.status = ?", 1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.PayWay, true, nil
}

// UserMoney 逐字对应：
//
//	User::where(['id'=>$userId, 'is_disable'=>0])->value('user_money')
//
// ⚠️ `is_disable = 0` 是硬编码的 —— 被禁用的用户查不到，value() 返回 null。
// ⚠️ `x_user` 有软删除（delete_time），但原文这里**没有显式过滤**；
//
//	ThinkPHP 的模型会自动加 delete_time IS NULL，所以 Go 侧也要加，
//	否则被软删的用户会返回余额而 PHP 返回 null。
func (r *balanceQueryRepo) UserMoney(
	ctx context.Context, userID uint32,
) (string, bool, error) {
	var row struct {
		UserMoney string `gorm:"column:user_money"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user").
		Select("user_money").
		Where("id = ?", userID).
		Where("is_disable = ?", 0).
		Where("delete_time IS NULL").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.UserMoney, true, nil
}
