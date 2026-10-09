package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// articleRepo 对应 app/api/logic/ArticleLogic.php + app/common/model/article/Article.php。
type articleRepo struct {
	db     *gorm.DB
	prefix string
}

func NewArticleRepo(db *gorm.DB, c *conf.Data) biz.ArticleRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &articleRepo{db: db, prefix: prefix}
}

// ListBrief 逐字对应：
//
//	Article::where(['cid' => $cid, 'is_show' => 1])
//	    ->whereRaw('delete_time is null')
//	    ->order('sort', 'desc')
//	    ->field('id,title')
//	    ->select()
//
// ⚠️ 三个必须照搬的点：
//  1. `is_show = 1` 过滤（下架的文章不返回）
//  2. `delete_time is null` —— 原文是**显式 whereRaw**，不是模型软删除
//     （x_article.delete_time 是 int(11)，可空）
//  3. `order('sort','desc')` —— 排序影响返回顺序，漏了前端看到的就是另一个次序
func (r *articleRepo) ListBrief(ctx context.Context, cid int32) ([]biz.ArticleBrief, error) {
	var rows []struct {
		ID    int32  `gorm:"column:id"`
		Title string `gorm:"column:title"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"article").
		Select("id", "title").
		Where("cid = ? AND is_show = ?", cid, 1).
		Where("delete_time IS NULL").
		Order("sort DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.ArticleBrief, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.ArticleBrief{ID: x.ID, Title: x.Title})
	}
	return out, nil
}
