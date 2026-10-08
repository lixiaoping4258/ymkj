package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type whitelistRepo struct {
	db *gorm.DB
}

func NewWhitelistRepo(db *gorm.DB) biz.WhitelistRepo {
	return &whitelistRepo{db: db}
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
func (r *whitelistRepo) PurchasePayWay(ctx context.Context, purchaseID uint64) (int32, bool, error) {
	var row struct {
		PayWay *int32 `gorm:"column:pay_way"`
	}
	err := r.db.WithContext(ctx).
		Model(&model.MarketPurchase{}).
		Select("pay_way").
		Where("id = ?", purchaseID).
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
