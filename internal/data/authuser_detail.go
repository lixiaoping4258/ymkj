package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// authUserDetailRepo 对应 AppUserLists::detail() 的两段查询。
type authUserDetailRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAuthUserDetailRepo(db *gorm.DB, c *conf.Data) *authUserDetailRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &authUserDetailRepo{db: db, prefix: prefix}
}

// FindAuthByID 逐字对应 `AppAuth::where('id', $id)->findOrEmpty()->toArray()`。
//
// ⚠️ 真实 SQL 是 **`SELECT *`**：
//
//	SELECT * FROM `x_app_auth` WHERE (`id` = ?) AND `x_app_auth`.`delete_time` IS NULL LIMIT 1
//
// ⚠️🔴 所以返回的是**全部 10 列**：
//
//	id, app_id, uid, app_uid, app_nickname, app_avatar, token, start, expire, delete_time
//
// **其中 `token` 是三方授权凭据，会被原样返回给前端** —— 原项目如此，
// 照搬（不改动逻辑），但已在 README 的原系统问题里记录为凭据外泄面。
//
// ⚠️ `delete_time` 在软删除过滤下恒为 NULL，但仍会出现在结果里（PHP 的 toArray 也会带上）。
func (r *authUserDetailRepo) FindAuthByID(
	ctx context.Context, id int64,
) (map[string]any, bool, error) {
	var row struct {
		ID          int64      `gorm:"column:id"`
		AppID       int32      `gorm:"column:app_id"`
		UID         int32      `gorm:"column:uid"`
		AppUID      *int32     `gorm:"column:app_uid"`
		AppNickname *string    `gorm:"column:app_nickname"`
		AppAvatar   *string    `gorm:"column:app_avatar"`
		Token       *string    `gorm:"column:token"`
		Start       *time.Time `gorm:"column:start"`
		Expire      *time.Time `gorm:"column:expire"`
		DeleteTime  *time.Time `gorm:"column:delete_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth").
		Select("*").
		Where("id = ?", id).
		Where("delete_time IS NULL").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	// 键名与列名一致（ThinkPHP 的 toArray 用列名）
	m := map[string]any{
		"id":           row.ID,
		"app_id":       row.AppID,
		"uid":          row.UID,
		"app_uid":      derefI32Any(row.AppUID),
		"app_nickname": derefStrOrNil(row.AppNickname),
		"app_avatar":   derefStrOrNil(row.AppAvatar),
		"token":        derefStrOrNil(row.Token),
		"start":        formatTimeAny(row.Start),
		"expire":       formatTimeAny(row.Expire),
		"delete_time":  formatTimeAny(row.DeleteTime),
	}
	return m, true, nil
}

// FindAppDetail 逐字对应 `AppLists::detail($appId)`：
//
//	App::where('id', $appId)->field('id, name, logo, type, fee_rate, flag')
//	    ->findOrEmpty()->toArray()
//
// ⚠️ 6 列；**不处理 logo**（返回相对路径）。与 AppLists::lists() 不同。
// ⚠️ 查不到 -> 返回 (nil,false)，handler 输出 `[]`（PHP 的 toArray() 空数组）。
func (r *authUserDetailRepo) FindAppDetail(ctx context.Context, appID int64) (*biz.AppInfoRow, bool, error) {
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
	return &biz.AppInfoRow{
		ID: row.ID, Name: row.Name, Logo: row.Logo,
		Type: row.Type, FeeRate: row.FeeRate, Flag: row.Flag,
	}, true, nil
}

// derefI32Any nil -> null
func derefI32Any(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

// formatTimeAny nil -> null；否则 "Y-m-d H:i:s"
//
// ⚠️ datetime 列必须格式化（清单第 1 条）—— 直接输出 time.Time 会变 RFC3339。
func formatTimeAny(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Format("2006-01-02 15:04:05")
}
