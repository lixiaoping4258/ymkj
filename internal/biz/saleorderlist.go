package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SaleOrderListUsecase 对应 app/api/lists/market/sales/SaleOrderLists.php。
type SaleOrderListUsecase struct {
	repo SaleOrderListRepo
}

func NewSaleOrderListUsecase(repo SaleOrderListRepo) *SaleOrderListUsecase {
	return &SaleOrderListUsecase{repo: repo}
}

// SaleOrderListRepo 秒转订单列表（三张表）。
type SaleOrderListRepo interface {
	// ListOrders 对应 lists() —— **三张表**：
	//   SalesOrders
	//     INNER JOIN x_sales_orders_detail SalesOrdersDetail ON SalesOrdersDetail.order_id = SalesOrders.id
	//     INNER JOIN x_app_archive       AppArchive         ON SalesOrdersDetail.archive_id = AppArchive.id
	ListOrders(ctx context.Context, q SaleOrderQuery, orderBy string, offset, limit int) ([]map[string]any, error)
	// CountOrders 对应 count() —— **同样的三个 JOIN**（这一点两个方法是一致的）
	CountOrders(ctx context.Context, q SaleOrderQuery) (int64, error)
}

// SaleOrderQuery 对应 queryWhere()。
//
//	$where = [['SalesOrders.user_id','=',$this->userId]];
//	if (!empty($pay_state))   { $where[] = ['SalesOrders.pay_state','=', $pay_state]; }
//	if (!empty($order_state)) { $where[] = ['SalesOrders.order_state','=', $order_state]; }   // 🔴 见下
//	if (!empty($keyword))     { $where[] = ['SalesOrders.order_no','like','%'.$keyword.'%']; }
//	if (!empty($time_start))  { $where[] = ['SalesOrders.create_time','>=', strtotime($time_start)]; }
//	if (!empty($time_end))    { $where[] = ['SalesOrders.create_time','<=', strtotime($time_end) + 86400]; }
//
// 🔴🔴 **两处原项目缺陷，本实现刻意照搬**（照搬后接口行为与 PHP 一致）：
//
//  1. **`order_state` 这一列在 `x_sales_orders` 里根本不存在**
//     （实际列是 `state`）。传了 `order_state` 参数会直接
//     `ERROR 1054 (42S22): Unknown column 'order_state' in 'where clause'`
//     —— 已用 MySQL 实测确认。也就是说这个过滤**只能用"不传"的方式避开**。
//     照搬，不改成 `state`。
//
//  2. **`time_start` / `time_end` 的时间过滤实际是失效的**：
//     代码用 `strtotime()` 把日期串转成 **unix 时间戳（整数）**，而
//     `create_time` 列是 **datetime**。MySQL 比较 DATETIME 与整数时会把
//     datetime 转成 `YYYYMMDDHHMMSS` 形式的数字，于是：
//     `create_time >= 1780000000`  ->  20260527143024 >= 1780000000  -> **恒为真**
//     `create_time <= 1780086400`  ->  20260527143024 <= 1780086400  -> **恒为假**
//     **所以 time_start 形同虚设、time_end 会把结果全过滤掉。**
//     照搬，不修正成正确的 datetime 比较。
//
// ⚠️ `!empty()` 语义：0 / ” / '0' 都视为"未传"。
type SaleOrderQuery struct {
	UserID uint32
	// PayState 对应 `$params['pay_state']`（enum WAIT/PAYING/PAID/FAIL）
	PayState string
	// OrderState 对应 `$params['order_state']` —— ⚠️ 该列不存在，传了会 SQL 报错
	OrderState string
	// Keyword 对应 `$params['keyword']`，对 `order_no` 做 LIKE '%kw%'
	Keyword string
	// TimeStart / TimeEnd 对应 `$params['time_start']` / `time_end`（**已知失效**，见上）
	TimeStart int64
	TimeEnd   int64
	// HasTimeStart / HasTimeEnd 区分"没传"与"传了 0"
	HasTimeStart bool
	HasTimeEnd   bool
}

