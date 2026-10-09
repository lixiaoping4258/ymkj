package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// IndexService 对应 app/api/controller/IndexController.php 与 BannerController.php。
type IndexService struct {
	v1.UnimplementedIndexServiceServer
	index *biz.IndexUsecase
}

func NewIndexService(index *biz.IndexUsecase) *IndexService {
	return &IndexService{index: index}
}

// GetIndex 对应 IndexController::index -> IndexLogic::getIndexData()。
func (s *IndexService) GetIndex(ctx context.Context, _ *v1.GetIndexRequest) (*v1.GetIndexReply, error) {
	res, err := s.index.GetIndex(ctx)
	if err != nil {
		return nil, err
	}
	reply := &v1.GetIndexReply{}
	if res.Page != nil {
		reply.Page = &v1.DecoratePage{
			Type: res.Page.Type, Name: res.Page.Name,
			Data: res.Page.Data, Meta: res.Page.Meta,
		}
	}
	arts := make([]*v1.HomeArticle, 0, len(res.Article))
	for _, a := range res.Article {
		arts = append(arts, &v1.HomeArticle{
			Id: a.ID, Title: a.Title, Desc: a.Desc, Abstract: a.Abstract,
			Image: a.Image, Author: a.Author, CreateTime: a.CreateTime, Click: a.Click,
		})
	}
	reply.Article = arts
	return reply, nil
}

// GetDecorate 对应 IndexController::decorate -> IndexLogic::getDecorate($id)。
func (s *IndexService) GetDecorate(
	ctx context.Context, req *v1.GetDecorateRequest,
) (*v1.GetDecorateReply, error) {
	page, err := s.index.GetDecorate(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.GetDecorateReply{Page: &v1.DecoratePage{
		Type: page.Type, Name: page.Name, Data: page.Data, Meta: page.Meta,
	}}, nil
}

// GetBannerList 对应 BannerController::getList -> GetListLogic::getList()。
func (s *IndexService) GetBannerList(
	ctx context.Context, _ *v1.GetBannerListRequest,
) (*v1.GetBannerListReply, error) {
	rows, err := s.index.GetBannerList(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.BannerItem, 0, len(rows))
	for _, b := range rows {
		items = append(items, &v1.BannerItem{
			Id: b.ID, Title: b.Title, Image: b.Image, JumpUrl: b.JumpURL,
			Sort: b.Sort, CreateTime: b.CreateTime,
		})
	}
	return &v1.GetBannerListReply{Items: items}, nil
}
