package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// ticketAmountRepo 对应 AmountLogic::queryAmount 的三段查询。
type ticketAmountRepo struct {
	db     *gorm.DB
	prefix string
}

func NewTicketAmountRepo(db *gorm.DB, c *conf.Data) *ticketAmountRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &ticketAmountRepo{db: db, prefix: prefix}
}

// ListWallet 逐字对应：
//
//	UserWallet::where(['user_id'=>$userId])
//	    ->field('amount,type_id,frozen_amount')->select()->toArray();
//
// ⚠️ 表 x_user_wallet 共 7 列（id,user_id,amount,type_id,create_time,frozen_amount,update_time），
//
//	**只取 3 列**。
//
// ⚠️ x_user_wallet **没有 delete_time** -> 不加软删除过滤。
// ⚠️ 原文**没有 order** —— 顺序由 MySQL 决定；不过后面会按 type_id 重排，
//
//	所以原始顺序只影响"同 type_id 多行"时的相对次序（正常数据一个 user 一个 type 一行）。
func (r *ticketAmountRepo) ListWallet(
	ctx context.Context, userID uint32,
) ([]biz.TicketWalletRow, error) {
	var rows []struct {
		Amount       string `gorm:"column:amount"`
		TypeID       int32  `gorm:"column:type_id"`
		FrozenAmount string `gorm:"column:frozen_amount"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_wallet").
		Select("amount", "type_id", "frozen_amount").
		Where("user_id = ?", userID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.TicketWalletRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.TicketWalletRow{
			Amount: x.Amount, TypeID: x.TypeID, FrozenAmount: x.FrozenAmount,
		})
	}
	return out, nil
}

// SumWaitAmount 逐字对应：
//
//	UserGetTicket::where(['user_id'=>$userId,
//	                      'state'=>CheckStateEnum::STATE_WAIT,   // 0
//	                      'ticket_type'=>$typeID])
//	    ->sum('change_amount');
//
// ⚠️ `STATE_WAIT = 0`（枚举常量，不是字符串）。
// ⚠️ UserGetTicket **extends Model、无 use SoftDelete** -> 不加 delete_time 过滤。
// ⚠️ 无匹配行时 ThinkPHP 的 sum() 返回 **0**；用 COALESCE 对齐。
//
//	返回值是 **字符串**（decimal 求和 PHP 会按字符串返回）。
func (r *ticketAmountRepo) SumWaitAmount(
	ctx context.Context, userID uint32, typeID int32,
) (string, error) {
	var sum struct {
		Total string `gorm:"column:total"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_get_ticket").
		Select("COALESCE(SUM(change_amount), 0) AS total").
		Where("user_id = ?", userID).
		Where("state = ?", 0).
		Where("ticket_type = ?", typeID).
		Take(&sum).Error
	if err != nil {
		return "", err
	}
	return sum.Total, nil
}

// WithdrawStatus 逐字对应 `UserAccounts::where(['user_id'=>..,'app_id'=>3,'state'=>'ENABLE'])
// ->value('sync_order_num')`。
//
// ⚠️ `app_id = 3`（大海盛世）与 `state = 'ENABLE'` 都是**硬编码**。
// ⚠️ x_user_accounts **有软删除**（第 41 轮踩过）-> 加 delete_time IS NULL。
// ⚠️ 实测 x_user_accounts 里 **sync_order_num 大量为 NULL** -> `null > 0` 为 false。
//
//	这里用 (值, 是否取到) 表达：取不到或为 NULL 都 -> false。
func (r *ticketAmountRepo) WithdrawStatus(
	ctx context.Context, userID uint32,
) (int64, bool, error) {
	var row struct {
		SyncOrderNum *int64 `gorm:"column:sync_order_num"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_accounts").
		Select("sync_order_num").
		Where("user_id = ?", userID).
		Where("app_id = ?", 3).
		Where("state = ?", "ENABLE").
		Where("delete_time IS NULL").
		Limit(1).
		Find(&row).Error
	if err != nil {
		return 0, false, err
	}
	if row.SyncOrderNum == nil {
		// 没有行，或者行的值是 NULL -> `null > 0` 为 false
		return 0, false, nil
	}
	return *row.SyncOrderNum, true, nil
}

var _ = errors.Is // 保留 errors 导入（后续可能用到）
