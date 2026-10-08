package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

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
	// rawCache 对应 RedisLockService（裸 Redis，无前缀），可与 PHP 共用键空间
	rawCache biz.RawKeyCache
	// stale 提供 stale-while-revalidate 语义，供 salesOn 使用
	stale *biz.StaleCache
	log   *log.Helper
}

func NewMarketService(
	market *biz.MarketUsecase,
	purchase *biz.PurchaseFaceUsecase,
	sale *biz.SaleFaceUsecase,
	stock *biz.StockUsecase,
	archive *biz.ArchiveUsecase,
	fee *biz.FeeUsecase,
	purchaseState *biz.PurchaseStateUsecase,
	rawCache biz.RawKeyCache,
	stale *biz.StaleCache,
	logger log.Logger,
) *MarketService {
	return &MarketService{
		market: market, purchase: purchase, sale: sale,
		stock: stock, archive: archive, fee: fee,
		purchaseState: purchaseState, rawCache: rawCache, stale: stale,
		log: log.NewHelper(logger),
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
	q = httpx.QueryFrom(ctx)

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
	q = httpx.QueryFrom(ctx)
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
	q = httpx.QueryFrom(ctx)
	page := biz.ParsePageParams(q)

	// 10 秒简单缓存，对应原实现的 RedisLockService::get/set（裸 Redis，可共用键）。
	// 原实现在命中时直接返回整个信封；这里缓存 data 部分 ——
	// 外层 {code,show,msg} 由 encoder 确定性补上，最终字节一致。
	cacheKey := cacheKeyWithParams("purchase:salesOut:", q)
	if v, err := s.rawCache.Get(ctx, cacheKey); err == nil && v != nil {
		if s, ok := v.(string); ok && s != "" {
			return &v1.RawData{Json: json.RawMessage(s)}, nil
		}
	}

	raw, err := s.purchaseState.List(ctx, biz.MktPurchaseStateComplete, in.GetId(), page)
	if err != nil {
		if errors.Is(err, biz.ErrPurchaseListNotFound) {
			// 对应原实现 throw Exception('记录不存在') 被 catch 后的 fail()
			return nil, httpx.Fail("记录不存在")
		}
		return nil, bizFail(err)
	}
	if err := s.rawCache.Set(ctx, cacheKey, string(raw), salesOutCacheTTL); err != nil {
		// 原实现同样不检查 RedisLockService::set 的返回值，所以这里也不让接口失败。
		// 但不能像之前那样 `_ = err` 静默吞掉 —— 缓存整体失效是难查的问题。
		s.log.WithContext(ctx).Warnf("写入 salesOut 缓存失败 key=%s: %v", cacheKey, err)
	}
	return &v1.RawData{Json: raw}, nil
}

// PurchaseOn 对应 PurchaseController::salesOn（state=WANTED）。
//
// 原实现套的是 stale-while-revalidate：
//
//	$params = $this->request->get(); ksort($params);
//	$cacheKey = 'purchase:salesOn:' . md5(json_encode($params));
//	$lockKey  = 'lock:' . $cacheKey;
//	$payload  = json_decode(RedisLockService::get($cacheKey), true);
//	if ($payload && $payload['expire_at'] > time()) { return json($payload['value']); }
//	$token = RedisLockService::tryLock($lockKey, 5000);
//	if ($token === false) {
//	    if ($payload) { return json($payload['value']); }   // 用旧值顶着，不排队
//	    return $this->dataLists(new PurchaseStateLists(['state' => STATE_WANTED]));
//	}
//	try {
//	    $data  = $this->dataLists(new PurchaseStateLists(['state' => STATE_WANTED]));
//	    $value = $data->getData();
//	    RedisLockService::set($cacheKey, json_encode(['value'=>$value,'expire_at'=>time()+5]), 15);
//	    return json($value);
//	} finally { RedisLockService::unlock($lockKey, $token); }
//
// 这些语义都在 biz.StaleCache 里（第 7 轮实现 + 8 条单测），这里只做参数装配。
func (s *MarketService) PurchaseOn(ctx context.Context, in *v1.PurchaseOnRequest) (*v1.RawData, error) {
	q := url.Values{}
	q = httpx.QueryFrom(ctx)
	page := biz.ParsePageParams(q)
	cacheKey := cacheKeyWithParams("purchase:salesOn:", q)

	raw, err := s.stale.Serve(ctx, biz.StaleOptions{
		Key:         cacheKey,
		LockKey:     "lock:" + cacheKey,
		LogicalTTL:  5 * time.Second,
		PhysicalTTL: 15 * time.Second,
		LockTTL:     5 * time.Second,
	}, func(bctx context.Context) (json.RawMessage, error) {
		return s.purchaseState.List(bctx, biz.MktPurchaseStateWanted, in.GetId(), page)
	})
	if err != nil {
		if errors.Is(err, biz.ErrPurchaseListNotFound) {
			return nil, httpx.Fail("记录不存在")
		}
		return nil, bizFail(err)
	}
	return &v1.RawData{Json: raw}, nil
}

// salesOutCacheTTL 对应原实现 RedisLockService::set($cacheKey, ..., 10)。
const salesOutCacheTTL = 10 * time.Second

// cacheKeyWithParams 复刻 PHP 的 md5(json_encode($_GET))。
//
// ⚠️ **与 PHP 算出的 hash 不同**：PHP 的 json_encode 保留数组插入顺序，
// 同一个请求参数顺序不同就是两个 key（salesOn 里靠 ksort 缓解，salesOut 没有）。
// 这里统一按键名排序再拼接，让语义相同的请求命中同一条缓存。
//
// 影响范围有限：键空间是共用裸 Redis 的，但两边各算各的 key，
// **不会互相破坏，只是不共享**（同一请求会被缓存两份）。这是刻意的取舍：
// 为了共用而精确复刻 PHP 的 json_encode 字节序，代价和风险都更高。
func cacheKeyWithParams(prefix string, q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.Join(q[k], ","))
		b.WriteByte('&')
	}
	sum := md5.Sum([]byte(b.String()))
	return prefix + hex.EncodeToString(sum[:])
}
