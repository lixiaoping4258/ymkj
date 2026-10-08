package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
	"github.com/lixiaoping4258/ymkj/internal/pkg/pbconv"
)

// MarketService 对应原项目 app/api/controller/v1/market/PurchaseController.php。
type MarketService struct {
	v1.UnimplementedMarketServiceServer
	market        *biz.MarketUsecase
	purchase      *biz.PurchaseFaceUsecase
	sale          *biz.SaleFaceUsecase
	stock         *biz.StockUsecase
	archive       *biz.ArchiveUsecase
	fee           *biz.FeeUsecase
	purchaseState *biz.PurchaseStateUsecase
}

func NewMarketService(
	market *biz.MarketUsecase,
	purchase *biz.PurchaseFaceUsecase,
	sale *biz.SaleFaceUsecase,
	stock *biz.StockUsecase,
	archive *biz.ArchiveUsecase,
	fee *biz.FeeUsecase,
	purchaseState *biz.PurchaseStateUsecase,
) *MarketService {
	return &MarketService{
		market: market, purchase: purchase, sale: sale,
		stock: stock, archive: archive, fee: fee,
		purchaseState: purchaseState,
	}
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

// StockLookAll 对应 PurchaseController::lookAll。
func (s *MarketService) StockLookAll(ctx context.Context, _ *v1.StockLookAllRequest) (*v1.StockLookAllReply, error) {
	res, err := s.stock.LookAll(ctx, biz.UserIDFromContext(ctx))
	if err != nil {
		return nil, bizFail(err)
	}
	return &v1.StockLookAllReply{State: res.State, Num: res.Num}, nil
}

// PurchaseInfo 对应 PurchaseController::purchaseInfo。
//
// 原实现：
//
//	$params = (new AppArchivePurchaseInfoValidate())->get()->goCheck();
//	$archive = MarketListPurchaseLogic::getArchive($params['id']);
//	if ($archive === false) { return $this->fail(MarketListPurchaseLogic::getError()); }
//	return $this->data($archive);
//
// validate 的规则是 'id' => 'require'，缺失时提示"档案ID不能为空"。
func (s *MarketService) PurchaseInfo(
	ctx context.Context, in *v1.PurchaseInfoRequest,
) (*v1.PurchaseInfoReply, error) {
	idStr := strings.TrimSpace(in.GetId())
	if idStr == "" {
		return nil, httpx.Fail("档案ID不能为空")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		// 原 validate 只校验了 require，非数字会被 getArchive 当成 0 查不到，
		// 最终也是"未找到档案信息"
		return nil, httpx.Fail("未找到档案信息")
	}

	row, err := s.archive.GetArchive(ctx, id)
	if err != nil {
		return nil, bizFail(err)
	}
	if row == nil {
		return nil, httpx.Fail("未找到档案信息")
	}

	reply := &v1.PurchaseInfoReply{}
	if v, ok := row["id"].(int64); ok {
		reply.Id = v
	}
	if v, ok := row["collection_id"].(string); ok {
		reply.CollectionId = v
	}
	if v, ok := row["name"].(string); ok {
		reply.Name = v
	}
	reply.Images = pbconv.ToValue(row["images"])
	if v, ok := row["issuer"].(string); ok {
		reply.Issuer = v
	}
	if v, ok := row["platform_name"].(string); ok {
		reply.PlatformName = v
	}
	if v, ok := row["platform_id"].(int64); ok {
		reply.PlatformId = v
	}
	return reply, nil
}

// SaleInfo 对应 SaleController::saleInfo。
//
// 原实现：
//
//	$params = (new AppArchiveSaleInfoValidate())->get()->goCheck();
//	$data = MarketListSalesLogic::findSales($params['id']);
//	if (empty($data)) { return $this->fail(MarketListSalesLogic::getError()); }   // 未找到艺术品信息
//	$archive = SaleLogic::getArchive($data['archive_id']);
//	if ($archive === false) { return $this->fail(SaleLogic::getError()); }        // 未找到档案信息
//	return $this->data($archive);
func (s *MarketService) SaleInfo(
	ctx context.Context, in *v1.SaleInfoRequest,
) (*v1.SaleInfoReply, error) {
	idStr := strings.TrimSpace(in.GetId())
	if idStr == "" {
		return nil, httpx.Fail("ID不能为空")
	}
	salesID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, httpx.Fail("未找到艺术品信息")
	}

	// 第一步：由秒转列表 id 找 archive_id（找不到是"未找到艺术品信息"）
	archiveID, found, err := s.archive.FindSalesArchiveID(ctx, salesID)
	if err != nil {
		return nil, bizFail(err)
	}
	if !found {
		return nil, httpx.Fail("未找到艺术品信息")
	}

	// 第二步：取档案封面（找不到是"未找到档案信息"，是另一条错误信息）
	row, err := s.archive.GetSaleArchive(ctx, archiveID)
	if err != nil {
		return nil, bizFail(err)
	}
	if row == nil {
		return nil, httpx.Fail("未找到档案信息")
	}

	reply := &v1.SaleInfoReply{}
	if v, ok := row["id"].(int64); ok {
		reply.Id = v
	}
	if v, ok := row["name"].(string); ok {
		reply.Name = v
	}
	reply.Images = pbconv.ToValue(row["images"])
	if v, ok := row["issuer"].(string); ok {
		reply.Issuer = v
	}
	if v, ok := row["platform_name"].(string); ok {
		reply.PlatformName = v
	}
	return reply, nil
}

