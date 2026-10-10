package service

import (
	"context"

	"google.golang.org/protobuf/types/known/wrapperspb"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// ArticleService 对应 app/api/controller/v1/article/ArticleController.php。
type ArticleService struct {
	v1.UnimplementedArticleServiceServer
	articles *biz.ArticleUsecase
	detail   *biz.ArticleDetailUsecase
}

func NewArticleService(
	articles *biz.ArticleUsecase, detail *biz.ArticleDetailUsecase,
) *ArticleService {
	return &ArticleService{articles: articles, detail: detail}
}

// GetArticleDetail 对应 ArticleController::detail（GET /v1/article/detail?id=N）。
//
// ⚠️ 免登录接口，但**会写库**（click_actual += 1）。
// ⚠️ 未登录时 userId = 0（对应 BaseApiController 的默认值）。
func (s *ArticleService) GetArticleDetail(
	ctx context.Context, req *v1.GetArticleDetailRequest,
) (*v1.GetArticleDetailReply, error) {
	userID := biz.UserIDFromContext(ctx) // 未登录时是 0

	res, found, err := s.detail.ArticleDetail(ctx, req.GetId(), uint32(userID))
	if err != nil {
		return nil, err
	}
	if !found {
		// getArticleDetailArr 返回 []，detail() 再加 collect 键
		// -> 对外是 {"collect": false}，只有这一个键。
		return &v1.GetArticleDetailReply{Collect: false}, nil
	}
	row := res.Row
	reply := &v1.GetArticleDetailReply{
		Id: row.ID, Cid: row.Cid, Title: row.Title,
		IsShow: row.IsShow,
		Click:  res.Click, Collect: res.Collect,
	}
	if row.Desc != nil {
		reply.Desc = wrapperspb.String(*row.Desc)
	}
	if row.Abstract != nil {
		reply.Abstract = wrapperspb.String(*row.Abstract)
	}
	if row.Image != nil {
		reply.Image = wrapperspb.String(*row.Image)
	}
	if row.Author != nil {
		reply.Author = wrapperspb.String(*row.Author)
	}
	if row.Content != nil {
		reply.Content = wrapperspb.String(*row.Content)
	}
	if row.Sort != nil {
		reply.Sort = wrapperspb.Int32(*row.Sort)
	}
	if row.CreateTime != nil {
		reply.CreateTime = wrapperspb.Int64(*row.CreateTime)
	}
	if row.UpdateTime != nil {
		reply.UpdateTime = wrapperspb.Int64(*row.UpdateTime)
	}
	if row.DeleteTime != nil {
		reply.DeleteTime = wrapperspb.Int64(*row.DeleteTime)
	}
	return reply, nil
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
