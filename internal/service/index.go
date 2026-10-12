package service

import (
	"context"

	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// IndexService 对应 app/api/controller/IndexController.php 与 BannerController.php。
type IndexService struct {
	v1.UnimplementedIndexServiceServer
	index *biz.IndexUsecase
	cfg   *biz.IndexConfigUsecase
}

func NewIndexService(index *biz.IndexUsecase, cfg *biz.IndexConfigUsecase) *IndexService {
	return &IndexService{index: index, cfg: cfg}
}

// GetIndexConfig 对应 IndexController::config -> IndexLogic::getConfigData()。
//
// ⚠️ domain 与 webPage.url **都取自当前请求**（原文 request()->domain()），
//
//	不能用配置里的固定域名。
func (s *IndexService) GetIndexConfig(
	ctx context.Context, _ *v1.GetIndexConfigRequest,
) (*v1.GetIndexConfigReply, error) {
	domain := domainFrom(ctx)
	res, err := s.cfg.GetConfig(ctx, domain)
	if err != nil {
		return nil, err
	}

	reply := &v1.GetIndexConfigReply{
		Domain:  res.Domain,
		Version: res.Version,
		Style: &v1.TabbarStyle{
			DefaultColor:  res.Style.DefaultColor,
			SelectedColor: res.Style.SelectedColor,
		},
		Login: &v1.LoginConfig{
			LoginWay:       res.Login.LoginWay,
			CoerceMobile:   res.Login.CoerceMobile,
			LoginAgreement: res.Login.LoginAgreement,
			ThirdAuth:      res.Login.ThirdAuth,
			WechatAuth:     res.Login.WechatAuth,
			QqAuth:         res.Login.QqAuth,
		},
		Website: &v1.WebsiteConfig{
			H5Favicon: res.Website.H5Favicon,
			ShopName:  res.Website.ShopName,
			ShopLogo:  res.Website.ShopLogo,
		},
		WebPage: &v1.WebPageConfig{
			Status:     res.WebPage.Status,
			PageStatus: res.WebPage.PageStatus,
			PageUrl:    res.WebPage.PageURL,
			Url:        res.WebPage.URL,
		},
	}

	// tabbar：selected/unselected 要经 getFileUrl 拼域名（原文在模型方法里做）
	tabs := make([]*v1.Tabbar, 0, len(res.Tabbar))
	for _, t := range res.Tabbar {
		item := &v1.Tabbar{
			Id: t.ID, Name: t.Name,
			Selected:   joinFileURLByDomain(domain, t.Selected),
			Unselected: joinFileURLByDomain(domain, t.Unselected),
			IsShow:     t.IsShow,
			CreateTime: t.CreateTime, UpdateTime: t.UpdateTime,
		}
		if t.Link != nil {
			if st, err := structpb.NewStruct(t.Link); err == nil {
				item.Link = st
			}
		}
		tabs = append(tabs, item)
	}
	reply.Tabbar = tabs

	// copyright：本项目无该配置 -> 空数组
	reply.Copyright = []*structpb.Struct{}
	return reply, nil
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

// joinFileURLByDomain 是 biz 层 joinFileURL 的 service 侧包装。
//
// ⚠️ 语义与 FileService::getFileUrl 一致：**已是完整 URL 的原样返回**，
//
//	否则拼上域名（去尾斜杠 + 去首斜杠）。
func joinFileURLByDomain(domain, uri string) string {
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	if domain == "" {
		return uri
	}
	d := strings.TrimRight(domain, "/")
	u := strings.TrimLeft(uri, "/")
	return d + "/" + u
}