// SaleOrderSort 对应 lists() 里的 sort switch（默认 `time_desc`）。
//
// ⚠️ **只有这 4 个白名单值**，其它值一律走 default（`SalesOrders.create_time desc`）。
//
//	已把 allowlist 固化在这里，避免把用户输入直接拼进 ORDER BY。
func SaleOrderSort(raw string) string {
	switch raw {
	case "price_asc":
		return "SalesOrders.total_price asc"
	case "price_desc":
		return "SalesOrders.total_price desc"
	case "time_asc":
		return "SalesOrders.create_time asc"
	case "time_desc":
		return "SalesOrders.create_time desc"
	default:
		// 原实现的 $orderRaw 初值就是 create_time desc，switch 不命中时保持该值
		return "SalesOrders.create_time desc"
	}
}

// SaleOrderSelect 逐字对应 lists() 的 field（8 项）。
//
// ⚠️ 注意 `SalesOrders.total_price` **只出现在排序里**，**不在字段清单里**
//
//	（字段清单里价格是 `SalesOrders.price`）—— 排序用的列竟然没被 select 出来。
//	原文如此，照搬。
const SaleOrderSelect = "SalesOrders.id AS id, SalesOrders.price AS price, " +
	"SalesOrders.pay_amount AS pay_amount, SalesOrders.order_no AS order_no, " +
	"SalesOrders.pay_state AS pay_state, SalesOrders.pay_expire_time AS pay_expire_time, " +
	"AppArchive.name AS archive_name, AppArchive.images AS archive_images"

// List 对应 `dataLists(new SaleOrderLists())`。
//
// 原文 lists()：
//
//	$result = SalesOrders::alias('SalesOrders')
//	    ->join('sales_orders_detail SalesOrdersDetail', 'SalesOrdersDetail.order_id=SalesOrders.id')
//	    ->join('app_archive AppArchive', 'SalesOrdersDetail.archive_id=AppArchive.id')
//	    ->where($this->queryWhere())->field($field)
//	    ->limit(...)->order($orderRaw)->select()->toArray();
//	foreach ($result as &$item) {
//	    if (is_string($item['archive_images'])) {
//	        $item['archive_images'] = json_decode($item['archive_images'], true);
//	    }
//	}
//
// ⚠️ 两个 JOIN 都是 **INNER JOIN**（ThinkPHP 的 join 默认）——
//
//	没有详情行或档案不存在的订单**不会出现**。
//
// ⚠️⚠️ **一个订单有多条详情行时，列表里会出现多行**（JOIN 放大）。
//
//	实测库里 2 单 2 详情行、恰好 1:1；但只要一单有多件商品就会重复。
//	`count()` 用的是同样的 JOIN，所以**总数也是放大后的数**，两者一致。
//	照搬，不 DISTINCT。
//
// ⚠️ `archive_images` 只在 **is_string** 时才 json_decode（没有 else）——
//
//	与 stock/goodsLook 同风格，但与 stock/list 的 `else { [] }` **不同**（清单第 13 条）。
func (uc *SaleOrderListUsecase) List(ctx context.Context, q SaleOrderQuery, sortRaw string, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountOrders(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListOrders(ctx, q, SaleOrderSort(sortRaw), p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		if s, ok := m["archive_images"].(string); ok {
			var v any
			if json.Unmarshal([]byte(s), &v) == nil {
				m["archive_images"] = v
			}
		}
	}
	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// ParseDateToUnix 近似 PHP 的 `strtotime($s)`（用于 time_start / time_end）。
//
// ⚠️ 只覆盖前端实际会送的三种格式。PHP 的 strtotime 还支持自然语言格式
//
//	（"next monday" 之类），这里不支持 —— 但**本接口的时间过滤本来就是失效的**
//	（见 SaleOrderQuery 的说明），解析精度不影响可观测行为。
func ParseDateToUnix(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrBadDate
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, ErrBadDate
}

// ErrBadDate 日期解析失败。
var ErrBadDate = errors.New("无法解析的日期")