// ShowTotalAmount 对应 PurchaseOrdersController::showTotalAmount。
//
// 原实现：
//
//	$params = $this->request->param();                  // GET/POST 都收
//	$appId = $params['app_id'] ?? '';
//	$amount = $params['amount'] ?? 0;
//	$unitPrice = $params['unit_price'] ?? '0';
//	$totalAmount = FeeAmountLogic::showTotalAmount((string)$appId, (int)$amount, (string)$unitPrice);
//	return $this->data(['totalAmount' => $totalAmount]);
func (s *MarketService) ShowTotalAmount(
	ctx context.Context, in *v1.ShowTotalAmountRequest,
) (*v1.ShowTotalAmountReply, error) {
	// (int)$amount 的语义：非数字转 0
	amount := 0
	if v, err := strconv.Atoi(strings.TrimSpace(in.GetAmount())); err == nil {
		amount = v
	}
	total, err := s.fee.ShowTotalAmount(ctx, in.GetAppId(), amount, in.GetUnitPrice())
	if err != nil {
		return nil, bizFail(err)
	}
	return &v1.ShowTotalAmountReply{TotalAmount: total}, nil
}

// PurchaseOut 对应 PurchaseController::salesOut。
//
// 原实现：
//
//	(new PurchaseSaleValidate())->get()->goCheck();
//	try {
//	    $params = md5(json_encode($this->request->get()));
//	    $cacheKey = sprintf('purchase:salesOut:%s', $params);
//	    $cache = RedisLockService::get($cacheKey);
//	    if ($cache) { return json(json_decode($cache, true)); }     // 缓存的是完整信封
//	    $data = $this->dataLists(new PurchaseStateLists(['state' => MarketPurchaseEnum::STATE_COMPLETE]));
//	    RedisLockService::set($cacheKey, json_encode($data->getData()), 10);
//	    return $data;
//	} catch (\Exception $e) { return $this->fail($e->getMessage()); }
//
// 与本文件其它接口一致，分页/筛选参数从原始 query 取。
func (s *MarketService) PurchaseOut(ctx context.Context, in *v1.PurchaseOutRequest) (*v1.RawData, error) {
	q := url.Values{}
	if h, ok := ctx.(khttp.Context); ok && h.Request() != nil {
		q = h.Request().URL.Query()
	}
	page := biz.ParsePageParams(q)

	raw, err := s.purchaseState.List(ctx, biz.MktPurchaseStateComplete, in.GetId(), page)
	if err != nil {
		if errors.Is(err, biz.ErrPurchaseListNotFound) {
			// 对应原实现 throw Exception('记录不存在') 被 catch 后的 fail()
			return nil, httpx.Fail("记录不存在")
		}
		return nil, bizFail(err)
	}
	return &v1.RawData{Json: raw}, nil
}
