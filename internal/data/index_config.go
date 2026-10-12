package data

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// indexConfigRepo 对应 DecorateTabbar::getTabbarLists()。
type indexConfigRepo struct {
	db     *gorm.DB
	prefix string
}

func NewIndexConfigRepo(db *gorm.DB, c *conf.Data) biz.IndexConfigRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &indexConfigRepo{db: db, prefix: prefix}
}

// ListTabbar 逐字对应：
//
//	$tabbar = self::select()->toArray();      // ⚠️ 没有 where —— 全表
//	foreach ($tabbar as &$item) {
//	    if (!empty($item['selected']))   { $item['selected']   = FileService::getFileUrl($item['selected']); }
//	    if (!empty($item['unselected'])) { $item['unselected'] = FileService::getFileUrl($item['unselected']); }
//	}
//
// ⚠️ 三个要点：
//  1. **没有 where**，连 is_show 都不过滤（实测当前 3 行都是 1，但代码确实没过滤）
//  2. 表**没有 delete_time**，所以没有软删除过滤
//  3. `link` 在模型里声明为 json 类型（`$json = ['link']` + `$jsonAssoc = true`），
//     所以**要解码成对象**，不是原样输出字符串
//
// 图片 URL 的拼接在 biz 层做（这里只返原始路径），因为拼接需要请求域名。
func (r *indexConfigRepo) ListTabbar(ctx context.Context) ([]biz.TabbarRow, error) {
	var rows []struct {
		ID         int64  `gorm:"column:id"`
		Name       string `gorm:"column:name"`
		Selected   string `gorm:"column:selected"`
		Unselected string `gorm:"column:unselected"`
		Link       string `gorm:"column:link"`
		IsShow     int32  `gorm:"column:is_show"`
		CreateTime int64  `gorm:"column:create_time"`
		UpdateTime int64  `gorm:"column:update_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"decorate_tabbar").
		Select("id", "name", "selected", "unselected", "link", "is_show", "create_time", "update_time").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.TabbarRow, 0, len(rows))
	for _, x := range rows {
		row := biz.TabbarRow{
			ID: x.ID, Name: x.Name, Selected: x.Selected, Unselected: x.Unselected,
			LinkJSON: x.Link, IsShow: x.IsShow,
			CreateTime: x.CreateTime, UpdateTime: x.UpdateTime,
		}
		// 对应模型的 json 类型字段解码（$jsonAssoc = true -> 关联数组 -> 对外是对象）
		if x.Link != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(x.Link), &m); err == nil {
				row.Link = m
			}
		}
		out = append(out, row)
	}
	return out, nil
}
