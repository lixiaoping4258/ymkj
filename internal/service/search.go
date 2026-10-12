package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// SearchService 对应 app/api/controller/SearchController.php。
type SearchService struct {
	v1.UnimplementedSearchServiceServer
	hot *biz.HotSearchUsecase
}

func NewSearchService(hot *biz.HotSearchUsecase) *SearchService {
	return &SearchService{hot: hot}
}

// GetHotLists 对应 SearchController::hotLists（GET /v1/search/hotLists）。
//
// 原实现：
//
//	public function hotLists() {
//	    return $this->data(SearchLogic::hotLists());
//	}
//
// ⚠️ 免登录（SearchController::$notNeedLogin = ['hotLists']）。
func (s *SearchService) GetHotLists(
	ctx context.Context, _ *v1.GetHotListsRequest,
) (*v1.GetHotListsReply, error) {
	res, err := s.hot.GetHotLists(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.HotSearchItem, 0, len(res.Data))
	for _, x := range res.Data {
		items = append(items, &v1.HotSearchItem{Name: x.Name, Sort: x.Sort})
	}
	return &v1.GetHotListsReply{Status: res.Status, Data: items}, nil
}
