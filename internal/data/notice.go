package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// noticeRepo 对应 app/common/model/notice/Notices.php。
type noticeRepo struct {
	db     *gorm.DB
	prefix string
}

func NewNoticeRepo(db *gorm.DB, c *conf.Data) biz.NoticeRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &noticeRepo{db: db, prefix: prefix}
}

// ListNotices 逐字对应：
//
//	Notices::field('id,title,content,create_time')
//	    ->where('state','=',1)
//	    ->orderRaw('create_time DESC')
//	    ->limit($this->limitOffset, $this->limitLength)
//	    ->select()->toArray();
//
// ⚠️ `Notices extends Model`（**不是** BaseModel），所以**没有软删除** ——
//
//	不要加 delete_time 过滤。（x_notices 表里也确实没有该列。）
//
// ⚠️ create_time 是 **datetime**，PHP 输出 'Y-m-d H:i:s' 字符串。
func (r *noticeRepo) ListNotices(ctx context.Context, offset, limit int) ([]biz.NoticeRow, error) {
	var rows []struct {
		ID         int64     `gorm:"column:id"`
		Title      string    `gorm:"column:title"`
		Content    *string   `gorm:"column:content"`
		CreateTime time.Time `gorm:"column:create_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"notices").
		Select("id", "title", "content", "create_time").
		Where("state = ?", biz.NoticeStateEnabled).
		Order("create_time DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.NoticeRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.NoticeRow{
			ID: x.ID, Title: x.Title, Content: x.Content,
			CreateTime: x.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

// CountNotices 逐字对应 `Notices::where('state','=',1)->count()`。
func (r *noticeRepo) CountNotices(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table(r.prefix+"notices").
		Where("state = ?", biz.NoticeStateEnabled).
		Count(&n).Error
	return n, err
}

// noticeNewRepo 对应用户侧公告（cid=2 的最新一条）。
type noticeNewRepo struct {
	db     *gorm.DB
	prefix string
}

func NewNoticeNewRepo(db *gorm.DB, c *conf.Data) biz.NoticeNewRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &noticeNewRepo{db: db, prefix: prefix}
}

// FindNewArticle 逐字对应：
//
//	Article::where('cid', 2)->field('id,title,content,create_time')
//	    ->where('is_show', 1)->whereNull('delete_time')
//	    ->order('sort','desc')->order('create_time','desc')
//	    ->findOrEmpty();
//
// ⚠️ Article 模型有 $deleteTime，但原文**又显式写了 whereNull('delete_time')**
//
//	（双保险）。Go 侧加一次即可。
//
// ⚠️ 两个 order 的先后不能调换：先按 sort 降序，sort 相同再按 create_time 降序。
func (r *noticeNewRepo) FindNewArticle(ctx context.Context) (*biz.NoticeNewRow, bool, error) {
	// ⚠️ x_article.create_time 是 **int(11)**，不是 datetime！
	//    （对照：x_notices.create_time 才是 datetime —— 两张表不一样，
	//     我第一版按 datetime 写导致 "unsupported Scan ... into type *time.Time"）
	var row struct {
		ID         int64   `gorm:"column:id"`
		Title      string  `gorm:"column:title"`
		Content    *string `gorm:"column:content"`
		CreateTime int64   `gorm:"column:create_time"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"article").
		Select("id", "title", "content", "create_time").
		Where("cid = ?", biz.NoticeNewCid).
		Where("is_show = ?", 1).
		Where("delete_time IS NULL").
		Order("sort DESC").
		Order("create_time DESC").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.NoticeNewRow{
		ID: row.ID, Title: row.Title, Content: row.Content,
		CreateTime: row.CreateTime, // int 时间戳，PHP 输出数字
	}, true, nil
}
