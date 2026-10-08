package biz

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/market/PurchaseController::index
//   app/api/lists/market/purchase/PurchaseFaceLists

// ProdTestUserIDs 对应 PurchaseStateLists::getProdTestUserList()。
//
// 生产环境（APP_DEBUG=false）下这批用户**跳过交易时段判断和缓存**，
// 并且查询时不过滤 AppArchive.state。
var ProdTestUserIDs = map[uint64]bool{6: true, 7: true, 1000: true}

// purchaseIndexCacheTTL 原实现是 20 秒整包缓存。
const purchaseIndexCacheTTL = 20 * time.Second

// 交易时段不开放时返回的空列表。注意 page_size 是**硬编码的 10**，
// 不是请求里的值 —— 原实现就是这么写的。
const purchaseIndexClosedPageSize = 10

// PurchaseFaceQuery 是兑换专区列表的查询条件。
//
// 字段与 PurchaseFaceLists 从 $this->params 里取的那几个一一对应。
type PurchaseFaceQuery struct {
	Page PageParams
	// 以下对应 $this->params['xxx']
	Keyword    string
	PriceStart float64
	PriceEnd   float64
	TimeStart  string
	TimeEnd    string
	Sort       string
}

// PurchaseFaceRow 是列表的一行（DB 原始形态，后处理在 ToMap 里做）。
type PurchaseFaceRow struct {
	ID             int64
	ArchiveID      int64
	Name           string
	Images         *string // DB 里是 json 字符串
	Issuer         string
	IssuerTime     *time.Time
	PurchaseAmount int64
	LowUnitPrice   *string // decimal(10,2)，PHP 输出字符串
	MaxUnitPrice   *string
	PurchaseLists  *int64
	PlatformName   string
}

// ToMap 逐行对照 PurchaseFaceLists::lists() 里那段 foreach 后处理。
//
// 三个要点：
//  1. purchase_lists 用 PHP 的 `?:` 语义（falsy 就变 "--"），所以 0 / null / "0" 都变 "--"
//  2. purchase_lists 变成 "--" 时，两个单价也一并变 "--"
//  3. images 是 json 字符串时解码成数组；解码失败按 PHP 的 json_decode 行为得到 null
func (r PurchaseFaceRow) ToMap() map[string]any {
	m := map[string]any{
		"id":         r.ID,
		"archive_id": r.ArchiveID,
		"name":       r.Name,
		"images":     decodeImages(r.Images),
		"issuer":     r.Issuer,
		// ⚠️ 必须用 formatDBTimeAny（NULL -> JSON null），不能用 formatDBTime（NULL -> ""）。
		// x_app_archive.issuer_time 是 datetime **NULLABLE**，实测 149 行里有 2 行为 NULL。
		// PHP 的 toArray() 对 NULL datetime 给的是 null，json_encode 出来就是 null。
		// 用 formatDBTime 会输出空串 —— 类型从 null 变字符串，是静默的契约破坏。
		// （PurchaseStateRow.ToMap 里一开始就用对了，这里是漏改。）
		"issuer_time":     formatDBTimeAny(r.IssuerTime),
		"purchase_amount": r.PurchaseAmount,
		"platform_name":   r.PlatformName,
	}

	var listsVal any = "--"
	if r.PurchaseLists != nil && PhpTruthy(*r.PurchaseLists) {
		listsVal = *r.PurchaseLists
	}
	m["purchase_lists"] = listsVal

	if listsVal == "--" {
		m["low_unit_price"] = "--"
		m["max_unit_price"] = "--"
	} else {
		m["low_unit_price"] = derefStr(r.LowUnitPrice)
		m["max_unit_price"] = derefStr(r.MaxUnitPrice)
	}
	return m
}

// PurchaseFaceRepo 兑换专区列表的数据访问。
type PurchaseFaceRepo interface {
	// List 返回分页后的行。noStateFilter=true 时不过滤 AppArchive.state
	// （对应生产测试用户分支里 $where = [] 的情况）。
	List(ctx context.Context, q PurchaseFaceQuery, offset, limit int, noStateFilter bool) ([]PurchaseFaceRow, error)
	Count(ctx context.Context, q PurchaseFaceQuery, noStateFilter bool) (int64, error)
}

// PurchaseFaceUsecase 兑换专区列表用例。
type PurchaseFaceUsecase struct {
	repo  PurchaseFaceRepo
	trade *TradeConfigUsecase
	cache Cache
	// appDebug 对应 env('APP_DEBUG')：为 true 时关闭测试用户特殊分支
	appDebug bool
	log      *log.Helper
}

func NewPurchaseFaceUsecase(
	repo PurchaseFaceRepo,
	trade *TradeConfigUsecase,
	cache Cache,
	app *AppFlags,
	logger log.Logger,
) *PurchaseFaceUsecase {
	debug := true
	if app != nil {
		debug = app.AppDebug
	}
	return &PurchaseFaceUsecase{
		repo:     repo,
		trade:    trade,
		cache:    cache,
		appDebug: debug,
		log:      log.NewHelper(logger),
	}
}

