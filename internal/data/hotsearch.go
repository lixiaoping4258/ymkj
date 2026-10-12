package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// hotSearchRepo 对应 app/common/model/HotSearch.php。
type hotSearchRepo struct {
	db     *gorm.DB
	prefix string
}

func NewHotSearchRepo(db *gorm.DB, c *conf.Data) biz.HotSearchRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &hotSearchRepo{db: db, prefix: prefix}
}

// ListHotSearch 逐字对应：
//
//	HotSearch::field(['name','sort'])
//	    ->order(['sort'=>'desc','id'=>'desc'])->select()->toArray()
//
// ⚠️ 排序是**两个字段**：sort desc 优先，sort 相同再 id desc。
//
//	只写一个会让同 sort 的记录顺序不稳定（前端看到的次序可能变）。
//
// ⚠️ `x_hot_search` **没有 delete_time 列**（已对建表结构核实：
//
//	id / name / sort / create_time），所以**不加**软删除过滤。
//	虽然模型是 `extends BaseModel`，但表里没有该列，加了会直接报 SQL 错误。
func (r *hotSearchRepo) ListHotSearch(ctx context.Context) ([]biz.HotSearchItem, error) {
	var rows []struct {
		Name string `gorm:"column:name"`
		Sort int32  `gorm:"column:sort"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"hot_search").
		Select("name", "sort").
		Order("sort DESC").
		Order("id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.HotSearchItem, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.HotSearchItem{Name: x.Name, Sort: x.Sort})
	}
	return out, nil
}
