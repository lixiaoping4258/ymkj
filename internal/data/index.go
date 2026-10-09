package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// indexRepo 对应 app/api/logic/IndexLogic.php + GetListLogic.php。
type indexRepo struct {
	db     *gorm.DB
	prefix string
}

func NewIndexRepo(db *gorm.DB, c *conf.Data) biz.IndexRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &indexRepo{db: db, prefix: prefix}
}

// FindDecoratePage 逐字对应：
//
//	DecoratePage::field(['type','name','data','meta'])->findOrEmpty($id)->toArray()
//
// ⚠️ x_decorate_page **没有 delete_time 列**（已对建表结构核实），所以没有软删除过滤。
// ⚠️ data / meta 是 text 列，模型没有 json 转换，PHP 输出的是**原始字符串**。
//
// 找不到时原文返回空模型的 toArray()，也就是 type=0/name=”/data=”/meta=”
// 四个字段都在的空对象 —— 这里返回一个零值行而不是 nil，保持同样的形状。
func (r *indexRepo) FindDecoratePage(ctx context.Context, id int32) (*biz.DecoratePageRow, error) {
	var row struct {
		Type int32  `gorm:"column:type"`
		Name string `gorm:"column:name"`
		Data string `gorm:"column:data"`
		Meta string `gorm:"column:meta"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"decorate_page").
		Select("type", "name", "data", "meta").
		Where("id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &biz.DecoratePageRow{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.DecoratePageRow{Type: row.Type, Name: row.Name, Data: row.Data, Meta: row.Meta}, nil
}

// ListBanners 逐字对应：
//
//	Db::name('banner')->where('is_show', 1)->whereNull('delete_time')
//	    ->order('sort','desc')->order('create_time','desc')
//	    ->field(['id','title','image','jump_url','sort','create_time'])
//	    ->select()->toArray();
//
// ⚠️ 两个 order 的**先后顺序**决定同 sort 时的次序，不能只留一个。
// ⚠️ `whereNull('delete_time')` 是显式条件（Db::name 不走模型软删除）。
// ⚠️ create_time 是 datetime，PHP 输出 'Y-m-d H:i:s' 字符串。
func (r *indexRepo) ListBanners(ctx context.Context) ([]biz.BannerRow, error) {
	var rows []struct {
		ID         int64     `gorm:"column:id"`
		Title      string    `gorm:"column:title"`
		Image      string    `gorm:"column:image"`
		JumpURL    string    `gorm:"column:jump_url"`
		Sort       int32     `gorm:"column:sort"`
		CreateTime time.Time `gorm:"column:create_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"banner").
		Select("id", "title", "image", "jump_url", "sort", "create_time").
		Where("is_show = ?", 1).
		Where("delete_time IS NULL").
		Order("sort DESC").
		Order("create_time DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.BannerRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.BannerRow{
			ID: x.ID, Title: x.Title, Image: x.Image, JumpURL: x.JumpURL,
			Sort: x.Sort, CreateTime: x.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

// ListArticles 逐字对应 getIndexData 里的文章查询：
//
//	Article::field(['id','title','desc','abstract','image','author',
//	                'click_actual','click_virtual','create_time'])
//	    ->where(['is_show' => 1])
//	    ->order(['id' => 'desc'])
//	    ->limit(20)
//	    ->append(['click'])                          // click = click_actual + click_virtual
//	    ->hidden(['click_actual','click_virtual'])   // 藏掉两列，只留 click
//	    ->select()->toArray();
//
// ⚠️ Article 模型有 `protected $deleteTime = 'delete_time'` —— **软删除过滤是自动的**，
//
//	必须在这里补 `delete_time IS NULL`，否则会把已删文章查出来。
//
// ⚠️ 对外只有 8 个键：id/title/desc/abstract/image/author/create_time/**click**。
func (r *indexRepo) ListArticles(ctx context.Context, limit int) ([]biz.ArticleRow, error) {
	var rows []struct {
		ID           int64  `gorm:"column:id"`
		Title        string `gorm:"column:title"`
		Desc         string `gorm:"column:desc"`
		Abstract     string `gorm:"column:abstract"`
		Image        string `gorm:"column:image"`
		Author       string `gorm:"column:author"`
		CreateTime   int64  `gorm:"column:create_time"`
		ClickActual  int64  `gorm:"column:click_actual"`
		ClickVirtual int64  `gorm:"column:click_virtual"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"article").
		Select("id", "title", "desc", "abstract", "image", "author",
			"click_actual", "click_virtual", "create_time").
		Where("is_show = ?", 1).
		Where("delete_time IS NULL").
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.ArticleRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.ArticleRow{
			ID: x.ID, Title: x.Title, Desc: x.Desc, Abstract: x.Abstract,
			Image: x.Image, Author: x.Author, CreateTime: x.CreateTime,
			Click: x.ClickActual + x.ClickVirtual,
		})
	}
	return out, nil
}
