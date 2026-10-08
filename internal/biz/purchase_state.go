package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目 app/api/lists/market/purchase/PurchaseStateLists.php，
// 服务于 GET /v1/market/purchase/on（WANTED）与 /v1/market/purchase/out（COMPLETE）。
//
// 注意它和 PurchaseFaceLists 不是一回事：这是**兑换单**列表（x_market_purchase），
// 上面那个是兑换专区列表（x_market_list_purchase）。

// 兑换单状态。x_market_purchase.state 是 enum('OFFLINE','WANTED','COMPLETE','EXPIRE')。
const (
	MktPurchaseStateWanted   = "WANTED"
	MktPurchaseStateComplete = "COMPLETE"
)

// PurchaseStateRow 是列表的一行（DB 原始形态）。
//
// 类型对照原表（information_schema 已核）：
//
//	id / archive_id / amount / receive_amount  -> 整数，PHP 输出数字
//	unit_price                                 -> decimal(10,2)，PHP 输出【字符串】
//	create_time / grab_time                    -> datetime，PHP 输出 'Y-m-d H:i:s'
//	grab_time                                  -> 可为 NULL，PHP 输出 null
//	pay_way                                    -> tinyint，[可为 NULL]
type PurchaseStateRow struct {
	ID            int64
	PayWay        *int32
	ArchiveID     int64
	Amount        int64
	ReceiveAmount int64
	UnitPrice     *string
	CreateTime    *time.Time
	GrabTime      *time.Time
	Name          string
	Issuer        string
	Images        *string
	PlatformName  string
}

// PurchaseStateRepo 兑换单列表的数据访问。
type PurchaseStateRepo interface {
	List(ctx context.Context, archiveID int64, state string, offset, limit int) ([]PurchaseStateRow, error)
	Count(ctx context.Context, archiveID int64, state string) (int64, error)
	// FindListArchiveID 对应 MarketListPurchaseLogic::findSales($id)：
	// 由兑换专区列表 id 找到 archive_id。返回 false 表示记录不存在
	// （原实现此时 throw Exception('记录不存在')）。
	FindListArchiveID(ctx context.Context, listID int64) (int64, bool, error)
	// StockNum 对应 PurchaseOrderStockLogic::getNum($purchaseId)：
	// 读裸 Redis 键 x_PurchaseOrder:Stock:lock:{id}。返回 false 表示键不存在。
	//
	// ❗ 原实现是 `(int)$redis->get($key)`，键不存在时返回 0，
	// 而调用处写的是 `$stockNum == null ? bcsub(...) : $stockNum` ——
	// PHP 里 **0 == null 为 true**，所以"库存为 0"会走 bcsub 兜底。
	// 这里用 (value, exists) 表达，由上层复刻那个判断。
	StockNum(ctx context.Context, purchaseID int64) (int64, bool, error)
}

// ErrPurchaseListNotFound 对应原实现里的 `throw new Exception('记录不存在')`。
// 控制器 catch 住之后返回 fail('记录不存在')。
var ErrPurchaseListNotFound = errors.New("记录不存在")