// AppFlags 把几个散落的环境开关收在一起，避免构造函数参数里塞裸 bool。
type AppFlags struct {
	// AppDebug 对应 .env 的 APP_DEBUG。
	// 它会同时影响「生产测试用户」和「列表 key 生成」等分支，配错会静默改变行为。
	AppDebug bool
}

// Index 对应 PurchaseController::index。
//
// 原实现（去掉注释后的骨架）：
//
//	if (!env('APP_DEBUG') && in_array($this->userId, [6,7,1000])) {
//	    return $this->dataLists(new PurchaseFaceLists());          // 直接返回，不做时段判断、不走缓存
//	}
//	$tradeSwitchCheck = TradeConfigService::checkTradeTime();
//	if ($tradeSwitchCheck['tradeSwitch'] == 1 && $tradeSwitchCheck['isOpen'] != 1) {
//	    return $this->data(['count'=>0,'extend'=>[],'lists'=>[],'page_no'=>1,'page_size'=>10]);
//	}
//	$cacheKey = 'purchase:indexList:' . md5(json_encode($this->request->get()));
//	if ($cache) { return json(json_decode($cache, true)); }
//	$res = $this->dataLists(new PurchaseFaceLists());
//	RedisLockService::set($cacheKey, json_encode($res->getData()), 20);
//
// 返回的是**信封 data 部分的 JSON**（RawData），外层 {code,show,msg} 由 encoder 补。
// 原实现缓存的是整个信封，这里缓存 data 部分 —— 因为外层是确定性的，
// 最终字节完全一致，但少一层解析。
func (uc *PurchaseFaceUsecase) Index(ctx context.Context, userID uint64, q PurchaseFaceQuery) (json.RawMessage, error) {
	isProdTest := !uc.appDebug && ProdTestUserIDs[userID]

	if !isProdTest {
		st, err := uc.trade.CheckTradeTime(ctx)
		if err != nil {
			return nil, err
		}
		if PhpLooseEqualsInt(st.TradeSwitch, 1) && st.IsOpen != 1 {
			// 不开放交易 -> 空列表。page_size 固定 10，与原实现一致
			return ListData{
				Lists:    []any{},
				Count:    0,
				PageNo:   1,
				PageSize: purchaseIndexClosedPageSize,
			}.ToJSON()
		}
	}

	cacheKey := purchaseIndexCacheKey(q)

	if !isProdTest {
		if v, err := uc.cache.Get(ctx, cacheKey); err != nil {
			uc.log.WithContext(ctx).Warnf("读取兑换专区缓存失败: %v", err)
		} else if s, ok := v.(string); ok && s != "" {
			return json.RawMessage(s), nil
		}
	}

	rows, err := uc.repo.List(ctx, q, q.Page.Offset, q.Page.Limit, isProdTest)
	if err != nil {
		return nil, err
	}
	count, err := uc.repo.Count(ctx, q, isProdTest)
	if err != nil {
		return nil, err
	}

	lists := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		lists = append(lists, r.ToMap())
	}

	raw, err := ListData{
		Lists:    lists,
		Count:    count,
		PageNo:   q.Page.PageNo,
		PageSize: q.Page.PageSize,
	}.ToJSON()
	if err != nil {
		return nil, err
	}

	// 生产测试用户不写缓存（原实现直接 return，走不到缓存那段）
	if !isProdTest {
		if err := uc.cache.Set(ctx, cacheKey, string(raw), purchaseIndexCacheTTL); err != nil {
			uc.log.WithContext(ctx).Warnf("写入兑换专区缓存失败: %v", err)
		}
	}
	return raw, nil
}

// purchaseIndexCacheKey 复刻 'purchase:indexList:' . md5(json_encode($get))。
//
// ⚠️ 与原实现**有意不同**：PHP 是对 `json_encode($_GET)` 取 md5，
// 而 PHP 数组的序列化顺序依赖请求参数顺序（`?a=1&b=2` 与 `?b=2&a=1` 会得到不同 key）。
// 这里改成按键名排序后拼接，让语义相同的请求命中同一条缓存。
//
// 不会造成不一致：键空间本来就是隔离的（带 xtravel:go: 前缀），
// 两边各算各的 key，只是 Go 侧的命中率更合理一点。
func purchaseIndexCacheKey(q PurchaseFaceQuery) string {
	keys := make([]string, 0, len(q.Page.Query))
	for k := range q.Page.Query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.Join(q.Page.Query[k], ","))
		b.WriteByte('&')
	}
	sum := md5.Sum([]byte(b.String()))
	return "purchase:indexList:" + hex.EncodeToString(sum[:])
}

// decodeImages 对应 PHP 的 `if (is_string($item['images'])) json_decode(...)`。
func decodeImages(s *string) any {
	if s == nil {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(*s), &v); err != nil {
		// PHP 的 json_decode 失败返回 null
		return nil
	}
	return v
}

// formatDBTime 把 DB 的 datetime 输出成 PHP 的 'Y-m-d H:i:s'。
//
// 必须走 time.Time 再格式化：DSN 里开了 parseTime=True，
// 驱动会把 datetime 转成 time.Time；若直接 Scan 进 string，
// database/sql 会用 RFC3339 格式化成 "2026-04-17T17:39:39+08:00"，与 PHP 不一致。
func formatDBTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
