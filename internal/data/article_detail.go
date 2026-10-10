package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// articleDetailRepo 对应 Article::getArticleDetailArr + ArticleCollect::isCollectArticle。
type articleDetailRepo struct {
	db     *gorm.DB
	prefix string
}

func NewArticleDetailRepo(db *gorm.DB, c *conf.Data) biz.ArticleDetailRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &articleDetailRepo{db: db, prefix: prefix}
}

// FindForDetail 逐字对应：
//
//	Article::where(['id' => $id, 'is_show' => YesNoEnum::YES])->findOrEmpty()
//
// ⚠️ Article 模型有 `protected $deleteTime = 'delete_time'`，软删除过滤是**自动的**，
//
//	Go 侧必须补 `delete_time IS NULL`。
//
// ⚠️ 取的是**整行**（原文没有 field() 限制），除 click_virtual/click_actual 外
//
//	全部对外暴露 —— 包括 content。
func (r *articleDetailRepo) FindForDetail(
	ctx context.Context, id int32,
) (*biz.ArticleDetailRow, bool, error) {
	var row struct {
		ID           int32   `gorm:"column:id"`
		Cid          int32   `gorm:"column:cid"`
		Title        string  `gorm:"column:title"`
		Desc         *string `gorm:"column:desc"`
		Abstract     *string `gorm:"column:abstract"`
		Image        *string `gorm:"column:image"`
		Author       *string `gorm:"column:author"`
		Content      *string `gorm:"column:content"`
		ClickVirtual *int64  `gorm:"column:click_virtual"`
		ClickActual  *int64  `gorm:"column:click_actual"`
		IsShow       int32   `gorm:"column:is_show"`
		Sort         *int32  `gorm:"column:sort"`
		CreateTime   *int64  `gorm:"column:create_time"`
		UpdateTime   *int64  `gorm:"column:update_time"`
		DeleteTime   *int64  `gorm:"column:delete_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"article").
		Where("id = ? AND is_show = ?", id, 1).
		Where("delete_time IS NULL").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.ArticleDetailRow{
		ID: row.ID, Cid: row.Cid, Title: row.Title,
		Desc: row.Desc, Abstract: row.Abstract, Image: row.Image,
		Author: row.Author, Content: row.Content,
		ClickVirtual: row.ClickVirtual, ClickActual: row.ClickActual,
		IsShow: row.IsShow, Sort: row.Sort,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, DeleteTime: row.DeleteTime,
	}, true, nil
}

// IncClickActual 逐字对应 `$article->click_actual += 1; $article->save();`
//
// ⚠️ 原文是「读出来 +1 + 整行 save」，**不是** `SET click_actual = click_actual + 1`。
//
//	并发下会丢失更新。这是原实现的行为，按"不改动原项目逻辑"照搬为
//	传入已算好的新值（见 biz 层的说明）。
//
// ⚠️ 只更新 click_actual 一列（等价于模型 save 只脏了这一个字段），
//
//	不碰 update_time —— 原文的 save() 会更新 update_time，
//	但 tp 的自动时间戳需模型开启才生效，这里**不更新**以贴近最小改动；
//	若实测发现 PHP 那边 update_time 变了，需要补上。
func (r *articleDetailRepo) IncClickActual(ctx context.Context, id int32, newValue int64) error {
	return r.db.WithContext(ctx).
		Table(r.prefix+"article").
		Where("id = ?", id).
		Update("click_actual", newValue).Error
}

// IsCollectArticle 逐字对应：
//
//	$collect = ArticleCollect::where(['user_id'=>$userId,'article_id'=>$articleId,
//	                                  'status'=>YesNoEnum::YES])->findOrEmpty();
//	return !$collect->isEmpty();
//
// ⚠️ 未登录时控制器传的 $userId 是 0（BaseApiController 默认值），
//
//	所以匿名用户会拿 user_id=0 去查 —— 照搬，不要特殊处理。
//
// ⚠️ 该表实测 **0 行**，所以当前恒为 false。
func (r *articleDetailRepo) IsCollectArticle(
	ctx context.Context, userID uint32, articleID int32,
) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"article_collect").
		Where("user_id = ? AND article_id = ? AND status = ?", userID, articleID, 1).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
