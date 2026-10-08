package service

import (
	"context"
	"net/url"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// MarketService 对应原项目 app/api/controller/v1/market/PurchaseController.php。
type MarketService struct {
	v1.UnimplementedMarketServiceServer
	market   *biz.MarketUsecase
	purchase *biz.PurchaseFaceUsecase
	sale     *biz.SaleFaceUsecase
}

func NewMarketService(
	market *biz.MarketUsecase,
	purchase *biz.PurchaseFaceUsecase,
	sale *biz.SaleFaceUsecase,
) *MarketService {
	return &MarketService{market: market, purchase: purchase, sale: sale}
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

// PurchaseIndex 对应 PurchaseController::index。
//
// ⚠️ 这里直接从 HTTP 上下文取**原始 query 参数**，而不是把它们逐个声明进 proto。
//
// 原因：这个接口的参数是分页/筛选/排序混合的十几个键（page_no / page_size /
// page_type / keyword / price_start / price_end / time_start / time_end / sort），
// 原实现是直接读 $this->request->get()，键名即契约。
// 逐个搬进 proto 既啰嗦，又会在新增筛选条件时反复改 proto。
//
// 代价：service 层耦合了 HTTP 传输层。对列表类接口这个取舍是划算的，
// 因为它们的契约本来就是"查询字符串"。写接口不这么干。
func (s *MarketService) PurchaseIndex(ctx context.Context, _ *v1.PurchaseIndexRequest) (*v1.RawData, error) {
	q := url.Values{}
	// 传进 handler 的 ctx 就是 Kratos 内部那个实现了 http.Context 的 wrapper
	// （见 transport/http/router.go: ctx := &wrapper{...}; h(ctx)）。
	// 断言失败也不影响可用性：退化成"没传参数"，即默认分页第一页。
	if h, ok := ctx.(khttp.Context); ok && h.Request() != nil {
		q = h.Request().URL.Query()
	}

	page := biz.ParsePageParams(q)
	query := biz.PurchaseFaceQuery{
		Page:       page,
		Keyword:    q.Get("keyword"),
		PriceStart: page.Float("price_start"),
		PriceEnd:   page.Float("price_end"),
		TimeStart:  q.Get("time_start"),
		TimeEnd:    q.Get("time_end"),
		Sort:       q.Get("sort"),
	}

	raw, err := s.purchase.Index(ctx, biz.UserIDFromContext(ctx), query)
	if err != nil {
		return nil, bizFail(err)
	}
	return &v1.RawData{Json: raw}, nil
}

// GetSaleCategories 对应 SaleController::categories。
// 硬编码一项，不查库；是 SaleController 里唯一免登录的方法。
func (s *MarketService) GetSaleCategories(
	_ context.Context, _ *v1.GetSaleCategoriesRequest,
) (*v1.GetSaleCategoriesReply, error) {
	cats := biz.SaleCategories()
	items := make([]*v1.SaleCategory, 0, len(cats))
	for _, c := range cats {
		items = append(items, &v1.SaleCategory{Title: c.Title, Key: c.Key})
	}
	return &v1.GetSaleCategoriesReply{Items: items}, nil
}

// SaleIndex 对应 SaleController::index -> SaleFaceLists。
//
// 与 PurchaseIndex 不同：没有交易时段判断、没有缓存、没有生产测试用户分支
// （原实现就是一句 `return $this->dataLists(new SaleFaceLists())`）。
// 参数获取方式与 PurchaseIndex 一致，见那里的说明。
func (s *MarketService) SaleIndex(ctx context.Context, _ *v1.SaleIndexRequest) (*v1.RawData, error) {
	q := url.Values{}
	if h, ok := ctx.(khttp.Context); ok && h.Request() != nil {
		q = h.Request().URL.Query()
	}
	page := biz.ParsePageParams(q)
	query := biz.SaleFaceQuery{
		Page:       page,
		Keyword:    q.Get("keyword"),
		PriceStart: page.Float("price_start"),
		PriceEnd:   page.Float("price_end"),
		TimeStart:  q.Get("time_start"),
		TimeEnd:    q.Get("time_end"),
		Sort:       q.Get("sort"),
	}
	raw, err := s.sale.Index(ctx, query)
	if err != nil {
		return nil, bizFail(err)
	}
	return &v1.RawData{Json: raw}, nil
}
