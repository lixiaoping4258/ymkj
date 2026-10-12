package data

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// authMyUserRepo 对应 app/api/lists/open/AuthMyUserLists.php。
type authMyUserRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAuthMyUserRepo(db *gorm.DB, c *conf.Data) *authMyUserRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &authMyUserRepo{db: db, prefix: prefix}
}

// List 逐字对应 AuthMyUserLists::lists()。
//
//	AppAuth::alias('AppAuth')
//	    ->join('x_app App', 'AppAuth.app_id=App.id')
//	    ->join('x_user User', 'AppAuth.uid=User.id')
//	    ->where(['AppAuth.uid' => $userId])
//	    ->field('AppAuth.id,AppAuth.app_id,AppAuth.app_nickname,AppAuth.app_avatar,
//	            AppAuth.expire,App.name as app_name,User.mobile')
//	    ->limit(...)->order('AppAuth.expire asc')->select()->toArray();
//
// ⚠️ 原文的 join 里写的是**带前缀的裸表名** `'x_app App'` / `'x_user User'`
//
//	（ThinkPHP 的 join() 不会自动加前缀），所以这里用 prefix 拼出来。
//
// ⚠️ 两个 JOIN 都是 **INNER JOIN** —— uid 在 x_user 里不存在时该行不返回。
// ⚠️ x_app_auth 有 delete_time（软删除），模型会自动加 -> Go 侧显式补上。
func (r *authMyUserRepo) List(
	ctx context.Context, userID uint32, offset, limit int,
) ([]biz.AuthMyUserRow, error) {
	var rows []struct {
		ID          int64     `gorm:"column:id"`
		AppID       int32     `gorm:"column:app_id"`
		AppNickname string    `gorm:"column:app_nickname"`
		AppAvatar   string    `gorm:"column:app_avatar"`
		Expire      time.Time `gorm:"column:expire"`
		AppName     string    `gorm:"column:app_name"`
		Mobile      string    `gorm:"column:mobile"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth AS AppAuth").
		Joins("INNER JOIN "+r.prefix+"app AS App ON AppAuth.app_id = App.id").
		Joins("INNER JOIN "+r.prefix+"user AS User ON AppAuth.uid = User.id").
		Select("AppAuth.id AS id, AppAuth.app_id AS app_id, "+
			"AppAuth.app_nickname AS app_nickname, AppAuth.app_avatar AS app_avatar, "+
			"AppAuth.expire AS expire, App.name AS app_name, User.mobile AS mobile").
		Where("AppAuth.uid = ?", userID).
		Where("AppAuth.delete_time IS NULL").
		Order("AppAuth.expire ASC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.AuthMyUserRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.AuthMyUserRow{
			ID: x.ID, AppID: x.AppID, AppNickname: x.AppNickname,
			AppAvatar: x.AppAvatar, Expire: x.Expire.Format("2006-01-02 15:04:05"),
			AppName: x.AppName, Mobile: biz.MaskMobile(x.Mobile),
		})
	}
	return out, nil
}

// Count 逐字对应 AuthMyUserLists::count() —— 🔴 **只有两个 JOIN**：
//
//	AppAuth::alias('AppAuth')
//	    ->join('x_app App', 'AppAuth.app_id=App.id')     // ⚠️ 少了 x_user
//	    ->where(['AppAuth.uid' => $userId])->count();
//
// ⚠️⚠️ 与 lists() 的 JOIN 数量**不一致**，这是原项目的实现细节，**刻意照搬**。
//
//	后果：当某条授权的 uid 在 x_user 里不存在时，lists() 不返回它但 count() 会计入
//	-> **count 可能大于列表实际长度**（末页出现空列表）。
//	不要"顺手"把这里补成三表 JOIN。
func (r *authMyUserRepo) Count(ctx context.Context, userID uint32) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth AS AppAuth").
		Joins("INNER JOIN "+r.prefix+"app AS App ON AppAuth.app_id = App.id").
		Where("AppAuth.uid = ?", userID).
		Where("AppAuth.delete_time IS NULL").
		Count(&n).Error
	return n, err
}
