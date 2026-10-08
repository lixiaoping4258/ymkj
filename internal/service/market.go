package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// MarketService 对应原项目 app/api/controller/v1/market/PurchaseController.php。
type MarketService struct {
	v1.UnimplementedMarketServiceServer
	market *biz.MarketUsecase
}

func NewMarketService(market *biz.MarketUsecase) *MarketService {
	return &MarketService{market: market}
}

// GetPayWay 对应 PurchaseController::payWay。
//
// 原实现是硬编码数组，不查库，也不需要任何参数。
func (s *MarketService) GetPayWay(ctx context.Context, _ *v1.GetPayWayRequest) (*v1.GetPayWayReply, error) {
	ways := s.market.PayWay(ctx)
	items := make([]*v1.PayWay, 0, len(ways))
	for _, w := range ways {
		items = append(items, &v1.PayWay{PayWay: w.PayWay, PayName: w.PayName})
	}
	return &v1.GetPayWayReply{Items: items}, nil
}

// CheckExchange 对应 PurchaseController::checkExchange。
//
// 原实现：
//
//	public function checkExchange(): Json {
//	    $params = request()->get();
//	    $data = GoodsLogic::checkExchange($this->userId, $params);
//	    if ($data === false) { return $this->fail(GoodsLogic::getError()); }
//	    return $this->data($data);      // $data 是 []
//	}
//
// $params 在原实现里根本没被用到（GoodsLogic::checkExchange 收了但没用），
// 所以这里也不解析查询参数。
func (s *MarketService) CheckExchange(ctx context.Context, _ *v1.CheckExchangeRequest) (*v1.CheckExchangeReply, error) {
	userID := biz.UserIDFromContext(ctx)
	if err := s.market.CheckExchange(ctx, userID); err != nil {
		return nil, bizFail(err)
	}
	return &v1.CheckExchangeReply{}, nil
}