// ToMap 逐行对应 PurchaseStateLists::lists() 里那段 foreach 后处理。
//
//	$stockNum = PurchaseOrderStockLogic::getNum($list['id']);
//	$list['available_amount'] = $stockNum == null ? bcsub($list['amount'], $list['receive_amount']) : $stockNum;
//	if (is_string($list['images'])) {
//	    $list['images'] = !empty($list['images']) ? json_decode($list['images'], true) : [];
//	    $list['integral'] = bcmul($list['amount'], $list['unit_price'], 2);
//	}
//
// ⚠️ `$stockNum == null` 在 PHP 里对 **0 也为真**（0 == null 是 true）。
// 所以"键不存在"和"库存恰好为 0"两种情况**都**走 bcsub 兜底。
// 这里用 (stockNum, stockExists) 显式表达，不要简化成 "exists ? num : fallback"。
//
// 三个类型细节：
//   - bcsub 只传两个参数 -> 结果没有小数位（整数相减就是 "0"/"5"）
//   - bcmul(..., 2) -> 两位小数字符串，如 "166.00"
//   - images 为空串时是 **[]** 而不是 null；grab_time 为 NULL 时是 **null**
func (r PurchaseStateRow) ToMap(stockNum int64, stockExists bool) map[string]any {
	m := map[string]any{
		"id":             r.ID,
		"pay_way":        r.PayWay,
		"archive_id":     r.ArchiveID,
		"amount":         r.Amount,
		"receive_amount": r.ReceiveAmount,
		"unit_price":     derefStrAny(r.UnitPrice),
		"create_time":    formatDBTimeAny(r.CreateTime),
		"grab_time":      formatDBTimeAny(r.GrabTime),
		"name":           r.Name,
		"issuer":         r.Issuer,
		"platform_name":  r.PlatformName,
	}

	if stockExists && stockNum != 0 {
		m["available_amount"] = stockNum
	} else {
		m["available_amount"] = bcSub(
			strconv.FormatInt(r.Amount, 10),
			strconv.FormatInt(r.ReceiveAmount, 10), 0)
	}

	// integral 是**写在 if (is_string(images)) 里面**的 ——
	// 只有 images 是字符串（从 DB 查出来必然是）才会被设置。
	if r.Images != nil {
		if *r.Images != "" {
			m["images"] = decodeImages(r.Images)
		} else {
			m["images"] = []any{}
		}
		m["integral"] = bcMul(strconv.FormatInt(r.Amount, 10), derefStr(r.UnitPrice), 2)
	} else {
		m["images"] = nil
	}
	return m
}

// PurchaseStateUsecase 兑换单列表用例（purchase/on 与 purchase/out 共用）。
type PurchaseStateUsecase struct {
	repo PurchaseStateRepo
	log  *log.Helper
}

func NewPurchaseStateUsecase(repo PurchaseStateRepo, logger log.Logger) *PurchaseStateUsecase {
	return &PurchaseStateUsecase{repo: repo, log: log.NewHelper(logger)}
}

// List 对应 PurchaseStateLists 的构造 + lists() + count()。
//
// 原实现的构造里会先 findSales($id) 查 archive_id，查不到就 throw；
// lists() 的最后对 WANTED 状态还有一次**分页之后**的过滤。
//
// ⚠️ 那次过滤发生在 limit 之后，所以 WANTED 状态下返回的行数可能少于 page_size，
// 而 count() 不做这个过滤 —— **count 与 lists 天然对不上**。
// 这是原实现的行为，迁移时保持，不要"顺手修好"。
func (uc *PurchaseStateUsecase) List(
	ctx context.Context, state string, listIDStr string, page PageParams,
) (json.RawMessage, error) {
	listID, err := strconv.ParseInt(strings.TrimSpace(listIDStr), 10, 64)
	if err != nil {
		return nil, ErrPurchaseListNotFound
	}
	archiveID, found, err := uc.repo.FindListArchiveID(ctx, listID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPurchaseListNotFound
	}

	rows, err := uc.repo.List(ctx, archiveID, state, page.Offset, page.Limit)
	if err != nil {
		return nil, err
	}
	count, err := uc.repo.Count(ctx, archiveID, state)
	if err != nil {
		return nil, err
	}

	lists := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		stockNum, stockExists, serr := uc.repo.StockNum(ctx, r.ID)
		if serr != nil {
			// ✅ 第 41 轮按"不改动原项目逻辑"的要求改回原行为：
			// 原实现里 getNum() 在 controller 的 try/catch 之内，
			// Redis 异常会被 catch 住 -> 整个列表 fail($e->getMessage())。
			// 我此前改成"记日志 + 走 bcsub 兜底"，那是一处有意的行为分歧，现已撤销。
			return nil, serr
		}
		m := r.ToMap(stockNum, stockExists)
		if state == MktPurchaseStateWanted {
			// 过滤掉可用数量为 0 的（在分页之后）
			if toInt64(m["available_amount"]) <= 0 {
				continue
			}
		}
		lists = append(lists, m)
	}

	return ListData{
		Lists:    lists,
		Count:    count,
		PageNo:   page.PageNo,
		PageSize: page.PageSize,
	}.ToJSON()
}

// formatDBTimeAny 与 formatDBTime 的区别：NULL 时返回 nil（JSON null）而不是空串。
func formatDBTimeAny(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02 15:04:05")
}
