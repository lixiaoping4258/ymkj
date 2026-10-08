package biz

import (
	"context"
	"time"
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
