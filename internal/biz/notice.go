package biz

import (
	"context"
	"time"
)

// NoticeUsecase 对应 app/api/lists/notices/PlatformNoticeLists.php。
//
// 原实现（逐行）：
//
//	class PlatformNoticeLists extends BaseApiDataLists {
//	    public function lists(): array {
//	        $orderRaw = 'create_time DESC';
//	        $filed = 'id,title,content,create_time';
//	        return Notices::field($filed)
//	            ->where('state', '=', 1)
//	            ->orderRaw($orderRaw)
//	            ->limit($this->limitOffset, $this->limitLength)
//	            ->select()->toArray();
//	    }
//	    public function count(): int {
//	        return Notices::where('state', '=', 1)->count();
//	    }
//	}
//
// 控制器：NoticeController::index -> $this->dataLists(new PlatformNoticeLists())
type NoticeUsecase struct {
	repo NoticeRepo
}

func NewNoticeUsecase(repo NoticeRepo) *NoticeUsecase {
	return &NoticeUsecase{repo: repo}
}

// NoticeRepo 公告表。
type NoticeRepo interface {
	// ListNotices 对应 lists() 的查询。
	ListNotices(ctx context.Context, offset, limit int) ([]NoticeRow, error)
	// CountNotices 对应 count()：`Notices::where('state','=',1)->count()`
	CountNotices(ctx context.Context) (int64, error)
}

// NoticeRow 对应 field('id,title,content,create_time')。
//
// ⚠️ x_notices.create_time 是 **datetime**（不是别的表那种 int），
//
//	所以 PHP 输出 'Y-m-d H:i:s' 字符串。
//
// ⚠️ content 是 text 可空。
type NoticeRow struct {
	ID         int64
	Title      string
	Content    *string
	CreateTime string
}

// NoticeStateEnabled 对应 `where('state','=',1)`。
const NoticeStateEnabled = 1

// List 返回公告列表 + 总数（对应 dataLists 的 lists()/count() 两个方法）。
func (uc *NoticeUsecase) List(ctx context.Context, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountNotices(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := uc.repo.ListNotices(ctx, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]any{
			"id":    r.ID,
			"title": r.Title,
			// content 可空；PHP 的 toArray() 对 NULL 列输出 null
			"content":     derefStrAny(r.Content),
			"create_time": r.CreateTime,
		})
	}
	return &ListData{
		Lists:    items,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// 保证 time 被使用（保留给将来可能的按时间过滤）。
var _ = time.Now
