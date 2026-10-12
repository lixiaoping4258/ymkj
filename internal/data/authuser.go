package data

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// authUserRepo 对应 app/api/lists/open/AppUserLists.php。
type authUserRepo struct {
	db     *gorm.DB
	prefix string
}

func NewAuthUserRepo(db *gorm.DB, c *conf.Data) *authUserRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &authUserRepo{db: db, prefix: prefix}
}

// List 的 SQL（已用真实 PHP 对模型 fetchSql 验证）：
//
//	SELECT `app_auth`.`id`,`app_auth`.`app_id`,`app_auth`.`app_nickname`,
//	       `app_auth`.`app_avatar`,`app_auth`.`start`,`app_auth`.`expire`,`u`.`mobile`
//	FROM `x_app_auth` `app_auth`
//	INNER JOIN `x_user` `u` ON `app_auth`.`uid`=`u`.`id`
//	WHERE ( ( `uid` = ? AND `app_id` = ? ) ) AND `app_auth`.`delete_time` IS NULL
//	ORDER BY `id` DESC LIMIT ?,?
//
// ⚠️ `delete_time IS NULL` 来自 **AppAuth 模型的 SoftDelete**（不是 Db::name()）——
//
//	见 README 清单第 8.5 条（探测必须用模型）。
//
// ⚠️ `ORDER BY id` 原文无限定名；MySQL 解析为左表 `app_auth.id`（已实测不报 ambiguous）。
//
//	这里显式写 `app_auth.id` 以固定该行为。
func (r *authUserRepo) List(
	ctx context.Context, userID uint32, appID int64, offset, limit int,
) ([]biz.AuthUserRow, error) {
	var rows []struct {
		ID          int64     `gorm:"column:id"`
		AppID       int32     `gorm:"column:app_id"`
		AppNickname string    `gorm:"column:app_nickname"`
		AppAvatar   string    `gorm:"column:app_avatar"`
		Start       time.Time `gorm:"column:start"`
		Expire      time.Time `gorm:"column:expire"`
		Mobile      string    `gorm:"column:mobile"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth AS app_auth").
		Joins("INNER JOIN "+r.prefix+"user AS u ON app_auth.uid = u.id").
		Select("app_auth.id AS id, app_auth.app_id AS app_id, "+
			"app_auth.app_nickname AS app_nickname, app_auth.app_avatar AS app_avatar, "+
			"app_auth.start AS start, app_auth.expire AS expire, u.mobile AS mobile").
		Where("uid = ?", userID).
		Where("app_id = ?", appID).
		Where("app_auth.delete_time IS NULL").
		Order("app_auth.id DESC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.AuthUserRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.AuthUserRow{
			ID: x.ID, AppID: x.AppID, AppNickname: x.AppNickname, AppAvatar: x.AppAvatar,
			Start:  x.Start.Format("2006-01-02 15:04:05"),
			Expire: x.Expire.Format("2006-01-02 15:04:05"),
			Mobile: x.Mobile,
		})
	}
	return out, nil
}

// Count 的 SQL（真实 PHP 对模型 fetchSql 验证）：
//
//	SELECT COUNT(*) AS think_count FROM `x_app_auth`
//	WHERE ( ( `uid` = ? AND `app_id` = ? ) ) AND `x_app_auth`.`delete_time` IS NULL
//
// 🔴 **没有任何 JOIN** —— 与 lists() 不同。照搬这个不一致（清单第 3 条）。
func (r *authUserRepo) Count(ctx context.Context, userID uint32, appID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_auth").
		Where("uid = ?", userID).
		Where("app_id = ?", appID).
		Where("delete_time IS NULL").
		Count(&n).Error
	return n, err
}

// ListAppInfo 逐字对应 `(new AppLists())->listsWithQuery([['id','in',...]])`：
//
//	App::where(array_merge([['state','=',1],['type','=',2]], [['id','in',$ids]]))
//	    ->order('id','desc')->limit($offset,$limit)
//	    ->field('id, name, state, logo, type, fee_rate, flag')->select()->toArray();
//
// ⚠️ **7 列**（比 AppLists::lists() 多 `state`）。
// ⚠️ **不处理 logo**（lists() 会经 get_image_url 拼域名）—— 返回库里的相对路径。
// ⚠️ `state=1 AND type=2` 也是条件的一部分 —— **已下架或非三方的 app 查不到**，
//
//	对应到外层就是 `app_info` 为 null。
//
// ⚠️ 分页参数用的是**当前请求的** offset/limit（见 biz 层说明）。
// ⚠️ App 模型无软删除（x_app 表也没有 delete_time）-> 不加过滤。
func (r *authUserRepo) ListAppInfo(
	ctx context.Context, appIDs []int64, offset, limit int,
) ([]biz.AppInfoRow, error) {
	var rows []struct {
		ID      int64  `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		State   int32  `gorm:"column:state"`
		Logo    string `gorm:"column:logo"`
		Type    int32  `gorm:"column:type"`
		FeeRate string `gorm:"column:fee_rate"`
		Flag    string `gorm:"column:flag"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Select("id", "name", "state", "logo", "type", "fee_rate", "flag").
		Where("state = ?", 1).
		Where("type = ?", 2).
		Where("id IN ?", appIDs).
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.AppInfoRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.AppInfoRow{
			ID: x.ID, Name: x.Name, State: x.State, Logo: x.Logo,
			Type: x.Type, FeeRate: x.FeeRate, Flag: x.Flag,
		})
	}
	return out, nil
}
