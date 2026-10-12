package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// openAppRepo 对应 app/common/model/App.php 的查询。
type openAppRepo struct {
	db     *gorm.DB
	prefix string
}

// NewOpenAppRepo 返回**具体类型**，以便 data.go 里用 wire.Bind 同时绑定两个接口。
func NewOpenAppRepo(db *gorm.DB, c *conf.Data) *openAppRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &openAppRepo{db: db, prefix: prefix}
}

// base 构造 lists/count 共用的条件：state=1 AND type=2。
func (r *openAppRepo) base(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Where("state = ?", biz.OpenAppStateEnabled).
		Where("type = ?", biz.OpenAppTypeThird)
}

// ListApps 逐字对应 AppLists::lists()。
//
// ⚠️ 排序是 `order('id','desc')`（单字段）。
// ⚠️ 只取 5 列，不含 state / fee_rate。
func (r *openAppRepo) ListApps(ctx context.Context, offset, limit int) ([]biz.OpenAppRow, error) {
	var rows []struct {
		ID   int64  `gorm:"column:id"`
		Name string `gorm:"column:name"`
		Logo string `gorm:"column:logo"`
		Type int32  `gorm:"column:type"`
		Flag string `gorm:"column:flag"`
	}
	err := r.base(ctx).
		Select("id", "name", "logo", "type", "flag").
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.OpenAppRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.OpenAppRow{
			ID: x.ID, Name: x.Name, Logo: x.Logo, Type: x.Type, Flag: x.Flag,
		})
	}
	return out, nil
}

// CountApps 逐字对应 AppLists::count() —— **同一个条件**。
func (r *openAppRepo) CountApps(ctx context.Context) (int64, error) {
	var n int64
	err := r.base(ctx).Count(&n).Error
	return n, err
}

// FindApp 逐字对应 AppLists::detail()：
//
//	App::where('id', $app_id)->field('id, name, logo, type, fee_rate, flag')->findOrEmpty()
//
// ⚠️ **不筛 state / type** —— 与 lists 不同，detail 能查到已下架或非三方的 app。
//
//	这是原文行为，照搬。
//
// ⚠️ 表里**没有 delete_time** 列（已核实：x_app 无该列）-> 不加软删除过滤。
func (r *openAppRepo) FindApp(ctx context.Context, appID int64) (*biz.OpenAppDetailRow, bool, error) {
	var row struct {
		ID      int64  `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		Logo    string `gorm:"column:logo"`
		Type    int32  `gorm:"column:type"`
		FeeRate string `gorm:"column:fee_rate"`
		Flag    string `gorm:"column:flag"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Select("id", "name", "logo", "type", "fee_rate", "flag").
		Where("id = ?", appID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.OpenAppDetailRow{
		ID: row.ID, Name: row.Name, Logo: row.Logo,
		Type: row.Type, FeeRate: row.FeeRate, Flag: row.Flag,
	}, true, nil
}

// FindAuthURL 逐字对应 OpenAppLogic::authUrl 的第一段查询：
//
//	App::where('state','=',1)->where('id','=',$appId)->find()
//
// ⚠️ **不筛 type** —— 与 AppLists 不同，type=1 的本地系统也能查到。
// ⚠️ 只取 auth_url 一列。
func (r *openAppRepo) FindAuthURL(ctx context.Context, appID int64) (string, bool, error) {
	var row struct {
		AuthURL *string `gorm:"column:auth_url"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Select("auth_url").
		Where("state = ?", 1).
		Where("id = ?", appID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if row.AuthURL == nil {
		return "", true, nil // 行存在但 auth_url 是 NULL -> 后面走"未获取到授权地址"
	}
	return *row.AuthURL, true, nil
}
