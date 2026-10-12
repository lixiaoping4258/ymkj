package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// addressRepo 对应 app/common/model/user/UserAddress.php 的查询。
type addressRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAddressRepo(db *gorm.DB, c *conf.Data) biz.AddressRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &addressRepo{db: db, prefix: prefix}
}

// addressFields 逐字对应原文的 `$fields = "id,is_default,address"`。
//
// ⚠️ 只取这三列 —— 表里还有 contact_user / contact_phone / fettle，原文不返回。
const addressFields = "id,is_default,address"

func (r *addressRepo) scan(ctx context.Context, q *gorm.DB) (*biz.AddressRow, bool, error) {
	var row struct {
		ID        int64  `gorm:"column:id"`
		IsDefault int32  `gorm:"column:is_default"`
		Address   string `gorm:"column:address"`
	}
	err := q.WithContext(ctx).
		Table(r.prefix+"user_address").
		Select("id", "is_default", "address").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.AddressRow{ID: row.ID, IsDefault: row.IsDefault, Address: row.Address}, true, nil
}

// FindByID 对应 `UserAddress::where(['user_id'=>$uid,'id'=>$id])->field($fields)->find()`。
//
// ⚠️ 查询条件里**同时带 user_id** —— 这是越权防护：不能查别人的地址。
func (r *addressRepo) FindByID(ctx context.Context, userID uint32, id int64) (*biz.AddressRow, bool, error) {
	return r.scan(ctx, r.db.Where("user_id = ? AND id = ?", userID, id))
}

// FindDefault 对应 `UserAddress::where(['user_id'=>$uid,'is_default'=>1])->field($fields)->find()`。
func (r *addressRepo) FindDefault(ctx context.Context, userID uint32) (*biz.AddressRow, bool, error) {
	return r.scan(ctx, r.db.Where("user_id = ? AND is_default = ?", userID, 1))
}

// FindFirst 对应：
//
//	UserAddress::where(['user_id'=>$userId])->field($fields)->order('id asc')->find();
//
// ⚠️ 是 `id asc`（取最早的一条），不是 desc。
// ⚠️ 表里**没有 delete_time** 列，所以没有软删除过滤。
func (r *addressRepo) FindFirst(ctx context.Context, userID uint32) (*biz.AddressRow, bool, error) {
	return r.scan(ctx, r.db.Where("user_id = ?", userID).Order("id ASC"))
}
