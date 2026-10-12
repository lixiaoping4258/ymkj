package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// addressDeleteRepo 对应 AddressLogic::delete 的两条语句。
type addressDeleteRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAddressDeleteRepo(db *gorm.DB, c *conf.Data) biz.AddressDeleteRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &addressDeleteRepo{db: db, prefix: prefix}
}

// owned 构造 `where(['id'=>$id,'user_id'=>$userId])`。
//
// ⚠️ 带 user_id -> 越权防护。
func (r *addressDeleteRepo) owned(ctx context.Context, id int64, userID uint32) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"user_address").
		Where("id = ?", id).
		Where("user_id = ?", userID)
}

// ExistsOwned 对应 `UserAddress::where([...])->find()`（只关心是否存在）。
func (r *addressDeleteRepo) ExistsOwned(ctx context.Context, id int64, userID uint32) (bool, error) {
	var n int64
	err := r.owned(ctx, id, userID).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// HardDelete 对应 `$address->delete()`。
//
// ⚠️⚠️ **硬删除**：x_user_address **没有 delete_time 列**，所以这不是软删除 ——
// 行真的消失，不可恢复。不要在这里加 delete_time 赋值。
//
// 条件里仍带 user_id（与前面的查询保持一致，避免 TOCTOU 期间被换成别人的行）。
func (r *addressDeleteRepo) HardDelete(ctx context.Context, id int64, userID uint32) error {
	return r.owned(ctx, id, userID).Delete(nil).Error
}
