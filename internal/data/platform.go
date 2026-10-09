package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// platformRepo 对应 app/api/logic/open/OpenAppLogic.php 的 x_app 查询。
type platformRepo struct {
	db     *gorm.DB
	prefix string
}

func NewPlatformRepo(db *gorm.DB, c *conf.Data) biz.PlatformRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &platformRepo{db: db, prefix: prefix}
}

// ListThirdApps 逐字对应：
//
//	App::where(['state' => $state, 'type' => 2])
//	    ->field('name,flag')->select()->toArray();
//
// ⚠️ x_app 表**没有 delete_time 列**（已对建表结构核实），所以原文没有软删除过滤，
// 这里也不加。
func (r *platformRepo) ListThirdApps(
	ctx context.Context, state int32, appType int32,
) ([]biz.PlatformItem, error) {
	var rows []struct {
		Name string `gorm:"column:name"`
		Flag string `gorm:"column:flag"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app").
		Select("name", "flag").
		Where("state = ? AND type = ?", state, appType).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.PlatformItem, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.PlatformItem{Name: x.Name, Flag: x.Flag})
	}
	return out, nil
}
