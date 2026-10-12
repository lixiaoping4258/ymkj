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

/* ------------------------------------------------- getNew（用户侧公告） */

// GetNewCacheKey 是 `app\api\logic\user\NoticeLogic::getNew()` 里的缓存键。
//
// ⚠️⚠️ **键名是 `purchase:getNew`，但它和采购/兑换毫无关系** ——
// 这是原项目的命名遗留（大概是从别处复制过来的）。照搬，不要"顺手改对"。
// 改成别的名字会导致与 PHP 的缓存不互通。
//
// ⚠️ 走 RedisLockService（裸 phpredis、无 TP 前缀）-> **与 PHP 共用同一把键**。
const GetNewCacheKey = "purchase:getNew"

// GetNewCacheTTL 对应 `RedisLockService::set($cacheKey, json_encode($res), 300)`。
const GetNewCacheTTL = 300 * time.Second

// NoticeNewUsecase 对应用户侧公告。
type NoticeNewUsecase struct {
	repo  NoticeNewRepo
	cache RawKeyCache
}

func NewNoticeNewUsecase(repo NoticeNewRepo, cache RawKeyCache) *NoticeNewUsecase {
	return &NoticeNewUsecase{repo: repo, cache: cache}
}

// NoticeNewRepo 取最新一条 cid=2 的文章。
type NoticeNewRepo interface {
	// FindNewArticle 对应：
	//   Article::where('cid', 2)->field('id,title,content,create_time')
	//       ->where('is_show', 1)->whereNull('delete_time')
	//       ->order('sort','desc')->order('create_time','desc')
	//       ->findOrEmpty();
	//
	// 返回 (行, 是否存在)。**不存在时调用方要输出 `[]` 而不是 null**。
	FindNewArticle(ctx context.Context) (*NoticeNewRow, bool, error)
}

// NoticeNewRow 对应 field('id,title,content,create_time')。
type NoticeNewRow struct {
	ID      int64
	Title   string
	Content *string
	// ⚠️ x_article.create_time 是 **int(11)** -> PHP 输出**数字**（不是日期串）
	CreateTime int64
}

// NoticeNewCid 对应方法体里硬编码的 `where('cid', 2)`。
const NoticeNewCid = 2

// GetNew 逐字对应 `app\api\logic\user\NoticeLogic::getNew()`。
//
// 返回 (行, 是否存在)；**不存在时** 控制器输出的是 `{"get_new": []}`。
//
// 原文：
//
//	$cacheKey = 'purchase:getNew';
//	$cache = RedisLockService::get($cacheKey);
//	if ($cache) { return json_decode($cache, true); }
//	$article = Article::where('cid',2)->field('id,title,content,create_time')
//	    ->where('is_show',1)->whereNull('delete_time')
//	    ->order('sort','desc')->order('create_time','desc')->findOrEmpty();
//	$res = $article->isEmpty() ? [] : $article->toArray();
//	RedisLockService::set($cacheKey, json_encode($res), 300);
//	return $res;
//
// ⚠️ 缓存里存的是**裸的 json**（`json_encode($res)`），命中时直接返回解析结果 ——
//
//	所以缓存值时可能是对象 `{...}`，也可能是空数组 `[]`。
func (uc *NoticeNewUsecase) GetNew(ctx context.Context) ([]byte, error) {
	// 缓存命中直接用（内容是 PHP json_encode 的结果）
	if v, err := uc.cache.Get(ctx, GetNewCacheKey); err == nil && v != nil {
		if s, ok := v.(string); ok && s != "" {
			return []byte(s), nil
		}
	}
	row, found, err := uc.repo.FindNewArticle(ctx)
	if err != nil {
		return nil, err
	}
	var body []byte
	if !found {
		// `$article->isEmpty() ? [] : ...` —— 不存在时是**空数组**
		body = []byte("[]")
	} else {
		body, err = MarshalNoHTMLEscape(map[string]any{
			"id":          row.ID,
			"title":       row.Title,
			"content":     derefStrAny(row.Content),
			"create_time": row.CreateTime,
		})
		if err != nil {
			return nil, err
		}
	}
	_ = uc.cache.Set(ctx, GetNewCacheKey, body, GetNewCacheTTL)
	return body, nil
}
