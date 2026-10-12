package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type whitelistRepo struct {
	db     *gorm.DB
	prefix string
}

// NewWhitelistRepo 返回**具体类型**，便于 data.go 里用 wire.Bind 绑定两个接口。
func NewWhitelistRepo(db *gorm.DB, cf *conf.Data) *whitelistRepo {
	prefix := "la_"
	if cf != nil && cf.Database != nil && cf.Database.Prefix != "" {
		prefix = cf.Database.Prefix
	}
	return &whitelistRepo{db: db, prefix: prefix}
}

// EnabledItems 对应 WhitelistItem::where('status',1)->column('code','id')。
// ThinkPHP 的 column('code','id') 返回 [id => code]。
func (r *whitelistRepo) EnabledItems(ctx context.Context) (map[uint64]string, error) {
	var rows []model.WhitelistItem
	err := r.db.WithContext(ctx).
		Model(&model.WhitelistItem{}).
		Select("id, code").
		Where("status = ?", 1).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.Code
	}
	return out, nil
}

// FindUserGroupID 对应 WhitelistUser::where('user_id',$userId)->find()。
func (r *whitelistRepo) FindUserGroupID(ctx context.Context, userID uint64) (uint64, bool, error) {
	var row model.WhitelistUser
	err := r.db.WithContext(ctx).
		Model(&model.WhitelistUser{}).
		Select("group_id").
		Where("user_id = ?", userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.GroupID, true, nil
}

// IsGroupEnabled 对应
// WhitelistGroup::where('id',..)->where('status',1)->find()。
func (r *whitelistRepo) IsGroupEnabled(ctx context.Context, groupID uint64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.WhitelistGroup{}).
		Where("id = ? AND status = ?", groupID, 1).
		Limit(1).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GroupItemEnabled 对应
// WhitelistGroupItem::where('group_id',..)->column('enabled','item_id')。
// 返回 item_id -> enabled。
func (r *whitelistRepo) GroupItemEnabled(ctx context.Context, groupID uint64) (map[uint64]int, error) {
	var rows []model.WhitelistGroupItem
	err := r.db.WithContext(ctx).
		Model(&model.WhitelistGroupItem{}).
		Select("item_id, enabled").
		Where("group_id = ?", groupID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]int, len(rows))
	for _, row := range rows {
		out[row.ItemID] = int(row.Enabled)
	}
	return out, nil
}

// PurchasePayWay 对应
// MarketPurchase::where([['id','=',$context['purchase_id']]])->field('id,pay_way')->findOrEmpty()。
//
// ⚠️ 同样要带 `delete_time IS NULL`（MarketPurchase 用了 SoftDelete，
// ThinkPHP 隐式加条件）。已软删除的兑换单在 PHP 侧查不到，
// 这里若查得到就会多做一次细粒度判断，行为就不一致了。
func (r *whitelistRepo) PurchasePayWay(ctx context.Context, purchaseID uint64) (int32, bool, error) {
	var row struct {
		PayWay *int32 `gorm:"column:pay_way"`
	}
	err := r.db.WithContext(ctx).
		Model(&model.MarketPurchase{}).
		Select("pay_way").
		Where("id = ?", purchaseID).
		Where(model.NotDeletedMktPurchase).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if row.PayWay == nil {
		return 0, true, nil
	}
	return *row.PayWay, true, nil
}

/* ------------------------------------------------- group 接口 */

// FindGroupRow 逐字对应 `WhitelistGroup::find($groupID)`。
//
// ⚠️ 真实 SQL 是 **`SELECT * FROM x_whitelist_group WHERE id = ?`**
//
//	（无 `delete_time` 过滤；该表也没有该列）。
//
// ⚠️ 用 map 扫描承接任意列，再由 biz 层把 int 时间戳转成 datetime 字符串
//
//	（已用真实 PHP 对拍：toArray() 输出的是 "2026-08-18 18:01:34"）。
//
// ⚠️ 表名前缀用 `conf` 里的（默认 la_）—— 这里沿用注入的 db 的 NamingStrategy？
//
//	不：GORM 的 Table() **不会**自动加前缀，必须手工拼。
func (r *whitelistRepo) FindGroupRow(ctx context.Context, groupID uint64) (map[string]any, bool, error) {
	var rows []map[string]any
	err := r.db.WithContext(ctx).
		Table(r.prefix+"whitelist_group").
		Where("id = ?", groupID).
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return rows[0], true, nil
}
