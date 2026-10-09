package biz

import "context"

// ArticleUsecase 对应 app/api/logic/ArticleLogic.php。
//
// 本批只实现 about / license 两条（原文里 article.php 真正启用的 3 条中的 2 条，
// detail 需要 getArticleDetailArr + isCollectArticle，单独做）。
type ArticleUsecase struct {
	repo ArticleRepo
}

func NewArticleUsecase(repo ArticleRepo) *ArticleUsecase {
	return &ArticleUsecase{repo: repo}
}

// ArticleRepo 文章查询端口。
type ArticleRepo interface {
	// ListBrief 对应：
	//   Article::where(['cid'=>$cid,'is_show'=>1])
	//       ->whereRaw('delete_time is null')->order('sort','desc')
	//       ->field('id,title')->select()
	//
	// ⚠️ 原文用的是 `whereRaw('delete_time is null')` 而**不是**模型软删除
	//（x_article.delete_time 是 int(11)，不是统一的 datetime）。
	// 照搬这个条件。
	ListBrief(ctx context.Context, cid int32) ([]ArticleBrief, error)
}

// ArticleBrief 是 field('id,title') 的两列。
type ArticleBrief struct {
	ID    int32
	Title string
}

// about 与 license 的 title 常量，逐字对应原文。
const (
	ArticleTitleAbout   = "关于我们"
	ArticleTitleLicense = "资质证照"
	// cid：1 = 关于我们类，3 = 资质证照类（实测 x_article 的 cid 分布为 1/2/3）
	ArticleCidAbout   = 1
	ArticleCidLicense = 3
)

// ArticleListResult 对应 `['title'=>..., 'lists'=>...]`。
type ArticleListResult struct {
	Title string
	Lists []ArticleBrief
}

// About 逐字对应 ArticleLogic::about()。
func (uc *ArticleUsecase) About(ctx context.Context) (*ArticleListResult, error) {
	lists, err := uc.repo.ListBrief(ctx, ArticleCidAbout)
	if err != nil {
		return nil, err
	}
	return &ArticleListResult{Title: ArticleTitleAbout, Lists: lists}, nil
}

// License 逐字对应 ArticleLogic::license()。
func (uc *ArticleUsecase) License(ctx context.Context) (*ArticleListResult, error) {
	lists, err := uc.repo.ListBrief(ctx, ArticleCidLicense)
	if err != nil {
		return nil, err
	}
	return &ArticleListResult{Title: ArticleTitleLicense, Lists: lists}, nil
}
