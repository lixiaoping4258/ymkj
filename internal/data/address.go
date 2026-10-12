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

/* ------------------------------------------------- 地址列表 */

type addressListRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAddressListRepo(db *gorm.DB, c *conf.Data) biz.AddressListRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &addressListRepo{db: db, prefix: prefix}
}

// baseJoin 构造与 PHP `hasWhere(”app”, ...)` 等价的 INNER JOIN。
//
// ⚠️ 必须是 INNER JOIN —— 见 biz 层 AddressListUsecase.List 的说明。
// ⚠️ list 与 count 共用这一个方法，保证 JOIN 条件一致（否则分页会错位）。
func (r *addressListRepo) baseJoin(ctx context.Context, userID uint32) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"user_address AS ua").
		Joins("INNER JOIN "+r.prefix+"app AS app ON app.id = ua.app_id").
		Where("ua.user_id = ?", userID)
}

// ListAddresses 逐字对应 AddressLists::lists()。
func (r *addressListRepo) ListAddresses(
	ctx context.Context, userID uint32, offset, limit int,
) ([]map[string]any, error) {
	var rows []struct {
		PlatformName *string `gorm:"column:platform_name"`
		Flag         *string `gorm:"column:flag"`
		ID           int64   `gorm:"column:id"`
		ContactUser  *string `gorm:"column:contact_user"`
		ContactPhone *string `gorm:"column:contact_phone"`
		Address      string  `gorm:"column:address"`
		Fettle       *string `gorm:"column:fettle"`
		IsDefault    int32   `gorm:"column:is_default"`
	}
	err := r.baseJoin(ctx, userID).
		Select(biz.AddressListSelect).
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{
			"platform_name": derefStrOrNil(x.PlatformName),
			"flag":          derefStrOrNil(x.Flag),
			"id":            x.ID,
			"contact_user":  derefStrOrNil(x.ContactUser),
			"contact_phone": derefStrOrNil(x.ContactPhone),
			"address":       x.Address,
			"fettle":        derefStrOrNil(x.Fettle),
			"is_default":    x.IsDefault,
		})
	}
	return out, nil
}

// CountAddresses 逐字对应 AddressLists::count() —— **同一个 INNER JOIN**。
func (r *addressListRepo) CountAddresses(ctx context.Context, userID uint32) (int64, error) {
	var n int64
	err := r.baseJoin(ctx, userID).Count(&n).Error
	return n, err
}

// derefStrOrNil 把 *string 转成 any：nil -> null，否则字符串。
//
// ⚠️ 这是**对外契约**的要求，不是风格问题：
// PHP 的 toArray() 对可空列输出 null，用普通 string 会变成 ""，
// 类型和值都不对（前端区分得了 null 和空串）。
func derefStrOrNil(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}
