package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// ArticleService 对应 app/api/controller/v1/article/ArticleController.php。
type ArticleService struct {
	v1.UnimplementedArticleServiceServer
	articles *biz.ArticleUsecase
}

func NewArticleService(articles *biz.ArticleUsecase) *ArticleService {
	return &ArticleService{articles: articles}
}

// GetArticleAbout 对应 ArticleController::about。
//
// 原实现：
//
//	public function about(): Json {
//	    return $this->data(ArticleLogic::about());
//	}
func (s *ArticleService) GetArticleAbout(
	ctx context.Context, _ *v1.GetArticleAboutRequest,
) (*v1.ArticleListReply, error) {
	res, err := s.articles.About(ctx)
	if err != nil {
		return nil, err
	}
	return toArticleListReply(res), nil
}

// GetArticleLicenses 对应 ArticleController::license（路由名是 licenses，处理器名是 license）。
func (s *ArticleService) GetArticleLicenses(
	ctx context.Context, _ *v1.GetArticleLicensesRequest,
) (*v1.ArticleListReply, error) {
	res, err := s.articles.License(ctx)
	if err != nil {
		return nil, err
	}
	return toArticleListReply(res), nil
}

func toArticleListReply(res *biz.ArticleListResult) *v1.ArticleListReply {
	items := make([]*v1.ArticleItem, 0, len(res.Lists))
	for _, x := range res.Lists {
		items = append(items, &v1.ArticleItem{Id: x.ID, Title: x.Title})
	}
	return &v1.ArticleListReply{Title: res.Title, Lists: items}
}
