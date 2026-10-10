package biz

import (
	"context"
	"time"
)

// ArticleDetailUsecase 对应 ArticleLogic::detail + Article::getArticleDetailArr。
//
// ⚠️⚠️ **这个接口看着是只读的，实际会写库**：
//
//	$article->click_actual += 1;
//	$article->save();
//
// 也就是每请求一次，点击量就 +1。迁移时不能当成纯查询 ——
// 它需要写权限、有自己的并发语义（并发请求会互相覆盖），
// 而且**在 PHP 与 Go 并行期间两边都会加**（见下方说明）。
type ArticleDetailUsecase struct {
	repo ArticleDetailRepo
}

func NewArticleDetailUsecase(repo ArticleDetailRepo) *ArticleDetailUsecase {
	return &ArticleDetailUsecase{repo: repo}
}

// ArticleDetailRepo 文章详情。
type ArticleDetailRepo interface {
	// FindForDetail 对应：
	//   Article::where(['id'=>$id, 'is_show'=>YesNoEnum::YES])->findOrEmpty()
	// 带 ThinkPHP 的隐式软删除过滤（Article 有 $deleteTime）。
	FindForDetail(ctx context.Context, id int32) (*ArticleDetailRow, bool, error)

	// IncClickActual 对应 `$article->click_actual += 1; $article->save();`
	//
	// ⚠️ 原文是「读出来 +1 + 整行 save」，不是 `click_actual = click_actual + 1`。
	//    并发下会丢失更新（两个请求都读到 N，都写 N+1）。这是**原实现的行为**，
	//    按"不改动原项目逻辑"照搬为「读-改-写」而不是原子自增。
	//    已记入 README 的原系统缺陷。
	IncClickActual(ctx context.Context, id int32, newValue int64) error

	// IsCollectArticle 对应 ArticleCollect::isCollectArticle($userId, $articleId)。
	IsCollectArticle(ctx context.Context, userID uint32, articleID int32) (bool, error)
}

// ArticleDetailRow 对应 x_article 的**全部列**（toArray 输出整行）。
//
// ⚠️ 原文 `->hidden(['click_virtual','click_actual'])` 只藏了这两列，
//
//	其余全部暴露 —— 包括 content、delete_time、update_time。
type ArticleDetailRow struct {
	ID           int32
	Cid          int32
	Title        string
	Desc         *string // 可空
	Abstract     *string // 可空
	Image        *string // 可空
	Author       *string // 可空
	Content      *string // 可空
	ClickVirtual *int64  // 可空；**不对外**（hidden）
	ClickActual  *int64  // 可空；**不对外**（hidden）
	IsShow       int32
	Sort         *int32 // 可空
	CreateTime   *int64 // 可空
	UpdateTime   *int64 // 可空
	DeleteTime   *int64 // 可空
}

// ArticleDetailResult 是 detail() 的返回值：整行 + click + collect。
type ArticleDetailResult struct {
	Row ArticleDetailRow
	// Click 是计算列 = click_actual + click_virtual（append(['click'])）
	Click int64
	// Collect 对应 `$article['collect'] = ArticleCollect::isCollectArticle($userId, $articleId);`
	Collect bool
}

// ArticleDetail 逐字对应 ArticleLogic::detail($articleId, $userId)：
//
//	$article = Article::getArticleDetailArr($articleId);
//	$article['collect'] = ArticleCollect::isCollectArticle($userId, $articleId);
//	return $article;
//
// 而 getArticleDetailArr 是：
//
//	$article = Article::where(['id'=>$id,'is_show'=>YES])->findOrEmpty();
//	if ($article->isEmpty()) { return []; }        // ← 不存在时返回**空数组**
//	$article->click_actual += 1; $article->save(); // ← 写库
//	return $article->append(['click'])->hidden(['click_virtual','click_actual'])->toArray();
//
// ⚠️ **找不到时返回的空数组里没有 collect 键** —— 因为 detail() 是在
//
//	getArticleDetailArr 的返回值上加 `$article['collect'] = ...`，
//	而空数组 `[]` 加一个键会变成 `['collect'=>false]`。
//	所以"文章不存在"时对外是 `{"collect": false}`，不是 `{}` 也不是 `[]`。
//	（PHP 里 `$a = []; $a['collect'] = false; json_encode($a)` = `{"collect":false}`）
func (uc *ArticleDetailUsecase) ArticleDetail(
	ctx context.Context, articleID int32, userID uint32,
) (*ArticleDetailResult, bool, error) {
	row, found, err := uc.repo.FindForDetail(ctx, articleID)
	if err != nil {
		return nil, false, err
	}
	if !found {
		// getArticleDetailArr 返回 []；调用方再加 collect 键。
		return nil, false, nil
	}

	// 点击量 +1（读-改-写，照搬原文的非原子写法）
	actualAfter := derefI64(row.ClickActual) + 1
	if err := uc.repo.IncClickActual(ctx, articleID, actualAfter); err != nil {
		return nil, false, err
	}

	// 返回的 click 用的是**自增之后**的值（原文先 save 再 append('click')）
	clickAfter := actualAfter + derefI64(row.ClickVirtual)

	collect, err := uc.repo.IsCollectArticle(ctx, userID, articleID)
	if err != nil {
		return nil, false, err
	}
	return &ArticleDetailResult{Row: *row, Click: clickAfter, Collect: collect}, true, nil
}

// ArticleDetailNotFoundResult 对应"文章不存在"时的对外形态：
// getArticleDetailArr 返回 []，detail() 加上 collect 键 -> {"collect": false}。
func ArticleDetailNotFoundResult() *ArticleDetailResult {
	return &ArticleDetailResult{Collect: false}
}

func derefI64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// 保证 time 被使用（可空的时间列后续若需要格式化会用到）。
var _ = time.Now
